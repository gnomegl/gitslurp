package github

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/gnomegl/gitslurp/internal/models"
	"github.com/gnomegl/gitslurp/internal/scanner"
	"github.com/gnomegl/gitslurp/internal/utils"
	gh "github.com/google/go-github/v57/github"
	"github.com/schollz/progressbar/v3"
	"slices"
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

			if cfg.TimestampAnalysis {
				info.TimestampAnalysis = utils.AnalyzeTimestamp(info.AuthorDate)
			}
		}

		if commit.Commit.Committer != nil {
			info.CommitterName = commit.Commit.Committer.GetName()
			info.CommitterEmail = commit.Commit.Committer.GetEmail()
			info.CommitterDate = commit.Commit.Committer.GetDate().Time
		}

		if info.AuthorEmail == "" && info.CommitterEmail == "" {
			info.AuthorName = "Anonymous"
			info.AuthorEmail = ""
			if info.CommitterName == "" {
				info.CommitterName = "Anonymous"
			}
			if info.CommitterEmail == "" {
				info.CommitterEmail = ""
			}
		}

		if checkSecrets || cfg.ShowInteresting {
			secretScanner := scanner.NewScanner(cfg.ShowInteresting)

			message := commit.GetCommit().GetMessage()
			info.Secrets = append(info.Secrets, scanContent(secretScanner, message, "commit message", checkSecrets, cfg.ShowInteresting)...)

			for _, file := range commit.Files {
				filename := file.GetFilename()
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

func ProcessRepos(ctx context.Context, pool *ClientPool, repos []*gh.Repository, checkSecrets bool, cfg *Config, targetUserIdentifiers map[string]bool, showTargetOnly bool) map[string]*models.EmailDetails {
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
		progressDescription = "[cyan]Processing repositories (secrets + patterns)[reset]"
	} else if checkSecrets {
		progressDescription = "[cyan]Processing repositories (secrets)[reset]"
	} else if cfg.ShowInteresting {
		progressDescription = "[cyan]Processing repositories (patterns)[reset]"
	} else {
		progressDescription = "[cyan]Processing repositories[reset]"
	}

	bar := progressbar.NewOptions(len(repos),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(10),
		progressbar.OptionSetDescription(progressDescription),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]#[reset]",
			SaucerHead:    "[green]#[reset]",
			SaucerPadding: "-",
			BarStart:      "[",
			BarEnd:        "]",
		}))

	for _, repo := range repos {
		wg.Add(1)
		go func(repo *gh.Repository) {
			defer wg.Done()
			sem <- true
			defer func() { <-sem }()

			mc := pool.GetClient()
			var allCommits []*gh.RepositoryCommit
			opts := &gh.CommitsListOptions{
				ListOptions: gh.ListOptions{PerPage: 100},
			}

			for {
				commits, resp, err := mc.Client.Repositories.ListCommits(ctx, repo.GetOwner().GetLogin(), repo.GetName(), opts)
				if resp != nil {
					mc.UpdateRateLimit(resp.Rate.Remaining, resp.Rate.Reset.Time)
				}
				if err != nil {
					break
				}
				allCommits = append(allCommits, commits...)
				if resp.NextPage == 0 {
					break
				}
				opts.Page = resp.NextPage
			}

			var repoCommits []models.CommitInfo
			for _, commit := range allCommits {
				if checkSecrets || cfg.ShowInteresting {
					fullCommit, getResp, err := mc.Client.Repositories.GetCommit(ctx, repo.GetOwner().GetLogin(), repo.GetName(), commit.GetSHA(), &gh.ListOptions{})
					if getResp != nil {
						mc.UpdateRateLimit(getResp.Rate.Remaining, getResp.Rate.Reset.Time)
					}
					if err == nil {
						commit = fullCommit
					}
				}
				commitInfo := ProcessCommit(commit, checkSecrets, cfg)
				repoCommits = append(repoCommits, commitInfo)
			}

			mutex.Lock()
			aggregateCommits(emails, repoCommits, repo.GetFullName(), targetUserIdentifiers, showTargetOnly)
			mutex.Unlock()

			bar.Add(1)
		}(repo)
	}

	wg.Wait()
	bar.Finish()
	return emails
}

type EmailUpdate struct {
	Email   string
	Details *models.EmailDetails
	RepoName string
}

func ProcessReposStreaming(ctx context.Context, pool *ClientPool, repos []*gh.Repository, checkSecrets bool, cfg *Config, targetUserIdentifiers map[string]bool, showTargetOnly bool, updateChan chan<- EmailUpdate) map[string]*models.EmailDetails {
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
		progressDescription = "[cyan]Processing repositories (secrets + patterns)[reset]"
	} else if checkSecrets {
		progressDescription = "[cyan]Processing repositories (secrets)[reset]"
	} else if cfg.ShowInteresting {
		progressDescription = "[cyan]Processing repositories (patterns)[reset]"
	} else {
		progressDescription = "[cyan]Processing repositories[reset]"
	}

	bar := progressbar.NewOptions(len(repos),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(10),
		progressbar.OptionSetDescription(progressDescription),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]#[reset]",
			SaucerHead:    "[green]#[reset]",
			SaucerPadding: "-",
			BarStart:      "[",
			BarEnd:        "]",
		}))

	for _, repo := range repos {
		wg.Add(1)
		go func(repo *gh.Repository) {
			defer wg.Done()
			sem <- true
			defer func() { <-sem }()

			mc := pool.GetClient()
			var allCommits []*gh.RepositoryCommit
			opts := &gh.CommitsListOptions{
				ListOptions: gh.ListOptions{PerPage: 100},
			}

			for {
				commits, resp, err := mc.Client.Repositories.ListCommits(ctx, repo.GetOwner().GetLogin(), repo.GetName(), opts)
				if resp != nil {
					mc.UpdateRateLimit(resp.Rate.Remaining, resp.Rate.Reset.Time)
				}
				if err != nil {
					break
				}
				allCommits = append(allCommits, commits...)
				if resp.NextPage == 0 {
					break
				}
				opts.Page = resp.NextPage
			}

			var repoCommits []models.CommitInfo
			for _, commit := range allCommits {
				if checkSecrets || cfg.ShowInteresting {
					fullCommit, getResp, err := mc.Client.Repositories.GetCommit(ctx, repo.GetOwner().GetLogin(), repo.GetName(), commit.GetSHA(), &gh.ListOptions{})
					if getResp != nil {
						mc.UpdateRateLimit(getResp.Rate.Remaining, getResp.Rate.Reset.Time)
					}
					if err == nil {
						commit = fullCommit
					}
				}
				commitInfo := ProcessCommit(commit, checkSecrets, cfg)
				repoCommits = append(repoCommits, commitInfo)
			}

			mutex.Lock()
			newEmails := aggregateCommitsStreaming(emails, repoCommits, repo.GetFullName(), targetUserIdentifiers, showTargetOnly)
			for email, details := range newEmails {
				if updateChan != nil {
					updateChan <- EmailUpdate{
						Email:   email,
						Details: details,
						RepoName: repo.GetFullName(),
					}
				}
			}
			mutex.Unlock()

			bar.Add(1)
		}(repo)
	}

	wg.Wait()
	bar.Finish()
	if updateChan != nil {
		close(updateChan)
	}
	return emails
}

func aggregateCommitsStreaming(emails map[string]*models.EmailDetails, commits []models.CommitInfo, repoName string, targetUserIdentifiers map[string]bool, showTargetOnly bool) map[string]*models.EmailDetails {
	newEmails := make(map[string]*models.EmailDetails)

	for _, commit := range commits {
		if commit.AuthorEmail == "" {
			continue
		}

		if showTargetOnly && targetUserIdentifiers != nil {
			isTargetUser := targetUserIdentifiers[commit.AuthorEmail] || targetUserIdentifiers[commit.AuthorName]
			if !isTargetUser {
				continue
			}
		}

		email := commit.AuthorEmail
		isNew := false
		if _, exists := emails[email]; !exists {
			emails[email] = &models.EmailDetails{
				Names:   make(map[string]struct{}),
				Commits: make(map[string][]models.CommitInfo),
			}
			isNew = true
		}

		details := emails[email]
		details.Names[commit.AuthorName] = struct{}{}
		details.Commits[repoName] = append(details.Commits[repoName], commit)
		details.CommitCount++

		if isNew {
			detailsCopy := &models.EmailDetails{
				Names:       make(map[string]struct{}),
				Commits:     make(map[string][]models.CommitInfo),
				CommitCount: details.CommitCount,
			}
			for name := range details.Names {
				detailsCopy.Names[name] = struct{}{}
			}
			for repo, commits := range details.Commits {
				detailsCopy.Commits[repo] = append([]models.CommitInfo{}, commits...)
			}
			newEmails[email] = detailsCopy
		}
	}

	return newEmails
}

func scanContent(scanner *scanner.Scanner, text, location string, checkSecrets bool, showInteresting bool) []string {
	var findings []string
	if matches := scanner.ScanText(text); len(matches) > 0 {
		for _, match := range matches {
			if match.Type == "Secret" && checkSecrets {
				findings = append(findings, fmt.Sprintf("%s: %s (in %s)", match.Name, match.Value, location))
			} else if match.Type == "Interesting" && showInteresting {
				findings = append(findings, fmt.Sprintf("INTERESTING: %s: %s (in %s)", match.Name, match.Value, location))
			}
		}
	}
	return findings
}

func ProcessGists(ctx context.Context, pool *ClientPool, gists []*gh.Gist, checkSecrets bool, cfg *Config) map[string]*models.EmailDetails {
	emails := make(map[string]*models.EmailDetails)

	for _, gist := range gists {
		if gist.Owner == nil || gist.Owner.Login == nil {
			continue
		}

		commitInfo := models.CommitInfo{
			Hash:        gist.GetID(),
			URL:         gist.GetHTMLURL(),
			AuthorName:  gist.GetOwner().GetLogin(),
			AuthorEmail: "",
		}

		if checkSecrets || cfg.ShowInteresting {
			secretScanner := scanner.NewScanner(cfg.ShowInteresting)

			commitInfo.Secrets = append(commitInfo.Secrets, scanContent(secretScanner, gist.GetDescription(), "description", checkSecrets, cfg.ShowInteresting)...)

			for filename, file := range gist.Files {
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

func isPackageManagerFile(filename string) bool {
	packageFiles := []string{
		"package.json", "package-lock.json",
		"yarn.lock", ".yarnrc", ".yarnrc.yml",
		"pnpm-lock.yaml", ".pnpmrc",
		"npm-shrinkwrap.json", ".npmrc",
		"composer.json", "composer.lock",
		"Gemfile", "Gemfile.lock",
		"requirements.txt", "poetry.lock", "Pipfile", "Pipfile.lock",
		"go.mod", "go.sum",
		"build.gradle", "gradle.properties", "settings.gradle",
		"pom.xml", "build.xml",
		"mix.exs", "mix.lock",
		"sbt.build", "build.sbt",
		"cargo.toml", "cargo.lock",
	}

	return slices.Contains(packageFiles, filename)
}

func aggregateCommits(emails map[string]*models.EmailDetails, commits []models.CommitInfo, repoName string, targetUserIdentifiers map[string]bool, showTargetOnly bool) {
	for _, commit := range commits {
		if commit.AuthorEmail == "" {
			continue
		}

		if showTargetOnly && targetUserIdentifiers != nil {
			isTargetUser := targetUserIdentifiers[commit.AuthorEmail] || targetUserIdentifiers[commit.AuthorName]
			if !isTargetUser {
				continue
			}
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
