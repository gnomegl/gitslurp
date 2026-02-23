package github

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/gnomegl/gitslurp/internal/models"
	"github.com/gnomegl/gitslurp/internal/scanner"
	gh "github.com/google/go-github/v57/github"
	"github.com/schollz/progressbar/v3"
)

func ProcessUserEvents(ctx context.Context, pool *ClientPool, username string, checkSecrets bool, cfg *Config, targetUserIdentifiers map[string]bool, showTargetOnly bool) map[string]*models.EmailDetails {
	if cfg == nil {
		cfg = &Config{}
		*cfg = DefaultConfig()
	}

	emails := make(map[string]*models.EmailDetails)

	fmt.Println()
	if checkSecrets && cfg.ShowInteresting {
		color.Cyan("Quick Mode: Recent Activity Scan (Secrets & Patterns)")
	} else if checkSecrets {
		color.Cyan("Quick Mode: Recent Activity Scan (Secrets)")
	} else if cfg.ShowInteresting {
		color.Cyan("Quick Mode: Recent Activity Scan (Patterns)")
	} else {
		color.Cyan("Quick Mode: Recent Activity Scan")
	}

	color.Yellow("[!] Use --deep flag for complete commit history across all repos")
	fmt.Println()
	color.Blue("Fetching recent GitHub events from API...")

	var allEvents []*gh.Event
	opts := &gh.ListOptions{PerPage: 100}

	bar := progressbar.NewOptions(-1,
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionSetDescription("[cyan]Fetching event stream[reset]"),
		progressbar.OptionSetWidth(20),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]#[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: "[white].[reset]",
			BarStart:      "[blue]|[reset]",
			BarEnd:        "[blue]|[reset]",
		}))

	mc := pool.GetClient()
	for {
		events, resp, err := mc.Client.Activity.ListEventsPerformedByUser(ctx, username, false, opts)
		if resp != nil {
			mc.UpdateRateLimit(resp.Rate.Remaining, resp.Rate.Reset.Time)
		}
		if err != nil {
			color.Yellow("[!]  Warning: Could not fetch user events: %v", err)
			break
		}

		allEvents = append(allEvents, events...)
		bar.Add(len(events))

		if resp.NextPage == 0 || len(allEvents) >= 300 {
			break
		}
		opts.Page = resp.NextPage
	}

	bar.Finish()

	if len(allEvents) == 0 {
		color.Yellow("[!] No recent events found for user: %s", username)
		return emails
	}

	color.Green("[+] Found %d recent events", len(allEvents))
	fmt.Println()
	color.Blue("Analyzing events for commit data...")

	commitCount := 0
	processBar := progressbar.NewOptions(len(allEvents),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(20),
		progressbar.OptionSetDescription("[cyan]Processing events[reset]"),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]#[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: "[white].[reset]",
			BarStart:      "[blue]|[reset]",
			BarEnd:        "[blue]|[reset]",
		}))

	for _, event := range allEvents {
		if event.Type != nil && *event.Type == "PushEvent" {
			commits := processEventCommits(event, checkSecrets, cfg)
			commitCount += len(commits)
			aggregateCommits(emails, commits, event.Repo.GetFullName(), targetUserIdentifiers, showTargetOnly)
		}
		processBar.Add(1)
	}

	processBar.Finish()

	fmt.Println()
	if commitCount > 0 {
		color.Green("[+] Extracted %d commits from %d push events", commitCount, len(allEvents))
	} else {
		color.Yellow("[!] No commits found in recent events")
	}

	return emails
}

func processEventCommits(event *gh.Event, checkSecrets bool, cfg *Config) []models.CommitInfo {
	var commits []models.CommitInfo

	payloadData := event.Payload()
	if payloadData == nil {
		return commits
	}

	payload, ok := payloadData.(map[string]interface{})
	if !ok {
		return commits
	}

	commitsData, ok := payload["commits"].([]interface{})
	if !ok {
		return commits
	}

	for _, commitData := range commitsData {
		commit, ok := commitData.(map[string]interface{})
		if !ok {
			continue
		}

		var commitInfo models.CommitInfo

		if sha, ok := commit["sha"].(string); ok {
			commitInfo.Hash = sha
		}
		if message, ok := commit["message"].(string); ok {
			commitInfo.Message = message
		}
		if url, ok := commit["url"].(string); ok {
			commitInfo.URL = url
		}

		if author, ok := commit["author"].(map[string]interface{}); ok {
			if name, ok := author["name"].(string); ok {
				commitInfo.AuthorName = name
			}
			if email, ok := author["email"].(string); ok {
				commitInfo.AuthorEmail = email
			}
		}

		if event.CreatedAt != nil {
			commitInfo.AuthorDate = event.CreatedAt.Time
			commitInfo.CommitterDate = event.CreatedAt.Time
		}

		if commitInfo.CommitterName == "" {
			commitInfo.CommitterName = commitInfo.AuthorName
		}
		if commitInfo.CommitterEmail == "" {
			commitInfo.CommitterEmail = commitInfo.AuthorEmail
		}

		if commitInfo.AuthorEmail == "" && commitInfo.AuthorName == "" {
			continue
		}

		if (checkSecrets || cfg.ShowInteresting) && commitInfo.Message != "" {
			secretScanner := scanner.NewScanner(cfg.ShowInteresting)
			commitInfo.Secrets = append(commitInfo.Secrets,
				scanContent(secretScanner, commitInfo.Message, "commit message", checkSecrets, cfg.ShowInteresting)...)
		}

		commits = append(commits, commitInfo)
	}

	return commits
}

func RateLimitedProcessRepos(ctx context.Context, pool *ClientPool, repos []*gh.Repository, checkSecrets bool, cfg *Config, targetUserIdentifiers map[string]bool, showTargetOnly bool) map[string]*models.EmailDetails {
	if cfg == nil {
		cfg = &Config{}
		*cfg = DefaultConfig()
	}

	emails := make(map[string]*models.EmailDetails)

	rateLimiter := time.NewTicker(time.Millisecond * 200)
	defer rateLimiter.Stop()

	totalRepos := len(repos)
	totalCommitsProcessed := 0
	totalDirectCommits := 0
	totalMergeCommits := 0

	progressDescription := "[cyan]Processing repositories[reset]"
	if cfg.QuickMode {
		progressDescription = "[cyan]Processing repositories (quick)[reset]"
	}

	bar := progressbar.NewOptions(totalRepos,
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(10),
		progressbar.OptionSetDescription(progressDescription),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]#[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: "[white].[reset]",
			BarStart:      "[blue]|[reset]",
			BarEnd:        "[blue]|[reset]",
		}))

	for _, repo := range repos {
		<-rateLimiter.C

		mc := pool.GetClient()
		repoDirectCommits := 0
		repoMergeCommits := 0
		var allRepoCommits []*gh.RepositoryCommit

		perPage := 100
		if cfg.QuickMode {
			perPage = 50
		}

		opts := &gh.CommitsListOptions{
			ListOptions: gh.ListOptions{PerPage: perPage},
		}

		for {
			<-rateLimiter.C
			commits, resp, _ := mc.Client.Repositories.ListCommits(ctx, repo.GetOwner().GetLogin(), repo.GetName(), opts)
			if resp != nil {
				mc.UpdateRateLimit(resp.Rate.Remaining, resp.Rate.Reset.Time)
			}

			for _, commit := range commits {
				if len(commit.Parents) <= 1 {
					repoDirectCommits++
				} else {
					repoMergeCommits++
				}
			}

			allRepoCommits = append(allRepoCommits, commits...)

			if resp == nil || resp.NextPage == 0 || cfg.QuickMode {
				break
			}
			opts.Page = resp.NextPage
		}

		var repoCommitInfos []models.CommitInfo
		for _, commit := range allRepoCommits {
			if (checkSecrets || cfg.ShowInteresting) && !cfg.QuickMode {
				<-rateLimiter.C
				fullCommit, getResp, err := mc.Client.Repositories.GetCommit(ctx, repo.GetOwner().GetLogin(), repo.GetName(), commit.GetSHA(), &gh.ListOptions{})
				if getResp != nil {
					mc.UpdateRateLimit(getResp.Rate.Remaining, getResp.Rate.Reset.Time)
				}
				if err == nil {
					commit = fullCommit
				}
			}

			commitInfo := ProcessCommit(commit, checkSecrets, cfg)
			if commitInfo.AuthorEmail != "" && strings.Contains(commitInfo.AuthorEmail, "@") {
				repoCommitInfos = append(repoCommitInfos, commitInfo)
			}
		}

		aggregateCommits(emails, repoCommitInfos, repo.GetFullName(), targetUserIdentifiers, showTargetOnly)

		totalCommitsProcessed += len(allRepoCommits)
		totalDirectCommits += repoDirectCommits
		totalMergeCommits += repoMergeCommits

		bar.Add(1)
	}

	bar.Finish()

	if len(emails) > 0 {
		domainStats := make(map[string]int)
		for email := range emails {
			if strings.Contains(email, "@") {
				domain := strings.Split(email, "@")[1]
				domainStats[domain]++
			}
		}

		fmt.Println()
		color.Cyan("Email Domain Distribution (Top 10):")
		type domainCount struct {
			domain string
			count  int
		}
		var domains []domainCount
		for domain, count := range domainStats {
			domains = append(domains, domainCount{domain, count})
		}

		for i := 0; i < len(domains)-1; i++ {
			for j := i + 1; j < len(domains); j++ {
				if domains[i].count < domains[j].count {
					domains[i], domains[j] = domains[j], domains[i]
				}
			}
		}

		for i, d := range domains {
			if i >= 10 {
				break
			}
			fmt.Printf("  %s: %d contributors\n", d.domain, d.count)
		}
	}

	return emails
}

func ProcessReposLimited(ctx context.Context, pool *ClientPool, repos []*gh.Repository, checkSecrets bool, cfg *Config, targetUserIdentifiers map[string]bool, showTargetOnly bool) map[string]*models.EmailDetails {
	if cfg == nil {
		cfg = &Config{}
		*cfg = DefaultConfig()
	}

	emails := make(map[string]*models.EmailDetails)

	maxRepos := 10
	maxCommitsPerRepo := 50

	if len(repos) > maxRepos {
		color.Yellow("[>] Processing only %d most recent repositories (out of %d total)", maxRepos, len(repos))
		repos = repos[:maxRepos]
	}

	color.Blue("[>] Light processing: %d repos, max %d recent commits each", len(repos), maxCommitsPerRepo)

	var progressDescription string
	if checkSecrets && cfg.ShowInteresting {
		progressDescription = "[cyan]Processing repos (secrets + patterns)[reset]"
	} else if checkSecrets {
		progressDescription = "[cyan]Processing repos (secrets)[reset]"
	} else if cfg.ShowInteresting {
		progressDescription = "[cyan]Processing repos (patterns)[reset]"
	} else {
		progressDescription = "[cyan]Processing repositories[reset]"
	}

	bar := progressbar.NewOptions(len(repos),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(20),
		progressbar.OptionSetDescription(progressDescription),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: "[white].[reset]",
			BarStart:      "[blue]|[reset]",
			BarEnd:        "[blue]|[reset]",
		}))

	for _, repo := range repos {
		time.Sleep(time.Millisecond * 100)

		mc := pool.GetClient()
		opts := &gh.CommitsListOptions{
			ListOptions: gh.ListOptions{PerPage: maxCommitsPerRepo},
		}

		commits, resp, _ := mc.Client.Repositories.ListCommits(ctx, repo.GetOwner().GetLogin(), repo.GetName(), opts)
		if resp != nil {
			mc.UpdateRateLimit(resp.Rate.Remaining, resp.Rate.Reset.Time)
		}

		var repoCommits []models.CommitInfo
		for _, commit := range commits {
			commitInfo := ProcessCommit(commit, false, cfg)
			repoCommits = append(repoCommits, commitInfo)
		}

		aggregateCommits(emails, repoCommits, repo.GetFullName(), targetUserIdentifiers, showTargetOnly)
		bar.Add(1)
	}

	bar.Finish()
	return emails
}
