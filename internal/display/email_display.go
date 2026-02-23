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

type ColorPrinter struct {
	isTarget bool
}

func (cp *ColorPrinter) PrintEmail(email string, names []string, commitCount int, isTarget bool, isSimilar bool, isOrgEmployee bool) {
	nameStr := strings.Join(names, ", ")

	if isTarget {
		color.Green("[TARGET] %s (%d commits)", email, commitCount)
		if nameStr != "" {
			fmt.Printf("  Names: %s\n", nameStr)
		}
	} else if isSimilar {
		color.Yellow("[SIMILAR] %s (%d commits)", email, commitCount)
		if nameStr != "" {
			fmt.Printf("  Names: %s\n", nameStr)
		}
	} else if isOrgEmployee {
		color.Yellow("%s (%d commits)", email, commitCount)
		if nameStr != "" {
			fmt.Printf("  Names: %s\n", nameStr)
		}
	} else {
		color.White("%s (%d commits)", email, commitCount)
		if nameStr != "" {
			fmt.Printf("  Names: %s\n", nameStr)
		}
	}
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
		displayEmailDomains(ctx)
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
		printer.PrintEmail(update.Email, names, update.Details.CommitCount, isTargetUser, false, isOrgEmployee)
		fmt.Println()
	}
}

func displayEmailDomains(ctx *Context) {
	domainCounts := make(map[string]int)

	for email := range ctx.Emails {
		if strings.Contains(email, "@") {
			domain := strings.Split(email, "@")[1]
			domainCounts[domain]++
		}
	}

	if len(domainCounts) == 0 {
		return
	}

	type domainEntry struct {
		domain string
		count  int
	}

	var sorted []domainEntry
	for d, c := range domainCounts {
		sorted = append(sorted, domainEntry{d, c})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	headerColor.Print("EMAIL DOMAINS")
	fmt.Println(" (Top 10)")
	limit := 10
	if len(sorted) < limit {
		limit = len(sorted)
	}
	for _, entry := range sorted[:limit] {
		fmt.Printf("  %s %d contributors\n", color.WhiteString(entry.domain+":"), entry.count)
	}
	fmt.Println()
}

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

		isSimilar := false
		if isTargetUser {
			result.totalCommits += entry.Details.CommitCount
			result.targetAccounts[entry.Email] = names
		} else if isOrgEmployee {
			if hasSimilarNames {
				result.similarOrgMembers[entry.Email] = names
				isSimilar = true
			} else {
				result.orgMembers[entry.Email] = names
			}
		} else if hasSimilarNames {
			result.similarAccounts[entry.Email] = names
			isSimilar = true
		}

		printer.PrintEmail(entry.Email, names, entry.Details.CommitCount, isTargetUser, isSimilar, isOrgEmployee)

		if shouldShowCommitDetails(opts) {
			displayCommitDetails(entry, isTargetUser, ctx)
		}

		fmt.Println()
	}

	return result
}

func shouldShowCommitDetails(opts *DisplayOptions) bool {
	return opts.ShowDetails || opts.CheckSecrets || opts.ShowInteresting
}

func displayResults(ctx *Context, result *EmailProcessResult) {
	displayRepositoryStats(ctx.Emails, ctx.UserIdentifiers)

	if ctx.Cfg.TimestampAnalysis {
		displayTimestampAnalysis(ctx.Emails, ctx.UserIdentifiers)
	}

	displaySummary(result.targetAccounts, result.similarAccounts, result.orgMembers, result.similarOrgMembers, ctx.IsOrg, ctx.OrgDomain, result.totalCommits, result.totalContributors)
}

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
