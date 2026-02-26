package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/yanskun/gh-dispatch/internal/branch"
	"github.com/yanskun/gh-dispatch/internal/workflow"
)

// --- スタイル・型定義 ---
var (
	docStyle = lipgloss.NewStyle().Margin(1, 2)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("63")).
			MarginBottom(1)

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	valueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Bold(true)

	inputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			Bold(true)

	requiredStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203")).
			Bold(true)

	hintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Italic(true).
			MarginTop(1)
)

type state int

const (
	selectingWorkflow state = iota
	selectingBranch
	enteringInputs
	confirming
	executing
)

type item struct {
	title, desc string
	fileName    string                    // 実行時にファイル名が必要
	inputs      map[string]workflow.Input // workflow_dispatch の inputs
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
	workflowInputs   map[string]workflow.Input
	userInputs       map[string]string
	inputKeys        []string
	currentInputIdx  int
	inputBuffer      string
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

		// inputs 入力中の処理
		if m.state == enteringInputs {
			if msg.String() == "enter" {
				// 現在の入力を保存
				key := m.inputKeys[m.currentInputIdx]
				if m.inputBuffer == "" && m.workflowInputs[key].Default != "" {
					m.userInputs[key] = m.workflowInputs[key].Default
				} else {
					m.userInputs[key] = m.inputBuffer
				}
				m.inputBuffer = ""

				// 次の入力へ
				m.currentInputIdx++
				if m.currentInputIdx >= len(m.inputKeys) {
					m.state = confirming
				}
				return m, nil
			} else if msg.String() == "backspace" {
				if len(m.inputBuffer) > 0 {
					_, size := utf8.DecodeLastRuneInString(m.inputBuffer)
					m.inputBuffer = m.inputBuffer[:len(m.inputBuffer)-size]
				}
				return m, nil
			}
			// 通常の文字入力（マルチバイト文字・IME確定も含む）
			if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
				m.inputBuffer += string(msg.Runes)
				return m, nil
			}
			return m, nil
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
				// inputs がある場合は入力画面へ、ない場合は確認画面へ
				if len(m.selectedWorkflow.inputs) > 0 {
					m.state = enteringInputs
					m.workflowInputs = m.selectedWorkflow.inputs
					m.userInputs = make(map[string]string)
					m.inputKeys = []string{}
					for key := range m.workflowInputs {
						m.inputKeys = append(m.inputKeys, key)
					}
					m.currentInputIdx = 0
					m.inputBuffer = ""
				} else {
					m.state = confirming
				}
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
	if m.state == enteringInputs {
		key := m.inputKeys[m.currentInputIdx]
		input := m.workflowInputs[key]

		var output strings.Builder

		// タイトル
		output.WriteString(titleStyle.Render(fmt.Sprintf("Workflow Input [%d/%d]", m.currentInputIdx+1, len(m.inputKeys))))
		output.WriteString("\n\n")

		// Input 名
		output.WriteString(labelStyle.Render("Input: "))
		output.WriteString(valueStyle.Render(key))
		output.WriteString("\n")

		// Description
		if input.Description != "" {
			output.WriteString(labelStyle.Render("Description: "))
			output.WriteString(input.Description)
			output.WriteString("\n")
		}

		// Required
		if input.Required {
			output.WriteString(requiredStyle.Render("Required: yes"))
			output.WriteString("\n")
		}

		// Default
		if input.Default != "" {
			output.WriteString(labelStyle.Render("Default: "))
			output.WriteString(valueStyle.Render(input.Default))
			output.WriteString("\n")
		}

		output.WriteString("\n")
		output.WriteString(labelStyle.Render("Value: "))
		output.WriteString(inputStyle.Render(m.inputBuffer))
		output.WriteString(inputStyle.Render("█")) // カーソル

		output.WriteString("\n")
		output.WriteString(hintStyle.Render("Press Enter to continue (or use default), Ctrl+C to cancel"))

		return docStyle.Render(output.String())
	}
	if m.state == confirming {
		var output strings.Builder

		// タイトル
		output.WriteString(titleStyle.Render("Confirm Dispatch"))
		output.WriteString("\n\n")

		// Workflow
		output.WriteString(labelStyle.Render("Workflow: "))
		output.WriteString(valueStyle.Render(m.selectedWorkflow.title))
		output.WriteString("\n\n")

		// Branch
		output.WriteString(labelStyle.Render("Branch: "))
		output.WriteString(valueStyle.Render(m.selectedBranch.title))
		output.WriteString("\n")

		// Inputs
		if len(m.userInputs) > 0 {
			output.WriteString("\n")
			output.WriteString(labelStyle.Render("Inputs:"))
			output.WriteString("\n")
			for key, value := range m.userInputs {
				output.WriteString(labelStyle.Render("  • "))
				output.WriteString(labelStyle.Render(key + ": "))
				output.WriteString(valueStyle.Render(value))
				output.WriteString("\n")
			}
		}

		output.WriteString("\n")
		output.WriteString(hintStyle.Render("Are you sure? (y/N)"))

		return docStyle.Render(output.String())
	}
	if m.state == executing {
		return "" // 実行ログはmain関数側で出力するため何も表示しない
	}
	if m.quitting {
		return "\nQuit.\n"
	}
	return docStyle.Render(m.list.View())
}

// --- Main ---
func main() {
	// 1. 実行ディレクトリのリポジトリ情報を取得
	repoInfo, err := repository.Current()
	if err != nil {
		log.Fatal("Could not determine current repository. Are you in a git-managed directory with a remote?")
	}

	owner, repo := repoInfo.Owner, repoInfo.Name

	// リポジトリのルートパスを取得
	rootPath := ""
	if out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output(); err == nil {
		rootPath = strings.TrimSpace(string(out))
	} else {
		log.Fatal("Could not determine repository root. Are you in a git-managed directory?")
	}

	client, err := api.DefaultRESTClient()
	if err != nil {
		log.Fatal(err)
	}

	// 2. Workflow 一覧取得 (internalパッケージを使用)
	workflowsDir := filepath.Join(rootPath, ".github", "workflows")
	wfs, err := workflow.LoadDispatchableWorkflows(workflowsDir)
	if err != nil {
		log.Fatalf("Failed to scan workflows: %v", err)
	}

	if len(wfs) == 0 {
		fmt.Println("No workflows with 'workflow_dispatch' trigger found in .github/workflows.")
		return
	}

	wfItems := []list.Item{}
	for _, wf := range wfs {
		wfItems = append(wfItems, item{
			title:    wf.Name,
			desc:     wf.Path,
			fileName: wf.FileName,
			inputs:   wf.Inputs,
		})
	}

	// 3. Branch 一覧取得
	brRes, err := branch.FetchBranches(client, owner, repo)
	if err != nil {
		log.Fatal(err)
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

		params := workflow.DispatchParams{
			Owner:        finalModel.owner,
			Repo:         finalModel.repo,
			WorkflowFile: workflowFile,
			Ref:          finalModel.selectedBranch.title,
			Inputs:       finalModel.userInputs,
		}

		err := workflow.RunDispatch(client, params)
		if err != nil {
			log.Fatalf("❌ Failed to dispatch: %v", err)
		}

		fmt.Println("✅ Successfully dispatched!")
		fmt.Printf("\nFor more information about the run, try:\n  gh run list --workflow=%s\n", workflowFile)
	}
}
