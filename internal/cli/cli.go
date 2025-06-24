package cli

import (
	"github.com/urfave/cli/v2"
	"github.com/gnomegl/gitslurp/internal/utils"
)

const helpTemplate = `{{.Name}} - {{.Usage}}

Usage: {{.HelpName}} [options] <username|email>

Options:
   {{range .VisibleFlags}}{{.}}
   {{end}}`

func NewApp(action cli.ActionFunc) *cli.App {
	cli.AppHelpTemplate = helpTemplate

	return &cli.App{
		Name:    "gitslurp",
		Usage:   "OSINT tool to analyze GitHub user's commit history across repositories",
		Version: "v" + utils.GetVersion(),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "token",
				Aliases: []string{"t"},
				Usage:   "GitHub personal access token",
				EnvVars: []string{"GITSLURP_GITHUB_TOKEN"},
			},
			&cli.BoolFlag{
				Name:    "details",
				Aliases: []string{"d"},
				Usage:   "Show detailed commit information",
			},
			&cli.BoolFlag{
				Name:    "secrets",
				Aliases: []string{"s"},
				Usage:   "Enable sniffing for secrets in commits 🐽",
			},
			&cli.BoolFlag{
				Name:    "all",
				Aliases: []string{"a"},
				Usage:   "Show commits from all contributors in the target's repositories",
			},
			&cli.BoolFlag{
				Name:    "interesting",
				Aliases: []string{"i"},
				Usage:   "Get interesting strings ⭐",
			},
			&cli.BoolFlag{
				Name:    "show-watchers",
				Aliases: []string{"w"},
				Usage:   "Show users who watched/starred the repository",
			},
			&cli.BoolFlag{
				Name:    "show-forkers",
				Aliases: []string{"f"},
				Usage:   "Show users who forked the repository",
			},
			&cli.BoolFlag{
				Name:    "output-format",
				Aliases: []string{"o"},
				Usage:   "Output format (json, csv, text)",
			},
			&cli.BoolFlag{
				Name:    "no-slurp",
				Aliases: []string{"n"},
				Usage:   "Skip repository enumeration process",
			},
		},
		Action:    action,
		ArgsUsage: "<username|email>",
		Authors: []*cli.Author{
			{Name: "gnomegl"},
		},
	}
}