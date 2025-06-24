package display

import (
	"fmt"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/gnomegl/gitslurp/internal/github"
	"github.com/gnomegl/gitslurp/internal/models"
	gh "github.com/google/go-github/v57/github"
)

// Context holds all the data needed for formatting and displaying results
type Context struct {
	Emails          map[string]*models.EmailDetails
	ShowDetails     bool
	CheckSecrets    bool
	LookupEmail     string
	KnownUsername   string
	User            *gh.User
	ShowTargetOnly  bool
	IsOrg           bool
	Cfg             *github.Config
	UserIdentifiers map[string]bool
	TargetNames     map[string]bool
}

// EmailEntry represents a single email with its details for sorting
type EmailEntry struct {
	Email   string
	Details *models.EmailDetails
}

// UserInfo shows basic user information
func UserInfo(user *gh.User, isOrg bool) {
	if user == nil {
		return
	}

	if isOrg {
		if user.GetName() != "" {
			fmt.Printf("Name: %s\n", user.GetName())
		}
	} else {
		if user.GetName() != "" {
			fmt.Printf("Name: %s\n", user.GetName())
		}
		if user.GetBio() != "" {
			fmt.Printf("Bio: %s\n", user.GetBio())
		}
	}
}

// Results shows all the collected information about emails and commits
func Results(emails map[string]*models.EmailDetails, showDetails bool, checkSecrets bool, 
	lookupEmail string, knownUsername string, user *gh.User, showTargetOnly bool, isOrg bool, cfg *github.Config) {
	
	sortedEmails := sortEmailsByCommitCount(emails)
	userIdentifiers := buildUserIdentifiers(knownUsername, lookupEmail, user)
	targetNames := extractTargetUserNames(emails, userIdentifiers)

	ctx := &Context{
		Emails:          emails,
		ShowDetails:     showDetails,
		CheckSecrets:    checkSecrets,
		LookupEmail:     lookupEmail,
		KnownUsername:   knownUsername,
		User:            user,
		ShowTargetOnly:  showTargetOnly,
		IsOrg:           isOrg,
		Cfg:             cfg,
		UserIdentifiers: userIdentifiers,
		TargetNames:     targetNames,
	}

	totalCommits := 0
	totalContributors := 0
	targetAccounts := make(map[string][]string)
	similarAccounts := make(map[string][]string)

	displayAccountInfo(user, isOrg)
	fmt.Println("\nCollected author information:")

	for _, entry := range sortedEmails {
		isTargetUser := isTargetUserEmail(entry.Email, entry.Details, userIdentifiers)
		totalContributors++

		if showTargetOnly && !isTargetUser {
			continue
		}

		names := extractNames(entry.Details)
		
		if isTargetUser {
			totalCommits += entry.Details.CommitCount
			targetAccounts[entry.Email] = names
			displayTargetUser(entry.Email, names, entry.Details.CommitCount)
		} else {
			displayNonTargetUser(entry.Email, names, entry.Details.CommitCount, targetNames, similarAccounts, userIdentifiers, showTargetOnly)
		}

		if ctx.ShowDetails || ctx.CheckSecrets || ctx.Cfg.ShowInteresting {
			displayCommitDetails(entry, isTargetUser, ctx)
		}
	}

	displayTotals(showTargetOnly, totalCommits, totalContributors)
	displaySummary(targetAccounts, similarAccounts)
}

// BuildUserIdentifiers creates a map of user identifiers for quick lookups
func BuildUserIdentifiers(username, lookupEmail string, user *gh.User) map[string]bool {
	identifiers := map[string]bool{
		username:    true,
		lookupEmail: true,
	}
	
	if user != nil {
		identifiers[user.GetLogin()] = true
		identifiers[user.GetName()] = true
		identifiers[user.GetEmail()] = true
	}
	
	return identifiers
}

// Helper functions

func sortEmailsByCommitCount(emails map[string]*models.EmailDetails) []EmailEntry {
	var sortedEmails []EmailEntry
	for email, details := range emails {
		sortedEmails = append(sortedEmails, EmailEntry{email, details})
	}

	sort.Slice(sortedEmails, func(i, j int) bool {
		return sortedEmails[i].Details.CommitCount > sortedEmails[j].Details.CommitCount
	})

	return sortedEmails
}

func buildUserIdentifiers(username, lookupEmail string, user *gh.User) map[string]bool {
	identifiers := map[string]bool{
		username:    true,
		lookupEmail: true,
	}
	
	if user != nil {
		identifiers[user.GetLogin()] = true
		identifiers[user.GetName()] = true
		identifiers[user.GetEmail()] = true
	}
	
	return identifiers
}

func extractTargetUserNames(emails map[string]*models.EmailDetails, userIdentifiers map[string]bool) map[string]bool {
	targetNames := make(map[string]bool)
	
	for email, details := range emails {
		isTargetUser := userIdentifiers[email]
		if !isTargetUser {
			for name := range details.Names {
				if userIdentifiers[name] {
					isTargetUser = true
					break
				}
			}
		}
		
		if isTargetUser {
			for name := range details.Names {
				// Split names by space and comma
				nameParts := strings.FieldsFunc(name, func(c rune) bool {
					return c == ' ' || c == ','
				})
				for _, part := range nameParts {
					part = strings.TrimSpace(part)
					if part != "" {
						targetNames[part] = true
					}
				}
			}
		}
	}
	
	return targetNames
}

func isTargetUserEmail(email string, details *models.EmailDetails, userIdentifiers map[string]bool) bool {
	if userIdentifiers[email] {
		return true
	}
	
	for name := range details.Names {
		if userIdentifiers[name] {
			return true
		}
	}
	
	return false
}

func extractNames(details *models.EmailDetails) []string {
	names := make([]string, 0, len(details.Names))
	for name := range details.Names {
		names = append(names, name)
	}
	return names
}

func hasMatchingTargetNames(names []string, targetNames map[string]bool) bool {
	for _, name := range names {
		nameParts := strings.FieldsFunc(name, func(c rune) bool {
			return c == ' ' || c == ','
		})
		for _, part := range nameParts {
			part = strings.TrimSpace(part)
			if targetNames[part] {
				return true
			}
		}
	}
	return false
}

func displayAccountInfo(user *gh.User, isOrg bool) {
	if user != nil {
		accountType := "User"
		if isOrg {
			accountType = "Organization"
		}
		fmt.Printf("\n%s Account Information:\n", accountType)
	}
}

func displayTargetUser(email string, names []string, commitCount int) {
	color.HiYellow("📍 %s (Target User)", email)
	color.HiGreen("  Names used: %s", strings.Join(names, ", "))
	color.HiGreen("  Total Commits: %d", commitCount)
}

func displayNonTargetUser(email string, names []string, commitCount int, targetNames map[string]bool, similarAccounts map[string][]string, userIdentifiers map[string]bool, showTargetOnly bool) {
	if !showTargetOnly && userIdentifiers[email] {
		color.HiYellow(email)
		color.HiWhite("  Names: %s", strings.Join(names, ", "))
		color.HiWhite("  Total Commits: %d", commitCount)
	} else if hasMatchingTargetNames(names, targetNames) {
		similarAccounts[email] = names
		color.Yellow("👁️  %s (Similar Account)", email)
		color.Magenta("  Names used: %s", strings.Join(names, ", "))
		color.Magenta("  Total Commits: %d", commitCount)
	} else {
		color.Yellow(email)
		color.White("  Names: %s", strings.Join(names, ", "))
		color.White("  Total Commits: %d", commitCount)
	}
}

func displayCommitDetails(entry EmailEntry, isTargetUser bool, ctx *Context) {
	for repoName, commits := range entry.Details.Commits {
		if !shouldShowRepo(commits, ctx) {
			continue
		}

		displayRepoHeader(repoName, isTargetUser, entry.Email, ctx)

		for i := range commits {
			commit := &commits[i]
			if !shouldShowCommit(*commit, ctx) {
				continue
			}

			isTargetCommit := isTargetUser || ctx.UserIdentifiers[commit.AuthorName] || ctx.UserIdentifiers[commit.AuthorEmail]
			
			if ctx.ShowDetails || len(commit.Secrets) > 0 {
				displayCommitInfo(*commit, isTargetCommit, entry.Email, ctx)
			}

			if len(commit.Secrets) > 0 {
				displaySecrets(commit.Secrets, isTargetCommit, entry.Email, ctx)
			}
		}
	}
}

func shouldShowRepo(commits []models.CommitInfo, ctx *Context) bool {
	if ctx.ShowDetails {
		return true
	}

	for _, commit := range commits {
		if (ctx.CheckSecrets && len(commit.Secrets) > 0) ||
			(ctx.Cfg.ShowInteresting && len(commit.Secrets) > 0) {
			return true
		}
	}
	
	return false
}

func shouldShowCommit(commit models.CommitInfo, ctx *Context) bool {
	return ctx.ShowDetails ||
		(ctx.CheckSecrets && len(commit.Secrets) > 0) ||
		(ctx.Cfg.ShowInteresting && len(commit.Secrets) > 0)
}

func displayRepoHeader(repoName string, isTargetUser bool, email string, ctx *Context) {
	if isTargetUser {
		color.HiGreen("  📂 Repo: %s", repoName)
	} else if !ctx.ShowTargetOnly && ctx.UserIdentifiers[email] {
		color.HiWhite("  📂 Repo: %s", repoName)
	} else {
		color.Green("  Repo: %s", repoName)
	}
}

func displayCommitInfo(commit models.CommitInfo, isTargetCommit bool, email string, ctx *Context) {
	if isTargetCommit {
		displayTargetCommitInfo(commit)
	} else if !ctx.ShowTargetOnly && ctx.UserIdentifiers[email] {
		displayUserCommitInfo(commit)
	} else {
		displayRegularCommitInfo(commit)
	}

	if commit.IsOwnRepo {
		color.Cyan("    Owner: true")
	}
	if commit.IsFork {
		color.Cyan("    Fork: true")
	}
}

func displayTargetCommitInfo(commit models.CommitInfo) {
	if commit.AuthorName == "" {
		color.HiMagenta("    ⚔️ Commit: %s", commit.Hash)
		color.HiBlue("    🔗 URL: %s", commit.URL)
		color.HiWhite("    👻 Author: anonymous")
	} else {
		color.HiMagenta("    ⭐ Commit: %s", commit.Hash)
		color.HiBlue("    🔗 URL: %s", commit.URL)
		color.HiWhite("    👤 Author: %s <%s>", commit.AuthorName, commit.AuthorEmail)
	}
}

func displayUserCommitInfo(commit models.CommitInfo) {
	if commit.AuthorName == "" {
		color.Magenta("    ⚔️ Commit: %s", commit.Hash)
		color.Blue("    🔗 URL: %s", commit.URL)
		color.White("    👻 Author: anonymous")
	} else {
		color.Magenta("    ⭐ Commit: %s", commit.Hash)
		color.Blue("    🔗 URL: %s", commit.URL)
		color.White("    👤 Author: %s <%s>", commit.AuthorName, commit.AuthorEmail)
	}
}

func displayRegularCommitInfo(commit models.CommitInfo) {
	if commit.AuthorName == "" {
		color.Magenta("    ⚔️ Commit: %s", commit.Hash)
	} else {
		color.Magenta("    Commit: %s", commit.Hash)
	}
	color.Blue("    URL: %s", commit.URL)
	if commit.AuthorName == "" {
		color.White("    👻 Author: anonymous")
	} else {
		color.White("    Author: %s <%s>", commit.AuthorName, commit.AuthorEmail)
	}
}

func displaySecrets(secrets []string, isTargetCommit bool, email string, ctx *Context) {
	var foundSecrets, foundPatterns bool
	
	for _, secret := range secrets {
		if strings.HasPrefix(secret, "⭐") {
			if !foundPatterns && ctx.Cfg.ShowInteresting {
				displayPatternHeader(isTargetCommit, email, ctx)
				foundPatterns = true
			}
			if ctx.Cfg.ShowInteresting {
				displayPattern(secret, isTargetCommit)
			}
		} else {
			if !foundSecrets && ctx.CheckSecrets {
				displaySecretHeader(isTargetCommit, email, ctx)
				foundSecrets = true
			}
			if ctx.CheckSecrets {
				displaySecret(secret, isTargetCommit)
			}
		}
	}
}

func displayPatternHeader(isTargetCommit bool, email string, ctx *Context) {
	if isTargetCommit {
		color.HiYellow("    ⭐ Found patterns:")
	} else if !ctx.ShowTargetOnly && ctx.UserIdentifiers[email] {
		color.Yellow("    ⭐ Found patterns:")
	} else {
		color.Yellow("    ⭐ Found patterns:")
	}
}

func displayPattern(pattern string, isTargetCommit bool) {
	if isTargetCommit {
		color.HiYellow("      %s", pattern)
	} else {
		color.Yellow("      %s", pattern)
	}
}

func displaySecretHeader(isTargetCommit bool, email string, ctx *Context) {
	if isTargetCommit {
		color.HiRed("    🐽 Found secrets:")
	} else if !ctx.ShowTargetOnly && ctx.UserIdentifiers[email] {
		color.Red("    🐽 Found secrets:")
	} else {
		color.Red("    🐽 Found secrets:")
	}
}

func displaySecret(secret string, isTargetCommit bool) {
	if isTargetCommit {
		color.HiRed("      - %s", secret)
	} else {
		color.Red("      - %s", secret)
	}
}

func displayTotals(showTargetOnly bool, totalCommits, totalContributors int) {
	if showTargetOnly {
		color.HiCyan("\nTotal commits by target user: %d", totalCommits)
	} else {
		color.HiCyan("\nTotal contributors: %d", totalContributors)
	}
}

func displaySummary(targetAccounts, similarAccounts map[string][]string) {
	if len(targetAccounts) == 0 && len(similarAccounts) == 0 {
		return
	}

	fmt.Println("\n" + strings.Repeat("─", 60))
	color.HiCyan("SUMMARY")
	fmt.Println(strings.Repeat("─", 60))
	
	if len(targetAccounts) > 0 {
		color.HiGreen("\n📍 Target User Accounts:")
		for email, names := range targetAccounts {
			color.Green("  • %s", email)
			if len(names) > 0 {
				color.Green("    Names: %s", strings.Join(names, ", "))
			}
		}
	}
	
	if len(similarAccounts) > 0 {
		color.HiMagenta("\n👁️  Similar Accounts:")
		for email, names := range similarAccounts {
			color.Magenta("  • %s", email)
			if len(names) > 0 {
				color.Magenta("    Names: %s", strings.Join(names, ", "))
			}
		}
	}
	fmt.Println()
}

