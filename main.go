package main

import (
	"context"
	"log"
	"os"

	"git.sr.ht/~gnome/gitslurp/internal/auth"
	cliPkg "git.sr.ht/~gnome/gitslurp/internal/cli"
	"git.sr.ht/~gnome/gitslurp/internal/config"
	"git.sr.ht/~gnome/gitslurp/internal/github"
	"git.sr.ht/~gnome/gitslurp/internal/service"
	"github.com/urfave/cli/v2"
)

const helpTemplate = `{{.Name}} - {{.Usage}}

Usage: {{.HelpName}} [options] <username|email>

Options:
   {{range .VisibleFlags}}{{.}}
   {{end}}`

func runApp(c *cli.Context) error {
	appConfig, err := config.ParseConfig(c)
	if err != nil {
		return err
	}

	if appConfig == nil {
		return nil
	}

	ctx := context.Background()
	client, err := auth.SetupGitHubClient(c, ctx)
	if err != nil {
		return err
	}

	token := github.GetToken(c)
	orchestrator := service.NewOrchestrator(client, appConfig, token)
	return orchestrator.Run(ctx)
}

func main() {
	log.SetFlags(0)

	app := cliPkg.NewApp(runApp)

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
