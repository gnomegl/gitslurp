package platform

import (
	"context"
	"time"

	"github.com/gnomegl/gitslurp/v2/internal/models"
)

type Platform string

const (
	GitHub   Platform = "github"
	GitLab   Platform = "gitlab"
	Codeberg Platform = "codeberg"
)

type UserInfo struct {
	Login       string
	Name        string
	Email       string
	Company     string
	Location    string
	Bio         string
	Blog        string
	Twitter     string
	Followers   int
	Following   int
	PublicRepos int
	PublicGists int
	CreatedAt   time.Time
	UpdatedAt   time.Time
	AvatarURL   string
	IsOrg       bool
}

type Repository struct {
	Owner    string
	Name     string
	FullName string
	Fork     bool
	HTMLURL  string
}

type ScanConfig struct {
	CheckSecrets    bool
	ShowInteresting bool
	QuickMode       bool
	TimestampAnalysis bool
	IncludeForks    bool
	SkipNodeModules bool
	PerPage         int
	MaxConcurrent   int
}

func DefaultScanConfig() ScanConfig {
	return ScanConfig{
		PerPage:         100,
		MaxConcurrent:   5,
		SkipNodeModules: true,
	}
}

type Provider interface {
	Name() Platform

	GetUser(ctx context.Context, username string) (*UserInfo, error)
	IsOrganization(ctx context.Context, name string) (bool, error)
	UserExists(ctx context.Context, username string) (bool, error)

	ListUserRepos(ctx context.Context, username string, includeForks bool) ([]*Repository, error)
	ListOrgRepos(ctx context.Context, orgName string) ([]*Repository, error)

	ListCommits(ctx context.Context, owner, repo string, cfg ScanConfig) ([]models.CommitInfo, error)
	GetCommitDetail(ctx context.Context, owner, repo, sha string) ([]CommitFile, string, error)

	SearchCommitsByUser(ctx context.Context, username string, cfg ScanConfig) (map[string]*models.EmailDetails, error)
}

type CommitFile struct {
	Filename string
	Patch    string
}
