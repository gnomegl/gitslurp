package github

import (
	"context"
	"fmt"

	"github.com/gnomegl/gitslurp/internal/models"
	"github.com/gnomegl/gitslurp/internal/scanner"
	gh "github.com/google/go-github/v57/github"
)

type ExternalCommit struct {
	Email      string
	Name       string
	Repository string
	CommitInfo models.CommitInfo
}

func FetchExternalContributions(ctx context.Context, client *gh.Client, username string, checkSecrets bool, cfg *Config) (map[string]*models.EmailDetails, error) {
	if cfg == nil {
		cfg = &Config{}
		*cfg = DefaultConfig()
	}

	emails := make(map[string]*models.EmailDetails)

	query := fmt.Sprintf("author:%s -user:%s", username, username)
	opts := &gh.SearchOptions{
		Sort:  "author-date",
		Order: "asc",
		ListOptions: gh.ListOptions{
			PerPage: 100,
		},
	}

	result, _, err := client.Search.Commits(ctx, query, opts)
	if err != nil {
		return nil, fmt.Errorf("error searching commits: %v", err)
	}

	if result.GetTotal() == 0 {
		return emails, nil
	}

	totalCount := result.GetTotal()
	var allResults []*gh.CommitResult
	allResults = append(allResults, result.Commits...)

	if totalCount > 100 {
		opts2 := &gh.SearchOptions{
			Sort:  "author-date",
			Order: "desc",
			ListOptions: gh.ListOptions{
				PerPage: 100,
			},
		}
		result2, _, err := client.Search.Commits(ctx, query, opts2)
		if err == nil && result2 != nil {
			allResults = append(allResults, result2.Commits...)
		}

		if totalCount > 200 {
			middlePage := 5
			if totalCount <= 2000 {
				middlePage = (totalCount / 100) / 2
			}
			opts3 := &gh.SearchOptions{
				Sort:  "author-date",
				Order: "asc",
				ListOptions: gh.ListOptions{
					PerPage: 100,
					Page:    middlePage,
				},
			}
			result3, _, err := client.Search.Commits(ctx, query, opts3)
			if err == nil && result3 != nil {
				allResults = append(allResults, result3.Commits...)
			}
		}
	}

	for _, commitResult := range allResults {
		if commitResult.Commit == nil || commitResult.Commit.Author == nil {
			continue
		}

		email := commitResult.Commit.Author.GetEmail()
		name := commitResult.Commit.Author.GetName()
		repoName := commitResult.Repository.GetFullName()

		if email == "noreply@github.com" {
			continue
		}

		if _, ok := emails[email]; !ok {
			emails[email] = &models.EmailDetails{
				Names:   make(map[string]struct{}),
				Commits: make(map[string][]models.CommitInfo),
			}
		}

		emails[email].Names[name] = struct{}{}

		commitInfo := models.CommitInfo{
			Hash:        commitResult.GetSHA(),
			URL:         commitResult.GetHTMLURL(),
			AuthorName:  name,
			AuthorEmail: email,
			Message:     commitResult.Commit.GetMessage(),
			RepoName:    repoName,
			IsOwnRepo:   false,
			IsFork:      false,
			IsExternal:  true,
		}

		if commitResult.Commit.Author != nil && commitResult.Commit.Author.Date != nil {
			commitInfo.AuthorDate = commitResult.Commit.Author.Date.Time
		}

		if commitResult.Commit.Committer != nil {
			commitInfo.CommitterName = commitResult.Commit.Committer.GetName()
			commitInfo.CommitterEmail = commitResult.Commit.Committer.GetEmail()
			if commitResult.Commit.Committer.Date != nil {
				commitInfo.CommitterDate = commitResult.Commit.Committer.Date.Time
			}
		}

		if checkSecrets || cfg.ShowInteresting {
			secretScanner := scanner.NewScanner(cfg.ShowInteresting)
			if commitInfo.Message != "" {
				matches := secretScanner.ScanText(commitInfo.Message)
				for _, match := range matches {
					if match.Type == "Secret" && checkSecrets {
						commitInfo.Secrets = append(commitInfo.Secrets, fmt.Sprintf("%s: %s", match.Name, match.Value))
					} else if match.Type == "Interesting" && cfg.ShowInteresting {
						commitInfo.Secrets = append(commitInfo.Secrets, fmt.Sprintf("INTERESTING: %s: %s", match.Name, match.Value))
					}
				}
			}
		}

		emails[email].Commits[repoName] = append(emails[email].Commits[repoName], commitInfo)
		emails[email].CommitCount++
	}

	return emails, nil
}

func mergeExternalContributions(existingEmails map[string]*models.EmailDetails, externalEmails map[string]*models.EmailDetails) {
	for email, extDetails := range externalEmails {
		if existing, ok := existingEmails[email]; ok {
			for name := range extDetails.Names {
				existing.Names[name] = struct{}{}
			}
			for repo, commits := range extDetails.Commits {
				existing.Commits[repo] = append(existing.Commits[repo], commits...)
			}
			existing.CommitCount += extDetails.CommitCount
		} else {
			existingEmails[email] = extDetails
		}
	}
}
