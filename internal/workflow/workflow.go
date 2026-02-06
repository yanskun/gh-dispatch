package workflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// RESTClient はAPIリクエストを行うためのインターフェース
type RESTClient interface {
	Request(method string, path string, body io.Reader) (*http.Response, error)
}

// Workflow はワークフローの基本情報を表します
type Workflow struct {
	Name     string
	Path     string
	FileName string
	Inputs   map[string]Input
}

// Input はworkflow_dispatchのinput定義を表します
type Input struct {
	Description string   `yaml:"description"`
	Required    bool     `yaml:"required"`
	Default     string   `yaml:"default"`
	Type        string   `yaml:"type"`
	Options     []string `yaml:"options"`
}

// workflowYAML はYAMLファイルのパース用構造体
type workflowYAML struct {
	Name string `yaml:"name"`
	On   any    `yaml:"on"`
}

// DispatchParams はワークフロー実行リクエストに必要なパラメータ
type DispatchParams struct {
	Owner        string
	Repo         string
	WorkflowFile string
	Ref          string
	Inputs       map[string]string
}

// LoadDispatchableWorkflows は指定ディレクトリ内の workflow_dispatch を持つワークフローを検索します
func LoadDispatchableWorkflows(workflowsDir string) ([]Workflow, error) {
	var workflows []Workflow

	entries, err := os.ReadDir(workflowsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return workflows, fmt.Errorf("directory %s not found", workflowsDir)
		}
		return workflows, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yml" && ext != ".yaml" {
			continue
		}

		path := filepath.Join(workflowsDir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var wf workflowYAML
		if err := yaml.Unmarshal(content, &wf); err != nil {
			continue
		}

		inputs := extractInputs(wf.On)
		if inputs != nil || hasWorkflowDispatch(wf.On) {
			title := wf.Name
			if title == "" {
				title = entry.Name()
			}

			// 相対パスに変換 (.github/workflows/xxx.yml)
			relativePath := filepath.Join(".github", "workflows", entry.Name())

			workflows = append(workflows, Workflow{
				Name:     title,
				Path:     relativePath,
				FileName: entry.Name(),
				Inputs:   inputs,
			})
		}
	}

	return workflows, nil
}

// hasWorkflowDispatch はトリガー設定に workflow_dispatch が含まれているか判定します
func hasWorkflowDispatch(on any) bool {
	switch v := on.(type) {
	case string:
		return v == "workflow_dispatch"
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == "workflow_dispatch" {
				return true
			}
		}
	case map[string]any:
		_, ok := v["workflow_dispatch"]
		return ok
	}
	return false
}

// extractInputs は workflow_dispatch の inputs を抽出します
func extractInputs(on any) map[string]Input {
	m, ok := on.(map[string]any)
	if !ok {
		return nil
	}

	wd, ok := m["workflow_dispatch"]
	if !ok {
		return nil
	}

	wdMap, ok := wd.(map[string]any)
	if !ok {
		return nil
	}

	inputsRaw, ok := wdMap["inputs"]
	if !ok {
		return nil
	}

	inputsMap, ok := inputsRaw.(map[string]any)
	if !ok {
		return nil
	}

	inputs := make(map[string]Input)
	for key, val := range inputsMap {
		inputMap, ok := val.(map[string]any)
		if !ok {
			continue
		}

		input := Input{}
		if desc, ok := inputMap["description"].(string); ok {
			input.Description = desc
		}
		if req, ok := inputMap["required"].(bool); ok {
			input.Required = req
		}
		if def, ok := inputMap["default"].(string); ok {
			input.Default = def
		}
		if typ, ok := inputMap["type"].(string); ok {
			input.Type = typ
		}
		if opts, ok := inputMap["options"].([]any); ok {
			for _, opt := range opts {
				if optStr, ok := opt.(string); ok {
					input.Options = append(input.Options, optStr)
				}
			}
		}

		inputs[key] = input
	}

	if len(inputs) == 0 {
		return nil
	}

	return inputs
}

// createDispatchRequest はAPIエンドポイントとJSONペイロードを構築・検証します
func createDispatchRequest(params DispatchParams) (string, []byte, error) {
	if params.Owner == "" || params.Repo == "" {
		return "", nil, fmt.Errorf("owner and repo are required")
	}
	if params.WorkflowFile == "" {
		return "", nil, fmt.Errorf("workflow file is required")
	}
	if params.Ref == "" {
		return "", nil, fmt.Errorf("ref (branch) is required")
	}

	endpoint := fmt.Sprintf("repos/%s/%s/actions/workflows/%s/dispatches",
		params.Owner, params.Repo, params.WorkflowFile)

	payload := map[string]any{
		"ref": params.Ref,
	}

	if len(params.Inputs) > 0 {
		payload["inputs"] = params.Inputs
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	return endpoint, body, nil
}

// RunDispatch は指定されたパラメータでワークフローを実行します
func RunDispatch(client RESTClient, params DispatchParams) error {
	endpoint, body, err := createDispatchRequest(params)
	if err != nil {
		return err
	}

	resp, err := client.Request(http.MethodPost, endpoint, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to dispatch request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}
