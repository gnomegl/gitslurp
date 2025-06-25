package github

import (
	"context"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/gnomegl/gitslurp/internal/models"
	"github.com/gnomegl/gitslurp/internal/scanner"
	gh "github.com/google/go-github/v57/github"
	"github.com/schollz/progressbar/v3"
)

// ProcessUserEvents processes user events using the GitHub Events API (more efficient)
func ProcessUserEvents(ctx context.Context, client *gh.Client, username string, checkSecrets bool, cfg *Config, targetUserIdentifiers map[string]bool, showTargetOnly bool) map[string]*models.EmailDetails {
	if cfg == nil {
		cfg = &Config{}
		*cfg = DefaultConfig()
	}

	emails := make(map[string]*models.EmailDetails)

	var progressDescription string
	if checkSecrets && cfg.ShowInteresting {
		progressDescription = "[cyan]Processing recent events for secrets and patterns 🐽[reset]"
	} else if checkSecrets {
		progressDescription = "[cyan]Processing recent events for secrets 🐽[reset]"
	} else if cfg.ShowInteresting {
		progressDescription = "[cyan]Processing recent events for patterns ⭐[reset]"
	} else {
		progressDescription = "[cyan]Processing recent user activity[reset]"
	}

	color.Blue("\n%s", strings.Replace(progressDescription, "[cyan]", "", -1))
	color.Blue("(Use --deep flag for complete commit history)")

	// Fetch user events
	var allEvents []*gh.Event
	opts := &gh.ListOptions{PerPage: 100}
	
	bar := progressbar.NewOptions(-1,
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionSetDescription("[cyan]Fetching events...[reset]"),
		progressbar.OptionSetWidth(20),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]█[reset]",
			SaucerHead:    "[green]█[reset]",
			SaucerPadding: "[white]░[reset]",
			BarStart:      "[blue]▐[reset]",
			BarEnd:        "[blue]▌[reset]",
		}))

	for {
		events, resp, err := client.Activity.ListEventsPerformedByUser(ctx, username, false, opts)
		if err != nil {
			color.Yellow("⚠️  Warning: Could not fetch user events: %v", err)
			break
		}
		
		allEvents = append(allEvents, events...)
		bar.Add(len(events))
		
		if resp.NextPage == 0 || len(allEvents) >= 300 { // Limit to recent activity
			break
		}
		opts.Page = resp.NextPage
	}
	
	bar.Finish()

	if len(allEvents) == 0 {
		color.Yellow("⚠️  No recent events found for user: %s", username)
		return emails
	}

	color.Green("✅ Found %d recent events", len(allEvents))

	// Process push events to extract commit information
	commitCount := 0
	processBar := progressbar.NewOptions(len(allEvents),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(20),
		progressbar.OptionSetDescription("[cyan]Processing events[reset]"),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]█[reset]",
			SaucerHead:    "[green]█[reset]",
			SaucerPadding: "[white]░[reset]",
			BarStart:      "[blue]▐[reset]",
			BarEnd:        "[blue]▌[reset]",
		}))

	for _, event := range allEvents {
		if event.Type != nil && *event.Type == "PushEvent" {
			commits := processEventCommits(ctx, client, event, checkSecrets, cfg)
			commitCount += len(commits)
			aggregateCommits(emails, commits, event.Repo.GetFullName(), targetUserIdentifiers, showTargetOnly)
		}
		processBar.Add(1)
	}
	
	processBar.Finish()

	if commitCount > 0 {
		color.Green("✅ Processed %d commits from recent push events", commitCount)
	}

	return emails
}

// processEventCommits extracts commit information from push events
func processEventCommits(ctx context.Context, client *gh.Client, event *gh.Event, checkSecrets bool, cfg *Config) []models.CommitInfo {
	var commits []models.CommitInfo
	
	// Get the payload - it's a function that returns interface{}
	payloadData := event.Payload()
	if payloadData == nil {
		return commits
	}

	// Parse push event payload
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
		
		// Extract basic commit info
		if sha, ok := commit["sha"].(string); ok {
			commitInfo.Hash = sha
		}
		if message, ok := commit["message"].(string); ok {
			commitInfo.Message = message
		}
		if url, ok := commit["url"].(string); ok {
			commitInfo.URL = url
		}

		// Extract author info
		if author, ok := commit["author"].(map[string]interface{}); ok {
			if name, ok := author["name"].(string); ok {
				commitInfo.AuthorName = name
			}
			if email, ok := author["email"].(string); ok {
				commitInfo.AuthorEmail = email
			}
		}

		// Set timestamp from event
		if event.CreatedAt != nil {
			commitInfo.AuthorDate = event.CreatedAt.Time
			commitInfo.CommitterDate = event.CreatedAt.Time
		}

		// Set committer info (usually same as author for push events)
		if commitInfo.CommitterName == "" {
			commitInfo.CommitterName = commitInfo.AuthorName
		}
		if commitInfo.CommitterEmail == "" {
			commitInfo.CommitterEmail = commitInfo.AuthorEmail
		}

		// Skip anonymous commits unless they have some identifying info
		if commitInfo.AuthorEmail == "" && commitInfo.AuthorName == "" {
			continue
		}

		// Scan commit message for secrets/patterns if enabled
		if (checkSecrets || cfg.ShowInteresting) && commitInfo.Message != "" {
			secretScanner := scanner.NewScanner(cfg.ShowInteresting)
			commitInfo.Secrets = append(commitInfo.Secrets, 
				scanContent(secretScanner, commitInfo.Message, "commit message", checkSecrets, cfg.ShowInteresting)...)
		}

		commits = append(commits, commitInfo)
	}

	return commits
}

// RateLimitedProcessRepos performs comprehensive contributor enumeration for --deep mode
func RateLimitedProcessRepos(ctx context.Context, client *gh.Client, repos []*gh.Repository, checkSecrets bool, cfg *Config, targetUserIdentifiers map[string]bool, showTargetOnly bool) map[string]*models.EmailDetails {
	if cfg == nil {
		cfg = &Config{}
		*cfg = DefaultConfig()
	}

	color.Yellow("🔍 Deep crawl mode: Comprehensive contributor enumeration")
	color.Blue("📊 This will analyze ALL commits across ALL repositories")
	color.Yellow("⏳ This may take several minutes depending on repository size")

	emails := make(map[string]*models.EmailDetails)
	
	// Rate limiting setup
	rateLimiter := time.NewTicker(time.Millisecond * 200) // 5 requests per second max
	defer rateLimiter.Stop()

	totalRepos := len(repos)
	totalCommitsProcessed := 0
	totalDirectCommits := 0
	totalMergeCommits := 0

	// Progress tracking
	bar := progressbar.NewOptions(totalRepos,
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(20),
		progressbar.OptionSetDescription("[cyan]Deep analysis of repositories[reset]"),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]█[reset]",
			SaucerHead:    "[green]█[reset]",
			SaucerPadding: "[white]░[reset]",
			BarStart:      "[blue]▐[reset]",
			BarEnd:        "[blue]▌[reset]",
		}))

	for _, repo := range repos {
		<-rateLimiter.C // Rate limit

		repoDirectCommits := 0
		repoMergeCommits := 0
		var allRepoCommits []*gh.RepositoryCommit

		// Fetch ALL commits from this repository (paginated)
		opts := &gh.CommitsListOptions{
			ListOptions: gh.ListOptions{PerPage: 100},
		}

		for {
			<-rateLimiter.C // Rate limit each API call
			commits, resp, err := client.Repositories.ListCommits(ctx, repo.GetOwner().GetLogin(), repo.GetName(), opts)
			if err != nil {
				color.Yellow("⚠️  Skipping repo %s: %v", repo.GetFullName(), err)
				break
			}

			// Classify commits (direct vs merge)
			for _, commit := range commits {
				if len(commit.Parents) <= 1 {
					repoDirectCommits++
				} else {
					repoMergeCommits++
				}
			}

			allRepoCommits = append(allRepoCommits, commits...)
			
			if resp.NextPage == 0 {
				break
			}
			opts.Page = resp.NextPage
		}

		// Process commits for this repository
		var repoCommitInfos []models.CommitInfo
		for _, commit := range allRepoCommits {
			// For deep mode, optionally fetch full commit details for secrets scanning
			if checkSecrets || cfg.ShowInteresting {
				<-rateLimiter.C // Rate limit
				fullCommit, _, err := client.Repositories.GetCommit(ctx, repo.GetOwner().GetLogin(), repo.GetName(), commit.GetSHA(), &gh.ListOptions{})
				if err == nil {
					commit = fullCommit
				}
			}
			
			commitInfo := ProcessCommit(commit, checkSecrets, cfg)
			// Only include commits with email addresses for contributor analysis
			if commitInfo.AuthorEmail != "" && strings.Contains(commitInfo.AuthorEmail, "@") {
				repoCommitInfos = append(repoCommitInfos, commitInfo)
			}
		}

		// Aggregate commits for this repository
		aggregateCommits(emails, repoCommitInfos, repo.GetFullName(), targetUserIdentifiers, showTargetOnly)

		totalCommitsProcessed += len(allRepoCommits)
		totalDirectCommits += repoDirectCommits
		totalMergeCommits += repoMergeCommits

		bar.Add(1)
	}

	bar.Finish()

	// Display comprehensive statistics
	color.Green("\n✅ Deep Analysis Complete!")
	color.Blue("📊 Statistics:")
	color.Blue("   • Total repositories analyzed: %d", totalRepos)
	color.Blue("   • Total commits processed: %d", totalCommitsProcessed)
	color.Blue("   • Direct commits: %d", totalDirectCommits)
	color.Blue("   • Merge commits: %d", totalMergeCommits)
	color.Blue("   • Unique contributors with emails: %d", len(emails))

	// Display email domain analysis
	if len(emails) > 0 {
		domainStats := make(map[string]int)
		for email := range emails {
			if strings.Contains(email, "@") {
				domain := strings.Split(email, "@")[1]
				domainStats[domain]++
			}
		}

		color.Blue("\n📧 Email Domain Distribution:")
		// Sort domains by count
		type domainCount struct {
			domain string
			count  int
		}
		var domains []domainCount
		for domain, count := range domainStats {
			domains = append(domains, domainCount{domain, count})
		}
		
		// Simple sort by count (descending)
		for i := 0; i < len(domains)-1; i++ {
			for j := i + 1; j < len(domains); j++ {
				if domains[i].count < domains[j].count {
					domains[i], domains[j] = domains[j], domains[i]
				}
			}
		}

		for i, d := range domains {
			if i >= 10 { // Show top 10 domains
				break
			}
			color.Blue("   • %s: %d contributors", d.domain, d.count)
		}
	}

	return emails
}

// ProcessReposLimited processes only recent commits from repos (API-friendly fallback)
func ProcessReposLimited(ctx context.Context, client *gh.Client, repos []*gh.Repository, checkSecrets bool, cfg *Config, targetUserIdentifiers map[string]bool, showTargetOnly bool) map[string]*models.EmailDetails {
	if cfg == nil {
		cfg = &Config{}
		*cfg = DefaultConfig()
	}

	emails := make(map[string]*models.EmailDetails)
	
	// Limit repos but process more recent commits from each
	maxRepos := 10
	maxCommitsPerRepo := 50
	
	if len(repos) > maxRepos {
		color.Yellow("⚡ Processing only %d most recent repositories (out of %d total)", maxRepos, len(repos))
		repos = repos[:maxRepos]
	}

	color.Blue("⚡ Light processing: %d repos, max %d recent commits each", len(repos), maxCommitsPerRepo)

	// Add progress bar for fallback processing
	var progressDescription string
	if checkSecrets && cfg.ShowInteresting {
		progressDescription = "[cyan]Processing repos for secrets and patterns 🐽[reset]"
	} else if checkSecrets {
		progressDescription = "[cyan]Processing repos for secrets 🐽[reset]"
	} else if cfg.ShowInteresting {
		progressDescription = "[cyan]Processing repos for patterns ⭐[reset]"
	} else {
		progressDescription = "[cyan]Processing repositories[reset]"
	}

	bar := progressbar.NewOptions(len(repos),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(20),
		progressbar.OptionSetDescription(progressDescription),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]█[reset]",
			SaucerHead:    "[green]█[reset]",
			SaucerPadding: "[white]░[reset]",
			BarStart:      "[blue]▐[reset]",
			BarEnd:        "[blue]▌[reset]",
		}))

	for _, repo := range repos {
		// Small delay to be nice to the API
		time.Sleep(time.Millisecond * 100)
		
		// Get only recent commits
		opts := &gh.CommitsListOptions{
			ListOptions: gh.ListOptions{PerPage: maxCommitsPerRepo},
		}
		
		commits, _, err := client.Repositories.ListCommits(ctx, repo.GetOwner().GetLogin(), repo.GetName(), opts)
		if err != nil {
			color.Yellow("⚠️  Skipping repo %s: %v", repo.GetFullName(), err)
			bar.Add(1)
			continue
		}

		var repoCommits []models.CommitInfo
		for _, commit := range commits {
			// Don't fetch full commit details for secrets scanning in light mode
			commitInfo := ProcessCommit(commit, false, cfg) // Force checkSecrets to false
			repoCommits = append(repoCommits, commitInfo)
		}

		aggregateCommits(emails, repoCommits, repo.GetFullName(), targetUserIdentifiers, showTargetOnly)
		bar.Add(1)
	}

	bar.Finish()
	return emails
}