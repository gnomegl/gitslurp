package display

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

func outputJSON(w io.Writer, ctx *Context, matcher *UserMatcher) {
	sortedEmails := sortEmailsByCommitCount(ctx.Emails)
	encoder := json.NewEncoder(w)

	totalCommits := 0
	for _, entry := range sortedEmails {
		isTarget := matcher.IsTargetUser(entry.Email, entry.Details)
		if isTarget {
			totalCommits += entry.Details.CommitCount
		}
	}

	meta := NDJSONMeta{
		Target:            ctx.KnownUsername,
		IsOrg:             ctx.IsOrg,
		TotalCommits:      totalCommits,
		TotalContributors: len(sortedEmails),
	}

	if ctx.User != nil {
		meta.User = &JSONUser{
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

	encoder.Encode(meta)

	for _, entry := range sortedEmails {
		isTarget := matcher.IsTargetUser(entry.Email, entry.Details)

		if ctx.ShowTargetOnly && !isTarget {
			continue
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

		encoder.Encode(jsonEntry)
	}
}

func outputCSV(w io.Writer, ctx *Context, matcher *UserMatcher) {
	sortedEmails := sortEmailsByCommitCount(ctx.Emails)

	writer := csv.NewWriter(w)
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
		fmt.Fprintf(w, "Error writing CSV headers: %v\n", err)
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
					fmt.Fprintf(w, "Error writing CSV row: %v\n", err)
					return
				}
			}
		}
	}
}
