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
	if c.String("token") != "" {
		token := c.String("token")
		configDir, err := os.UserConfigDir()
		if err == nil && configDir != "" {
			configPath := filepath.Join(configDir, "gitslurp")
			os.MkdirAll(configPath, 0700)
			tokenFile := filepath.Join(configPath, "token")
			if err := os.WriteFile(tokenFile, []byte(token), 0600); err == nil {
				color.Green("Token saved successfully")
			}
		}
		return token
	}

	token := os.Getenv("GITSLURP_GITHUB_TOKEN")
	if token != "" {
		return token
	}

	configDir, err := os.UserConfigDir()
	if err == nil {
		tokenFile := filepath.Join(configDir, "gitslurp", "token")
		if data, err := os.ReadFile(tokenFile); err == nil {
			token = strings.TrimSpace(string(data))
			if token != "" {
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

func GetUsernameForEmail(ctx context.Context, client *github.Client, email string) (string, error) {
	searchQuery := fmt.Sprintf("in:email %s", email)
	opts := &github.SearchOptions{
		ListOptions: github.ListOptions{
			PerPage: 1,
		},
	}

	result, _, err := client.Search.Users(ctx, searchQuery, opts)
	if err != nil {
		return "", fmt.Errorf("error searching for user: %v", err)
	}

	if len(result.Users) == 0 {
		return "", nil
	}

	return result.Users[0].GetLogin(), nil
}

func GetUserByEmail(ctx context.Context, client *github.Client, email string) (*github.User, error) {
	searchQuery := fmt.Sprintf("in:email %s", email)
	opts := &github.SearchOptions{
		ListOptions: github.ListOptions{
			PerPage: 1,
		},
	}

	result, _, err := client.Search.Users(ctx, searchQuery, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to search for user: %w", err)
	}

	if len(result.Users) == 0 {
		return nil, nil
	}

	return result.Users[0], nil
}

func UserExists(ctx context.Context, client *github.Client, username string) (bool, error) {
	_, resp, err := client.Users.Get(ctx, username)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func ValidateToken(ctx context.Context, client *github.Client) error {
	// Try to fetch authenticated user info to validate token
	_, resp, err := client.Users.Get(ctx, "")
	if err != nil {
		if resp != nil {
			switch resp.StatusCode {
			case 401:
				return fmt.Errorf("invalid GitHub token")
			case 403:
				// Rate limited - skip validation, token is likely valid
				color.Yellow("⚠️  Rate limited, skipping token validation")
				return nil
			}
		}
		return fmt.Errorf("error validating token: %v", err)
	}
	return nil
}

func CheckDeleteRepoPermissions(ctx context.Context, client *github.Client) (bool, error) {
	// Check token permissions by examining the X-OAuth-Scopes header
	_, resp, err := client.Users.Get(ctx, "")
	if err != nil {
		if resp != nil && resp.StatusCode == 403 {
			// Rate limited - assume permissions are sufficient to avoid blocking
			color.Yellow("⚠️  Rate limited, skipping permission check")
			return true, nil
		}
		return false, fmt.Errorf("error checking permissions: %v", err)
	}
	
	if resp == nil || resp.Header == nil {
		return false, nil
	}
	
	scopes := resp.Header.Get("X-OAuth-Scopes")
	if scopes == "" {
		return false, nil
	}
	
	// Check if delete_repo scope is present
	scopeList := strings.Split(scopes, ", ")
	for _, scope := range scopeList {
		scope = strings.TrimSpace(scope)
		if scope == "delete_repo" {
			return true, nil
		}
	}
	
	return false, nil
}
