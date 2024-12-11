package github

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gnomegl/gitslurp/internal/models"
	"github.com/gnomegl/gitslurp/internal/scanner"

	"github.com/briandowns/spinner"
	"github.com/google/go-github/v57/github"
)

func FetchRepos(ctx context.Context, client *github.Client, username string) ([]*github.Repository, error) {
	var allRepos []*github.Repository
	opt := &github.RepositoryListByUserOptions{
		ListOptions: github.ListOptions{PerPage: 100},
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

func FetchCommits(ctx context.Context, client *github.Client, owner, repo string, isFork bool, since *time.Time, checkSecrets bool, findLinks bool) ([]models.CommitInfo, error) {
	var allCommits []models.CommitInfo
	opt := &github.CommitsListOptions{
		ListOptions: github.ListOptions{PerPage: 100},
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
				content, err := fetchCommitContent(ctx, client, owner, repo, commit.GetSHA())
				if err == nil {
					if checkSecrets {
						commitInfo.Secrets = scanner.CheckForSecrets(content)
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

func ProcessRepos(ctx context.Context, client *github.Client, repos []*github.Repository, checkSecrets bool, findLinks bool) map[string]*models.EmailDetails {
	emails := make(map[string]*models.EmailDetails)
	var mu sync.Mutex
	var wg sync.WaitGroup

	s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
	s.Prefix = "Processing repositories "
	s.Start()

	semaphore := make(chan struct{}, 5)

	for _, repo := range repos {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(repo *github.Repository) {
			defer func() {
				<-semaphore
				wg.Done()
			}()

			var since *time.Time
			if repo.GetFork() {
				createdAt := repo.GetCreatedAt()
				since = &createdAt.Time
			}

			commits, err := FetchCommits(ctx, client, repo.GetOwner().GetLogin(), repo.GetName(), repo.GetFork(), since, checkSecrets, findLinks)
			if err != nil {
				log.Printf("Error fetching commits for %s: %v", repo.GetName(), err)
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

				details := emails[email]
				details.Names[commit.AuthorName] = struct{}{}
				details.Commits[repo.GetName()] = append(details.Commits[repo.GetName()], commit)
				details.CommitCount++
			}
			mu.Unlock()
		}(repo)
	}

	wg.Wait()
	s.Stop()
	return emails
}

func fetchCommitContent(ctx context.Context, client *github.Client, owner, repo, sha string) (string, error) {
	commit, _, err := client.Repositories.GetCommit(ctx, owner, repo, sha, nil)
	if err != nil {
		return "", err
	}

	var content strings.Builder
	content.WriteString(commit.Commit.GetMessage())
	content.WriteString("\n")

	for _, file := range commit.Files {
		if file.GetPatch() != "" {
			content.WriteString(file.GetPatch())
			content.WriteString("\n")
		}
	}

	return content.String(), nil
}
