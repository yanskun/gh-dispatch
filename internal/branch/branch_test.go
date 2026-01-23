package branch

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

// mockRESTClient は branch.RESTClient のモックです
type mockRESTClient struct {
	ResponseData interface{}
	Error        error
}

func (m *mockRESTClient) Get(path string, response interface{}) error {
	if m.Error != nil {
		return m.Error
	}
	
	b, _ := json.Marshal(m.ResponseData)
	return json.Unmarshal(b, response)
}

func TestFetchBranches(t *testing.T) {
	tests := []struct {
		name          string
		mockData      []Branch
		mockError     error
		owner         string
		repo          string
		want          []Branch
		wantErrString string
	}{
		{
			name: "success",
			mockData: []Branch{
				{Name: "main"},
				{Name: "develop"},
			},
			owner: "user",
			repo:  "repo",
			want: []Branch{
				{Name: "main"},
				{Name: "develop"},
			},
		},
		{
			name:      "api error",
			mockError: fmt.Errorf("api error"),
			owner:     "user",
			repo:      "repo",
			want:      nil,
			wantErrString: "failed to fetch branches: api error",
		},
		{
			name:     "empty branches",
			mockData: []Branch{},
			owner:    "user",
			repo:     "repo",
			want:     []Branch{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockRESTClient{
				ResponseData: tt.mockData,
				Error:        tt.mockError,
			}

			got, err := FetchBranches(client, tt.owner, tt.repo)

			if tt.wantErrString != "" {
				if err == nil {
					t.Errorf("FetchBranches() expected error containing %q, got nil", tt.wantErrString)
				} else if err.Error() != tt.wantErrString {
					t.Errorf("FetchBranches() error = %v, want %v", err, tt.wantErrString)
				}
				return
			}

			if err != nil {
				t.Fatalf("FetchBranches() unexpected error: %v", err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FetchBranches() = %v, want %v", got, tt.want)
			}
		})
	}
}