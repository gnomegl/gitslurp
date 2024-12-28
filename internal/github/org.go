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

// FetchGists retrieves all public gists for a given username
func FetchGists(ctx context.Context, client *github.Client, username string, cfg *Config) ([]*github.Gist, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	var allGists []*github.Gist
	opt := &github.GistListOptions{
		ListOptions: github.ListOptions{PerPage: cfg.PerPage},
	}

	for {
		gists, resp, err := client.Gists.List(ctx, username, opt)
		if err != nil {
			return nil, fmt.Errorf("error fetching gists: %v", err)
		}

		// Fetch the content of each gist
		for _, gist := range gists {
			gistContent, _, err := client.Gists.Get(ctx, gist.GetID())
			if err != nil {
				// Log warning but continue with other gists
				color.Yellow("⚠️  Warning: Could not fetch content for gist %s: %v", gist.GetID(), err)
				continue
			}
			// Update the files with their content
			gist.Files = gistContent.Files
		}

		allGists = append(allGists, gists...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return allGists, nil
}
