package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v57/github"
)

// based: unified repo fetching for orgs
func FetchOrgRepos(ctx context.Context, client *github.Client, orgName string, cfg *Config) ([]*github.Repository, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	var allRepos []*github.Repository
	opt := &github.RepositoryListByOrgOptions{
		Type:        "public",
		ListOptions: github.ListOptions{PerPage: cfg.PerPage},
	}

	for {
		repos, resp, err := client.Repositories.ListByOrg(ctx, orgName, opt)
		if err != nil {
			return nil, fmt.Errorf("error fetching repositories: %v", err)
		}
		allRepos = append(allRepos, repos...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return allRepos, nil
}
