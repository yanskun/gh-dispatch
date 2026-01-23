package workflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

// mockRESTClient は workflow.RESTClient のモックです
type mockRESTClient struct {
	ResponseCode int
	Error        error
}

func (m *mockRESTClient) Request(method string, path string, body io.Reader) (*http.Response, error) {
	if m.Error != nil {
		return nil, m.Error
	}
	return &http.Response{
		StatusCode: m.ResponseCode,
		Body:       io.NopCloser(bytes.NewReader([]byte{})),
	}, nil
}

func TestRunDispatch(t *testing.T) {
	tests := []struct {
		name          string
		params        DispatchParams
		mockCode      int
		mockError     error
		wantErrString string
	}{
		{
			name: "success",
			params: DispatchParams{
				Owner:        "user",
				Repo:         "repo",
				WorkflowFile: "test.yml",
				Ref:          "main",
			},
			mockCode: 204,
		},
		{
			name: "create request error (missing param)",
			params: DispatchParams{
				Owner: "user",
			},
			wantErrString: "owner and repo are required",
		},
		{
			name: "api error",
			params: DispatchParams{
				Owner:        "user",
				Repo:         "repo",
				WorkflowFile: "test.yml",
				Ref:          "main",
			},
			mockError:     fmt.Errorf("network error"),
			wantErrString: "failed to dispatch request: network error",
		},
		{
			name: "unexpected status code",
			params: DispatchParams{
				Owner:        "user",
				Repo:         "repo",
				WorkflowFile: "test.yml",
				Ref:          "main",
			},
			mockCode:      500,
			wantErrString: "unexpected status code: 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockRESTClient{
				ResponseCode: tt.mockCode,
				Error:        tt.mockError,
			}

			err := RunDispatch(client, tt.params)

			if tt.wantErrString != "" {
				if err == nil {
					t.Errorf("RunDispatch() expected error containing %q, got nil", tt.wantErrString)
				} else if err.Error() != tt.wantErrString {
					t.Errorf("RunDispatch() error = %v, want %v", err, tt.wantErrString)
				}
				return
			}

			if err != nil {
				t.Errorf("RunDispatch() unexpected error: %v", err)
			}
		})
	}
}

func TestCreateDispatchRequest(t *testing.T) {
	tests := []struct {
		name          string
		params        DispatchParams
		wantEndpoint  string
		wantBodyRef   string
		wantErrString string
	}{
		{
			name: "valid request",
			params: DispatchParams{
				Owner:        "yanskun",
				Repo:         "gh-dispatch",
				WorkflowFile: "deploy.yml",
				Ref:          "main",
			},
			wantEndpoint: "repos/yanskun/gh-dispatch/actions/workflows/deploy.yml/dispatches",
			wantBodyRef:  "main",
		},
		{
			name: "missing owner/repo",
			params: DispatchParams{
				Owner:        "",
				Repo:         "",
				WorkflowFile: "test.yml",
				Ref:          "develop",
			},
			wantErrString: "owner and repo are required",
		},
		{
			name: "missing workflow file",
			params: DispatchParams{
				Owner: "user",
				Repo:  "repo",
				Ref:   "main",
			},
			wantErrString: "workflow file is required",
		},
		{
			name: "missing ref",
			params: DispatchParams{
				Owner:        "user",
				Repo:         "repo",
				WorkflowFile: "ci.yml",
			},
			wantErrString: "ref (branch) is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 小文字に変更
			gotEndpoint, gotBody, err := createDispatchRequest(tt.params)

			if tt.wantErrString != "" {
				if err == nil {
					t.Errorf("createDispatchRequest() expected error containing %q, got nil", tt.wantErrString)
				} else if err.Error() != tt.wantErrString {
					t.Errorf("createDispatchRequest() error = %v, want %v", err, tt.wantErrString)
				}
				return
			}

			if err != nil {
				t.Fatalf("createDispatchRequest() unexpected error: %v", err)
			}

			if gotEndpoint != tt.wantEndpoint {
				t.Errorf("createDispatchRequest() endpoint = %v, want %v", gotEndpoint, tt.wantEndpoint)
			}

			var payload map[string]interface{}
			if err := json.Unmarshal(gotBody, &payload); err != nil {
				t.Fatalf("createDispatchRequest() returned invalid JSON: %v", err)
			}

			if ref, ok := payload["ref"].(string); !ok || ref != tt.wantBodyRef {
				t.Errorf("JSON body 'ref' = %v, want %v", payload["ref"], tt.wantBodyRef)
			}
		})
	}
}

func TestHasWorkflowDispatch(t *testing.T) {
	tests := []struct {
		name string
		on   interface{}
		want bool
	}{
		{
			name: "string workflow_dispatch",
			on:   "workflow_dispatch",
			want: true,
		},
		{
			name: "string push",
			on:   "push",
			want: false,
		},
		{
			name: "list with workflow_dispatch",
			on:   []interface{}{"push", "workflow_dispatch"},
			want: true,
		},
		{
			name: "list without workflow_dispatch",
			on:   []interface{}{"push", "pull_request"},
			want: false,
		},
		{
			name: "map with workflow_dispatch key",
			on:   map[string]interface{}{"workflow_dispatch": nil, "push": nil},
			want: true,
		},
		{
			name: "map without workflow_dispatch key",
			on:   map[string]interface{}{"push": nil, "pull_request": nil},
			want: false,
		},
		{
			name: "nil",
			on:   nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 小文字に変更
			if got := hasWorkflowDispatch(tt.on); got != tt.want {
				t.Errorf("hasWorkflowDispatch() = %v, want %v", got, tt.want)
			}
		})
	}
}
