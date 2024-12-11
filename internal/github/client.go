package github

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/google/go-github/v57/github"
	"github.com/urfave/cli/v2"
	"golang.org/x/oauth2"
)

func GetGithubClient(token string) *github.Client {
	if token == "" {
		return github.NewClient(nil)
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(context.Background(), ts)
	return github.NewClient(tc)
}

func GetToken(c *cli.Context) string {
	token := os.Getenv("GITHUB_TOKEN")
	if token != "" {
		return token
	}

	if c.String("token") != "" {
		return c.String("token")
	}

	configDir, err := os.UserConfigDir()
	if err == nil {
		tokenFile := filepath.Join(configDir, "gitslurp", "token")
		if data, err := os.ReadFile(tokenFile); err == nil {
			token = strings.TrimSpace(string(data))
			if token != "" {
				color.Green("Using saved token from config file")
				return token
			}
		}
	}

	color.Yellow("\nA GitHub personal access token is recommended to avoid rate limits and access private repositories.")
	color.Blue("To create a new token:")
	fmt.Println("1. Visit: https://github.com/settings/tokens")
	fmt.Println("2. Click 'Generate new token' (classic)")
	fmt.Println("3. Give it a name (e.g. 'gitslurp')")
	fmt.Println("4. Select the following scopes:")
	color.Green("   - repo (for private repos)")
	color.Green("   - read:user")
	color.Green("   - user:email")
	fmt.Println("5. Click 'Generate token' at the bottom")
	fmt.Println("6. Copy the token and paste it below")
	fmt.Println("\nNote: The token will be saved locally for future use")

	fmt.Print("\nPaste your token here (or press Enter to continue without one): ")
	var input string
	fmt.Scanln(&input)
	token = strings.TrimSpace(input)

	if token != "" {
		if configDir != "" {
			configPath := filepath.Join(configDir, "gitslurp")
			os.MkdirAll(configPath, 0700)
			tokenFile := filepath.Join(configPath, "token")
			if err := os.WriteFile(tokenFile, []byte(token), 0600); err == nil {
				color.Green("Token saved successfully")
			}
		}
	} else {
		color.Yellow("\nRunning without a token. You may hit rate limits and won't see private repository information.")
	}

	return token
}
