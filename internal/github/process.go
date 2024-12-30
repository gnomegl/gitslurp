package github

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/gnomegl/gitslurp/internal/models"
	"github.com/gnomegl/gitslurp/internal/scanner"
	gh "github.com/google/go-github/v57/github"
	"github.com/schollz/progressbar/v3"
)

// ProcessCommit processes a single commit for secrets and links
func ProcessCommit(commit *gh.RepositoryCommit, checkSecrets bool, showLinks bool, cfg *Config) models.CommitInfo {
	commitInfo := models.CommitInfo{
		Hash:        commit.GetSHA(),
		URL:         commit.GetHTMLURL(),
		AuthorName:  commit.GetCommit().GetAuthor().GetName(),
		AuthorEmail: commit.GetCommit().GetAuthor().GetEmail(),
	}

	if checkSecrets {
		// Create a new scanner with the interesting flag from config
		secretScanner := scanner.NewScanner(cfg.ShowInteresting)

		// Scan commit message and patch
		message := commit.GetCommit().GetMessage()
		if matches := secretScanner.ScanText(message); len(matches) > 0 {
			for _, match := range matches {
				if match.Type == "Secret" {
					commitInfo.Secrets = append(commitInfo.Secrets, fmt.Sprintf("%s: %s", match.Name, match.Value))
				} else if match.Type == "Interesting" {
					commitInfo.Secrets = append(commitInfo.Secrets, fmt.Sprintf("⭐ %s: %s", match.Name, match.Value))
				}
			}
		}
	}

	if showLinks {
		message := commit.GetCommit().GetMessage()
		links := ExtractLinks(message)
		commitInfo.Links = links
	}

	return commitInfo
}

// ExtractLinks extracts URLs from text
func ExtractLinks(text string) []string {
	var links []string
	words := strings.Fields(text)

	for _, word := range words {
		if strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://") {
			if _, err := url.ParseRequestURI(word); err == nil {
				links = append(links, word)
			}
		}
	}

	return links
}

// ProcessRepos processes repositories concurrently with rate limiting and progress tracking
func ProcessRepos(ctx context.Context, client *gh.Client, repos []*gh.Repository, checkSecrets bool, showLinks bool, cfg *Config) map[string]*models.EmailDetails {
	if cfg == nil {
		cfg = &Config{}
		*cfg = DefaultConfig()
	}

	emails := make(map[string]*models.EmailDetails)
	var mutex sync.Mutex
	sem := make(chan bool, cfg.MaxConcurrentRequests)
	var wg sync.WaitGroup

	bar := progressbar.NewOptions(len(repos),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetDescription("[cyan]Sniffing repositories 🐽[reset]"),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}))

	for _, repo := range repos {
		wg.Add(1)
		go func(repo *gh.Repository) {
			defer wg.Done()
			sem <- true
			defer func() { <-sem }()

			commits, _, err := client.Repositories.ListCommits(ctx, repo.GetOwner().GetLogin(), repo.GetName(), nil)
			if err != nil {
				return
			}

			var repoCommits []models.CommitInfo
			for _, commit := range commits {
				commitInfo := ProcessCommit(commit, checkSecrets, showLinks, cfg)
				repoCommits = append(repoCommits, commitInfo)
			}

			mutex.Lock()
			aggregateCommits(emails, repoCommits, repo.GetFullName())
			mutex.Unlock()

			bar.Add(1)
		}(repo)
	}

	wg.Wait()
	bar.Finish()
	return emails
}

// ProcessGists processes gists for commit information
func ProcessGists(ctx context.Context, client *gh.Client, gists []*gh.Gist, checkSecrets bool, showLinks bool, cfg *Config) map[string]*models.EmailDetails {
	emails := make(map[string]*models.EmailDetails)

	for _, gist := range gists {
		if gist.Owner == nil || gist.Owner.Login == nil {
			continue
		}

		commitInfo := models.CommitInfo{
			Hash:        gist.GetID(),
			URL:         gist.GetHTMLURL(),
			AuthorName:  gist.GetOwner().GetLogin(),
			AuthorEmail: "", // Gists don't expose email directly
		}

		if checkSecrets {
			secretScanner := scanner.NewScanner(cfg.ShowInteresting)

			// Scan gist description and content
			if matches := secretScanner.ScanText(gist.GetDescription()); len(matches) > 0 {
				for _, match := range matches {
					if match.Type == "Secret" {
						commitInfo.Secrets = append(commitInfo.Secrets, fmt.Sprintf("%s: %s", match.Name, match.Value))
					} else if match.Type == "Interesting" {
						commitInfo.Secrets = append(commitInfo.Secrets, fmt.Sprintf("⭐ %s: %s", match.Name, match.Value))
					}
				}
			}
		}

		if showLinks {
			links := ExtractLinks(gist.GetDescription())
			commitInfo.Links = links
		}

		email := commitInfo.AuthorEmail
		if email == "" {
			email = fmt.Sprintf("%s@users.noreply.github.com", gist.GetOwner().GetLogin())
		}

		if _, exists := emails[email]; !exists {
			emails[email] = &models.EmailDetails{
				Names:       make(map[string]struct{}),
				Commits:     make(map[string][]models.CommitInfo),
				CommitCount: 0,
			}
		}

		emails[email].Names[commitInfo.AuthorName] = struct{}{}
		gistName := fmt.Sprintf("gist:%s", gist.GetID())
		emails[email].Commits[gistName] = append(emails[email].Commits[gistName], commitInfo)
		emails[email].CommitCount++
	}

	return emails
}

// email -> commit mapping
func aggregateCommits(emails map[string]*models.EmailDetails, commits []models.CommitInfo, repoName string) {
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

		details := emails[email]
		details.Names[commit.AuthorName] = struct{}{}
		details.Commits[repoName] = append(details.Commits[repoName], commit)
		details.CommitCount++
	}
}
