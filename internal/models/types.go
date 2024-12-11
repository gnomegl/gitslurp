package models

import "time"

type CommitInfo struct {
	Hash        string
	URL         string
	AuthorName  string
	AuthorEmail string
	Message     string
	Date        time.Time
	RepoName    string
	IsFork      bool
	Secrets     []string
	Links       []string
	IsOwnRepo   bool
}

type EmailDetails struct {
	Names          map[string]struct{}
	Commits        map[string][]CommitInfo
	CommitCount    int
	IsUserEmail    bool
	GithubUsername string
}
