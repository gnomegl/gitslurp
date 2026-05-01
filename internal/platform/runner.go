package platform

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/gnomegl/gitslurp/v2/internal/models"
	"github.com/schollz/progressbar/v3"
)

type Runner struct {
	provider Provider
	config   ScanConfig
}

func NewRunner(provider Provider, cfg ScanConfig) *Runner {
	return &Runner{provider: provider, config: cfg}
}

func (r *Runner) DisplayUserInfo(info *UserInfo, isOrg bool) {
	if info == nil {
		return
	}

	headerColor := color.New(color.Bold, color.FgCyan)

	fmt.Println()
	platformLabel := strings.ToUpper(string(r.provider.Name()))
	if isOrg {
		headerColor.Printf("%s ORGANIZATION: %s\n", platformLabel, info.Login)
	} else {
		headerColor.Printf("%s USER: %s\n", platformLabel, info.Login)
	}

	printIfSet := func(label, value string) {
		if value != "" {
			fmt.Printf("%s %s\n", color.WhiteString(label+":"), value)
		}
	}

	printIfSet("Name", info.Name)
	printIfSet("Email", info.Email)
	printIfSet("Company", info.Company)
	printIfSet("Location", info.Location)
	printIfSet("Bio", info.Bio)
	printIfSet("Website", info.Blog)
	if info.Twitter != "" {
		fmt.Printf("%s @%s\n", color.WhiteString("Twitter:"), info.Twitter)
	}

	fmt.Println()
	if isOrg {
		if info.PublicRepos > 0 {
			fmt.Printf("%s %d\n", color.WhiteString("Repos:"), info.PublicRepos)
		}
	} else {
		parts := []string{
			fmt.Sprintf("%s %d", color.WhiteString("Repos:"), info.PublicRepos),
		}
		if info.PublicGists > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", color.WhiteString("Gists:"), info.PublicGists))
		}
		parts = append(parts,
			fmt.Sprintf("%s %d", color.WhiteString("Followers:"), info.Followers),
			fmt.Sprintf("%s %d", color.WhiteString("Following:"), info.Following),
		)
		fmt.Println(strings.Join(parts, "  "))
	}

	if !info.CreatedAt.IsZero() || !info.UpdatedAt.IsZero() {
		var dateParts []string
		if !info.CreatedAt.IsZero() {
			dateParts = append(dateParts, fmt.Sprintf("%s %s", color.WhiteString("Created:"), info.CreatedAt.Format("2006-01-02")))
		}
		if !info.UpdatedAt.IsZero() {
			dateParts = append(dateParts, fmt.Sprintf("%s %s", color.WhiteString("Updated:"), info.UpdatedAt.Format("2006-01-02")))
		}
		fmt.Println(strings.Join(dateParts, "  "))
	}

	fmt.Println()
}

func (r *Runner) FetchAndProcessRepos(ctx context.Context, username string, isOrg bool) (map[string]*models.EmailDetails, error) {
	var repos []*Repository
	var err error

	fmt.Println()
	color.Blue("Enumerating %s repositories...", r.provider.Name())

	if isOrg {
		repos, err = r.provider.ListOrgRepos(ctx, username)
	} else {
		repos, err = r.provider.ListUserRepos(ctx, username, r.config.IncludeForks)
	}
	if err != nil {
		return nil, err
	}

	if len(repos) == 0 {
		return nil, fmt.Errorf("no repositories found for %s on %s", username, r.provider.Name())
	}

	color.Green("[+] Found %d repositories", len(repos))
	fmt.Println()

	emails := make(map[string]*models.EmailDetails)
	var mu sync.Mutex

	maxConcurrent := r.config.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 5
	}
	sem := make(chan struct{}, maxConcurrent)
	rateLimiter := time.NewTicker(200 * time.Millisecond)
	defer rateLimiter.Stop()

	bar := progressbar.NewOptions(len(repos),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(10),
		progressbar.OptionSetDescription("[cyan]Processing repositories[reset]"),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]#[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: "[white].[reset]",
			BarStart:      "[blue]|[reset]",
			BarEnd:        "[blue]|[reset]",
		}))

	var wg sync.WaitGroup
	for _, repo := range repos {
		wg.Add(1)
		go func(repo *Repository) {
			defer wg.Done()
			<-rateLimiter.C
			sem <- struct{}{}
			defer func() { <-sem }()

			commits, err := r.provider.ListCommits(ctx, repo.Owner, repo.Name, r.config)
			if err != nil {
				bar.Add(1)
				return
			}

			mu.Lock()
			for _, commit := range commits {
				if commit.AuthorEmail == "" {
					continue
				}
				email := commit.AuthorEmail
				if _, exists := emails[email]; !exists {
					emails[email] = &models.EmailDetails{
						Names:   make(map[string]struct{}),
						Commits: make(map[string][]models.CommitInfo),
					}
				}
				emails[email].Names[commit.AuthorName] = struct{}{}
				emails[email].Commits[repo.FullName] = append(emails[email].Commits[repo.FullName], commit)
				emails[email].CommitCount++
			}
			mu.Unlock()

			bar.Add(1)
		}(repo)
	}

	wg.Wait()
	bar.Finish()

	extEmails, err := r.provider.SearchCommitsByUser(ctx, username, r.config)
	if err == nil && len(extEmails) > 0 {
		for email, details := range extEmails {
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

	return emails, nil
}
