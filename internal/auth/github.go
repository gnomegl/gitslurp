package auth

import (
	"context"
	"fmt"

	"github.com/gnomegl/gitslurp/internal/github"
	"github.com/gnomegl/gitslurp/internal/utils"
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
	release, _, err := client.Repositories.GetLatestRelease(ctx, "gnomegl", "gitslurp")
	if err != nil {
		return
	}

	latestVersion := release.GetTagName()
	if latestVersion[0] == 'v' {
		latestVersion = latestVersion[1:]
	}
	
	if latestVersion != utils.GetVersion() {
		fmt.Printf("\x1b[33mA new version of gitslurp is available: %s (you're running %s)\x1b[0m\n", latestVersion, utils.GetVersion())
		fmt.Println("\x1b[33mTo update: \x1b[0m")
		fmt.Println("\x1b[36mgo install github.com/gnomegl/gitslurp@latest\x1b[0m")
		fmt.Println()
	}
}