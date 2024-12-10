package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/google/go-github/v57/github"
	"github.com/urfave/cli/v2"
	"golang.org/x/oauth2"
)

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)aws_access_key.*=.*`),
	regexp.MustCompile(`(?i)aws_secret.*=.*`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`(?i)private_key.*=.*`),
	regexp.MustCompile(`(?i)secret.*=.*[0-9a-zA-Z]{16,}`),
	regexp.MustCompile(`(?i)password.*=.*[0-9a-zA-Z]{8,}`),
	regexp.MustCompile(`(?i)token.*=.*[0-9a-zA-Z]{8,}`),
	regexp.MustCompile(`-----BEGIN ((RSA|DSA|EC|PGP|OPENSSH) )?PRIVATE KEY( BLOCK)?-----`),
	regexp.MustCompile(`(?i)github[_\-\.]?token.*=.*[0-9a-zA-Z]{35,40}`),
	regexp.MustCompile(`(?i)api[_\-\.]?key.*=.*[0-9a-zA-Z]{16,}`),
}

var urlPattern = regexp.MustCompile(`https?://[^\s<>"]+|www\.[^\s<>"]+`)

type CommitInfo struct {
	Hash        string
	URL         string
	AuthorName  string
	AuthorEmail string
	Message     string
	Date        time.Time
	RepoName    string
	IsFork      bool
	Secrets     []string
	Links       []string
}

type EmailDetails struct {
	Names       map[string]struct{}
	Commits     map[string][]CommitInfo
	CommitCount int
}

func getGithubClient(token string) *github.Client {
	if token == "" {
		return github.NewClient(nil)
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(context.Background(), ts)
	return github.NewClient(tc)
}

func getToken(c *cli.Context) string {
	token := os.Getenv("GITHUB_TOKEN")
	if token != "" {
		return token
	}

	if c.String("token") != "" {
		return c.String("token")
	}

	configDir, err := os.UserConfigDir()
	if err == nil {
		tokenFile := filepath.Join(configDir, "gitslurp", "token")
		if data, err := os.ReadFile(tokenFile); err == nil {
			token = strings.TrimSpace(string(data))
			if token != "" {
				color.Green("Using saved token from config file")
				return token
			}
		}
	}

	color.Yellow("\nA GitHub personal access token is recommended to avoid rate limits and access private repositories.")
	color.Blue("To create a new token:")
	fmt.Println("1. Visit: https://github.com/settings/tokens")
	fmt.Println("2. Click 'Generate new token' (classic)")
	fmt.Println("3. Give it a name (e.g. 'gitslurp')")
	fmt.Println("4. Select the following scopes:")
	color.Green("   - repo (for private repos)")
	color.Green("   - read:user")
	color.Green("   - user:email")
	fmt.Println("5. Click 'Generate token' at the bottom")
	fmt.Println("6. Copy the token and paste it below")
	fmt.Println("\nNote: The token will be saved locally for future use")

	fmt.Print("\nPaste your token here (or press Enter to continue without one): ")
	var input string
	fmt.Scanln(&input)
	token = strings.TrimSpace(input)

	if token != "" {
		if configDir != "" {
			configPath := filepath.Join(configDir, "gitslurp")
			os.MkdirAll(configPath, 0700)
			tokenFile := filepath.Join(configPath, "token")
			if err := os.WriteFile(tokenFile, []byte(token), 0600); err == nil {
				color.Green("Token saved successfully")
			}
		}
	} else {
		color.Yellow("\nRunning without a token. You may hit rate limits and won't see private repository information.")
	}

	return token
}

func fetchRepos(ctx context.Context, client *github.Client, username string) ([]*github.Repository, error) {
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

func checkForSecrets(content string) []string {
	var secrets []string
	for _, pattern := range secretPatterns {
		matches := pattern.FindAllString(content, -1)
		secrets = append(secrets, matches...)
	}
	return secrets
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

func extractLinks(content string) []string {
	matches := urlPattern.FindAllString(content, -1)
	uniqueLinks := make(map[string]struct{})
	for _, link := range matches {
		uniqueLinks[link] = struct{}{}
	}

	links := make([]string, 0, len(uniqueLinks))
	for link := range uniqueLinks {
		links = append(links, link)
	}
	sort.Strings(links)
	return links
}

func fetchCommits(ctx context.Context, client *github.Client, owner, repo string, isFork bool, since *time.Time, checkSecrets bool, findLinks bool) ([]CommitInfo, error) {
	var allCommits []CommitInfo
	opt := &github.CommitsListOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}
	if since != nil {
		opt.Since = *since
	}

	for {
		commits, resp, err := client.Repositories.ListCommits(ctx, owner, repo, opt)
		if err != nil {
			return nil, fmt.Errorf("error fetching commits: %v", err)
		}

		for _, commit := range commits {
			if commit.Commit == nil || commit.Commit.Author == nil {
				continue
			}

			commitInfo := CommitInfo{
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
						commitInfo.Secrets = checkForSecrets(content)
					}
					if findLinks {
						commitInfo.Links = extractLinks(content)
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

func processRepos(ctx context.Context, client *github.Client, repos []*github.Repository, checkSecrets bool, findLinks bool) map[string]*EmailDetails {
	emails := make(map[string]*EmailDetails)
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

			commits, err := fetchCommits(ctx, client, repo.GetOwner().GetLogin(), repo.GetName(), repo.GetFork(), since, checkSecrets, findLinks)
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
					emails[email] = &EmailDetails{
						Names:   make(map[string]struct{}),
						Commits: make(map[string][]CommitInfo),
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

func displayResults(emails map[string]*EmailDetails, showDetails bool, checkSecrets bool, showLinks bool) {
	type emailEntry struct {
		Email   string
		Details *EmailDetails
	}

	var sortedEmails []emailEntry
	for email, details := range emails {
		sortedEmails = append(sortedEmails, emailEntry{email, details})
	}

	sort.Slice(sortedEmails, func(i, j int) bool {
		return sortedEmails[i].Details.CommitCount > sortedEmails[j].Details.CommitCount
	})

	fmt.Println("\nCollected author information:")
	for _, entry := range sortedEmails {
		color.Yellow(entry.Email)

		if showDetails || checkSecrets || showLinks {
			for repoName, commits := range entry.Details.Commits {
				color.Green("  Repo: %s", repoName)
				for _, commit := range commits {
					shouldShowCommit := showDetails ||
						(checkSecrets && len(commit.Secrets) > 0) ||
						(showLinks && len(commit.Links) > 0)

					if !shouldShowCommit {
						continue
					}

					color.Magenta("    Commit: %s", commit.Hash)
					color.Blue("    URL: %s", commit.URL)
					color.White("    Author: %s", commit.AuthorName)
					if commit.IsFork {
						color.Cyan("    Fork: true")
					}

					if checkSecrets && len(commit.Secrets) > 0 {
						color.Red("    Potential secrets found:")
						for _, secret := range commit.Secrets {
							color.Red("      - %s", secret)
						}
					}

					if showLinks && len(commit.Links) > 0 {
						color.Blue("    Links found:")
						for _, link := range commit.Links {
							color.Blue("      - %s", link)
						}
					}
				}
			}
		} else {
			names := make([]string, 0, len(entry.Details.Names))
			for name := range entry.Details.Names {
				names = append(names, name)
			}
			color.White("  Names: %s", strings.Join(names, ", "))
			color.White("  Total Commits: %d", entry.Details.CommitCount)
		}
	}
}

func runApp(c *cli.Context) error {
	if c.NArg() < 1 {
		return cli.ShowAppHelp(c)
	}

	username := c.Args().First()
	token := getToken(c)
	showDetails := c.Bool("details")
	checkSecrets := c.Bool("secrets")
	showLinks := c.Bool("links")

	client := getGithubClient(token)
	ctx := context.Background()

	color.Blue("Fetching public repositories for user: %s", username)
	repos, err := fetchRepos(ctx, client, username)
	if err != nil {
		return fmt.Errorf("error fetching repositories: %v", err)
	}

	emails := processRepos(ctx, client, repos, checkSecrets, showLinks)
	displayResults(emails, showDetails, checkSecrets, showLinks)
	return nil
}

func main() {
	app := &cli.App{
		Name:  "gitslurp",
		Usage: "Analyze GitHub user's commit history across repositories",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "details",
				Aliases: []string{"d"},
				Usage:   "Show detailed commit information",
			},
			&cli.BoolFlag{
				Name:    "secrets",
				Aliases: []string{"s"},
				Usage:   "Check for potential secrets in commits",
			},
			&cli.BoolFlag{
				Name:    "links",
				Aliases: []string{"l"},
				Usage:   "Extract links from commits",
			},
			&cli.StringFlag{
				Name:    "token",
				Aliases: []string{"t"},
				Usage:   "GitHub personal access token",
				EnvVars: []string{"GITHUB_TOKEN"},
			},
		},
		Action: runApp,
		Authors: []*cli.Author{
			{
				Name: "GitSlurp Team",
			},
		},
		Version: "1.0.0",
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
