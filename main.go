package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/gnomegl/gitslurp/internal/auth"
	cliPkg "github.com/gnomegl/gitslurp/internal/cli"
	"github.com/gnomegl/gitslurp/internal/config"
	"github.com/gnomegl/gitslurp/internal/service"
	"github.com/urfave/cli/v2"
)

func hasStructuredOutputFlag() bool {
	for _, arg := range os.Args[1:] {
		if arg == "--json" || arg == "--csv" {
			return true
		}
		if arg == "--" {
			return false
		}
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
			if strings.ContainsRune(arg, 'j') {
				return true
			}
		}
	}
	return false
}

func main() {
	realStdout := os.Stdout
	realStderr := os.Stderr

	if hasStructuredOutputFlag() {
		devNull, _ := os.Open(os.DevNull)
		defer devNull.Close()
		os.Stdout = devNull
		os.Stderr = devNull
		color.Output = io.Discard
	}

	app := cliPkg.NewApp(func(c *cli.Context) error {
		appConfig, err := config.ParseConfig(c)
		if err != nil {
			return err
		}

		if appConfig == nil {
			return nil
		}

		ctx := c.Context
		pool, err := auth.SetupClientPool(c, ctx, appConfig)
		if err != nil {
			return err
		}

		orchestrator := service.NewOrchestrator(pool, appConfig, realStdout)
		return orchestrator.Run(ctx)
	})

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(realStderr, err)
		os.Exit(1)
	}
}
