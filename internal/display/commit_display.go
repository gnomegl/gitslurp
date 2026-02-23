package display

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/gnomegl/gitslurp/internal/models"
)

type CommitDisplayer struct {
	ctx *Context
}

func NewCommitDisplayer(ctx *Context) *CommitDisplayer {
	return &CommitDisplayer{ctx: ctx}
}

func (cd *CommitDisplayer) DisplayForEntry(entry EmailEntry, isTargetUser bool) {
	for repoName, commits := range entry.Details.Commits {
		if !cd.shouldShowRepo(commits) {
			continue
		}

		color.Cyan("  %s", repoName)

		shown := 0
		for i := range commits {
			commit := &commits[i]
			if !cd.shouldShowCommit(*commit) {
				continue
			}

			if shown >= 5 {
				remaining := len(commits) - shown
				if remaining > 0 {
					fmt.Printf("    ... and %d more\n", remaining)
				}
				break
			}

			fmt.Printf("    %s %s\n", commit.Hash[:min(8, len(commit.Hash))], commit.AuthorDate.Format("2006-01-02 15:04"))

			if cd.ctx.ShowDetails {
				msg := commit.Message
				if idx := indexOf(msg, '\n'); idx >= 0 {
					msg = msg[:idx]
				}
				if len(msg) > 60 {
					msg = msg[:60]
				}
				if msg != "" {
					fmt.Printf("      %s\n", msg)
				}
			}

			if len(commit.Secrets) > 0 {
				cd.displaySecrets(commit.Secrets)
			}

			shown++
		}
	}
}

func displayCommitDetails(entry EmailEntry, isTargetUser bool, ctx *Context) {
	displayer := NewCommitDisplayer(ctx)
	displayer.DisplayForEntry(entry, isTargetUser)
}

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

func (cd *CommitDisplayer) shouldShowCommit(commit models.CommitInfo) bool {
	return cd.ctx.ShowDetails ||
		(len(commit.Secrets) > 0 && (cd.ctx.CheckSecrets || cd.ctx.Cfg.ShowInteresting))
}

func (cd *CommitDisplayer) displaySecrets(secrets []string) {
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		displaySecretLine(secret)
	}
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
