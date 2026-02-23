package github

import (
	"context"
	"fmt"

	"github.com/fatih/color"
	"github.com/google/go-github/v57/github"
)

// based: unified repo fetching for orgs
func FetchOrgRepos(ctx context.Context, client *github.Client, orgName string, cfg *Config) ([]*github.Repository, error) {
	if cfg == nil {
		cfg = &Config{}
		*cfg = DefaultConfig()
	}

	fmt.Println()
	color.Blue("Enumerating organization repositories...")

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

	color.Green("[+] Found %d organization repositories", len(allRepos))

	return allRepos, nil
}

// IsOrganization checks if the given name belongs to a GitHub organization
func IsOrganization(ctx context.Context, client *github.Client, name string) (bool, error) {
	_, resp, err := client.Organizations.Get(ctx, name)
	if err != nil {
		// If we get a 404, it's not an org
		if resp != nil && resp.StatusCode == 404 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
