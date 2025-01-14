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

func ProcessCommit(commit *gh.RepositoryCommit, checkSecrets bool, cfg *Config) models.CommitInfo {
	var info models.CommitInfo

	if commit.Commit != nil {
		info.Message = commit.Commit.GetMessage()
		info.Hash = commit.GetSHA()
		info.URL = commit.GetHTMLURL()

		if commit.Commit.Author != nil {
			info.AuthorName = commit.Commit.Author.GetName()
			info.AuthorEmail = commit.Commit.Author.GetEmail()
			info.AuthorDate = commit.Commit.Author.GetDate().Time
		}

		if commit.Commit.Committer != nil {
			info.CommitterName = commit.Commit.Committer.GetName()
			info.CommitterEmail = commit.Commit.Committer.GetEmail()
			info.CommitterDate = commit.Commit.Committer.GetDate().Time
		}

		if info.AuthorEmail == "" && info.CommitterEmail == "" {
			info.AuthorName = "🥷 Anonymous"
			info.AuthorEmail = ""
			if info.CommitterName == "" {
				info.CommitterName = "🥷 Anonymous"
			}
			if info.CommitterEmail == "" {
				info.CommitterEmail = ""
			}
		}

		if checkSecrets || cfg.ShowInteresting {
			secretScanner := scanner.NewScanner(cfg.ShowInteresting)

			// Scan commit message
			message := commit.GetCommit().GetMessage()
			info.Secrets = append(info.Secrets, scanContent(secretScanner, message, "commit message", checkSecrets, cfg.ShowInteresting)...)

			// Scan files changed in the commit
			for _, file := range commit.Files {
				filename := file.GetFilename()
				// Skip node_modules and package manager files
				if cfg.SkipNodeModules && (strings.Contains(filename, "/node_modules/") || strings.HasPrefix(filename, "node_modules/") || filename == "Cargo.lock") {
					continue
				}
				if isPackageManagerFile(filename) {
					continue
				}

				if file.GetPatch() != "" {
					info.Secrets = append(info.Secrets, scanContent(secretScanner, file.GetPatch(), filename, checkSecrets, cfg.ShowInteresting)...)
				}
			}
		}
	}

	return info
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
func ProcessRepos(ctx context.Context, client *gh.Client, repos []*gh.Repository, checkSecrets bool, cfg *Config) map[string]*models.EmailDetails {
	if cfg == nil {
		cfg = &Config{}
		*cfg = DefaultConfig()
	}

	emails := make(map[string]*models.EmailDetails)
	var mutex sync.Mutex
	sem := make(chan bool, cfg.MaxConcurrentRequests)
	var wg sync.WaitGroup

	var progressDescription string
	if checkSecrets && cfg.ShowInteresting {
		progressDescription = "[cyan]Sniffing repositories for secrets and patterns 🐽[reset]"
	} else if checkSecrets {
		progressDescription = "[cyan]Sniffing repositories for secrets 🐽[reset]"
	} else if cfg.ShowInteresting {
		progressDescription = "[cyan]Slurping repositories for interesting patterns ⭐[reset]"
	} else {
		progressDescription = "[cyan]Slurping repositories[reset]"
	}

	bar := progressbar.NewOptions(len(repos),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetDescription(progressDescription),
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
				// If we're scanning for secrets or patterns, fetch the full commit with files
				if checkSecrets || cfg.ShowInteresting {
					fullCommit, _, err := client.Repositories.GetCommit(ctx, repo.GetOwner().GetLogin(), repo.GetName(), commit.GetSHA(), &gh.ListOptions{})
					if err == nil {
						commit = fullCommit
					}
				}
				commitInfo := ProcessCommit(commit, checkSecrets, cfg)
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

// scanContent scans text for secrets and interesting patterns
func scanContent(scanner *scanner.Scanner, text, location string, checkSecrets bool, showInteresting bool) []string {
	var findings []string
	if matches := scanner.ScanText(text); len(matches) > 0 {
		for _, match := range matches {
			if match.Type == "Secret" && checkSecrets {
				findings = append(findings, fmt.Sprintf("%s: %s (in %s)", match.Name, match.Value, location))
			} else if match.Type == "Interesting" && showInteresting {
				findings = append(findings, fmt.Sprintf("⭐ %s: %s (in %s)", match.Name, match.Value, location))
			}
		}
	}
	return findings
}

// ProcessGists processes gists for commit information
func ProcessGists(ctx context.Context, client *gh.Client, gists []*gh.Gist, checkSecrets bool, cfg *Config) map[string]*models.EmailDetails {
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

		if checkSecrets || cfg.ShowInteresting {
			secretScanner := scanner.NewScanner(cfg.ShowInteresting)

			// Scan gist description
			commitInfo.Secrets = append(commitInfo.Secrets, scanContent(secretScanner, gist.GetDescription(), "description", checkSecrets, cfg.ShowInteresting)...)

			// Scan each file's content
			for filename, file := range gist.Files {
				// Skip node_modules and package manager files
				if cfg.SkipNodeModules && (strings.Contains(string(filename), "/node_modules/") || strings.HasPrefix(string(filename), "node_modules/")) {
					continue
				}
				if isPackageManagerFile(string(filename)) {
					continue
				}

				if content := file.GetContent(); content != "" {
					commitInfo.Secrets = append(commitInfo.Secrets, scanContent(secretScanner, content, string(filename), checkSecrets, cfg.ShowInteresting)...)
				}
			}
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

// isPackageManagerFile returns true if the filename is a package manager file
func isPackageManagerFile(filename string) bool {
	packageFiles := []string{
		"package.json", "package-lock.json", // npm
		"yarn.lock", ".yarnrc", ".yarnrc.yml", // yarn
		"pnpm-lock.yaml", ".pnpmrc", // pnpm
		"npm-shrinkwrap.json", ".npmrc", // npm
		"composer.json", "composer.lock", // php
		"Gemfile", "Gemfile.lock", // ruby
		"requirements.txt", "poetry.lock", "Pipfile", "Pipfile.lock", // python
		"go.mod", "go.sum", // golang
		"build.gradle", "gradle.properties", "settings.gradle", // gradle
		"pom.xml", "build.xml", // maven
		"mix.exs", "mix.lock", // elixir
		"sbt.build", "build.sbt", // scala
		"cargo.toml", "cargo.lock", // rust
	}
	
	for _, file := range packageFiles {
		if filename == file {
			return true
		}
	}
	return false
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
