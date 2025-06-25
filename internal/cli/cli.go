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
		Usage:   "OSINT tool to analyze GitHub user's recent activity and commit history",
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
				Usage:   "🐽 Enable sniffing for secrets in commits",
			},
			&cli.BoolFlag{
				Name:    "interesting",
				Aliases: []string{"i"},
				Usage:   "⭐ Get interesting strings",
			},
			&cli.BoolFlag{
				Name:    "show-stargazers",
				Aliases: []string{"S"},
				Usage:   "Show users who starred the repository",
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
				Name:    "profile-only",
				Aliases: []string{"p"},
				Usage:   "Show user profile only, skip repository analysis",
			},
			&cli.BoolFlag{
				Name:    "quick",
				Aliases: []string{"q"},
				Usage:   "⚡ Quick mode - fetch ~50 most recent commits per repo",
			},
		},
		Action:    action,
		ArgsUsage: "<username|email>",
		Authors: []*cli.Author{
			{Name: "gnomegl"},
		},
	}
}
