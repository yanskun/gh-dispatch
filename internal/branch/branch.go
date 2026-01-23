package branch

import (
	"fmt"
)

// RESTClient はAPIリクエストを行うためのインターフェース
type RESTClient interface {
	Get(path string, response interface{}) error
}

// Branch はブランチの基本情報を表します
type Branch struct {
	Name string `json:"name"`
}

// FetchBranches は指定されたリポジトリのブランチ一覧を取得します
func FetchBranches(client RESTClient, owner, repo string) ([]Branch, error) {
	var branches []Branch
	path := fmt.Sprintf("repos/%s/%s/branches", owner, repo)

	err := client.Get(path, &branches)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch branches: %w", err)
	}

	return branches, nil
}
