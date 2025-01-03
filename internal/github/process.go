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
func ProcessCommit(commit *gh.RepositoryCommit, checkSecrets bool, cfg *Config) models.CommitInfo {
	commitInfo := models.CommitInfo{
		Hash:        commit.GetSHA(),
		URL:         commit.GetHTMLURL(),
		AuthorName:  commit.GetCommit().GetAuthor().GetName(),
		AuthorEmail: commit.GetCommit().GetAuthor().GetEmail(),
	}

	// Create scanner if we're checking secrets or interesting patterns
	if checkSecrets || cfg.ShowInteresting {
		secretScanner := scanner.NewScanner(cfg.ShowInteresting)

		// Scan commit message
		message := commit.GetCommit().GetMessage()
		if matches := secretScanner.ScanText(message); len(matches) > 0 {
			for _, match := range matches {
				if match.Type == "Secret" && checkSecrets {
					commitInfo.Secrets = append(commitInfo.Secrets, fmt.Sprintf("%s: %s", match.Name, match.Value))
				} else if match.Type == "Interesting" && cfg.ShowInteresting {
					commitInfo.Secrets = append(commitInfo.Secrets, fmt.Sprintf("⭐ %s: %s", match.Name, match.Value))
				}
			}
		}

		// Also scan the files changed in the commit
		for _, file := range commit.Files {
			filename := file.GetFilename()
			// Skip node_modules
			if cfg.SkipNodeModules && (strings.Contains(filename, "/node_modules/") || strings.HasPrefix(filename, "node_modules/")) {
				continue
			}
			// Skip package manager files
			switch filename {
			case "package.json", "package-lock.json", // npm
				"yarn.lock", ".yarnrc", ".yarnrc.yml", // yarn
				"pnpm-lock.yaml", ".pnpmrc", // pnpm
				"npm-shrinkwrap.json", ".npmrc": // npm
				continue
			}

			if file.GetPatch() != "" {
				if matches := secretScanner.ScanText(file.GetPatch()); len(matches) > 0 {
					for _, match := range matches {
						if match.Type == "Secret" && checkSecrets {
							commitInfo.Secrets = append(commitInfo.Secrets, fmt.Sprintf("%s: %s (in %s)", match.Name, match.Value, file.GetFilename()))
						} else if match.Type == "Interesting" && cfg.ShowInteresting {
							commitInfo.Secrets = append(commitInfo.Secrets, fmt.Sprintf("⭐ %s: %s (in %s)", match.Name, match.Value, file.GetFilename()))
						}
					}
				}
			}
		}
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
			if matches := secretScanner.ScanText(gist.GetDescription()); len(matches) > 0 {
				for _, match := range matches {
					if match.Type == "Secret" && checkSecrets {
						commitInfo.Secrets = append(commitInfo.Secrets, fmt.Sprintf("%s: %s (in description)", match.Name, match.Value))
					} else if match.Type == "Interesting" && cfg.ShowInteresting {
						commitInfo.Secrets = append(commitInfo.Secrets, fmt.Sprintf("⭐ %s: %s (in description)", match.Name, match.Value))
					}
				}
			}

			// Scan each file's content
			for filename, file := range gist.Files {
				// Skip node_modules
				if cfg.SkipNodeModules && (strings.Contains(string(filename), "/node_modules/") || strings.HasPrefix(string(filename), "node_modules/")) {
					continue
				}
				// Skip package manager files
				switch filename {
				case "package.json", "package-lock.json", // npm
					"yarn.lock", ".yarnrc", ".yarnrc.yml", // yarn
					"pnpm-lock.yaml", ".pnpmrc", // pnpm
					"npm-shrinkwrap.json", ".npmrc": // npm
					continue
				}

				if content := file.GetContent(); content != "" {
					if matches := secretScanner.ScanText(content); len(matches) > 0 {
						for _, match := range matches {
							if match.Type == "Secret" && checkSecrets {
								commitInfo.Secrets = append(commitInfo.Secrets, fmt.Sprintf("%s: %s (in %s)", match.Name, match.Value, filename))
							} else if match.Type == "Interesting" && cfg.ShowInteresting {
								commitInfo.Secrets = append(commitInfo.Secrets, fmt.Sprintf("⭐ %s: %s (in %s)", match.Name, match.Value, filename))
							}
						}
					}
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
