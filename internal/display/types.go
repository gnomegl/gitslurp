package display

import (
	"time"

	"github.com/gnomegl/gitslurp/internal/github"
	"github.com/gnomegl/gitslurp/internal/models"
	gh "github.com/google/go-github/v57/github"
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

type EmailEntry struct {
	Email   string
	Details *models.EmailDetails
}

type DisplayOptions struct {
	ShowDetails     bool
	CheckSecrets    bool
	ShowInteresting bool
	ShowTargetOnly  bool
}

type EmailProcessResult struct {
	totalCommits      int
	totalContributors int
	targetAccounts    map[string][]string
	similarAccounts   map[string][]string
	orgMembers        map[string][]string
	similarOrgMembers map[string][]string
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

