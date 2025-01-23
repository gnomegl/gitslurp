package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/gnomegl/gitslurp/internal/art"
	gh "github.com/google/go-github/v57/github"
	"github.com/urfave/cli/v2"

	"github.com/gnomegl/gitslurp/internal/github"
	"github.com/gnomegl/gitslurp/internal/models"
	"github.com/gnomegl/gitslurp/internal/utils"
)

const helpTemplate = `{{.Name}} - {{.Usage}}

Usage: {{.HelpName}} [options] <username|email>

Options:
   {{range .VisibleFlags}}{{.}}
   {{end}}`

func checkLatestVersion(ctx context.Context, client *gh.Client) {
	release, _, err := client.Repositories.GetLatestRelease(ctx, "gnomegl", "gitslurp")
	if err != nil {
		return // Silently fail version check
	}

	latestVersion := strings.TrimPrefix(release.GetTagName(), "v")
	if latestVersion != utils.GetVersion() {
		color.Yellow("A new version of gitslurp is available: %s (you're running %s)",
			latestVersion, utils.GetVersion())
		color.Yellow("To update: ")
		color.Cyan("go install github.com/gnomegl/gitslurp@latest")
		fmt.Println()
	}
}

func runApp(c *cli.Context) error {
	allArgs := os.Args[1:]

	showDetails := false
	checkSecrets := false
	showTargetOnly := true
	showInteresting := false
	noSlurp := false
	var target string

	for _, arg := range allArgs {
		switch arg {
		case "-d", "--details":
			showDetails = true
		case "-s", "--secrets":
			checkSecrets = true
		case "-a", "--all":
			showTargetOnly = false
		case "-i", "--interesting":
			showInteresting = true
		case "-n", "--no-slurp":
			noSlurp = true
		default:
			if !strings.HasPrefix(arg, "-") {
				target = arg
			}
		}
	}

	if target == "" {
		return cli.ShowAppHelp(c)
	}

	token := github.GetToken(c)
	client := github.GetGithubClient(token)
	checkLatestVersion(context.Background(), client)

	cfg := github.DefaultConfig()
	cfg.ShowInteresting = showInteresting
	input := target

	if token != "" {
		if err := github.ValidateToken(context.Background(), client); err != nil {
			return fmt.Errorf("token validation failed: %v", err)
		}
	}

	username := input
	var lookupEmail string
	if strings.Contains(input, "@") {
		lookupEmail = input
		color.Blue("\nLooking up GitHub user for email: %s", input)

		user, err := github.GetUserByEmail(context.Background(), client, input)
		if err != nil {
			color.Red("❌ Error: %v", err)
			return nil
		}

		if user == nil {
			color.Red("❌ No GitHub user found for email: %s", input)
			return nil
		}

		username = user.GetLogin()
		color.Green("Found GitHub user: %s", username)
	} else {
		color.Blue("\nTarget user: %s", username)
	}
	fmt.Println()

	var user *gh.User
	var isOrg bool
	var err error

	if lookupEmail == "" {
		// Check if target is an organization
		isOrg, err = github.IsOrganization(context.Background(), client, username)
		if err != nil {
			color.Red("❌ Error checking organization status: %v", err)
			return nil
		}

		if isOrg {
			// Force show all commits for organizations
			showTargetOnly = false
			color.Green("✅ Organization detected: %s", username)
		}

		user, _, err = client.Users.Get(context.Background(), username)
		if err != nil {
			color.Red("❌ Error fetching details: %v", err)
			return nil
		}

		if isOrg {
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

	if noSlurp {
		return nil
	}

	var repos []*gh.Repository
	var gists []*gh.Gist
	if isOrg {
		repos, err = github.FetchOrgRepos(context.Background(), client, username, &cfg)
	} else {
		repos, err = github.FetchRepos(context.Background(), client, username, &cfg)
		if err != nil {
			color.Red("❌ Error: %v", err)
			return nil
		}

		// Only fetch gists for users, not organizations
		gists, err = github.FetchGists(context.Background(), client, username, &cfg)
		if err != nil {
			// We only warn for gist errors since they're not critical
			color.Yellow("⚠️  Warning: Could not fetch gists: %v", err)
		}
	}

	if err != nil {
		color.Red("❌ Error: %v", err)
		return nil
	}

	if len(repos) == 0 && len(gists) == 0 {
		if isOrg {
			color.Red("❌ No public repositories found for organization: %s", username)
		} else {
			color.Red("❌ No public repositories or gists found for user: %s", username)
		}
		return nil
	}

	// Event tracking flags
	showWatchEvents := c.Bool("show-watchers")
	showForkEvents := c.Bool("show-forkers")

	watchers := make(map[string]struct{}) // Using map to deduplicate
	forkers := make(map[string]struct{})

	opts := &gh.ListOptions{
		PerPage: 100, // Get maximum items per page
	}

	// collect watchers and forkers for each repository
	for _, repo := range repos {
		if showWatchEvents {
			stargazers, _, err := client.Activity.ListStargazers(context.Background(), repo.GetOwner().GetLogin(), repo.GetName(), opts)
			if err != nil {
				color.Yellow("⚠️  Warning: Could not fetch stargazers for %s: %v", repo.GetFullName(), err)
				continue
			}
			for _, stargazer := range stargazers {
				watchers[stargazer.User.GetLogin()] = struct{}{}
			}
		}

		if showForkEvents {
			forks, _, err := client.Repositories.ListForks(context.Background(), repo.GetOwner().GetLogin(), repo.GetName(), &gh.RepositoryListForksOptions{
				ListOptions: *opts,
			})
			if err != nil {
				color.Yellow("⚠️  Warning: Could not fetch forks for %s: %v", repo.GetFullName(), err)
				continue
			}
			for _, fork := range forks {
				forkers[fork.GetOwner().GetLogin()] = struct{}{}
			}
		}
	}

	// use sorted slices
	var watchersList []string
	for watcher := range watchers {
		watchersList = append(watchersList, watcher)
	}
	sort.Strings(watchersList)

	var forkersList []string
	for forker := range forkers {
		forkersList = append(forkersList, forker)
	}
	sort.Strings(forkersList)

	if showForkEvents {
		forkersFile := fmt.Sprintf("%s_forkers.txt", target)
		content := strings.Join(forkersList, "\n")
		if len(forkersList) > 50 {
			if err := os.WriteFile(forkersFile, []byte(content), 0644); err != nil {
				return fmt.Errorf("failed to write forkers file: %v", err)
			}
			fmt.Printf("\nForkers list exceeds 50 entries, written to %s\n", forkersFile)
		} else if len(forkersList) > 0 {
			fmt.Println("\nRepository Forkers:")
			for _, forker := range forkersList {
				fmt.Printf("🔱 %s\n", forker)
			}
			if err := os.WriteFile(forkersFile, []byte(content), 0644); err != nil {
				return fmt.Errorf("failed to write forkers file: %v", err)
			}
		} else {
			fmt.Println("\nNo forks found")
		}
	}

	if showWatchEvents {
		if len(watchersList) > 50 {
      watchersFile := fmt.Sprintf("%s_watchers.txt", target)
			content := strings.Join(watchersList, "\n")
			if err := os.WriteFile(watchersFile, []byte(content), 0644); err != nil {
				return fmt.Errorf("failed to write watchers file: %v", err)
			}
			fmt.Printf("\nWatchers list exceeds 50 entries, written to %s\n", watchersFile)
		} else if len(watchersList) > 0 {
			fmt.Println("\nRepository Watchers:")
			for _, watcher := range watchersList {
				fmt.Printf("👁️  %s\n", watcher)
			}
		} else {
			fmt.Println("\nNo watchers found")
		}
	}

	emails := github.ProcessRepos(context.Background(), client, repos, checkSecrets, &cfg)

	if len(gists) > 0 && (checkSecrets || cfg.ShowInteresting) {
		var scanType string
		if checkSecrets && cfg.ShowInteresting {
			scanType = "secrets and patterns"
		} else if checkSecrets {
			scanType = "secrets"
		} else {
			scanType = "interesting patterns"
		}
		color.Blue("\nProcessing %d public gists for %s...", len(gists), scanType)
		gistEmails := github.ProcessGists(context.Background(), client, gists, checkSecrets, &cfg)

		for email, details := range gistEmails {
			if existing, ok := emails[email]; ok {
				// Merge names
				for name := range details.Names {
					existing.Names[name] = struct{}{}
				}
				// Merge commits
				for repoName, commits := range details.Commits {
					existing.Commits[repoName] = append(existing.Commits[repoName], commits...)
				}
				existing.CommitCount += details.CommitCount
			} else {
				emails[email] = details
			}
		}
	}

	if len(emails) == 0 {
		if isOrg {
			if len(repos) > 0 {
				color.Yellow("\n⚔️  All commits in this organization's repositories are anonymous")
			} else {
				return fmt.Errorf("no repositories found for organization: %s", username)
			}
		} else {
			return fmt.Errorf("no commits or gists found for user: %s", username)
		}
	} else {
		displayResults(emails, showDetails, checkSecrets, lookupEmail, username, user, showTargetOnly, isOrg, &cfg)
	}

	return nil
}

func isUserIdentifier(identifier string, userIdentifiers map[string]bool) bool {
	return userIdentifiers[identifier]
}

func displayResults(emails map[string]*models.EmailDetails, showDetails bool, checkSecrets bool, lookupEmail string, knownUsername string, user *gh.User, showTargetOnly bool, isOrg bool, cfg *github.Config) {
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

		totalContributors++

		if showTargetOnly && !isTargetUser {
			continue
		}

		if isTargetUser {
			totalCommits += entry.Details.CommitCount
			color.HiYellow("📍 %s (Target User)", entry.Email)
			names := make([]string, 0, len(entry.Details.Names))
			for name := range entry.Details.Names {
				names = append(names, name)
			}
			color.HiGreen("Names used: %s", strings.Join(names, ", "))
			color.HiGreen("Total Commits: %d", entry.Details.CommitCount)
		} else if !showTargetOnly && isUserIdentifier(entry.Email, userIdentifiers) {
			color.HiYellow(entry.Email)
			names := make([]string, 0, len(entry.Details.Names))
			for name := range entry.Details.Names {
				names = append(names, name)
			}
			color.HiWhite("  Names: %s", strings.Join(names, ", "))
			color.HiWhite("  Total Commits: %d", entry.Details.CommitCount)
		} else {
			color.Yellow(entry.Email)
			names := make([]string, 0, len(entry.Details.Names))
			for name := range entry.Details.Names {
				names = append(names, name)
			}
			color.White("  Names: %s", strings.Join(names, ", "))
			color.White("  Total Commits: %d", entry.Details.CommitCount)
		}

		if showDetails || checkSecrets || cfg.ShowInteresting {
			for repoName, commits := range entry.Details.Commits {
				// only show repo header if we're showing details or have secrets/patterns in this repo
				repoHasContent := false
				if showDetails {
					repoHasContent = true
				} else {
					// check if repo has any content worth showing
					for _, commit := range commits {
						if (checkSecrets && len(commit.Secrets) > 0) ||
							(cfg.ShowInteresting && len(commit.Secrets) > 0) {
							repoHasContent = true
							break
						}
					}
				}

				if !repoHasContent {
					continue
				}

				if isTargetUser {
					color.HiGreen("  📂 Repo: %s", repoName)
				} else if !showTargetOnly && isUserIdentifier(entry.Email, userIdentifiers) {
					color.HiWhite("  📂 Repo: %s", repoName)
				} else {
					color.Green("  Repo: %s", repoName)
				}

				for _, commit := range commits {
					shouldShowCommit := showDetails ||
						(checkSecrets && len(commit.Secrets) > 0) ||
						(cfg.ShowInteresting && len(commit.Secrets) > 0)

					if !shouldShowCommit {
						continue
					}

					isTargetCommit := isTargetUser || isUserIdentifier(commit.AuthorName, userIdentifiers) || isUserIdentifier(commit.AuthorEmail, userIdentifiers)

					// only show commit details if showing details or if we found secrets/patterns
					if showDetails || len(commit.Secrets) > 0 {
						if isTargetCommit {
							if commit.AuthorName == "" {
								color.HiMagenta("    ⚔️ Commit: %s", commit.Hash)
							} else {
								color.HiMagenta("    ⭐ Commit: %s", commit.Hash)
							}
							color.HiBlue("    🔗 URL: %s", commit.URL)
							if commit.AuthorName == "" {
								color.HiWhite("    👻 Author: anonymous")
							} else {
								color.HiWhite("    👤 Author: %s <%s>", commit.AuthorName, commit.AuthorEmail)
							}
						} else if !showTargetOnly && isUserIdentifier(entry.Email, userIdentifiers) {
							if commit.AuthorName == "" {
								color.Magenta("    ⚔️ Commit: %s", commit.Hash)
							} else {
								color.Magenta("    ⭐ Commit: %s", commit.Hash)
							}
							color.Blue("    🔗 URL: %s", commit.URL)
							if commit.AuthorName == "" {
								color.White("    👻 Author: anonymous")
							} else {
								color.White("    👤 Author: %s <%s>", commit.AuthorName, commit.AuthorEmail)
							}
						} else {
							if commit.AuthorName == "" {
								color.Magenta("    ⚔️ Commit: %s", commit.Hash)
							} else {
								color.Magenta("    Commit: %s", commit.Hash)
							}
							color.Blue("    URL: %s", commit.URL)
							if commit.AuthorName == "" {
								color.White("    👻 Author: anonymous")
							} else {
								color.White("    Author: %s <%s>", commit.AuthorName, commit.AuthorEmail)
							}
						}

						if commit.IsOwnRepo {
							color.Cyan("    Owner: true")
						}
						if commit.IsFork {
							color.Cyan("    Fork: true")
						}
					}

					if len(commit.Secrets) > 0 {
						if isTargetCommit {
							var foundSecrets, foundPatterns bool
							for _, secret := range commit.Secrets {
								if strings.HasPrefix(secret, "⭐") {
									if !foundPatterns && cfg.ShowInteresting {
										color.HiYellow("    ⭐ Found patterns:")
										foundPatterns = true
									}
									if cfg.ShowInteresting {
										color.HiYellow("      %s", secret)
									}
								} else {
									if !foundSecrets && checkSecrets {
										color.HiRed("    🐽 Found secrets:")
										foundSecrets = true
									}
									if checkSecrets {
										color.HiRed("      - %s", secret)
									}
								}
							}
						} else if !showTargetOnly && isUserIdentifier(entry.Email, userIdentifiers) {
							var foundSecrets, foundPatterns bool
							for _, secret := range commit.Secrets {
								if strings.HasPrefix(secret, "⭐") {
									if !foundPatterns && cfg.ShowInteresting {
										color.Yellow("    ⭐ Found patterns:")
										foundPatterns = true
									}
									if cfg.ShowInteresting {
										color.Yellow("      %s", secret)
									}
								} else {
									if !foundSecrets && checkSecrets {
										color.Red("    🐽 Found secrets:")
										foundSecrets = true
									}
									if checkSecrets {
										color.Red("      - %s", secret)
									}
								}
							}
						} else {
							var foundSecrets, foundPatterns bool
							for _, secret := range commit.Secrets {
								if strings.HasPrefix(secret, "⭐") {
									if !foundPatterns && cfg.ShowInteresting {
										color.Yellow("    ⭐ Found patterns:")
										foundPatterns = true
									}
									if cfg.ShowInteresting {
										color.Yellow("      %s", secret)
									}
								} else {
									if !foundSecrets && checkSecrets {
										color.Red("    🐽 Found secrets:")
										foundSecrets = true
									}
									if checkSecrets {
										color.Red("      - %s", secret)
									}
								}
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
	log.SetFlags(0)

	app := &cli.App{
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
		Action:    runApp,
		ArgsUsage: "<username|email>",
		Before: func(c *cli.Context) error {
			if len(os.Args) == 1 && !c.Bool("help") && !c.Bool("version") {
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
			{Name: "gnomegl"},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
