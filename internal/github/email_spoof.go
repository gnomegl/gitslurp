package github

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"github.com/fatih/color"
	"github.com/google/go-github/v57/github"
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

func IsValidEmail(input string) bool {
	return emailRegex.MatchString(input)
}

func GetUsernameFromEmailSpoof(ctx context.Context, client *github.Client, email string, token string) (string, error) {
	color.Yellow("[@] Attempting email spoofing method for: %s", email)
	
	user, _, err := client.Users.Get(ctx, "")
	if err != nil {
		return "", fmt.Errorf("GitHub token required for email spoofing method - please provide a valid token")
	}
	
	tempDir, err := ioutil.TempDir("", "gitslurp-spoof-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	repoName := fmt.Sprintf("temp-spoof-%d", time.Now().Unix())
	
	repo := &github.Repository{
		Name:        github.String(repoName),
		Private:     github.Bool(true),
		AutoInit:    github.Bool(false),
		Description: github.String("Temporary repository for email spoofing - will be deleted automatically"),
	}
	
	createdRepo, _, err := client.Repositories.Create(ctx, "", repo)
	if err != nil {
		return "", fmt.Errorf("failed to create repository (check token permissions): %v", err)
	}
	
	defer func() {
		color.Yellow("[-] Cleaning up temporary repository...")
		_, err := client.Repositories.Delete(ctx, user.GetLogin(), repoName)
		if err != nil {
			color.Red("[!] Warning: Failed to delete temporary repository %s: %v", repoName, err)
		}
	}()

	repoPath := filepath.Join(tempDir, repoName)
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create repo directory: %v", err)
	}
	
	if err := runGitCommand(repoPath, "init"); err != nil {
		return "", fmt.Errorf("failed to initialize git repo: %v", err)
	}
	
	// Use authenticated clone URL with token
	authenticatedURL := fmt.Sprintf("https://%s@github.com/%s/%s.git", token, user.GetLogin(), repoName)
	if err := runGitCommand(repoPath, "remote", "add", "origin", authenticatedURL); err != nil {
		return "", fmt.Errorf("failed to add remote: %v", err)
	}

	// create a dummy file
	dummyFile := filepath.Join(repoPath, "temp.txt")
	if err := ioutil.WriteFile(dummyFile, []byte("temp file for email spoofing"), 0644); err != nil {
		return "", fmt.Errorf("failed to create dummy file: %v", err)
	}

	// Configure git with the target email
	if err := runGitCommand(repoPath, "config", "user.email", email); err != nil {
		return "", fmt.Errorf("failed to set git email: %v", err)
	}
	
	if err := runGitCommand(repoPath, "config", "user.name", "TempUser"); err != nil {
		return "", fmt.Errorf("failed to set git name: %v", err)
	}

	if err := runGitCommand(repoPath, "add", "temp.txt"); err != nil {
		return "", fmt.Errorf("failed to add file: %v", err)
	}
	
	if err := runGitCommand(repoPath, "commit", "-m", "temp commit for email spoofing"); err != nil {
		return "", fmt.Errorf("failed to commit: %v", err)
	}

	if err := runGitCommand(repoPath, "branch", "-M", "master"); err != nil {
		return "", fmt.Errorf("failed to rename branch: %v", err)
	}
	
	if err := runGitCommand(repoPath, "push", "-u", "origin", "master"); err != nil {
		return "", fmt.Errorf("failed to push: %v", err)
	}

	// Give GitHub API time to sync after push
	time.Sleep(3 * time.Second)

	commits, _, err := client.Repositories.ListCommits(ctx, createdRepo.GetOwner().GetLogin(), repoName, &github.CommitsListOptions{
		ListOptions: github.ListOptions{PerPage: 1},
	})
	if err != nil || len(commits) == 0 {
		return "", fmt.Errorf("failed to get commits: %v", err)
	}

	commitSHA := commits[0].GetSHA()
	
	commit, _, err := client.Repositories.GetCommit(ctx, createdRepo.GetOwner().GetLogin(), repoName, commitSHA, nil)
	if err == nil && commit.GetAuthor() != nil && commit.GetAuthor().GetLogin() != "" {
		username := commit.GetAuthor().GetLogin()
		color.Green("[+] Found username via API: %s", username)
		return username, nil
	}

	// if api doesn't provide username, temporarily make repo public and scrape
	color.Yellow("[o] Temporarily making repository public for web scraping...")
	
	repoUpdate := &github.Repository{
		Private: github.Bool(false),
	}
	
	_, _, err = client.Repositories.Edit(ctx, createdRepo.GetOwner().GetLogin(), repoName, repoUpdate)
	if err != nil {
		return "", fmt.Errorf("failed to make repository public: %v", err)
	}

	time.Sleep(2 * time.Second)

	commitURL := fmt.Sprintf("https://github.com/%s/%s/commit/%s", createdRepo.GetOwner().GetLogin(), repoName, commitSHA)
	username, err := scrapeUsernameFromCommitPage(commitURL)
	if err != nil {
		return "", fmt.Errorf("failed to scrape username: %v", err)
	}

	color.Green("[+] Found username via scraping: %s", username)
	return username, nil
}

// executes a git command in the specified directory
func runGitCommand(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git command failed: %s, output: %s", err, string(output))
	}
	return nil
}

// scrapes the GitHub commit page to extract the username
func scrapeUsernameFromCommitPage(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch commit page: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}

	html := string(body)
	
	// <a class="commit-author" href="/username">
	usernameRegex1 := regexp.MustCompile(`<a[^>]+class="[^"]*commit-author[^"]*"[^>]+href="/([^"]+)"`)
	if matches := usernameRegex1.FindStringSubmatch(html); len(matches) > 1 {
		return matches[1], nil
	}

	// check commit info
	usernameRegex2 := regexp.MustCompile(`href="/([\w-]+)"[^>]*>[^<]*</a>[^<]*authored`)
	if matches := usernameRegex2.FindStringSubmatch(html); len(matches) > 1 {
		return matches[1], nil
	}

	// avatar link pattern
	usernameRegex3 := regexp.MustCompile(`<img[^>]+alt="@([^"]+)"[^>]*class="[^"]*avatar[^"]*"`)
	if matches := usernameRegex3.FindStringSubmatch(html); len(matches) > 1 {
		return matches[1], nil
	}

	return "", fmt.Errorf("could not extract username from commit page")
}
