package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/gnomegl/gitslurp/internal/config"
	"github.com/gnomegl/gitslurp/internal/github"
	gh "github.com/google/go-github/v57/github"
	"github.com/urfave/cli/v2"
)

func SetupClientPool(c *cli.Context, ctx context.Context, appConfig *config.AppConfig) (*github.ClientPool, error) {
	var tokens []string

	if appConfig.TokenFile != "" {
		var err error
		tokens, err = github.ReadTokenFile(appConfig.TokenFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read token file: %v", err)
		}
		if c.String("token") != "" {
			color.Yellow("[!] --token-file takes precedence over --token")
		}
	} else {
		token := github.GetToken(c)
		if token != "" {
			tokens = []string{token}
		}
	}

	var proxies []string
	if appConfig.ProxyFile != "" {
		var err error
		proxies, err = github.ReadProxyFile(appConfig.ProxyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read proxy file: %v", err)
		}
	} else if appConfig.Proxy != "" {
		proxy := appConfig.Proxy
		if !strings.Contains(proxy, "://") {
			proxy = "http://" + proxy
		}
		proxies = []string{proxy}
	}

	pool, err := github.NewClientPool(tokens, proxies)
	if err != nil {
		return nil, fmt.Errorf("failed to create client pool: %v", err)
	}

	primary := pool.GetClient()
	checkLatestVersion(ctx, primary.Client)

	if pool.PrimaryToken() != "" {
		if err := github.ValidateToken(ctx, primary.Client); err != nil {
			return nil, fmt.Errorf("token validation failed: %v", err)
		}
	}

	if pool.Size() > 1 {
		color.Green("[+] Token pool initialized with %d tokens", pool.Size())
	}

	return pool, nil
}

func checkLatestVersion(ctx context.Context, client *gh.Client) {
	// Version checking disabled for sr.ht - no equivalent API
	// To check for updates manually: go install github.com/gnomegl/gitslurp@latest
}
