package display

import (
	"fmt"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/gnomegl/gitslurp/internal/models"
)

func displayRepositoryStats(emails map[string]*models.EmailDetails, userIdentifiers map[string]bool) {
	ownRepos := make(map[string]bool)
	externalRepos := make(map[string]bool)
	var externalCommits, ownCommits int

	externalEmailData := make(map[string]map[string]int)

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
					} else {
						ownRepos[repo] = true
						ownCommits++
					}
				}
			}
		}
	}

	if len(externalRepos) == 0 {
		return
	}

	fmt.Println()
	headerColor.Println("EXTERNAL CONTRIBUTIONS")
	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("%s %d\n", color.WhiteString("External repositories:"), len(externalRepos))
	fmt.Printf("%s %d\n", color.WhiteString("External commits:"), externalCommits)
	fmt.Printf("%s %d\n", color.WhiteString("Own repo commits:"), ownCommits)
	if ownCommits+externalCommits > 0 {
		percentage := float64(externalCommits) / float64(externalCommits+ownCommits) * 100
		fmt.Printf("%s %.1f%%\n", color.WhiteString("External %:"), percentage)
	}
	fmt.Println()

	sortedEmails := make([]string, 0, len(externalEmailData))
	for email := range externalEmailData {
		sortedEmails = append(sortedEmails, email)
	}
	sort.Strings(sortedEmails)

	for _, email := range sortedEmails {
		repoMap := externalEmailData[email]
		emailDetails := emails[email]
		names := extractNames(emailDetails)

		color.Green("%s", email)
		if len(names) > 0 {
			fmt.Printf("  Names: %s\n", strings.Join(names, ", "))
		}

		var totalRepoCommits int
		for _, count := range repoMap {
			totalRepoCommits += count
		}
		fmt.Printf("  %s\n", color.WhiteString("Repositories (%d commits total):", totalRepoCommits))

		repoNames := make([]string, 0, len(repoMap))
		for repo := range repoMap {
			repoNames = append(repoNames, repo)
		}
		sort.Strings(repoNames)

		for _, repo := range repoNames {
			commitCount := repoMap[repo]
			fmt.Printf("    - %s (%d commits)\n", repo, commitCount)
		}
		fmt.Println()
	}
}

func displaySummary(targetAccounts, similarAccounts, orgMembers, similarOrgMembers map[string][]string, isOrg bool, orgDomain string, totalCommits, totalContributors int) {
	if len(targetAccounts) == 0 && len(similarAccounts) == 0 && len(orgMembers) == 0 && len(similarOrgMembers) == 0 {
		return
	}

	fmt.Println()
	headerColor.Println("SUMMARY")
	fmt.Println(strings.Repeat("-", 60))

	if len(targetAccounts) > 0 {
		fmt.Println()
		boldGreen := color.New(color.Bold, color.FgGreen)
		boldGreen.Println("Target Accounts:")
		for email, names := range targetAccounts {
			color.Green("%s", email)
			if len(names) > 0 {
				fmt.Printf("  Names: %s\n", strings.Join(names, ", "))
			}
		}
	}

	if len(similarAccounts) > 0 {
		fmt.Println()
		boldYellow := color.New(color.Bold, color.FgYellow)
		boldYellow.Print("Similar Accounts:")
		fmt.Println(" (share names with target)")
		i := 0
		for email, names := range similarAccounts {
			if i >= 10 {
				fmt.Printf("  ... and %d more similar accounts\n", len(similarAccounts)-10)
				break
			}
			color.Yellow("%s", email)
			if len(names) > 0 {
				fmt.Printf("  Names: %s\n", strings.Join(names, ", "))
			}
			i++
		}
	}

	if isOrg && (len(orgMembers) > 0 || len(similarOrgMembers) > 0) {
		fmt.Println()
		if orgDomain != "" {
			fmt.Printf("Organization Members (@%s):\n", orgDomain)
		} else {
			fmt.Println("Organization Members:")
		}

		if len(similarOrgMembers) > 0 {
			fmt.Println()
			color.Yellow("Similar to Target (Possible Alternate Accounts):")
			for email, names := range similarOrgMembers {
				color.Yellow("  %s", email)
				if len(names) > 0 {
					fmt.Printf("    Names: %s\n", strings.Join(names, ", "))
				}
			}
		}

		if len(orgMembers) > 0 {
			if len(similarOrgMembers) > 0 {
				fmt.Println("\nOther Members:")
			}
			for email, names := range orgMembers {
				color.Yellow("  %s", email)
				if len(names) > 0 {
					fmt.Printf("    Names: %s\n", strings.Join(names, ", "))
				}
			}
		}
	}

	fmt.Println()
	fmt.Printf("%s %d\n", color.WhiteString("Target accounts:"), len(targetAccounts))
	fmt.Printf("%s %d\n", color.WhiteString("Similar accounts:"), len(similarAccounts))
	fmt.Printf("%s %d\n", color.WhiteString("Total target commits:"), totalCommits)
	fmt.Printf("%s %d\n", color.WhiteString("Total contributors:"), totalContributors)
}
