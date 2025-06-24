package config

import (
	"github.com/urfave/cli/v2"
)

type AppConfig struct {
	ShowDetails     bool
	CheckSecrets    bool
	ShowTargetOnly  bool
	ShowInteresting bool
	NoSlurp         bool
	ShowWatchers    bool
	ShowForkers     bool
	Target          string
}

func ParseConfig(c *cli.Context) (*AppConfig, error) {
	if c.NArg() == 0 {
		return nil, cli.ShowAppHelp(c)
	}

	return &AppConfig{
		ShowDetails:     c.Bool("details"),
		CheckSecrets:    c.Bool("secrets"),
		ShowTargetOnly:  false,
		ShowInteresting: c.Bool("interesting"),
		NoSlurp:         c.Bool("no-slurp"),
		ShowWatchers:    c.Bool("show-watchers"),
		ShowForkers:     c.Bool("show-forkers"),
		Target:          c.Args().First(),
	}, nil
}