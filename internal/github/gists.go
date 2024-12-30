package github

import (
	"context"
	"fmt"

	"github.com/fatih/color"
	"github.com/google/go-github/v57/github"
)

// FetchGists retrieves all public gists for a given username
func FetchGists(ctx context.Context, client *github.Client, username string, cfg *Config) ([]*github.Gist, error) {
	if cfg == nil {
		cfg = &Config{}
		*cfg = DefaultConfig()
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
