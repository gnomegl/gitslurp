package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v2"
)

type AppConfig struct {
	ShowDetails       bool
	CheckSecrets      bool
	SecretsScope      string
	ShowTargetOnly    bool
	ShowInteresting   bool
	ProfileOnly       bool
	ShowStargazers    bool
	ShowForkers       bool
	QuickMode         bool
	TimestampAnalysis bool
	IncludeForks      bool

	SpiderMode   bool
	SpiderDepth  int
	MinRepos     int
	MinFollowers int
	MaxNodes     int
	SpiderOutput string

	OutputFormat string
	Target       string
	Platform     string
	Token        string

	TokenFile string
	Proxy     string
	ProxyFile string
}

// valid scopes for --secrets flag
var validSecretsScopes = map[string]bool{
	"target": true, "members": true, "followers": true,
	"following": true, "stargazers": true,
}

// isSecretsScope checks if a value looks like a valid --secrets scope argument
// (as opposed to a username that got consumed as the flag value)
func isSecretsScope(val string) bool {
	for _, part := range strings.Split(val, ",") {
		if !validSecretsScopes[strings.TrimSpace(strings.ToLower(part))] {
			return false
		}
	}
	return true
}

// NormalizeArgs preprocesses os.Args to handle -s/--secrets with optional value.
// If -s is followed by a non-scope argument (i.e. a username), we insert "target"
// as the default value so the username isn't consumed as the flag value.
// Must be called before cli.App.Run.
func NormalizeArgs() {
	args := os.Args
	var normalized []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		normalized = append(normalized, arg)

		if arg == "-s" || arg == "--secrets" {
			// Check if next arg exists and is a valid scope
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				if isSecretsScope(args[i+1]) {
					// Next arg is a valid scope, let cli consume it normally
					continue
				}
				// Next arg is not a scope (probably a username), insert default
				normalized = append(normalized, "target")
			} else {
				// -s is the last arg or next is another flag, insert default
				normalized = append(normalized, "target")
			}
		}
	}

	os.Args = normalized
}

// extracts the username/email from command line args, ignoring flags
func findTarget() (string, error) {
	args := os.Args[1:]
	var targets []string

	// known flags that take values
	flagsWithValues := map[string]bool{
		"-t": true, "--token": true,
		"--token-file": true,
		"-P": true, "--proxy": true,
		"--proxy-file": true,
		"--depth":          true,
		"--min-repos":      true,
		"--min-followers":  true,
		"--max-nodes":      true,
		"--spider-output":  true,
		"--platform":       true,
		"-s": true, "--secrets": true,
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if strings.HasPrefix(arg, "-") {
			if flagsWithValues[arg] {
				if i+1 < len(args) {
					i++
				}
			}
			continue
		}

		// this is a non-flag argument, could be our target
		targets = append(targets, arg)
	}

	if len(targets) == 0 {
		return "", cli.Exit("Error: No username or email provided", 1)
	}

	if len(targets) > 1 {
		return "", cli.Exit("Error: Only one username or email should be provided", 1)
	}

	return targets[0], nil
}

func ParseConfig(c *cli.Context) (*AppConfig, error) {
	target, err := findTarget()
	if err != nil {
		if len(os.Args) <= 1 {
			return nil, cli.ShowAppHelp(c)
		}
		return nil, err
	}

	outputFormat := "text"
	if c.Bool("json") {
		outputFormat = "json"
	} else if c.Bool("csv") {
		outputFormat = "csv"
	}

	secretsVal := c.String("secrets")
	// If the flag is present but has no value, cli sets it to the string "true" for BoolFlag migration.
	// But since we changed to StringFlag, we need to handle when user passes -s with no arg.
	// urfave/cli will require a value for StringFlag, so -s alone won't work.
	// We handle this in findTarget by checking if "secrets" appears as a bare flag.
	checkSecrets := secretsVal != ""

	platformVal := c.String("platform")
	switch strings.ToLower(platformVal) {
	case "github", "gitlab", "codeberg", "":
	default:
		return nil, fmt.Errorf("unsupported platform: %q (valid: github, gitlab, codeberg)", platformVal)
	}

	return &AppConfig{
		ShowDetails:       c.Bool("details"),
		CheckSecrets:      checkSecrets,
		SecretsScope:      secretsVal,
		ShowTargetOnly:    false,
		ShowInteresting:   c.Bool("interesting"),
		ProfileOnly:       c.Bool("profile-only"),
		ShowStargazers:    c.Bool("show-stargazers"),
		ShowForkers:       c.Bool("show-forkers"),
		QuickMode:         c.Bool("quick"),
		TimestampAnalysis: c.Bool("timestamp-analysis"),
		IncludeForks:      c.Bool("include-forks"),

		SpiderMode:   c.Bool("spider"),
		SpiderDepth:  c.Int("depth"),
		MinRepos:     c.Int("min-repos"),
		MinFollowers: c.Int("min-followers"),
		MaxNodes:     c.Int("max-nodes"),
		SpiderOutput: c.String("spider-output"),

		OutputFormat: outputFormat,
		Target:       target,

		Platform:  c.String("platform"),
		Token:     c.String("token"),

		TokenFile: c.String("token-file"),
		Proxy:     c.String("proxy"),
		ProxyFile: c.String("proxy-file"),
	}, nil
}
