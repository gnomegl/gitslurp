package trufflehog

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/gnomegl/gitslurp/v2/internal/github"
	gh "github.com/google/go-github/v57/github"
	"github.com/schollz/progressbar/v3"
)

// ScanScope defines what to scan with trufflehog
type ScanScope struct {
	Target     bool
	Members    bool
	Followers  bool
	Following  bool
	Stargazers bool
}

// ParseScanScope parses the --secrets flag value into a ScanScope
func ParseScanScope(value string) (*ScanScope, error) {
	if value == "" || value == "true" {
		return &ScanScope{Target: true}, nil
	}

	scope := &ScanScope{}
	parts := strings.Split(value, ",")
	for _, part := range parts {
		switch strings.TrimSpace(strings.ToLower(part)) {
		case "target":
			scope.Target = true
		case "members":
			scope.Members = true
		case "followers":
			scope.Followers = true
		case "following":
			scope.Following = true
		case "stargazers":
			scope.Stargazers = true
		default:
			return nil, fmt.Errorf("unknown secrets scope: %q (valid: target,members,followers,following,stargazers)", part)
		}
	}

	return scope, nil
}

// Finding represents a single trufflehog finding
type Finding struct {
	DetectorName string `json:"DetectorName"`
	Verified     bool   `json:"Verified"`
	Raw          string `json:"Raw"`
	RawV2        string `json:"RawV2"`
	SourceType   int    `json:"SourceType"`
	SourceName   string `json:"SourceName"`
	ExtraData    map[string]interface{} `json:"ExtraData"`
	SourceMeta   struct {
		Data struct {
			Repository string `json:"Repository"`
			Commit     string `json:"Commit"`
			Email      string `json:"Email"`
			File       string `json:"File"`
			Link       string `json:"link"`
			Timestamp  string `json:"Timestamp"`
		} `json:"Data"`
	} `json:"SourceMetadata"`
}

// ScanResult holds results for a single user
type ScanResult struct {
	Username string
	Findings []Finding
	Error    error
}

// Runner coordinates trufflehog scanning
type Runner struct {
	pool            *github.ClientPool
	scope           *ScanScope
	concurrency     int
	outputDir       string
	discoveredUsers []string // GitHub logins discovered from commit analysis
}

func NewRunner(pool *github.ClientPool, scope *ScanScope) *Runner {
	return &Runner{
		pool:        pool,
		scope:       scope,
		concurrency: 3,
		outputDir:   "trufflehog_results",
	}
}

// SetDiscoveredUsers sets the list of GitHub logins discovered from commit analysis.
// When scope includes "members", these users are used instead of the Members API
// (which only returns public members).
func (r *Runner) SetDiscoveredUsers(users []string) {
	r.discoveredUsers = users
}

// checkTrufflehog verifies trufflehog is installed
func checkTrufflehog() error {
	_, err := exec.LookPath("trufflehog")
	if err != nil {
		return fmt.Errorf("trufflehog not found in PATH — install from https://github.com/trufflesecurity/trufflehog")
	}
	return nil
}

// Run executes the trufflehog scanning pipeline
func (r *Runner) Run(ctx context.Context, target string, isOrg bool) error {
	if err := checkTrufflehog(); err != nil {
		return err
	}

	// Collect all usernames to scan
	users, err := r.resolveUsers(ctx, target, isOrg)
	if err != nil {
		return fmt.Errorf("failed to resolve scan targets: %v", err)
	}

	if len(users) == 0 {
		color.Yellow("[!] No users resolved for trufflehog scanning")
		return nil
	}

	// Create output directory
	if err := os.MkdirAll(r.outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	fmt.Println()
	color.Cyan("═══════════════════════════════════════════════════════════════")
	color.Cyan("  🐷 TruffleHog Secret Scanner")
	color.Cyan("═══════════════════════════════════════════════════════════════")
	fmt.Printf("  Targets: %d users\n", len(users))
	fmt.Printf("  Output:  %s/\n", r.outputDir)
	fmt.Printf("  Scope:   %s\n", r.scopeDescription())
	color.Cyan("═══════════════════════════════════════════════════════════════")
	fmt.Println()

	// Scan all users
	results := r.scanUsers(ctx, users)

	// Display and save results
	r.displayResults(results)

	return nil
}

func (r *Runner) scopeDescription() string {
	var parts []string
	if r.scope.Target {
		parts = append(parts, "target")
	}
	if r.scope.Members {
		parts = append(parts, "members")
	}
	if r.scope.Followers {
		parts = append(parts, "followers")
	}
	if r.scope.Following {
		parts = append(parts, "following")
	}
	if r.scope.Stargazers {
		parts = append(parts, "stargazers")
	}
	return strings.Join(parts, ", ")
}

// resolveUsers builds the list of GitHub usernames to scan
func (r *Runner) resolveUsers(ctx context.Context, target string, isOrg bool) ([]string, error) {
	seen := make(map[string]bool)
	var users []string

	addUser := func(login string) {
		lower := strings.ToLower(login)
		if !seen[lower] {
			seen[lower] = true
			users = append(users, login)
		}
	}

	if r.scope.Target {
		addUser(target)
	}

	client := r.pool.GetClient().Client

	if r.scope.Members {
		// First use discovered users from commit analysis (more complete than API)
		if len(r.discoveredUsers) > 0 {
			color.Blue("[*] Using %d contributors discovered from commit analysis", len(r.discoveredUsers))
			for _, u := range r.discoveredUsers {
				addUser(u)
			}
		}

		// Also try the Members API for org targets (may find members who haven't committed)
		if isOrg {
			color.Blue("[*] Fetching public organization members for %s...", target)
			members, err := r.fetchOrgMembers(ctx, client, target)
			if err != nil {
				color.Yellow("[!] Failed to fetch org members: %v", err)
			} else {
				newCount := 0
				for _, m := range members {
					if !seen[strings.ToLower(m)] {
						newCount++
					}
					addUser(m)
				}
				if newCount > 0 {
					color.Green("[+] Found %d additional public members via API", newCount)
				}
			}
		} else if len(r.discoveredUsers) == 0 {
			color.Yellow("[!] --secrets members: no contributors discovered (try running without -p to analyze commits first)")
		}
	}

	if r.scope.Followers {
		color.Blue("[*] Fetching followers for %s...", target)
		followers, err := r.fetchFollowers(ctx, client, target)
		if err != nil {
			color.Yellow("[!] Failed to fetch followers: %v", err)
		} else {
			color.Green("[+] Found %d followers", len(followers))
			for _, f := range followers {
				addUser(f)
			}
		}
	}

	if r.scope.Following {
		color.Blue("[*] Fetching following for %s...", target)
		following, err := r.fetchFollowing(ctx, client, target)
		if err != nil {
			color.Yellow("[!] Failed to fetch following: %v", err)
		} else {
			color.Green("[+] Found %d following", len(following))
			for _, f := range following {
				addUser(f)
			}
		}
	}

	if r.scope.Stargazers {
		color.Blue("[*] Fetching stargazers across %s's repos...", target)
		stargazers, err := r.fetchStargazers(ctx, client, target, isOrg)
		if err != nil {
			color.Yellow("[!] Failed to fetch stargazers: %v", err)
		} else {
			color.Green("[+] Found %d unique stargazers", len(stargazers))
			for _, s := range stargazers {
				addUser(s)
			}
		}
	}

	return users, nil
}

func (r *Runner) fetchOrgMembers(ctx context.Context, client *gh.Client, org string) ([]string, error) {
	var members []string
	opts := &gh.ListMembersOptions{
		ListOptions: gh.ListOptions{PerPage: 100},
	}

	for {
		users, resp, err := client.Organizations.ListMembers(ctx, org, opts)
		if err != nil {
			return members, err
		}
		for _, u := range users {
			members = append(members, u.GetLogin())
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return members, nil
}

func (r *Runner) fetchFollowers(ctx context.Context, client *gh.Client, login string) ([]string, error) {
	var followers []string
	opts := &gh.ListOptions{PerPage: 100}

	for {
		users, resp, err := client.Users.ListFollowers(ctx, login, opts)
		if err != nil {
			return followers, err
		}
		for _, u := range users {
			followers = append(followers, u.GetLogin())
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return followers, nil
}

func (r *Runner) fetchFollowing(ctx context.Context, client *gh.Client, login string) ([]string, error) {
	var following []string
	opts := &gh.ListOptions{PerPage: 100}

	for {
		users, resp, err := client.Users.ListFollowing(ctx, login, opts)
		if err != nil {
			return following, err
		}
		for _, u := range users {
			following = append(following, u.GetLogin())
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return following, nil
}

func (r *Runner) fetchStargazers(ctx context.Context, client *gh.Client, target string, isOrg bool) ([]string, error) {
	seen := make(map[string]bool)
	var stargazers []string

	// Get repos for the target
	var repos []*gh.Repository
	var err error
	if isOrg {
		opts := &gh.RepositoryListByOrgOptions{
			Type:        "public",
			ListOptions: gh.ListOptions{PerPage: 100},
		}
		repos, _, err = client.Repositories.ListByOrg(ctx, target, opts)
	} else {
		opts := &gh.RepositoryListByUserOptions{
			Type:        "owner",
			Sort:        "stars",
			ListOptions: gh.ListOptions{PerPage: 100},
		}
		repos, _, err = client.Repositories.ListByUser(ctx, target, opts)
	}
	if err != nil {
		return nil, err
	}

	// Only check top 10 repos by stars to avoid rate limit exhaustion
	maxRepos := 10
	if len(repos) < maxRepos {
		maxRepos = len(repos)
	}

	for _, repo := range repos[:maxRepos] {
		opts := &gh.ListOptions{PerPage: 100}
		users, _, err := client.Activity.ListStargazers(ctx, target, repo.GetName(), opts)
		if err != nil {
			continue
		}
		for _, u := range users {
			login := u.User.GetLogin()
			if !seen[login] {
				seen[login] = true
				stargazers = append(stargazers, login)
			}
		}
	}

	return stargazers, nil
}

// scanUsers runs trufflehog against each user concurrently
func (r *Runner) scanUsers(ctx context.Context, users []string) []ScanResult {
	results := make([]ScanResult, len(users))
	sem := make(chan struct{}, r.concurrency)
	var wg sync.WaitGroup

	bar := progressbar.NewOptions(len(users),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(15),
		progressbar.OptionSetDescription("[cyan]Scanning repos for secrets[reset]"),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]#[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: "-",
			BarStart:      "[",
			BarEnd:        "]",
		}))

	for i, user := range users {
		wg.Add(1)
		go func(idx int, username string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			findings, err := r.scanUser(ctx, username)
			results[idx] = ScanResult{
				Username: username,
				Findings: findings,
				Error:    err,
			}
			bar.Add(1)
		}(i, user)
	}

	wg.Wait()
	bar.Finish()
	return results
}

// scanUser runs trufflehog against a single GitHub user and returns findings
func (r *Runner) scanUser(ctx context.Context, username string) ([]Finding, error) {
	token := r.pool.PrimaryToken()

	args := []string{
		"github",
		"--json",
		"--no-update",
		"--org=" + username,
		"--results=verified,unknown",
	}
	if token != "" {
		args = append(args, "--token="+token)
	}

	cmd := exec.CommandContext(ctx, "trufflehog", args...)
	cmd.Stderr = nil // suppress trufflehog's stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start trufflehog: %v", err)
	}

	var findings []Finding
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer for large findings

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var finding Finding
		if err := json.Unmarshal([]byte(line), &finding); err != nil {
			continue // skip malformed lines
		}
		findings = append(findings, finding)
	}

	// Don't check cmd.Wait() error — trufflehog exits 183 when findings exist
	cmd.Wait()

	// Save per-user results
	if len(findings) > 0 {
		r.saveUserResults(username, findings)
	}

	return findings, nil
}

func (r *Runner) saveUserResults(username string, findings []Finding) {
	outPath := filepath.Join(r.outputDir, username+".json")
	data, err := json.MarshalIndent(findings, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(outPath, data, 0644)
}

// displayResults shows a summary of all findings
func (r *Runner) displayResults(results []ScanResult) {
	fmt.Println()
	color.Cyan("═══════════════════════════════════════════════════════════════")
	color.Cyan("  🐷 TruffleHog Results Summary")
	color.Cyan("═══════════════════════════════════════════════════════════════")

	totalFindings := 0
	totalVerified := 0
	usersWithFindings := 0
	usersScanned := 0
	usersFailed := 0

	var summaryLines []string

	for _, result := range results {
		if result.Error != nil {
			usersFailed++
			continue
		}
		usersScanned++

		if len(result.Findings) == 0 {
			continue
		}

		usersWithFindings++
		verified := 0
		detectors := make(map[string]int)

		for _, f := range result.Findings {
			totalFindings++
			if f.Verified {
				verified++
				totalVerified++
			}
			detectors[f.DetectorName]++
		}

		// Build detector breakdown
		var detectorParts []string
		for name, count := range detectors {
			detectorParts = append(detectorParts, fmt.Sprintf("%s(%d)", name, count))
		}

		line := fmt.Sprintf("  %-25s %3d findings (%d verified)  %s",
			result.Username, len(result.Findings), verified, strings.Join(detectorParts, ", "))
		summaryLines = append(summaryLines, line)
	}

	if totalFindings == 0 {
		color.Green("\n  No secrets found across %d users scanned", usersScanned)
	} else {
		fmt.Println()
		for _, line := range summaryLines {
			if strings.Contains(line, "verified)") && !strings.Contains(line, "0 verified)") {
				color.Red(line)
			} else {
				color.Yellow(line)
			}
		}

		fmt.Println()
		color.Cyan("───────────────────────────────────────────────────────────────")
		fmt.Printf("  Users scanned:        %d\n", usersScanned)
		fmt.Printf("  Users with findings:  %d\n", usersWithFindings)
		fmt.Printf("  Total findings:       %d\n", totalFindings)
		if totalVerified > 0 {
			color.Red("  Verified secrets:     %d", totalVerified)
		} else {
			fmt.Printf("  Verified secrets:     %d\n", totalVerified)
		}
		if usersFailed > 0 {
			color.Yellow("  Scan failures:        %d", usersFailed)
		}
	}

	fmt.Printf("\n  Results saved to: %s/\n", r.outputDir)
	color.Cyan("═══════════════════════════════════════════════════════════════")

	// Save aggregate summary
	r.saveSummary(results, usersScanned, totalFindings, totalVerified)
}

func (r *Runner) saveSummary(results []ScanResult, scanned, total, verified int) {
	summary := map[string]interface{}{
		"scan_time":           time.Now().UTC().Format(time.RFC3339),
		"users_scanned":       scanned,
		"total_findings":      total,
		"verified_findings":   verified,
		"scope":               r.scopeDescription(),
	}

	var userResults []map[string]interface{}
	for _, result := range results {
		if result.Error != nil || len(result.Findings) == 0 {
			continue
		}
		userResults = append(userResults, map[string]interface{}{
			"username":      result.Username,
			"finding_count": len(result.Findings),
			"findings":      result.Findings,
		})
	}
	summary["users"] = userResults

	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(filepath.Join(r.outputDir, "summary.json"), data, 0644)
}
