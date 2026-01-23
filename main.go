package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/repository"
	"gopkg.in/yaml.v3"
	"os/exec"
)

// --- スタイル・型定義 ---
var docStyle = lipgloss.NewStyle().Margin(1, 2)

type state int

const (
	selectingWorkflow state = iota
	selectingBranch
	confirming
	executing
)

type Workflow struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type workflowYAML struct {
	Name string      `yaml:"name"`
	On   interface{} `yaml:"on"`
}

type WorkflowsResponse struct {
	Workflows []Workflow `json:"workflows"`
}

type Branch struct {
	Name string `json:"name"`
}

type item struct {
	title, desc string
	fileName    string // 実行時にファイル名が必要
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title + " " + i.fileName }

// --- Bubble Tea Model ---
type model struct {
	list             list.Model
	state            state
	workflows        []list.Item
	branches         []list.Item
	selectedWorkflow item
	selectedBranch   item
	quitting         bool
	owner            string
	repo             string
	currentBranch    string // カレントブランチ名を保持
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}

		// 確認画面でのキー操作
		if m.state == confirming {
			switch msg.String() {
			case "y", "Y":
				m.state = executing
				return m, tea.Quit
			case "n", "N", "esc":
				m.quitting = true
				return m, tea.Quit
			default:
				return m, nil
			}
		}

		if msg.String() == "enter" {
			i, ok := m.list.SelectedItem().(item)
			if !ok {
				return m, nil
			}

			if m.state == selectingWorkflow {
				m.selectedWorkflow = i
				m.state = selectingBranch
				m.list.Title = fmt.Sprintf("Select a Branch (Current: %s)", m.currentBranch)
				m.list.ResetSelected()
				m.list.ResetFilter()

				// カレントブランチをデフォルト選択にする
				newItems := m.branches
				cmd := m.list.SetItems(newItems)

				for idx, it := range newItems {
					if it.(item).title == m.currentBranch {
						m.list.Select(idx)
						break
					}
				}

				return m, cmd
			} else if m.state == selectingBranch {
				m.selectedBranch = i
				m.state = confirming // 確認画面へ
				return m, nil
			}
		}
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
	}
	var cmd tea.Cmd
	// リスト操作は選択画面のみ有効
	if m.state == selectingWorkflow || m.state == selectingBranch {
		m.list, cmd = m.list.Update(msg)
	}
	return m, cmd
}

func (m model) View() string {
	if m.state == confirming {
		return fmt.Sprintf(
			"\nAre you sure you want to dispatch workflow:\n\n  %s\n\non branch:\n\n  %s\n\n(y/N)",
			docStyle.Render(m.selectedWorkflow.title),
			docStyle.Render(m.selectedBranch.title),
		)
	}
	if m.state == executing {
		return "" // 実行ログはmain関数側で出力するため何も表示しない
	}
	if m.quitting {
		return "\nQuit.\n"
	}
	return docStyle.Render(m.list.View())
}

// --- フィルタリングロジック ---
func getDispatchableWorkflows() ([]list.Item, error) {
	items := []list.Item{}
	workflowsDir := ".github/workflows"

	entries, err := os.ReadDir(workflowsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return items, fmt.Errorf("directory %s not found", workflowsDir)
		}
		return items, err
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
			items = append(items, item{
				title:    title,
				desc:     path,
				fileName: entry.Name(),
			})
		}
	}

	return items, nil
}

func hasWorkflowDispatch(on interface{}) bool {
	switch v := on.(type) {
	case string:
		return v == "workflow_dispatch"
	case []interface{}: // YAML list often unmarshals to []interface{}
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

// --- Main ---
func main() {
	// 1. 実行ディレクトリのリポジトリ情報を取得
	repoInfo, err := repository.Current()
	if err != nil {
		log.Fatal("Could not determine current repository. Are you in a git-managed directory with a remote?")
	}

	owner, repo := repoInfo.Owner, repoInfo.Name

	client, err := api.DefaultRESTClient()
	if err != nil {
		log.Fatal(err)
	}

	// 2. Workflow 一覧取得 (ローカルファイル解析)
	wfItems, err := getDispatchableWorkflows()
	if err != nil {
		log.Fatalf("Failed to scan workflows: %v", err)
	}

	if len(wfItems) == 0 {
		fmt.Println("No workflows with 'workflow_dispatch' trigger found in .github/workflows.")
		return
	}

	// 3. Branch 一覧取得
	var brRes []Branch
	err = client.Get(fmt.Sprintf("repos/%s/%s/branches", owner, repo), &brRes)
	if err != nil {
		log.Fatal("Failed to fetch branches: ", err)
	}

	brItems := []list.Item{}
	for _, b := range brRes {
		brItems = append(brItems, item{title: b.Name, desc: "Branch"})
	}

	// 4. カレントブランチ取得
	currentBranch := ""
	if out, err := exec.Command("git", "branch", "--show-current").Output(); err == nil {
		currentBranch = strings.TrimSpace(string(out))
	}

	// 5. Bubble Tea 実行
	initialModel := model{
		state:         selectingWorkflow,
		workflows:     wfItems,
		branches:      brItems,
		list:          list.New(wfItems, list.NewDefaultDelegate(), 0, 0),
		owner:         owner,
		repo:          repo,
		currentBranch: currentBranch,
	}
	initialModel.list.Title = "Select a Workflow"

	p := tea.NewProgram(initialModel, tea.WithAltScreen())
	finalModelMsg, err := p.Run()
	if err != nil {
		fmt.Printf("Error running program: %v", err)
		os.Exit(1)
	}

	finalModel := finalModelMsg.(model)

	// 5. 最終実行 (Dispatch)
	if finalModel.state == executing {
		fmt.Printf("🚀 Dispatching %s on branch %s...\n", finalModel.selectedWorkflow.title, finalModel.selectedBranch.title)

		// ファイル名を使用
		workflowFile := finalModel.selectedWorkflow.fileName

		dispatchEndpoint := fmt.Sprintf("repos/%s/%s/actions/workflows/%s/dispatches",
			finalModel.owner, finalModel.repo, workflowFile)

		payload := map[string]interface{}{
			"ref": finalModel.selectedBranch.title,
		}

		jsonBody, _ := json.Marshal(payload)
		resp, err := client.Request(http.MethodPost, dispatchEndpoint, bytes.NewBuffer(jsonBody))

		if err != nil {
			log.Fatalf("❌ Failed to dispatch: %v", err)
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Println("failed to close body:", err)
			}
		}()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			fmt.Println("✅ Successfully dispatched! Check your Actions tab.")
		} else {
			fmt.Printf("❌ Failed with status: %d\n", resp.StatusCode)
		}
	}
}
