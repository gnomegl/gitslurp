package models

import "time"

type CommitInfo struct {
	Hash          string
	URL           string
	AuthorName    string
	AuthorEmail   string
	AuthorDate    time.Time
	CommitterName  string
	CommitterEmail string
	CommitterDate  time.Time
	Message       string
	Secrets       []string
	Links         []string
	IsOwnRepo     bool
	IsFork        bool
	RepoName      string
}

type EmailDetails struct {
	Names          map[string]struct{}
	Commits        map[string][]CommitInfo
	CommitCount    int
	IsUserEmail    bool
	GithubUsername string
}
