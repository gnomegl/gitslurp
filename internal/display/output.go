package display

import (
	"fmt"
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

// displays email information with appropriate coloring
func (cp *ColorPrinter) PrintEmail(email string, names []string, commitCount int, isTarget bool) {
	if isTarget {
		color.HiYellow("📍 %s (Target User)", email)
		color.HiGreen("  Names used: %s", strings.Join(names, ", "))
		color.HiGreen("  Total Commits: %d", commitCount)
	} else {
		color.Yellow(email)
		color.White("  Names: %s", strings.Join(names, ", "))
		color.White("  Total Commits: %d", commitCount)
	}
}

// PrintSimilarAccount displays similar account information
func (cp *ColorPrinter) PrintSimilarAccount(email string, names []string, commitCount int) {
	fmt.Printf("👁️ %s (Similar Account)\n", email)
	color.Magenta("  Names used: %s", strings.Join(names, ", "))
	color.Magenta("  Total Commits: %d", commitCount)
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// getTerminalWidth returns the terminal width, defaulting to 80 if unavailable
func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 80 // Default fallback width
	}
	return width
}

// formatField creates a formatted field string with proper padding
func formatField(label, value string, width int) string {
	if value == "" {
		return ""
	}
	maxValueWidth := width - len(label) - 3 // Account for ": " and padding
	if len(value) > maxValueWidth {
		value = value[:maxValueWidth-3] + "..."
	}
	return fmt.Sprintf("%-*s %s", len(label)+1, label+":", value)
}

// UserInfo displays user profile information in a responsive 2-column layout
func UserInfo(user *gh.User, isOrg bool) {
	if user == nil {
		return
	}

	termWidth := getTerminalWidth()
	useTwoColumns := termWidth >= 100 // Switch to single column if terminal too narrow
	maxDisplayWidth := min(termWidth, 120) // Limit display width to prevent excessive spacing
	colWidth := (maxDisplayWidth - 4) / 2   // Account for spacing between columns

	fmt.Println()
	if isOrg {
		fmt.Print("🏢 ")
		color.HiCyan("ORGANIZATION PROFILE")
	} else {
		fmt.Print("👤 ")
		color.HiCyan("USER PROFILE")
	}
	fmt.Println(strings.Repeat("═", min(maxDisplayWidth-1, 80)))
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

// Results shows all the collected information about emails and commits
func Results(emails map[string]*models.EmailDetails, showDetails bool, checkSecrets bool,
	lookupEmail string, knownUsername string, user *gh.User, showTargetOnly bool, isOrg bool, cfg *github.Config) {

	matcher := NewUserMatcher(knownUsername, lookupEmail, user)
	matcher.targetNames = extractTargetUserNames(emails, matcher.identifiers)

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
	}

	result := processEmails(ctx, matcher)
	displayResults(ctx, result)
}

// EmailProcessResult holds the results of email processing
type EmailProcessResult struct {
	totalCommits      int
	totalContributors int
	targetAccounts    map[string][]string
	similarAccounts   map[string][]string
}

// processEmails processes all emails and returns aggregated results
func processEmails(ctx *Context, matcher *UserMatcher) *EmailProcessResult {
	sortedEmails := sortEmailsByCommitCount(ctx.Emails)
	result := &EmailProcessResult{
		targetAccounts:  make(map[string][]string),
		similarAccounts: make(map[string][]string),
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
		result.totalContributors++

		if opts.ShowTargetOnly && !isTargetUser {
			continue
		}

		names := extractNames(entry.Details)

		if isTargetUser {
			result.totalCommits += entry.Details.CommitCount
			result.targetAccounts[entry.Email] = names
			printer.PrintEmail(entry.Email, names, entry.Details.CommitCount, true)
		} else if matcher.HasMatchingNames(names) {
			result.similarAccounts[entry.Email] = names
			printer.PrintSimilarAccount(entry.Email, names, entry.Details.CommitCount)
		} else if !opts.ShowTargetOnly {
			printer.PrintEmail(entry.Email, names, entry.Details.CommitCount, false)
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
	displayAccountInfo(ctx.User, ctx.IsOrg)
	fmt.Println("\nCollected author information:")

	displayTotals(ctx.ShowTargetOnly, result.totalCommits, result.totalContributors)
	
	if ctx.Cfg.TimestampAnalysis {
		displayTimestampAnalysis(ctx.Emails, ctx.UserIdentifiers)
	}
	
	displaySummary(result.targetAccounts, result.similarAccounts)
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

func displayAccountInfo(user *gh.User, isOrg bool) {
	if user != nil {
		accountType := "User"
		if isOrg {
			accountType = "Organization"
		}
		fmt.Printf("\n%s Account Information:\n", accountType)
	}
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

// displayRepoHeader shows the repository header with appropriate coloring
func (cd *CommitDisplayer) displayRepoHeader(repoName string, isTargetUser bool, email string) {
	if isTargetUser {
		color.HiGreen("  📂 Repo: %s", repoName)
	} else if !cd.ctx.ShowTargetOnly && cd.ctx.UserIdentifiers[email] {
		color.HiWhite("  📂 Repo: %s", repoName)
	} else {
		color.Green("  Repo: %s", repoName)
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
}

// printCommitHighlight prints commit info with high emphasis
func (cd *CommitDisplayer) printCommitHighlight(commit models.CommitInfo, commitIcon, authorIcon string) {
	color.HiMagenta("%s %s", commitIcon, commit.Hash)
	color.HiBlue("    🔗 URL: %s", commit.URL)
	if commit.AuthorName == "" {
		color.HiWhite("%s anonymous", authorIcon)
	} else {
		color.HiWhite("%s %s <%s>", authorIcon, commit.AuthorName, commit.AuthorEmail)
	}
}

// printCommitMedium prints commit info with medium emphasis
func (cd *CommitDisplayer) printCommitMedium(commit models.CommitInfo, commitIcon, authorIcon string) {
	color.Magenta("%s %s", commitIcon, commit.Hash)
	color.Blue("    🔗 URL: %s", commit.URL)
	if commit.AuthorName == "" {
		color.White("%s anonymous", authorIcon)
	} else {
		color.White("%s %s <%s>", authorIcon, commit.AuthorName, commit.AuthorEmail)
	}
}

// printCommitRegular prints commit info with regular emphasis
func (cd *CommitDisplayer) printCommitRegular(commit models.CommitInfo, commitIcon, authorIcon string) {
	color.Magenta("%s %s", commitIcon, commit.Hash)
	color.Blue("    URL: %s", commit.URL)
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
		color.HiYellow("      %s", pattern)
	} else {
		color.Yellow("      %s", pattern)
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

func displaySummary(targetAccounts, similarAccounts map[string][]string) {
	if len(targetAccounts) == 0 && len(similarAccounts) == 0 {
		return
	}

	color.HiCyan("\nSUMMARY")
	fmt.Println(strings.Repeat("─", 60))

	if len(targetAccounts) > 0 {
		fmt.Println("\n📍  Target User Accounts:")
		for email, names := range targetAccounts {
			color.Yellow("  • %s", email)
			if len(names) > 0 {
				color.Green("    Names: %s", strings.Join(names, ", "))
			}
		}
	}

	if len(similarAccounts) > 0 {
		fmt.Println("\n👁️  Similar Accounts:")
		for email, names := range similarAccounts {
			color.Yellow("  • %s", email)
			if len(names) > 0 {
				color.Magenta("    Names: %s", strings.Join(names, ", "))
			}
		}
	}
	fmt.Println()
}

// displayTimestampAnalysis shows timestamp analysis results
func displayTimestampAnalysis(emails map[string]*models.EmailDetails, userIdentifiers map[string]bool) {
	color.HiCyan("\n🕐 TIMESTAMP ANALYSIS")
	fmt.Println(strings.Repeat("─", 60))
	
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
		color.Yellow("  • %.1f%% commits during unusual hours (10pm-6am local time)", unusualPct)
	}
	
	if weekendPct, ok := patterns["weekend_percentage"].(float64); ok && weekendPct > 0 {
		color.Cyan("  • %.1f%% commits on weekends", weekendPct)
	}
	
	if nightOwlPct, ok := patterns["night_owl_percentage"].(float64); ok && nightOwlPct > 10 {
		color.Magenta("  • %.1f%% night owl commits (10pm-2am local time)", nightOwlPct)
	}
	
	if earlyBirdPct, ok := patterns["early_bird_percentage"].(float64); ok && earlyBirdPct > 10 {
		color.Green("  • %.1f%% early bird commits (5am-7am local time)", earlyBirdPct)
	}
	
	if mostActiveHour, ok := patterns["most_active_hour"].(int); ok {
		color.Blue("  • Most active hour: %02d:00 local time", mostActiveHour)
	}
	
	if mostActiveDay, ok := patterns["most_active_day"].(time.Weekday); ok {
		color.Blue("  • Most active day: %s", mostActiveDay.String())
	}
	
	if mostActiveTZ, ok := patterns["most_active_timezone"].(string); ok && mostActiveTZ != "" {
		color.HiBlue("  • Most common timezone: %s", mostActiveTZ)
	}
	
	if tzDist, ok := patterns["timezone_distribution"].(map[string]int); ok && len(tzDist) > 1 {
		color.HiYellow("  • Multiple timezones detected: %d different zones", len(tzDist))
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
		color.White("    - %s: %d commits", zone.zone, zone.count)
	}
}

func displayUserTimestampAnalysis(email string, commits []models.CommitInfo) {
	patterns := utils.GetTimestampPatterns(commits)
	
	color.HiWhite("  %s (%d commits):", email, len(commits))
	
	if mostActiveTZ, ok := patterns["most_active_timezone"].(string); ok && mostActiveTZ != "" {
		color.HiBlue("    🌍 Primary timezone: %s", mostActiveTZ)
	}
	
	if tzDist, ok := patterns["timezone_distribution"].(map[string]int); ok && len(tzDist) > 1 {
		color.HiYellow("    📍 Multiple timezones: %d zones detected", len(tzDist))
	}
	
	if unusualPct, ok := patterns["unusual_hour_percentage"].(float64); ok && unusualPct > 30 {
		color.HiYellow("    ⚠️  %.1f%% unusual hour commits (in stated timezone)", unusualPct)
	}
	
	if nightOwlPct, ok := patterns["night_owl_percentage"].(float64); ok && nightOwlPct > 20 {
		color.HiMagenta("    🌙 %.1f%% night owl pattern (10pm-2am local)", nightOwlPct)
	}
	
	if earlyBirdPct, ok := patterns["early_bird_percentage"].(float64); ok && earlyBirdPct > 20 {
		color.HiGreen("    🌅 %.1f%% early bird pattern (5am-7am local)", earlyBirdPct)
	}
	
	if mostActiveHour, ok := patterns["most_active_hour"].(int); ok {
		color.HiCyan("    ⏰ Most active: %02d:00 local time", mostActiveHour)
	}
}

func displayAggregatedHourlyGraph(patterns map[string]interface{}) {
	hourDist, ok := patterns["hour_distribution"].(map[int]int)
	if !ok || len(hourDist) == 0 {
		return
	}
	
	// Find max commits for scaling
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
	
	// Scale factor for ASCII graph (max 25 chars wide)
	scale := 25.0 / float64(maxCommits)
	
	// Display graph with consistent spacing for all hours
	for hour := 0; hour < 24; hour++ {
		count := hourDist[hour]
		barLength := int(float64(count) * scale)
		
		// Create bar
		bar := strings.Repeat("█", barLength)
		
		// Always display each hour with consistent spacing
		fmt.Printf("%02d:00 │", hour)
		
		if count > 0 {
			// Display bar with count to the right
			var coloredBar string
			if hour >= 22 || hour <= 2 {
				coloredBar = color.HiRedString("%-25s", bar)
			} else if hour >= 5 && hour <= 7 {
				coloredBar = color.HiGreenString("%-25s", bar)
			} else if hour >= 9 && hour <= 17 {
				coloredBar = color.HiBlueString("%-25s", bar)
			} else {
				coloredBar = color.HiYellowString("%-25s", bar)
			}
			fmt.Printf("%s %d\n", coloredBar, count)
		} else {
			fmt.Printf("%-25s\n", "")
		}
	}
	
	color.White("     └─────────────────────────────┘")
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
			color.Yellow("  • %s at %s (%s)", commit.Hash[:8], localTimeStr, commit.TimestampAnalysis.CommitTimezone)
			if commit.TimestampAnalysis.TimeZoneHint != "" {
				color.White("    %s", commit.TimestampAnalysis.TimeZoneHint)
			}
		}
	} else if len(suspiciousCommits) > 15 {
		fmt.Printf("\n🔍 Found %d unusual hour commits (showing pattern summary above)\n", len(suspiciousCommits))
	}
}
