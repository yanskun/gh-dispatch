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
}

// workflowYAML はYAMLファイルのパース用構造体
type workflowYAML struct {
	Name string      `yaml:"name"`
	On   interface{} `yaml:"on"`
}

// DispatchParams はワークフロー実行リクエストに必要なパラメータ
type DispatchParams struct {
	Owner        string
	Repo         string
	WorkflowFile string
	Ref          string
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

		if hasWorkflowDispatch(wf.On) {
			title := wf.Name
			if title == "" {
				title = entry.Name()
			}
			workflows = append(workflows, Workflow{
				Name:     title,
				Path:     path,
				FileName: entry.Name(),
			})
		}
	}

	return workflows, nil
}

// hasWorkflowDispatch はトリガー設定に workflow_dispatch が含まれているか判定します
func hasWorkflowDispatch(on interface{}) bool {
	switch v := on.(type) {
	case string:
		return v == "workflow_dispatch"
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok && s == "workflow_dispatch" {
				return true
			}
		}
	case map[string]interface{}:
		_, ok := v["workflow_dispatch"]
		return ok
	}
	return false
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

	payload := map[string]interface{}{
		"ref": params.Ref,
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
