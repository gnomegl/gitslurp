package auth

import (
	"context"
	"fmt"

	"github.com/gnomegl/gitslurp/internal/github"
	gh "github.com/google/go-github/v57/github"
	"github.com/urfave/cli/v2"
)

func SetupGitHubClient(c *cli.Context, ctx context.Context) (*gh.Client, error) {
	token := github.GetToken(c)
	client := github.GetGithubClient(token)
	
	checkLatestVersion(ctx, client)

	if token != "" {
		if err := github.ValidateToken(ctx, client); err != nil {
			return nil, fmt.Errorf("token validation failed: %v", err)
		}
	}

	return client, nil
}

func checkLatestVersion(ctx context.Context, client *gh.Client) {
	// Version checking disabled for sr.ht - no equivalent API
	// To check for updates manually: go install git.sr.ht/~gnome/gitslurp@latest
}
