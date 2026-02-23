package display

import (
	"fmt"

	"github.com/fatih/color"
	gh "github.com/google/go-github/v57/github"
)

var headerColor = color.New(color.Bold, color.FgCyan)

func UserInfo(user *gh.User, isOrg bool) {
	if user == nil {
		return
	}

	fmt.Println()
	if isOrg {
		headerColor.Printf("ORGANIZATION: %s\n", user.GetLogin())
	} else {
		headerColor.Printf("USER: %s\n", user.GetLogin())
	}

	printField("Name", user.GetName())
	printField("Email", user.GetEmail())
	printField("Company", user.GetCompany())
	printField("Location", user.GetLocation())
	printField("Bio", user.GetBio())
	printField("Website", user.GetBlog())
	if user.GetTwitterUsername() != "" {
		fmt.Printf("%s @%s\n", color.WhiteString("Twitter:"), user.GetTwitterUsername())
	}

	fmt.Println()
	if isOrg {
		if user.GetPublicRepos() > 0 {
			fmt.Printf("%s %d\n", color.WhiteString("Repos:"), user.GetPublicRepos())
		}
	} else {
		fmt.Printf("%s %d  %s %d  %s %d  %s %d\n",
			color.WhiteString("Repos:"), user.GetPublicRepos(),
			color.WhiteString("Gists:"), user.GetPublicGists(),
			color.WhiteString("Followers:"), user.GetFollowers(),
			color.WhiteString("Following:"), user.GetFollowing())
	}

	if !user.GetCreatedAt().Time.IsZero() || !user.GetUpdatedAt().Time.IsZero() {
		parts := ""
		if !user.GetCreatedAt().Time.IsZero() {
			parts += fmt.Sprintf("%s %s", color.WhiteString("Created:"), user.GetCreatedAt().Time.Format("2006-01-02"))
		}
		if !user.GetUpdatedAt().Time.IsZero() {
			if parts != "" {
				parts += "  "
			}
			parts += fmt.Sprintf("%s %s", color.WhiteString("Updated:"), user.GetUpdatedAt().Time.Format("2006-01-02"))
		}
		fmt.Println(parts)
	}

	fmt.Println()
}

func printField(label, value string) {
	if value == "" {
		return
	}
	fmt.Printf("%s %s\n", color.WhiteString(label+":"), value)
}
