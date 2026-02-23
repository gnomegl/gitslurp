package service

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/gnomegl/gitslurp/internal/config"
	"github.com/gnomegl/gitslurp/internal/display"
	"github.com/gnomegl/gitslurp/internal/github"
	"github.com/gnomegl/gitslurp/internal/models"
	gh "github.com/google/go-github/v57/github"
)

type Orchestrator struct {
	client *gh.Client
	config *config.AppConfig
	token  string
}

func NewOrchestrator(client *gh.Client, cfg *config.AppConfig, token string) *Orchestrator {
	return &Orchestrator{
		client: client,
		config: cfg,
		token:  token,
	}
}

func (o *Orchestrator) Run(ctx context.Context) error {
	var oldStdout *os.File
	if o.config.OutputFormat == "json" || o.config.OutputFormat == "csv" {
		oldStdout = os.Stdout
		os.Stdout = os.Stderr
	}

	username, lookupEmail, err := o.resolveTarget(ctx)
	if err != nil {
		return err
	}
	fmt.Println()

	user, isOrg, err := o.fetchUserInfo(ctx, username, lookupEmail)
	if err != nil {
		return err
	}

	if isOrg {
		o.config.ShowTargetOnly = false
	}

	display.UserInfo(user, isOrg)

	if o.config.ProfileOnly {
		return nil
	}

	cfg := github.DefaultConfig()
	cfg.ShowInteresting = o.config.ShowInteresting
	cfg.QuickMode = o.config.QuickMode
	cfg.TimestampAnalysis = o.config.TimestampAnalysis
	cfg.IncludeForks = o.config.IncludeForks

	repos, gists, err := o.fetchReposAndGists(ctx, username, isOrg, &cfg, user)
	if err != nil {
		return err
	}

	if o.config.ShowStargazers || o.config.ShowForkers {
		err = o.processRepoEvents(ctx, repos)
		if err != nil {
			return err
		}
	}

	userIdentifiers := o.buildUserIdentifiers(username, lookupEmail, user)

	emails := github.RateLimitedProcessRepos(ctx, o.client, repos, o.config.CheckSecrets, &cfg, userIdentifiers, o.config.ShowTargetOnly)

	if len(gists) > 0 && (o.config.CheckSecrets || cfg.ShowInteresting) {
		emails = o.processGists(ctx, gists, emails, &cfg)
	}

	externalEmails, err := github.FetchExternalContributions(ctx, o.client, username, o.config.CheckSecrets, &cfg)
	if err == nil && len(externalEmails) > 0 {
		for email, details := range externalEmails {
			if existing, ok := emails[email]; ok {
				for name := range details.Names {
					existing.Names[name] = struct{}{}
				}
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
		return o.handleNoEmails(isOrg, username, len(repos))
	}

	if oldStdout != nil {
		os.Stdout = oldStdout
	}

	display.Results(emails, o.config.ShowDetails, o.config.CheckSecrets, lookupEmail, username, user, o.config.ShowTargetOnly, isOrg, &cfg, o.config.OutputFormat)

	if oldStdout != nil {
		os.Stdout = os.Stderr
	}

	github.DisplayRateLimit(ctx, o.client)

	if oldStdout != nil {
		os.Stdout = oldStdout
	}

	return nil
}

func (o *Orchestrator) resolveTarget(ctx context.Context) (username, lookupEmail string, err error) {
	username = o.config.Target

	if github.IsValidEmail(o.config.Target) {
		lookupEmail = o.config.Target
		fmt.Println()
		color.Blue("Target Email: %s", o.config.Target)

		hasDeleteRepo, permErr := github.CheckDeleteRepoPermissions(ctx, o.client)
		if permErr != nil {
			color.Yellow("[!] Warning: Could not check token permissions: %v", permErr)
		} else if !hasDeleteRepo {
			color.Red("\n[x] Your GitHub token lacks delete_repo permissions required for email-based investigations")
			color.Yellow("[!] To update your token permissions:")
			fmt.Println("1. Visit: https://github.com/settings/tokens")
			fmt.Println("2. Click on your existing gitslurp token")
			fmt.Println("3. Check the 'delete_repo' scope")
			fmt.Println("4. Click 'Update token' at the bottom")
			color.Blue("\nAlternatively, create a new token with delete_repo permissions:")
			fmt.Println("https://github.com/settings/tokens/new?description=gitslurp&scopes=repo,read:user,user:email,delete_repo")
			return "", "", fmt.Errorf("insufficient token permissions for email investigation")
		}

		user, err := github.GetUserByEmail(ctx, o.client, o.config.Target)
		if err != nil {
			color.Red("  [x] API search error: %v", err)
			fmt.Println()
			color.Yellow("  Attempting email spoofing method...")

			spoofedUsername, spoofErr := github.GetUsernameFromEmailSpoof(ctx, o.client, o.config.Target, o.token)
			if spoofErr != nil {
				color.Red("  [x] Email spoofing failed: %v", spoofErr)
				return "", "", fmt.Errorf("failed to resolve email %s: %v", o.config.Target, spoofErr)
			}

			username = spoofedUsername
			color.Green("  [+] Found GitHub account via spoofing: %s", username)
		} else if user == nil {
			fmt.Println()
			color.Yellow("  [!] No user found via API search")
			color.Yellow("  Attempting email spoofing method...")

			spoofedUsername, spoofErr := github.GetUsernameFromEmailSpoof(ctx, o.client, o.config.Target, o.token)
			if spoofErr != nil {
				color.Red("  [x] Email spoofing failed: %v", spoofErr)
				return "", "", fmt.Errorf("no GitHub user found for email: %s", o.config.Target)
			}

			username = spoofedUsername
			color.Green("  [+] Found GitHub account via spoofing: %s", username)
		} else {
			username = user.GetLogin()
			color.Green("  [+] Found GitHub account via API: %s", username)
		}
	} else {
		fmt.Println()
		color.Blue("Target Username: %s", username)
	}

	return username, lookupEmail, nil
}

func (o *Orchestrator) fetchUserInfo(ctx context.Context, username, lookupEmail string) (*gh.User, bool, error) {
	if lookupEmail != "" {
		return nil, false, nil
	}

	fmt.Println()
	color.Yellow("Checking account type...")

	isOrg, err := github.IsOrganization(ctx, o.client, username)
	if err != nil {
		color.Red("[x] Error checking organization status: %v", err)
		return nil, false, err
	}

	if isOrg {
		color.Green("[+] Organization account detected")
		color.Blue("Fetching organization profile...")
	} else {
		color.Green("[+] User account detected")
		color.Blue("Fetching user profile...")
	}

	user, _, err := o.client.Users.Get(ctx, username)
	if err != nil {
		color.Red("[x] Error fetching profile details: %v", err)
		return nil, false, err
	}

	if isOrg {
		color.Green("[+] Organization profile loaded: %s", user.GetLogin())
	} else {
		color.Green("[+] User profile loaded: %s", user.GetLogin())
	}

	return user, isOrg, nil
}

func (o *Orchestrator) fetchReposAndGists(ctx context.Context, username string, isOrg bool, cfg *github.Config, user *gh.User) ([]*gh.Repository, []*gh.Gist, error) {
	var repos []*gh.Repository
	var gists []*gh.Gist
	var err error

	if isOrg {
		repos, err = github.FetchOrgRepos(ctx, o.client, username, cfg)
	} else {
		repos, err = github.FetchReposWithUser(ctx, o.client, username, cfg, user)
		if err != nil {
			color.Red("[x] Error: %v", err)
			return nil, nil, err
		}
	}

	if err != nil {
		color.Red("[x] Error: %v", err)
		return nil, nil, err
	}

	if len(repos) == 0 && len(gists) == 0 {
		if isOrg {
			color.Red("[x] No public repositories found for organization: %s", username)
		} else {
			color.Red("[x] No public repositories or gists found for user: %s", username)
		}
		return nil, nil, fmt.Errorf("no repositories or gists found")
	}

	return repos, gists, nil
}

func (o *Orchestrator) processRepoEvents(ctx context.Context, repos []*gh.Repository) error {
	processor := NewRepoEventProcessor(o.client, o.config.Target)
	return processor.Process(ctx, repos, o.config.ShowStargazers, o.config.ShowForkers)
}

func (o *Orchestrator) buildUserIdentifiers(username, lookupEmail string, user *gh.User) map[string]bool {
	identifiers := map[string]bool{
		username:    true,
		lookupEmail: true,
	}

	if user != nil {
		identifiers[user.GetLogin()] = true
		identifiers[user.GetName()] = true
		identifiers[user.GetEmail()] = true
	}

	return identifiers
}

func (o *Orchestrator) processGists(ctx context.Context, gists []*gh.Gist, emails map[string]*models.EmailDetails, cfg *github.Config) map[string]*models.EmailDetails {
	var scanType string
	if o.config.CheckSecrets && cfg.ShowInteresting {
		scanType = "secrets and patterns"
	} else if o.config.CheckSecrets {
		scanType = "secrets"
	} else {
		scanType = "interesting patterns"
	}

	color.Blue("\nProcessing %d public gists for %s...", len(gists), scanType)
	gistEmails := github.ProcessGists(ctx, o.client, gists, o.config.CheckSecrets, cfg)

	for email, details := range gistEmails {
		if existing, ok := emails[email]; ok {
			for name := range details.Names {
				existing.Names[name] = struct{}{}
			}
			for repoName, commits := range details.Commits {
				existing.Commits[repoName] = append(existing.Commits[repoName], commits...)
			}
			existing.CommitCount += details.CommitCount
		} else {
			emails[email] = details
		}
	}

	return emails
}

func (o *Orchestrator) handleNoEmails(isOrg bool, username string, repoCount int) error {
	if isOrg {
		if repoCount > 0 {
			color.Yellow("\nAll commits in this organization's repositories are anonymous")
			return nil
		}
		return fmt.Errorf("no repositories found for organization: %s", username)
	}
	return fmt.Errorf("no commits or gists found for user: %s", username)
}

func (o *Orchestrator) outputEventList(list []string, filename, header, emoji string) error {
	if len(list) == 0 {
		fmt.Println("\n" + strings.Replace(header, ":", "", 1) + " - None found")
		return nil
	}

	content := strings.Join(list, "\n")

	if len(list) > 50 {
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %v", filename, err)
		}
		fmt.Printf("\n%s exceeds 50 entries, written to %s\n", strings.Replace(header, ":", "", 1), filename)
	} else {
		fmt.Println("\n" + header)
		for _, item := range list {
			fmt.Printf("  %s\n", item)
		}
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %v", filename, err)
		}
	}

	return nil
}
