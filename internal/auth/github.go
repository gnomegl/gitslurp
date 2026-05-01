package auth

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/gnomegl/gitslurp/v2/internal/config"
	"github.com/gnomegl/gitslurp/v2/internal/github"
	"github.com/gnomegl/gitslurp/v2/internal/utils"
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
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	local := utils.GetVersion()
	if local == "unknown" || local == "(devel)" {
		return
	}

	release, _, err := client.Repositories.GetLatestRelease(ctx, "gnomegl", "gitslurp")
	if err != nil || release.TagName == nil {
		return
	}

	remote := *release.TagName
	if utils.IsNewer(remote, local) {
		installPath := "github.com/gnomegl/gitslurp"
		if maj, _, _, err := utils.ParseVersion(remote); err == nil && maj >= 2 {
			installPath = fmt.Sprintf("%s/v%d", installPath, maj)
		}
		fmt.Fprintf(os.Stderr, "%s %s → %s\n",
			color.YellowString("[*] Update available:"),
			color.RedString("v%s", local),
			color.GreenString("%s", strings.TrimPrefix(remote, "v")))
		fmt.Fprintf(os.Stderr, "%s", color.YellowString("    Update now? [Y/n] "))

		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))

		if answer == "" || answer == "y" || answer == "yes" {
			fmt.Fprintf(os.Stderr, "%s\n", color.CyanString("[*] Running: go install %s@latest", installPath))
			cmd := exec.Command("go", "install", installPath+"@latest")
			cmd.Stdout = os.Stderr
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "%s %v\n", color.RedString("[!] Update failed:"), err)
			} else {
				fmt.Fprintf(os.Stderr, "%s Please re-run your command.\n", color.GreenString("[✓] Updated to %s.", strings.TrimPrefix(remote, "v")))
				os.Exit(0)
			}
		}
	}
}
