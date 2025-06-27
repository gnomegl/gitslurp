package config

import (
	"github.com/urfave/cli/v2"
	"os"
	"strings"
)

type AppConfig struct {
	ShowDetails     bool
	CheckSecrets    bool
	ShowTargetOnly  bool
	ShowInteresting bool
	ProfileOnly     bool
	ShowStargazers  bool
	ShowForkers     bool
	QuickMode       bool
	TimestampAnalysis bool
	Target          string
}

// extracts the username/email from command line args, ignoring flags
func findTarget() (string, error) {
	args := os.Args[1:] 
	var targets []string

	// known flags that take values
  // TODO: enumerate the flags for this
	flagsWithValues := map[string]bool{
		"-t": true, "--token": true,
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

	return &AppConfig{
		ShowDetails:     c.Bool("details"),
		CheckSecrets:    c.Bool("secrets"),
		ShowTargetOnly:  false,
		ShowInteresting: c.Bool("interesting"),
		ProfileOnly:     c.Bool("profile-only"),
		ShowStargazers:  c.Bool("show-stargazers"),
		ShowForkers:     c.Bool("show-forkers"),
		QuickMode:       c.Bool("quick"),
		TimestampAnalysis: c.Bool("timestamp-analysis"),
		Target:          target,
	}, nil
}

