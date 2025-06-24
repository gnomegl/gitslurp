package main

import (
	"context"
	"log"
	"os"

	"github.com/gnomegl/gitslurp/internal/auth"
	cliPkg "github.com/gnomegl/gitslurp/internal/cli"
	"github.com/gnomegl/gitslurp/internal/config"
	"github.com/gnomegl/gitslurp/internal/service"
	"github.com/urfave/cli/v2"
)

const helpTemplate = `{{.Name}} - {{.Usage}}

Usage: {{.HelpName}} [options] <username|email>

Options:
   {{range .VisibleFlags}}{{.}}
   {{end}}`

func runApp(c *cli.Context) error {
	// Parse config first to validate arguments
	appConfig, err := config.ParseConfig(c)
	if err != nil {
		return err
	}
	
	// Safety check - this should never happen if ParseConfig works correctly
	if appConfig == nil {
		return nil
	}

	ctx := context.Background()
	client, err := auth.SetupGitHubClient(c, ctx)
	if err != nil {
		return err
	}

	orchestrator := service.NewOrchestrator(client, appConfig)
	return orchestrator.Run(ctx)
}

func main() {
	log.SetFlags(0)
	
	app := cliPkg.NewApp(runApp)
	
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
