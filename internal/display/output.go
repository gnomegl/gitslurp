package display

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/gnomegl/gitslurp/internal/github"
	"github.com/gnomegl/gitslurp/internal/models"
	"github.com/gnomegl/gitslurp/internal/utils"
	gh "github.com/google/go-github/v57/github"
	"golang.org/x/term"
)

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
	OrgDomain       string
}

type StreamUpdate struct {
	Email    string
	Details  *models.EmailDetails
	RepoName string
}

// EmailEntry represents a single email with its details for sorting
type EmailEntry struct {
	Email   string
	Details *models.EmailDetails
}

// groups display-related configuration
type DisplayOptions struct {
	ShowDetails     bool
	CheckSecrets    bool
	ShowInteresting bool
	ShowTargetOnly  bool
}

// handles user identification logic
type UserMatcher struct {
	identifiers map[string]bool
	targetNames map[string]bool
}

// creates a new user matcher
func NewUserMatcher(username, lookupEmail string, user *gh.User) *UserMatcher {
	identifiers := buildUserIdentifiers(username, lookupEmail, user)
	return &UserMatcher{
		identifiers: identifiers,
		targetNames: make(map[string]bool),
	}
}

// checks if an email belongs to the target user
func (m *UserMatcher) IsTargetUser(email string, details *models.EmailDetails) bool {
	if m.identifiers[email] {
		return true
	}

	for name := range details.Names {
		if m.identifiers[name] {
			return true
		}
	}

	return false
}

// checks if names match target names
func (m *UserMatcher) HasMatchingNames(names []string) bool {
	for _, name := range names {
		nameParts := strings.FieldsFunc(name, func(c rune) bool {
			return c == ' ' || c == ','
		})
		for _, part := range nameParts {
			part = strings.TrimSpace(part)
			if m.targetNames[part] {
				return true
			}
		}
	}
	return false
}

// handles colored output with different styles
type ColorPrinter struct {
	isTarget bool
}

func (cp *ColorPrinter) PrintEmail(email string, names []string, commitCount int, isTarget bool, isOrgEmployee bool) {
	termInfo := getTerminalInfo()
	maxLen := termInfo.maxDisplay - 25

	nameStr := strings.Join(names, ", ")
	if len(nameStr) > maxLen {
		nameStr = truncateString(nameStr, maxLen)
	}
}


// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type terminalInfo struct {
	width      int
	maxDisplay int
	graphWidth int
}

func getTerminalInfo() *terminalInfo {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 80
	}

	return &terminalInfo{
		width:      width,
		maxDisplay: min(width-4, 120),
		graphWidth: min(width-20, 50),
	}
}

func formatField(label, value string, width int) string {
	if value == "" {
		return ""
	}
	maxValueWidth := width - len(label) - 3
	if len(value) > maxValueWidth && maxValueWidth > 10 {
		value = value[:maxValueWidth-3] + "..."
	}
	return fmt.Sprintf("%-*s %s", len(label)+1, label+":", value)
}

func truncateString(s string, maxLen int) string {
	if maxLen < 10 {
		maxLen = 10
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func getSeparator(termInfo *terminalInfo, maxWidth int) string {
	width := termInfo.maxDisplay
	if maxWidth > 0 && maxWidth < width {
		width = maxWidth
	}
	return strings.Repeat("-", width)
}

func UserInfo(user *gh.User, isOrg bool) {
	if user == nil {
		return
	}

	termInfo := getTerminalInfo()
	useTwoColumns := termInfo.width >= 100
	colWidth := (termInfo.maxDisplay - 4) / 2

	fmt.Println()
	if isOrg {
		fmt.Print("🏢 ")
		color.HiCyan("ORGANIZATION PROFILE")
	} else {
		fmt.Print("👤 ")
		color.HiCyan("USER PROFILE")
	}
	fmt.Println()

	// Basic info section
	color.HiWhite("Username: ")
	color.HiGreen("%s\n", user.GetLogin())

	// Collect all profile fields for display
	profileFields := []struct {
		label string
		value string
		color func(format string, a ...interface{})
		icon  string
	}{
		{"Name", user.GetName(), color.HiGreen, ""},
		{"Email", user.GetEmail(), color.HiGreen, ""},
		{"Organization", user.GetCompany(), color.HiYellow, "🏢"},
		{"Location", user.GetLocation(), color.HiMagenta, "📍"},
		{"Website", user.GetBlog(), color.HiBlue, "🔗"},
		{"Twitter", user.GetTwitterUsername(), color.HiBlue, "🐦"},
	}

	// Bio gets special treatment - always full width
	if user.GetBio() != "" {
		color.HiWhite("Bio: ")
		color.HiCyan("%s\n", user.GetBio())
		fmt.Println()
	}

	// Display profile fields in columns
	var leftCol, rightCol []string
	fieldCount := 0

	for _, field := range profileFields {
		if field.value == "" {
			continue
		}

		var displayValue string
		if field.icon != "" {
			if field.label == "Twitter" {
				displayValue = fmt.Sprintf("%s @%s", field.icon, field.value)
			} else {
				displayValue = fmt.Sprintf("%s %s", field.icon, field.value)
			}
		} else {
			displayValue = field.value
		}

		fieldStr := formatField(field.label, displayValue, colWidth)

		if useTwoColumns && fieldCount%2 == 0 {
			leftCol = append(leftCol, fieldStr)
		} else {
			rightCol = append(rightCol, fieldStr)
		}
		fieldCount++
	}

	// Print columns
	if useTwoColumns {
		maxRows := len(leftCol)
		if len(rightCol) > maxRows {
			maxRows = len(rightCol)
		}

		for i := 0; i < maxRows; i++ {
			left := ""
			right := ""

			if i < len(leftCol) {
				left = leftCol[i]
			}
			if i < len(rightCol) {
				right = rightCol[i]
			}

			if left != "" && right != "" {
				fmt.Printf("%-*s    %s\n", colWidth, left, right)
			} else if left != "" {
				fmt.Printf("%s\n", left)
			} else if right != "" {
				fmt.Printf("%-*s    %s\n", colWidth, "", right)
			}
		}
	} else {
		// Single column display
		allFields := append(leftCol, rightCol...)
		for _, field := range allFields {
			fmt.Printf("%s\n", field)
		}
	}

	fmt.Println()
	color.HiWhite("📅 Account Statistics\n")

	// Statistics section
	statsFields := []struct {
		label string
		value interface{}
		color func(format string, a ...interface{})
	}{}

	if isOrg {
		if user.GetPublicRepos() > 0 {
			statsFields = append(statsFields, struct {
				label string
				value interface{}
				color func(format string, a ...interface{})
			}{"Repositories", user.GetPublicRepos(), color.HiGreen})
		}
	} else {
		statsFields = []struct {
			label string
			value interface{}
			color func(format string, a ...interface{})
		}{
			{"Repositories", user.GetPublicRepos(), color.HiGreen},
			{"Gists", user.GetPublicGists(), color.HiGreen},
			{"Followers", user.GetFollowers(), color.HiCyan},
			{"Following", user.GetFollowing(), color.HiCyan},
		}
	}

	// Display stats in columns
	var leftStats, rightStats []string
	for i, stat := range statsFields {
		statStr := fmt.Sprintf("%s: %v", stat.label, stat.value)

		if useTwoColumns && i%2 == 0 {
			leftStats = append(leftStats, statStr)
		} else {
			rightStats = append(rightStats, statStr)
		}
	}

	// Print stats
	if useTwoColumns {
		maxRows := len(leftStats)
		if len(rightStats) > maxRows {
			maxRows = len(rightStats)
		}

		for i := 0; i < maxRows; i++ {
			left := ""
			right := ""

			if i < len(leftStats) {
				left = leftStats[i]
			}
			if i < len(rightStats) {
				right = rightStats[i]
			}

			if left != "" && right != "" {
				fmt.Printf("%-*s    %s\n", colWidth, left, right)
			} else if left != "" {
				fmt.Printf("%s\n", left)
			} else if right != "" {
				fmt.Printf("%-*s    %s\n", colWidth, "", right)
			}
		}
	} else {
		allStats := append(leftStats, rightStats...)
		for _, stat := range allStats {
			fmt.Printf("%s\n", stat)
		}
	}

	// Dates
	fmt.Println()
	if !user.GetCreatedAt().Time.IsZero() {
		color.HiWhite("Created: ")
		color.HiGreen("%s\n", user.GetCreatedAt().Time.Format("January 2, 2006"))
	}

	if !user.GetUpdatedAt().Time.IsZero() {
		color.HiWhite("Last Updated: ")
		color.HiYellow("%s\n", user.GetUpdatedAt().Time.Format("January 2, 2006"))
	}

	fmt.Println()
}

func Results(emails map[string]*models.EmailDetails, showDetails bool, checkSecrets bool,
	lookupEmail string, knownUsername string, user *gh.User, showTargetOnly bool, isOrg bool, cfg *github.Config, outputFormat string) {

	matcher := NewUserMatcher(knownUsername, lookupEmail, user)
	matcher.targetNames = extractTargetUserNames(emails, matcher.identifiers)

	orgDomain := ""
	if isOrg && user != nil {
		orgDomain = extractDomainFromWebsite(user.GetBlog())
	}

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
		UserIdentifiers: matcher.identifiers,
		TargetNames:     matcher.targetNames,
		OrgDomain:       orgDomain,
	}

	switch outputFormat {
	case "json":
		outputJSON(ctx, matcher)
	case "csv":
		outputCSV(ctx, matcher)
	default:
		result := processEmails(ctx, matcher)
		displayResults(ctx, result)
	}
}

func StreamResults(streamChan <-chan StreamUpdate, showDetails bool, checkSecrets bool,
	lookupEmail string, knownUsername string, user *gh.User, showTargetOnly bool, isOrg bool, cfg *github.Config) {

	matcher := NewUserMatcher(knownUsername, lookupEmail, user)

	orgDomain := ""
	if isOrg && user != nil {
		orgDomain = extractDomainFromWebsite(user.GetBlog())
	}

	fmt.Println()

	seenEmails := make(map[string]bool)
	printer := &ColorPrinter{}

	for update := range streamChan {
		if seenEmails[update.Email] {
			continue
		}
		seenEmails[update.Email] = true

		isTargetUser := matcher.IsTargetUser(update.Email, update.Details)
		isOrgEmployee := isOrg && isOrganizationEmail(update.Email, orgDomain)
		if showTargetOnly && !isTargetUser {
			continue
		}

		names := extractNames(update.Details)
		printer.PrintEmail(update.Email, names, update.Details.CommitCount, isTargetUser, isOrgEmployee)
		fmt.Println()
	}
}

// EmailProcessResult holds the results of email processing
type EmailProcessResult struct {
	totalCommits      int
	totalContributors int
	targetAccounts    map[string][]string
	similarAccounts   map[string][]string
	orgMembers        map[string][]string
	similarOrgMembers map[string][]string
}

// processEmails processes all emails and returns aggregated results
func processEmails(ctx *Context, matcher *UserMatcher) *EmailProcessResult {
	sortedEmails := sortEmailsByCommitCount(ctx.Emails)
	result := &EmailProcessResult{
		targetAccounts:    make(map[string][]string),
		similarAccounts:   make(map[string][]string),
		orgMembers:        make(map[string][]string),
		similarOrgMembers: make(map[string][]string),
	}

	printer := &ColorPrinter{}
	opts := &DisplayOptions{
		ShowDetails:     ctx.ShowDetails,
		CheckSecrets:    ctx.CheckSecrets,
		ShowInteresting: ctx.Cfg.ShowInteresting,
		ShowTargetOnly:  ctx.ShowTargetOnly,
	}

	for _, entry := range sortedEmails {
		isTargetUser := matcher.IsTargetUser(entry.Email, entry.Details)
		isOrgEmployee := ctx.IsOrg && isOrganizationEmail(entry.Email, ctx.OrgDomain)
		result.totalContributors++

		if opts.ShowTargetOnly && !isTargetUser {
			continue
		}

		names := extractNames(entry.Details)
		hasSimilarNames := matcher.HasMatchingNames(names)

		if isTargetUser {
			result.totalCommits += entry.Details.CommitCount
			result.targetAccounts[entry.Email] = names
			printer.PrintEmail(entry.Email, names, entry.Details.CommitCount, true, false)
		} else if isOrgEmployee {
			if hasSimilarNames {
				result.similarOrgMembers[entry.Email] = names
			} else {
				result.orgMembers[entry.Email] = names
			}
		} else if hasSimilarNames {
			result.similarAccounts[entry.Email] = names
		} else if !opts.ShowTargetOnly {
			printer.PrintEmail(entry.Email, names, entry.Details.CommitCount, false, false)
		}

		if shouldShowCommitDetails(opts) {
			displayCommitDetails(entry, isTargetUser, ctx)
		}
	}

	return result
}

// shouldShowCommitDetails checks if commit details should be displayed
func shouldShowCommitDetails(opts *DisplayOptions) bool {
	return opts.ShowDetails || opts.CheckSecrets || opts.ShowInteresting
}

// displayResults shows the final results
func displayResults(ctx *Context, result *EmailProcessResult) {
	displayRepositoryStats(ctx.Emails, ctx.UserIdentifiers)

	if ctx.Cfg.TimestampAnalysis {
		displayTimestampAnalysis(ctx.Emails, ctx.UserIdentifiers)
	}

	displaySummary(result.targetAccounts, result.similarAccounts, result.orgMembers, result.similarOrgMembers, ctx.IsOrg, ctx.OrgDomain)
}

// sortEmailsByCommitCount sorts emails by commit count in descending order
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

// buildUserIdentifiers creates a map of user identifiers for quick lookups
func buildUserIdentifiers(username, lookupEmail string, user *gh.User) map[string]bool {
	identifiers := make(map[string]bool)

	if username != "" {
		identifiers[username] = true
	}
	if lookupEmail != "" {
		identifiers[lookupEmail] = true
	}

	if user != nil {
		if login := user.GetLogin(); login != "" {
			identifiers[login] = true
		}
		if name := user.GetName(); name != "" {
			identifiers[name] = true
		}
		if email := user.GetEmail(); email != "" {
			identifiers[email] = true
		}
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

func extractNames(details *models.EmailDetails) []string {
	names := make([]string, 0, len(details.Names))
	for name := range details.Names {
		names = append(names, name)
	}
	return names
}

func extractDomainFromWebsite(website string) string {
	if website == "" {
		return ""
	}

	if !strings.HasPrefix(website, "http://") && !strings.HasPrefix(website, "https://") {
		website = "https://" + website
	}

	parsedURL, err := url.Parse(website)
	if err != nil {
		return ""
	}

	domain := parsedURL.Hostname()
	domain = strings.TrimPrefix(domain, "www.")

	return domain
}

func extractBaseDomain(domain string) string {
	domain = strings.ToLower(domain)

	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return domain
	}

	twoLevelTLDs := map[string]bool{
		"co.uk": true, "co.jp": true, "co.nz": true, "co.za": true,
		"com.au": true, "com.br": true, "com.cn": true, "com.mx": true,
		"ac.uk": true, "gov.uk": true, "org.uk": true,
	}

	if len(parts) >= 3 {
		lastTwo := parts[len(parts)-2] + "." + parts[len(parts)-1]
		if twoLevelTLDs[lastTwo] {
			if len(parts) >= 3 {
				return parts[len(parts)-3]
			}
		}
	}

	return parts[len(parts)-2]
}

func isOrganizationEmail(email, orgDomain string) bool {
	if orgDomain == "" || email == "" {
		return false
	}

	if !strings.Contains(email, "@") {
		return false
	}

	emailDomain := strings.Split(email, "@")[1]
	emailDomain = strings.ToLower(emailDomain)
	orgDomain = strings.ToLower(orgDomain)

	if emailDomain == orgDomain {
		return true
	}

	emailBase := extractBaseDomain(emailDomain)
	orgBase := extractBaseDomain(orgDomain)

	return emailBase == orgBase && emailBase != "" && orgBase != ""
}

// CommitDisplayer handles commit display logic
type CommitDisplayer struct {
	ctx *Context
}

// NewCommitDisplayer creates a new commit displayer
func NewCommitDisplayer(ctx *Context) *CommitDisplayer {
	return &CommitDisplayer{ctx: ctx}
}

// DisplayForEntry displays commits for an email entry
func (cd *CommitDisplayer) DisplayForEntry(entry EmailEntry, isTargetUser bool) {
	for repoName, commits := range entry.Details.Commits {
		if !cd.shouldShowRepo(commits) {
			continue
		}

		cd.displayRepoHeader(repoName, isTargetUser, entry.Email)

		for i := range commits {
			commit := &commits[i]
			if !cd.shouldShowCommit(*commit) {
				continue
			}

			isTargetCommit := isTargetUser || cd.ctx.UserIdentifiers[commit.AuthorName] || cd.ctx.UserIdentifiers[commit.AuthorEmail]

			if cd.ctx.ShowDetails || len(commit.Secrets) > 0 {
				cd.displayCommitInfo(*commit, isTargetCommit, entry.Email)
			}

			if len(commit.Secrets) > 0 {
				cd.displaySecrets(commit.Secrets, isTargetCommit, entry.Email)
			}
		}
	}
}

// displayCommitDetails is a wrapper for backward compatibility
func displayCommitDetails(entry EmailEntry, isTargetUser bool, ctx *Context) {
	displayer := NewCommitDisplayer(ctx)
	displayer.DisplayForEntry(entry, isTargetUser)
}

// shouldShowRepo checks if a repository should be displayed
func (cd *CommitDisplayer) shouldShowRepo(commits []models.CommitInfo) bool {
	if cd.ctx.ShowDetails {
		return true
	}

	for _, commit := range commits {
		if len(commit.Secrets) > 0 && (cd.ctx.CheckSecrets || cd.ctx.Cfg.ShowInteresting) {
			return true
		}
	}

	return false
}

// shouldShowCommit checks if a commit should be displayed
func (cd *CommitDisplayer) shouldShowCommit(commit models.CommitInfo) bool {
	return cd.ctx.ShowDetails ||
		(len(commit.Secrets) > 0 && (cd.ctx.CheckSecrets || cd.ctx.Cfg.ShowInteresting))
}

func (cd *CommitDisplayer) displayRepoHeader(repoName string, isTargetUser bool, email string) {
	termInfo := getTerminalInfo()
	maxLen := termInfo.maxDisplay - 15

	truncatedRepo := truncateString(repoName, maxLen)
	if isTargetUser {
		color.HiGreen("  📂 Repo: %s", truncatedRepo)
	} else if !cd.ctx.ShowTargetOnly && cd.ctx.UserIdentifiers[email] {
		color.HiWhite("  📂 Repo: %s", truncatedRepo)
	} else {
		color.Green("  Repo: %s", truncatedRepo)
	}
}

// displayCommitInfo shows commit information with appropriate coloring
func (cd *CommitDisplayer) displayCommitInfo(commit models.CommitInfo, isTargetCommit bool, email string) {
	commitIcon := "    Commit:"
	authorIcon := "    Author:"

	if commit.AuthorName == "" {
		commitIcon = "    ⚔️ Commit:"
		authorIcon = "    👻 Author:"
	} else if isTargetCommit {
		commitIcon = "    ⭐ Commit:"
		authorIcon = "    👤 Author:"
	}

	if isTargetCommit {
		cd.printCommitHighlight(commit, commitIcon, authorIcon)
	} else if !cd.ctx.ShowTargetOnly && cd.ctx.UserIdentifiers[email] {
		cd.printCommitMedium(commit, commitIcon, authorIcon)
	} else {
		cd.printCommitRegular(commit, commitIcon, authorIcon)
	}

	if commit.IsOwnRepo {
		color.Cyan("    Owner: true")
	}
	if commit.IsFork {
		color.Cyan("    Fork: true")
	}
	if commit.IsExternal {
		color.HiYellow("    🌍 External: true")
	}
}

func (cd *CommitDisplayer) printCommitHighlight(commit models.CommitInfo, commitIcon, authorIcon string) {
	termInfo := getTerminalInfo()
	urlMaxLen := termInfo.maxDisplay - 15

	color.HiMagenta("%s %s", commitIcon, commit.Hash)
	color.HiBlue("    🔗 URL: %s", truncateString(commit.URL, urlMaxLen))
	if commit.AuthorName == "" {
		color.HiWhite("%s anonymous", authorIcon)
	} else {
		color.HiWhite("%s %s <%s>", authorIcon, commit.AuthorName, commit.AuthorEmail)
	}
}

func (cd *CommitDisplayer) printCommitMedium(commit models.CommitInfo, commitIcon, authorIcon string) {
	termInfo := getTerminalInfo()
	urlMaxLen := termInfo.maxDisplay - 15

	color.Magenta("%s %s", commitIcon, commit.Hash)
	color.Blue("    🔗 URL: %s", truncateString(commit.URL, urlMaxLen))
	if commit.AuthorName == "" {
		color.White("%s anonymous", authorIcon)
	} else {
		color.White("%s %s <%s>", authorIcon, commit.AuthorName, commit.AuthorEmail)
	}
}

func (cd *CommitDisplayer) printCommitRegular(commit models.CommitInfo, commitIcon, authorIcon string) {
	termInfo := getTerminalInfo()
	urlMaxLen := termInfo.maxDisplay - 15

	color.Magenta("%s %s", commitIcon, commit.Hash)
	color.Blue("    URL: %s", truncateString(commit.URL, urlMaxLen))
	if commit.AuthorName == "" {
		color.White("%s anonymous", authorIcon)
	} else {
		color.White("%s %s <%s>", authorIcon, commit.AuthorName, commit.AuthorEmail)
	}
}

// SecretDisplayer handles secret and pattern display
type SecretDisplayer struct {
	secretsShown  map[string]bool
	patternsShown map[string]bool
}

// NewSecretDisplayer creates a new secret displayer
func NewSecretDisplayer() *SecretDisplayer {
	return &SecretDisplayer{
		secretsShown:  make(map[string]bool),
		patternsShown: make(map[string]bool),
	}
}

// displaySecrets shows found secrets and patterns
func (cd *CommitDisplayer) displaySecrets(secrets []string, isTargetCommit bool, email string) {
	sd := NewSecretDisplayer()

	for _, secret := range secrets {
		if strings.HasPrefix(secret, "⭐") {
			if cd.ctx.Cfg.ShowInteresting {
				sd.displayPattern(secret, isTargetCommit, email, cd.ctx)
			}
		} else {
			if cd.ctx.CheckSecrets {
				sd.displaySecret(secret, isTargetCommit, email, cd.ctx)
			}
		}
	}
}

// displayPattern shows a found pattern
func (sd *SecretDisplayer) displayPattern(pattern string, isTargetCommit bool, email string, ctx *Context) {
	key := fmt.Sprintf("%s-%t", email, isTargetCommit)
	if !sd.patternsShown[key] {
		if isTargetCommit {
			color.HiYellow("    ⭐ Found patterns:")
		} else {
			color.Yellow("    ⭐ Found patterns:")
		}
		sd.patternsShown[key] = true
	}

	if isTargetCommit {
		color.HiYellow("      - %s", pattern)
	} else {
		color.Yellow("      - %s", pattern)
	}
}

// displaySecret shows a found secret
func (sd *SecretDisplayer) displaySecret(secret string, isTargetCommit bool, email string, ctx *Context) {
	key := fmt.Sprintf("%s-%t", email, isTargetCommit)
	if !sd.secretsShown[key] {
		if isTargetCommit {
			color.HiRed("    🐽 Found secrets:")
		} else {
			color.Red("    🐽 Found secrets:")
		}
		sd.secretsShown[key] = true
	}

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

func displayRepositoryStats(emails map[string]*models.EmailDetails, userIdentifiers map[string]bool) {
	ownRepos := make(map[string]bool)
	externalRepos := make(map[string]bool)
	var externalCommits, ownCommits int

	externalEmailData := make(map[string]map[string]int)
	externalReposList := make(map[string]bool)

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
			for repo, commits := range details.Commits {
				for _, commit := range commits {
					if commit.IsExternal {
						externalRepos[repo] = true
						externalCommits++

						if externalEmailData[email] == nil {
							externalEmailData[email] = make(map[string]int)
						}
						externalEmailData[email][repo]++
						externalReposList[repo] = true
					} else {
						ownRepos[repo] = true
						ownCommits++
					}
				}
			}
		}
	}

	if len(externalRepos) > 0 {
		fmt.Println()
		fmt.Println()
		color.HiCyan("🌍 EXTERNAL CONTRIBUTIONS")
		fmt.Println(getSeparator(getTerminalInfo(), 60))
		color.Green("• %d external repositories contributed to", len(externalRepos))
		color.Green("• %d commits to external projects", externalCommits)
		if ownCommits > 0 {
			color.Blue("• %d commits to own repositories", ownCommits)
			percentage := float64(externalCommits) / float64(externalCommits+ownCommits) * 100
			color.Yellow("• %.1f%% of commits are external contributions", percentage)
		}

		fmt.Println()
		color.HiWhite("📧 Email addresses used in external contributions (%d unique):", len(externalEmailData))

		sortedEmails := make([]string, 0, len(externalEmailData))
		for email := range externalEmailData {
			sortedEmails = append(sortedEmails, email)
		}
		sort.Strings(sortedEmails)

		for _, email := range sortedEmails {
			repoMap := externalEmailData[email]
			emailDetails := emails[email]
			names := extractNames(emailDetails)
			color.Yellow("• %s", email)
			if len(names) > 0 {
				color.White("  Names: %s", strings.Join(names, ", "))
			}

			var repoNames []string
			var totalCommits int
			for repo, count := range repoMap {
				repoNames = append(repoNames, repo)
				totalCommits += count
			}
			sort.Strings(repoNames)

			color.Cyan("  Repositories (%d commits total):", totalCommits)
			for _, repo := range repoNames {
				commitCount := repoMap[repo]
				color.White("    - %s (%d commits)", repo, commitCount)
			}
			fmt.Println()
		}
	}
}

func displaySummary(targetAccounts, similarAccounts, orgMembers, similarOrgMembers map[string][]string, isOrg bool, orgDomain string) {
	if len(targetAccounts) == 0 && len(similarAccounts) == 0 && len(orgMembers) == 0 && len(similarOrgMembers) == 0 {
		return
	}

	termInfo := getTerminalInfo()
	color.HiCyan("\nSUMMARY")
	fmt.Println(getSeparator(termInfo, 60))

	if len(targetAccounts) > 0 {
		fmt.Println("\n📍  Target User Accounts:")
		for email, names := range targetAccounts {
			color.Yellow("• %s", email)
			if len(names) > 0 {
				color.Green("  Names: %s", strings.Join(names, ", "))
			}
		}
	}

	if len(similarAccounts) > 0 {
		fmt.Println("\n👁️  Similar Accounts:")
		for email, names := range similarAccounts {
			color.Yellow("• %s", email)
			if len(names) > 0 {
				color.Magenta("  Names: %s", strings.Join(names, ", "))
			}
		}
	}

	if isOrg && (len(orgMembers) > 0 || len(similarOrgMembers) > 0) {
		if orgDomain != "" {
			fmt.Printf("\n📌  Organization Members (@%s):\n", orgDomain)
		} else {
			fmt.Println("\n📌  Organization Members:")
		}

		if len(similarOrgMembers) > 0 {
			fmt.Println("\n👁️  Similar to Target (Possible Alternate Accounts):")
			for email, names := range similarOrgMembers {
				color.HiYellow("• %s", email)
				if len(names) > 0 {
					color.HiMagenta("  Names: %s", strings.Join(names, ", "))
				}
			}
		}

		if len(orgMembers) > 0 {
			if len(similarOrgMembers) > 0 {
				fmt.Println("\nOther Members:")
			}
			for email, names := range orgMembers {
				color.Yellow("• %s", email)
				if len(names) > 0 {
					color.Green("  Names: %s", strings.Join(names, ", "))
				}
			}
		}
	}

	fmt.Println()
}

// displayTimestampAnalysis shows timestamp analysis results
func displayTimestampAnalysis(emails map[string]*models.EmailDetails, userIdentifiers map[string]bool) {
	termInfo := getTerminalInfo()
	color.HiCyan("\n🕐 TIMESTAMP ANALYSIS")
	fmt.Println(getSeparator(termInfo, 60))

	targetCommits := make(map[string][]models.CommitInfo)

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
			for _, commits := range details.Commits {
				targetCommits[email] = append(targetCommits[email], commits...)
			}
		}
	}

	if len(targetCommits) == 0 {
		color.Yellow("No target user commits found for analysis.")
		return
	}

	var allTargetCommits []models.CommitInfo
	for _, commits := range targetCommits {
		allTargetCommits = append(allTargetCommits, commits...)
	}

	patterns := utils.GetTimestampPatterns(allTargetCommits)

	fmt.Printf("\n📊 Target User Commit Patterns (%d commits):\n", patterns["total_commits"])
	displayGeneralPatterns(patterns)

	// Display aggregated hourly graph for all target commits
	if len(allTargetCommits) >= 10 {
		fmt.Println()
		displayAggregatedHourlyGraph(patterns)
	}

	fmt.Println("\n📍 Individual Target User Analysis:")
	for email, commits := range targetCommits {
		if len(commits) >= 3 {
			displayUserTimestampAnalysis(email, commits)
		}
	}

	displaySuspiciousPatterns(allTargetCommits)
}

func displayGeneralPatterns(patterns map[string]interface{}) {
	if unusualPct, ok := patterns["unusual_hour_percentage"].(float64); ok && unusualPct > 0 {
		color.Yellow("• %.1f%% commits during unusual hours (10pm-6am local time)", unusualPct)
	}

	if weekendPct, ok := patterns["weekend_percentage"].(float64); ok && weekendPct > 0 {
		color.Cyan("• %.1f%% commits on weekends", weekendPct)
	}

	if nightOwlPct, ok := patterns["night_owl_percentage"].(float64); ok && nightOwlPct > 10 {
		color.Magenta("• %.1f%% night owl commits (10pm-2am local time)", nightOwlPct)
	}

	if earlyBirdPct, ok := patterns["early_bird_percentage"].(float64); ok && earlyBirdPct > 10 {
		color.Green("• %.1f%% early bird commits (5am-7am local time)", earlyBirdPct)
	}

	if mostActiveHour, ok := patterns["most_active_hour"].(int); ok {
		color.Blue("• Most active hour: %02d:00 local time", mostActiveHour)
	}

	if mostActiveDay, ok := patterns["most_active_day"].(time.Weekday); ok {
		color.Blue("• Most active day: %s", mostActiveDay.String())
	}

	if mostActiveTZ, ok := patterns["most_active_timezone"].(string); ok && mostActiveTZ != "" {
		color.HiBlue("• Most common timezone: %s", mostActiveTZ)
	}

	if tzDist, ok := patterns["timezone_distribution"].(map[string]int); ok && len(tzDist) > 1 {
		color.HiYellow("• Multiple timezones detected: %d different zones", len(tzDist))
		displayTimezoneDistribution(tzDist)
	}
}

func displayTimezoneDistribution(tzDist map[string]int) {
	type tzEntry struct {
		zone  string
		count int
	}

	var zones []tzEntry
	for tz, count := range tzDist {
		zones = append(zones, tzEntry{tz, count})
	}

	sort.Slice(zones, func(i, j int) bool {
		return zones[i].count > zones[j].count
	})

	for i, zone := range zones {
		if i >= 3 {
			break
		}
		color.White("  - %s: %d commits", zone.zone, zone.count)
	}
}

func displayUserTimestampAnalysis(email string, commits []models.CommitInfo) {
	patterns := utils.GetTimestampPatterns(commits)

	color.HiWhite("%s (%d commits):", email, len(commits))

	if mostActiveTZ, ok := patterns["most_active_timezone"].(string); ok && mostActiveTZ != "" {
		color.HiBlue("  🌍 Primary timezone: %s", mostActiveTZ)
	}

	if tzDist, ok := patterns["timezone_distribution"].(map[string]int); ok && len(tzDist) > 1 {
		color.HiYellow("  📍 Multiple timezones: %d zones detected", len(tzDist))
	}

	if unusualPct, ok := patterns["unusual_hour_percentage"].(float64); ok && unusualPct > 30 {
		color.HiYellow("  ⚠️  %.1f%% unusual hour commits (in stated timezone)", unusualPct)
	}

	if nightOwlPct, ok := patterns["night_owl_percentage"].(float64); ok && nightOwlPct > 20 {
		color.HiMagenta("  🌙 %.1f%% night owl pattern (10pm-2am local)", nightOwlPct)
	}

	if earlyBirdPct, ok := patterns["early_bird_percentage"].(float64); ok && earlyBirdPct > 20 {
		color.HiGreen("  🌅 %.1f%% early bird pattern (5am-7am local)", earlyBirdPct)
	}

	if mostActiveHour, ok := patterns["most_active_hour"].(int); ok {
		color.HiCyan("  ⏰ Most active: %02d:00 local time", mostActiveHour)
	}
}

func displayAggregatedHourlyGraph(patterns map[string]interface{}) {
	hourDist, ok := patterns["hour_distribution"].(map[int]int)
	if !ok || len(hourDist) == 0 {
		return
	}

	termInfo := getTerminalInfo()
	maxCommits := 0
	for _, count := range hourDist {
		if count > maxCommits {
			maxCommits = count
		}
	}

	if maxCommits == 0 {
		return
	}

	color.HiWhite("📊 Aggregated Hourly Activity Timeline (All Target Users):")

	barWidth := min(termInfo.graphWidth, 50)
	scale := float64(barWidth) / float64(maxCommits)

	for hour := 0; hour < 24; hour++ {
		count := hourDist[hour]
		barLength := int(float64(count) * scale)

		bar := strings.Repeat("#", barLength)

		fmt.Printf("%02d:00 |", hour)

		if count > 0 {
			var coloredBar string
			formatStr := fmt.Sprintf("%%-%ds", barWidth)
			if hour >= 22 || hour <= 2 {
				coloredBar = color.HiRedString(formatStr, bar)
			} else if hour >= 5 && hour <= 7 {
				coloredBar = color.HiGreenString(formatStr, bar)
			} else if hour >= 9 && hour <= 17 {
				coloredBar = color.HiBlueString(formatStr, bar)
			} else {
				coloredBar = color.HiYellowString(formatStr, bar)
			}
			fmt.Printf("%s %d\n", coloredBar, count)
		} else {
			fmt.Printf("%s\n", strings.Repeat(" ", barWidth))
		}
	}

	separatorLen := barWidth + 5
	color.White("     +%s+", strings.Repeat("-", separatorLen))
	color.White("     🔴 Night Owl  🟢 Early Bird  🔵 Work Hours  🟡 Other")
}

func displaySuspiciousPatterns(commits []models.CommitInfo) {
	suspiciousCommits := make([]models.CommitInfo, 0)

	for _, commit := range commits {
		if commit.TimestampAnalysis != nil && commit.TimestampAnalysis.IsUnusualHour {
			suspiciousCommits = append(suspiciousCommits, commit)
		}
	}

	if len(suspiciousCommits) > 0 && len(suspiciousCommits) <= 15 {
		fmt.Println("\n🔍 Unusual Hour Commits (Target Users):")

		sort.Slice(suspiciousCommits, func(i, j int) bool {
			return suspiciousCommits[i].AuthorDate.After(suspiciousCommits[j].AuthorDate)
		})

		for i, commit := range suspiciousCommits {
			if i >= 8 {
				break
			}

			localTimeStr := commit.AuthorDate.Format("2006-01-02 15:04:05")
			color.Yellow("• %s at %s (%s)", commit.Hash[:8], localTimeStr, commit.TimestampAnalysis.CommitTimezone)
			if commit.TimestampAnalysis.TimeZoneHint != "" {
				color.White("  %s", commit.TimestampAnalysis.TimeZoneHint)
			}
		}
	} else if len(suspiciousCommits) > 15 {
		fmt.Printf("\n🔍 Found %d unusual hour commits (showing pattern summary above)\n", len(suspiciousCommits))
	}
}

type JSONOutput struct {
	Target            string           `json:"target"`
	IsOrg             bool             `json:"is_org"`
	User              *JSONUser        `json:"user,omitempty"`
	Emails            []JSONEmailEntry `json:"emails"`
	TotalCommits      int              `json:"total_commits"`
	TotalContributors int              `json:"total_contributors"`
}

type JSONUser struct {
	Login       string `json:"login"`
	Name        string `json:"name,omitempty"`
	Email       string `json:"email,omitempty"`
	Company     string `json:"company,omitempty"`
	Location    string `json:"location,omitempty"`
	Bio         string `json:"bio,omitempty"`
	Blog        string `json:"blog,omitempty"`
	Twitter     string `json:"twitter,omitempty"`
	Followers   int    `json:"followers"`
	Following   int    `json:"following"`
	PublicRepos int    `json:"public_repos"`
}

type JSONEmailEntry struct {
	Email        string     `json:"email"`
	Names        []string   `json:"names"`
	CommitCount  int        `json:"commit_count"`
	IsTarget     bool       `json:"is_target"`
	Repositories []JSONRepo `json:"repositories"`
}

type JSONRepo struct {
	Name    string       `json:"name"`
	Commits []JSONCommit `json:"commits"`
}

type JSONCommit struct {
	Hash           string    `json:"hash"`
	URL            string    `json:"url"`
	Message        string    `json:"message,omitempty"`
	AuthorName     string    `json:"author_name"`
	AuthorEmail    string    `json:"author_email"`
	AuthorDate     time.Time `json:"author_date"`
	CommitterName  string    `json:"committer_name,omitempty"`
	CommitterEmail string    `json:"committer_email,omitempty"`
	Secrets        []string  `json:"secrets,omitempty"`
}

func outputJSON(ctx *Context, matcher *UserMatcher) {
	sortedEmails := sortEmailsByCommitCount(ctx.Emails)

	output := JSONOutput{
		Target:            ctx.KnownUsername,
		IsOrg:             ctx.IsOrg,
		Emails:            make([]JSONEmailEntry, 0),
		TotalCommits:      0,
		TotalContributors: len(sortedEmails),
	}

	if ctx.User != nil {
		output.User = &JSONUser{
			Login:       ctx.User.GetLogin(),
			Name:        ctx.User.GetName(),
			Email:       ctx.User.GetEmail(),
			Company:     ctx.User.GetCompany(),
			Location:    ctx.User.GetLocation(),
			Bio:         ctx.User.GetBio(),
			Blog:        ctx.User.GetBlog(),
			Twitter:     ctx.User.GetTwitterUsername(),
			Followers:   ctx.User.GetFollowers(),
			Following:   ctx.User.GetFollowing(),
			PublicRepos: ctx.User.GetPublicRepos(),
		}
	}

	for _, entry := range sortedEmails {
		isTarget := matcher.IsTargetUser(entry.Email, entry.Details)

		if ctx.ShowTargetOnly && !isTarget {
			continue
		}

		if isTarget {
			output.TotalCommits += entry.Details.CommitCount
		}

		jsonEntry := JSONEmailEntry{
			Email:        entry.Email,
			Names:        extractNames(entry.Details),
			CommitCount:  entry.Details.CommitCount,
			IsTarget:     isTarget,
			Repositories: make([]JSONRepo, 0),
		}

		for repoName, commits := range entry.Details.Commits {
			jsonRepo := JSONRepo{
				Name:    repoName,
				Commits: make([]JSONCommit, 0),
			}

			for _, commit := range commits {
				jsonCommit := JSONCommit{
					Hash:           commit.Hash,
					URL:            commit.URL,
					Message:        commit.Message,
					AuthorName:     commit.AuthorName,
					AuthorEmail:    commit.AuthorEmail,
					AuthorDate:     commit.AuthorDate,
					CommitterName:  commit.CommitterName,
					CommitterEmail: commit.CommitterEmail,
					Secrets:        commit.Secrets,
				}
				jsonRepo.Commits = append(jsonRepo.Commits, jsonCommit)
			}

			jsonEntry.Repositories = append(jsonEntry.Repositories, jsonRepo)
		}

		output.Emails = append(output.Emails, jsonEntry)
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
	}
}

func outputCSV(ctx *Context, matcher *UserMatcher) {
	sortedEmails := sortEmailsByCommitCount(ctx.Emails)

	writer := csv.NewWriter(os.Stdout)
	defer writer.Flush()

	headers := []string{
		"email",
		"names",
		"is_target",
		"commit_count",
		"repository",
		"commit_hash",
		"commit_url",
		"author_name",
		"author_email",
		"author_date",
		"committer_name",
		"committer_email",
		"secrets_found",
	}

	if err := writer.Write(headers); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing CSV headers: %v\n", err)
		return
	}

	for _, entry := range sortedEmails {
		isTarget := matcher.IsTargetUser(entry.Email, entry.Details)

		if ctx.ShowTargetOnly && !isTarget {
			continue
		}

		names := strings.Join(extractNames(entry.Details), "; ")
		isTargetStr := "false"
		if isTarget {
			isTargetStr = "true"
		}

		for repoName, commits := range entry.Details.Commits {
			for _, commit := range commits {
				secretsStr := ""
				if len(commit.Secrets) > 0 {
					secretsStr = strings.Join(commit.Secrets, " | ")
				}

				row := []string{
					entry.Email,
					names,
					isTargetStr,
					fmt.Sprintf("%d", entry.Details.CommitCount),
					repoName,
					commit.Hash,
					commit.URL,
					commit.AuthorName,
					commit.AuthorEmail,
					commit.AuthorDate.Format(time.RFC3339),
					commit.CommitterName,
					commit.CommitterEmail,
					secretsStr,
				}

				if err := writer.Write(row); err != nil {
					fmt.Fprintf(os.Stderr, "Error writing CSV row: %v\n", err)
					return
				}
			}
		}
	}
}
