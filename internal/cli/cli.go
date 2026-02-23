package cli

import (
	"github.com/gnomegl/gitslurp/internal/art"
	"github.com/gnomegl/gitslurp/internal/utils"
	"github.com/urfave/cli/v2"
)

const helpTemplate = `{{.Name}} - {{.Usage}}

Usage: {{.HelpName}} [options] <username|email>

Options:
   {{range .VisibleFlags}}{{.}}
   {{end}}
`

func NewApp(action cli.ActionFunc) *cli.App {
	cli.AppHelpTemplate = helpTemplate
	art.PrintLogo()

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
				Usage:   "Enable scanning for secrets in commits",
			},
			&cli.BoolFlag{
				Name:    "interesting",
				Aliases: []string{"i"},
				Usage:   "Get interesting strings",
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
				Name:    "json",
				Aliases: []string{"j"},
				Usage:   "Output results in JSON format",
			},
			&cli.BoolFlag{
				Name:  "csv",
				Usage: "Output results in CSV format",
			},
			&cli.BoolFlag{
				Name:    "profile-only",
				Aliases: []string{"p"},
				Usage:   "Show user profile only, skip repository analysis",
			},
			&cli.BoolFlag{
				Name:    "quick",
				Aliases: []string{"q"},
				Usage:   "Quick mode - fetch ~50 most recent commits per repo",
			},
			&cli.BoolFlag{
				Name:    "timestamp-analysis",
				Aliases: []string{"T"},
				Usage:   "Analyze commit timestamps for unusual patterns",
			},
			&cli.BoolFlag{
				Name:    "include-forks",
				Aliases: []string{"F"},
				Usage:   "Include forked repositories in the scan (default: only owned repos)",
			},
		},
		Action:    action,
		ArgsUsage: "<username|email>",
		Authors: []*cli.Author{
			{Name: "gnomegl"},
		},
	}
}
