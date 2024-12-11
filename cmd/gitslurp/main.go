//go:build !windows
// +build !windows

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/gnomegl/gitslurp/internal/github"
	"github.com/gnomegl/gitslurp/internal/models"
	"github.com/urfave/cli/v2"
)

func runApp(c *cli.Context) error {
	if c.NArg() < 1 {
		return cli.ShowAppHelp(c)
	}

	username := c.Args().First()
	token := github.GetToken(c)
	showDetails := c.Bool("details")
	checkSecrets := c.Bool("secrets")
	showLinks := c.Bool("links")

	client := github.GetGithubClient(token)
	ctx := context.Background()

	color.Blue("Fetching public repositories for user: %s", username)
	repos, err := github.FetchRepos(ctx, client, username)
	if err != nil {
		return fmt.Errorf("error fetching repositories: %v", err)
	}

	emails := github.ProcessRepos(ctx, client, repos, checkSecrets, showLinks)
	displayResults(emails, showDetails, checkSecrets, showLinks)
	return nil
}

func displayResults(emails map[string]*models.EmailDetails, showDetails bool, checkSecrets bool, showLinks bool) {
	type emailEntry struct {
		Email   string
		Details *models.EmailDetails
	}

	var sortedEmails []emailEntry
	for email, details := range emails {
		sortedEmails = append(sortedEmails, emailEntry{email, details})
	}

	sort.Slice(sortedEmails, func(i, j int) bool {
		return sortedEmails[i].Details.CommitCount > sortedEmails[j].Details.CommitCount
	})

	fmt.Println("\nCollected author information:")
	for _, entry := range sortedEmails {
		color.Yellow(entry.Email)

		if showDetails || checkSecrets || showLinks {
			for repoName, commits := range entry.Details.Commits {
				color.Green("  Repo: %s", repoName)
				for _, commit := range commits {
					shouldShowCommit := showDetails ||
						(checkSecrets && len(commit.Secrets) > 0) ||
						(showLinks && len(commit.Links) > 0)

					if !shouldShowCommit {
						continue
					}

					color.Magenta("    Commit: %s", commit.Hash)
					color.Blue("    URL: %s", commit.URL)
					color.White("    Author: %s", commit.AuthorName)
					if commit.IsFork {
						color.Cyan("    Fork: true")
					}

					if checkSecrets && len(commit.Secrets) > 0 {
						color.Red("    Potential secrets found:")
						for _, secret := range commit.Secrets {
							color.Red("      - %s", secret)
						}
					}

					if showLinks && len(commit.Links) > 0 {
						color.Blue("    Links found:")
						for _, link := range commit.Links {
							color.Blue("      - %s", link)
						}
					}
				}
			}
		} else {
			names := make([]string, 0, len(entry.Details.Names))
			for name := range entry.Details.Names {
				names = append(names, name)
			}
			color.White("  Names: %s", strings.Join(names, ", "))
			color.White("  Total Commits: %d", entry.Details.CommitCount)
		}
	}
}

func main() {
	app := &cli.App{
		Name:  "gitslurp",
		Usage: "Analyze GitHub user's commit history across repositories",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "details",
				Aliases: []string{"d"},
				Usage:   "Show detailed commit information",
			},
			&cli.BoolFlag{
				Name:    "secrets",
				Aliases: []string{"s"},
				Usage:   "Check for potential secrets in commits",
			},
			&cli.BoolFlag{
				Name:    "links",
				Aliases: []string{"l"},
				Usage:   "Extract links from commits",
			},
			&cli.StringFlag{
				Name:    "token",
				Aliases: []string{"t"},
				Usage:   "GitHub personal access token",
				EnvVars: []string{"GITHUB_TOKEN"},
			},
		},
		Action: runApp,
		Authors: []*cli.Author{
			{
				Name: "GitSlurp Team",
			},
		},
		Version: "1.0.0",
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
