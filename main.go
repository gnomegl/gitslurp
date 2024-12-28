package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/gnomegl/gitslurp/internal/art"
	"github.com/gnomegl/gitslurp/internal/github"
	"github.com/gnomegl/gitslurp/internal/models"
	gh "github.com/google/go-github/v57/github"
	"github.com/urfave/cli/v2"
)

const helpTemplate = `{{.Name}} - {{.Usage}}

Usage: {{.HelpName}} [options] <username|email>

Options:
   {{range .VisibleFlags}}{{.}}
   {{end}}`

// based: version injected at build time
var (
	version   = "dev"
	repoOwner = "gnomegl"
	repoName  = "gitslurp"
)

func checkLatestVersion(ctx context.Context, client *gh.Client) {
	release, _, err := client.Repositories.GetLatestRelease(ctx, repoOwner, repoName)
	if err != nil {
		return // Silently fail version check
	}

	latestVersion := strings.TrimPrefix(release.GetTagName(), "v")
	if latestVersion != version {
		color.Yellow("⚠️  A new version of gitslurp is available: %s (you're running %s)",
			latestVersion, version)
		color.Yellow("   Update at: https://github.com/%s/%s/releases/latest",
			repoOwner, repoName)
		fmt.Println()
	}
}

func runApp(c *cli.Context) error {
	token := github.GetToken(c)
	client := github.GetGithubClient(token)
	checkLatestVersion(context.Background(), client)

	cfg := github.DefaultConfig()

	// sneed: cli args parsing is jank but werks
	args := c.Args().Slice()
	if len(args) > 0 {
		firstArgIndex := -1
		for i, arg := range os.Args[1:] {
			if !strings.HasPrefix(arg, "-") {
				firstArgIndex = i + 1
				break
			}
		}

		if firstArgIndex > 0 {
			for _, arg := range os.Args[firstArgIndex+1:] {
				if strings.HasPrefix(arg, "-") {
					return cli.ShowAppHelp(c)
				}
			}
		}
	}

	if c.NArg() < 1 {
		return cli.ShowAppHelp(c)
	}

	input := c.Args().First()
	showDetails := c.Bool("details")
	checkSecrets := c.Bool("secrets")
	showLinks := c.Bool("links")
	showTargetOnly := true
	if c.Bool("all") {
		showTargetOnly = false
	}

	ctx := context.Background()

	if token != "" {
		if err := github.ValidateToken(ctx, client); err != nil {
			return fmt.Errorf("token validation failed: %v", err)
		}
	}

	username := input
	var lookupEmail string
	if strings.Contains(input, "@") {
		lookupEmail = input
		color.Blue("\nLooking up GitHub user for email: %s", input)

		user, err := github.GetUserByEmail(ctx, client, input)
		if err != nil {
			color.Red("❌ Error: %v", err)
			return nil
		}

		if user == nil {
			color.Red("❌ No GitHub user found for email: %s", input)
			return nil
		}

		username = user.GetLogin()
		color.Green("✓ Found GitHub user: %s", username)
	} else {
		color.Blue("\nTarget user: %s", username)
	}
	fmt.Println()

	var user *gh.User
	var isOrg bool
	var err error

	if lookupEmail == "" {
		user, _, err = client.Users.Get(ctx, username)
		if err != nil {
			color.Red("❌ Error fetching details: %v", err)
			return nil
		}

		if user.GetType() == "Organization" {
			isOrg = true
			color.Green("✅ Organization detected: %s", user.GetLogin())
			if user.GetName() != "" {
				fmt.Printf("Name: %s\n", user.GetName())
			}
		} else {
			color.Green("✅ User detected: %s", user.GetLogin())
			if user.GetName() != "" {
				fmt.Printf("Name: %s\n", user.GetName())
			}
			if user.GetBio() != "" {
				fmt.Printf("Bio: %s\n", user.GetBio())
			}
		}
	}

	var repos []*gh.Repository
	if isOrg {
		repos, err = github.FetchOrgRepos(ctx, client, username, cfg)
	} else {
		repos, err = github.FetchRepos(ctx, client, username, cfg)
	}

	if err != nil {
		color.Red("❌ Error: %v", err)
		return nil
	}

	if len(repos) == 0 {
		if isOrg {
			color.Red("❌ No public repositories found for organization: %s", username)
		} else {
			color.Red("❌ No public repositories found for user: %s", username)
		}
		return nil
	}

	emails := github.ProcessRepos(ctx, client, repos, checkSecrets, showLinks, cfg)
	if len(emails) == 0 {
		if isOrg {
			return fmt.Errorf("no commits found for organization: %s", username)
		}
		return fmt.Errorf("no commits found for user: %s", username)
	}

	// sneed: progress bar needs artificial delay to avoid race condition
	time.Sleep(500 * time.Millisecond)
	fmt.Println()

	displayResults(emails, showDetails, checkSecrets, showLinks, lookupEmail, username, user, showTargetOnly, isOrg)
	return nil
}

func isUserIdentifier(identifier string, userIdentifiers map[string]bool) bool {
	return userIdentifiers[identifier]
}

func displayResults(emails map[string]*models.EmailDetails, showDetails bool, checkSecrets bool, showLinks bool, lookupEmail string, knownUsername string, user *gh.User, showTargetOnly bool, isOrg bool) {
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

	userIdentifiers := map[string]bool{
		knownUsername: true,
		lookupEmail:   true,
	}
	if user != nil {
		userIdentifiers[user.GetLogin()] = true
		userIdentifiers[user.GetName()] = true
		userIdentifiers[user.GetEmail()] = true
	}

	totalCommits := 0
	totalContributors := 0

	if user != nil {
		accountType := "User"
		if isOrg {
			accountType = "Organization"
		}
		fmt.Printf("\n%s Account Information:\n", accountType)
	}

	fmt.Println("\nCollected author information:")
	for _, entry := range sortedEmails {
		// Check if this email or any associated names belong to target user
		isTargetUser := isUserIdentifier(entry.Email, userIdentifiers)
		if !isTargetUser {
			for name := range entry.Details.Names {
				if isUserIdentifier(name, userIdentifiers) {
					isTargetUser = true
					break
				}
			}
		}

		if showTargetOnly && !isTargetUser {
			continue
		}

		totalContributors++

		if isTargetUser {
			totalCommits += entry.Details.CommitCount
			color.HiYellow("📍 %s (Target User)", entry.Email)
			names := make([]string, 0, len(entry.Details.Names))
			for name := range entry.Details.Names {
				names = append(names, name)
			}
			color.HiGreen("  ✓ Names used: %s", strings.Join(names, ", "))
			color.HiGreen("  ✓ Total Commits: %d", entry.Details.CommitCount)
		} else {
			color.Yellow(entry.Email)
			names := make([]string, 0, len(entry.Details.Names))
			for name := range entry.Details.Names {
				names = append(names, name)
			}
			color.White("  Names: %s", strings.Join(names, ", "))
			color.White("  Total Commits: %d", entry.Details.CommitCount)
		}

		if showDetails || checkSecrets || showLinks {
			for repoName, commits := range entry.Details.Commits {
				if isTargetUser {
					color.HiGreen("  📂 Repo: %s", repoName)
				} else {
					color.Green("  Repo: %s", repoName)
				}

				for _, commit := range commits {
					shouldShowCommit := showDetails ||
						(checkSecrets && len(commit.Secrets) > 0) ||
						(showLinks && len(commit.Links) > 0)

					if !shouldShowCommit {
						continue
					}

					isTargetCommit := isTargetUser || isUserIdentifier(commit.AuthorName, userIdentifiers) || isUserIdentifier(commit.AuthorEmail, userIdentifiers)

					if isTargetCommit {
						color.HiMagenta("    ⭐ Commit: %s", commit.Hash)
						color.HiBlue("    🔗 URL: %s", commit.URL)
						color.HiWhite("    👤 Author: %s <%s>", commit.AuthorName, commit.AuthorEmail)
					} else {
						color.Magenta("    Commit: %s", commit.Hash)
						color.Blue("    URL: %s", commit.URL)
						color.White("    Author: %s <%s>", commit.AuthorName, commit.AuthorEmail)
					}

					if commit.IsOwnRepo {
						color.Cyan("    Owner: true")
					}
					if commit.IsFork {
						color.Cyan("    Fork: true")
					}

					if checkSecrets && len(commit.Secrets) > 0 {
						if isTargetCommit {
							color.HiRed("    ⚠️  Potential secrets found:")
							for _, secret := range commit.Secrets {
								color.HiRed("      - %s", secret)
							}
						} else {
							color.Red("    Potential secrets found:")
							for _, secret := range commit.Secrets {
								color.Red("      - %s", secret)
							}
						}
					}

					if showLinks && len(commit.Links) > 0 {
						if isTargetCommit {
							color.HiBlue("    🔍 Links found:")
							for _, link := range commit.Links {
								color.HiBlue("      - %s", link)
							}
						} else {
							color.Blue("    Links found:")
							for _, link := range commit.Links {
								color.Blue("      - %s", link)
							}
						}
					}
				}
			}
		}
	}

	if showTargetOnly {
		color.HiCyan("\nTotal commits by target user: %d", totalCommits)
	} else {
		color.HiCyan("\nTotal contributors: %d", totalContributors)
	}
}

func main() {
	cli.AppHelpTemplate = helpTemplate
	// Configure logger to only show the message
	log.SetFlags(0)

	app := &cli.App{
		Name:    "gitslurp",
		Usage:   "OSINT tool to analyze GitHub user's commit history across repositories",
		Version: version,
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
				Usage:   "Enable secret detection in commits",
			},
			&cli.BoolFlag{
				Name:    "links",
				Aliases: []string{"l"},
				Usage:   "Show URLs found in commit messages",
			},
			&cli.BoolFlag{
				Name:    "all",
				Aliases: []string{"a"},
				Usage:   "Show commits from all contributors in the target's repositories",
			},
		},
		Action: runApp,
		Before: func(c *cli.Context) error {
			if c.Args().Len() == 0 && !c.Bool("help") && !c.Bool("version") {
				art.PrintLogo()
				cli.ShowAppHelp(c)
				return cli.Exit("", 1)
			}
			if !c.Bool("help") && !c.Bool("version") {
				art.PrintLogo()
				fmt.Println()
			}
			return nil
		},
		Authors: []*cli.Author{
			{
				Name: "gnomegl",
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
