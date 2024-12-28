package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/gnomegl/gitslurp/internal/models"
	"github.com/gnomegl/gitslurp/internal/scanner"
	"github.com/google/go-github/v57/github"
)

// slurp gists into the same format as repos for unified analysis
func ProcessGists(ctx context.Context, client *github.Client, gists []*github.Gist, checkSecrets bool, showLinks bool, cfg *Config) map[string]*models.EmailDetails {
	emails := make(map[string]*models.EmailDetails)

	for _, gist := range gists {
		email := gist.Owner.GetEmail()
		if email == "" {
			email = fmt.Sprintf("%s@users.noreply.github.com", gist.Owner.GetLogin())
		}

		details, ok := emails[email]
		if !ok {
			details = &models.EmailDetails{
				Names:          make(map[string]struct{}),
				Commits:        make(map[string][]models.CommitInfo),
				CommitCount:    0,
				IsUserEmail:    true,
				GithubUsername: gist.Owner.GetLogin(),
			}
			emails[email] = details
		}

		details.Names[gist.Owner.GetLogin()] = struct{}{}
		if gist.Owner.GetName() != "" {
			details.Names[gist.Owner.GetName()] = struct{}{}
		}

		// gist -> commit conversion for normalization
		commit := models.CommitInfo{
			Hash: gist.GetID(),
			URL:  gist.GetHTMLURL(),

			AuthorName:  gist.Owner.GetLogin(),
			AuthorEmail: email,
			Message:     gist.GetDescription(),
			IsOwnRepo:   true, // gists are always user-owned
		}

		// Check for secrets and links in gist files
		for _, file := range gist.Files {
			content := file.GetContent()

			if checkSecrets {
				secrets := scanner.CheckForSecrets(content)
				commit.Secrets = append(commit.Secrets, secrets...)
			}

			if showLinks {
				links := scanner.ExtractLinks(content)
				commit.Links = append(commit.Links, links...)
			}
		}

		repoName := fmt.Sprintf("gist:%s", strings.Split(gist.GetID(), "-")[0])
		details.Commits[repoName] = append(details.Commits[repoName], commit)
		details.CommitCount++
	}

	return emails
}
