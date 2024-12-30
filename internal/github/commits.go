package github

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gnomegl/gitslurp/internal/models"
	"github.com/gnomegl/gitslurp/internal/scanner"

	"github.com/google/go-github/v57/github"
)

func FetchRepos(ctx context.Context, client *github.Client, username string, cfg *Config) ([]*github.Repository, error) {
	if cfg == nil {
		cfg = &Config{} // sneed why can't i do cfg = &DefaultConfig()
		*cfg = DefaultConfig()
	}

	var allRepos []*github.Repository
	opt := &github.RepositoryListByUserOptions{
		ListOptions: github.ListOptions{PerPage: cfg.PerPage},
		Type:        "public",
	}

	for {
		repos, resp, err := client.Repositories.ListByUser(ctx, username, opt)
		if err != nil {
			return nil, fmt.Errorf("error fetching repositories: %v", err)
		}
		allRepos = append(allRepos, repos...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return allRepos, nil
}

func FetchCommits(ctx context.Context, client *github.Client, owner, repo string, isFork bool, since *time.Time, checkSecrets bool, findLinks bool, cfg *Config) ([]models.CommitInfo, error) {
	if cfg == nil {
		cfg = &Config{}
		*cfg = DefaultConfig()
	}

	var allCommits []models.CommitInfo
	opt := &github.CommitsListOptions{
		ListOptions: github.ListOptions{PerPage: cfg.PerPage},
	}
	if since != nil {
		opt.Since = *since
	}

	for {
		commits, resp, err := client.Repositories.ListCommits(ctx, owner, repo, opt)
		if err != nil {
			if resp != nil && resp.StatusCode == 409 {
				// Repository is empty or in an invalid state
				return nil, fmt.Errorf("repository is empty or not accessible")
			}
			if resp != nil && resp.StatusCode == 404 {
				return nil, fmt.Errorf("repository not found or access denied")
			}
			return nil, fmt.Errorf("error fetching commits: %v", err)
		}

		for _, commit := range commits {
			if commit.Commit == nil || commit.Commit.Author == nil {
				continue
			}

			commitInfo := models.CommitInfo{
				Hash:        commit.GetSHA(),
				URL:         commit.GetHTMLURL(),
				AuthorName:  commit.Commit.Author.GetName(),
				AuthorEmail: commit.Commit.Author.GetEmail(),
				Message:     commit.Commit.GetMessage(),
				Date:        commit.Commit.Author.GetDate().Time,
				RepoName:    repo,
				IsFork:      isFork,
			}

			if checkSecrets || findLinks {
				content, err := fetchCommitContent(ctx, client, owner, repo, commit.GetSHA(), cfg)
				if err == nil {
					if checkSecrets {
						secretScanner := scanner.NewScanner(cfg.ShowInteresting)
						matches := secretScanner.ScanText(content)
						for _, match := range matches {
							if match.Type == "Secret" {
								commitInfo.Secrets = append(commitInfo.Secrets, fmt.Sprintf("%s: %s", match.Name, match.Value))
							} else if match.Type == "Interesting" {
								commitInfo.Secrets = append(commitInfo.Secrets, fmt.Sprintf("⭐ %s: %s", match.Name, match.Value))
							}
						}
					}
					if findLinks {
						commitInfo.Links = scanner.ExtractLinks(content)
					}
				}
			}

			allCommits = append(allCommits, commitInfo)
		}

		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return allCommits, nil
}

// get commit content
func fetchCommitContent(ctx context.Context, client *github.Client, owner, repo, sha string, cfg *Config) (string, error) {
	if cfg == nil {
		cfg = &Config{}
		*cfg = DefaultConfig()
	}

	commit, _, err := client.Repositories.GetCommit(ctx, owner, repo, sha, nil)
	if err != nil {
		return "", fmt.Errorf("failed to fetch commit %s: %w", sha, err)
	}
	var content strings.Builder
	content.WriteString(commit.Commit.GetMessage())
	content.WriteString("\n")

	for _, file := range commit.Files {
		filename := file.GetFilename()
		// js ecosystem is bloat
		if cfg.SkipNodeModules && (strings.Contains(filename, "/node_modules/") || strings.HasPrefix(filename, "node_modules/")) {
			continue
		}
		switch filename {
		case "package.json", "package-lock.json", "npm-shrinkwrap.json", ".npmrc", // npm
			"yarn.lock", ".yarnrc", ".yarnrc.yml", // yarn
			"pnpm-lock.yaml", ".pnpmrc": // pnpm
			continue
		}
		if strings.HasSuffix(filename, ".lock") {
			continue
		}
		if file.GetPatch() != "" {
			content.WriteString(file.GetPatch())
			content.WriteString("\n")
		}
	}

	return content.String(), nil
}
