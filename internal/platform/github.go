package platform

import (
	"context"
	"fmt"
	"strings"

	"github.com/gnomegl/gitslurp/v2/internal/models"
	"github.com/gnomegl/gitslurp/v2/internal/scanner"
	"github.com/gnomegl/gitslurp/v2/internal/utils"
	gh "github.com/google/go-github/v57/github"
	"golang.org/x/oauth2"
)

type GitHubProvider struct {
	client *gh.Client
	token  string
}

func NewGitHubProvider(token string) *GitHubProvider {
	var client *gh.Client
	if token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		tc := oauth2.NewClient(context.Background(), ts)
		client = gh.NewClient(tc)
	} else {
		client = gh.NewClient(nil)
	}
	return &GitHubProvider{client: client, token: token}
}

func (g *GitHubProvider) Name() Platform { return GitHub }

func (g *GitHubProvider) GetUser(ctx context.Context, username string) (*UserInfo, error) {
	user, _, err := g.client.Users.Get(ctx, username)
	if err != nil {
		return nil, err
	}
	return ghUserToInfo(user), nil
}

func (g *GitHubProvider) IsOrganization(ctx context.Context, name string) (bool, error) {
	_, resp, err := g.client.Organizations.Get(ctx, name)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (g *GitHubProvider) UserExists(ctx context.Context, username string) (bool, error) {
	_, resp, err := g.client.Users.Get(ctx, username)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (g *GitHubProvider) ListUserRepos(ctx context.Context, username string, includeForks bool) ([]*Repository, error) {
	var allRepos []*Repository
	opt := &gh.RepositoryListByUserOptions{
		ListOptions: gh.ListOptions{PerPage: 100},
		Type:        "all",
	}

	for {
		repos, resp, err := g.client.Repositories.ListByUser(ctx, username, opt)
		if err != nil {
			return nil, err
		}
		for _, repo := range repos {
			if !includeForks && repo.GetFork() {
				continue
			}
			allRepos = append(allRepos, &Repository{
				Owner:    repo.GetOwner().GetLogin(),
				Name:     repo.GetName(),
				FullName: repo.GetFullName(),
				Fork:     repo.GetFork(),
				HTMLURL:  repo.GetHTMLURL(),
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return allRepos, nil
}

func (g *GitHubProvider) ListOrgRepos(ctx context.Context, orgName string) ([]*Repository, error) {
	var allRepos []*Repository
	opt := &gh.RepositoryListByOrgOptions{
		Type:        "public",
		ListOptions: gh.ListOptions{PerPage: 100},
	}

	for {
		repos, resp, err := g.client.Repositories.ListByOrg(ctx, orgName, opt)
		if err != nil {
			return nil, err
		}
		for _, repo := range repos {
			allRepos = append(allRepos, &Repository{
				Owner:    repo.GetOwner().GetLogin(),
				Name:     repo.GetName(),
				FullName: repo.GetFullName(),
				Fork:     repo.GetFork(),
				HTMLURL:  repo.GetHTMLURL(),
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return allRepos, nil
}

func (g *GitHubProvider) ListCommits(ctx context.Context, owner, repo string, cfg ScanConfig) ([]models.CommitInfo, error) {
	perPage := cfg.PerPage
	if cfg.QuickMode {
		perPage = 50
	}

	opts := &gh.CommitsListOptions{
		ListOptions: gh.ListOptions{PerPage: perPage},
	}

	var allCommits []models.CommitInfo
	for {
		commits, resp, err := g.client.Repositories.ListCommits(ctx, owner, repo, opts)
		if err != nil {
			if resp != nil && resp.StatusCode == 409 {
				return nil, fmt.Errorf("repository is empty or not accessible")
			}
			return nil, err
		}

		for _, c := range commits {
			if c.GetCommit() == nil || c.GetCommit().GetAuthor() == nil {
				continue
			}

			info := models.CommitInfo{
				Hash:        c.GetSHA(),
				URL:         c.GetHTMLURL(),
				AuthorName:  c.GetCommit().GetAuthor().GetName(),
				AuthorEmail: c.GetCommit().GetAuthor().GetEmail(),
				Message:     c.GetCommit().GetMessage(),
				RepoName:    fmt.Sprintf("%s/%s", owner, repo),
			}

			if c.GetAuthor() != nil {
				info.AuthorLogin = c.GetAuthor().GetLogin()
			}
			if c.GetCommit().GetAuthor() != nil {
				info.AuthorDate = c.GetCommit().GetAuthor().GetDate().Time
			}
			if c.GetCommit().GetCommitter() != nil {
				info.CommitterName = c.GetCommit().GetCommitter().GetName()
				info.CommitterEmail = c.GetCommit().GetCommitter().GetEmail()
				info.CommitterDate = c.GetCommit().GetCommitter().GetDate().Time
			}

			if cfg.TimestampAnalysis {
				info.TimestampAnalysis = utils.AnalyzeTimestamp(info.AuthorDate)
			}

			if cfg.CheckSecrets || cfg.ShowInteresting {
				secretScanner := scanner.NewScanner(cfg.ShowInteresting)
				if info.Message != "" {
					for _, match := range secretScanner.ScanText(info.Message) {
						if match.Type == "Secret" && cfg.CheckSecrets {
							info.Secrets = append(info.Secrets, fmt.Sprintf("%s: %s (in commit message)", match.Name, match.Value))
						} else if match.Type == "Interesting" && cfg.ShowInteresting {
							info.Secrets = append(info.Secrets, fmt.Sprintf("INTERESTING: %s: %s (in commit message)", match.Name, match.Value))
						}
					}
				}

				files, _, err := g.GetCommitDetail(ctx, owner, repo, c.GetSHA())
				if err == nil {
					for _, file := range files {
						if cfg.SkipNodeModules && (strings.Contains(file.Filename, "/node_modules/") || strings.HasPrefix(file.Filename, "node_modules/")) {
							continue
						}
						if file.Patch != "" {
							for _, match := range secretScanner.ScanText(file.Patch) {
								if match.Type == "Secret" && cfg.CheckSecrets {
									info.Secrets = append(info.Secrets, fmt.Sprintf("%s: %s (in %s)", match.Name, match.Value, file.Filename))
								} else if match.Type == "Interesting" && cfg.ShowInteresting {
									info.Secrets = append(info.Secrets, fmt.Sprintf("INTERESTING: %s: %s (in %s)", match.Name, match.Value, file.Filename))
								}
							}
						}
					}
				}
			}

			allCommits = append(allCommits, info)
		}

		if resp.NextPage == 0 || cfg.QuickMode {
			break
		}
		opts.Page = resp.NextPage
	}
	return allCommits, nil
}

func (g *GitHubProvider) GetCommitDetail(ctx context.Context, owner, repo, sha string) ([]CommitFile, string, error) {
	commit, _, err := g.client.Repositories.GetCommit(ctx, owner, repo, sha, nil)
	if err != nil {
		return nil, "", err
	}
	var files []CommitFile
	for _, f := range commit.Files {
		files = append(files, CommitFile{
			Filename: f.GetFilename(),
			Patch:    f.GetPatch(),
		})
	}
	return files, commit.GetCommit().GetMessage(), nil
}

func (g *GitHubProvider) SearchCommitsByUser(ctx context.Context, username string, cfg ScanConfig) (map[string]*models.EmailDetails, error) {
	emails := make(map[string]*models.EmailDetails)
	query := fmt.Sprintf("author:%s -user:%s", username, username)
	opts := &gh.SearchOptions{
		Sort:  "author-date",
		Order: "asc",
		ListOptions: gh.ListOptions{
			PerPage: 100,
		},
	}

	result, _, err := g.client.Search.Commits(ctx, query, opts)
	if err != nil {
		return nil, err
	}

	for _, cr := range result.Commits {
		if cr.Commit == nil || cr.Commit.Author == nil {
			continue
		}
		email := cr.Commit.Author.GetEmail()
		name := cr.Commit.Author.GetName()
		repoName := cr.Repository.GetFullName()

		if email == "noreply@github.com" {
			continue
		}

		if _, ok := emails[email]; !ok {
			emails[email] = &models.EmailDetails{
				Names:   make(map[string]struct{}),
				Commits: make(map[string][]models.CommitInfo),
			}
		}
		emails[email].Names[name] = struct{}{}

		info := models.CommitInfo{
			Hash:        cr.GetSHA(),
			URL:         cr.GetHTMLURL(),
			AuthorName:  name,
			AuthorEmail: email,
			Message:     cr.Commit.GetMessage(),
			RepoName:    repoName,
			IsExternal:  true,
		}
		if cr.Commit.Author.Date != nil {
			info.AuthorDate = cr.Commit.Author.Date.Time
		}
		if cr.Commit.Committer != nil {
			info.CommitterName = cr.Commit.Committer.GetName()
			info.CommitterEmail = cr.Commit.Committer.GetEmail()
			if cr.Commit.Committer.Date != nil {
				info.CommitterDate = cr.Commit.Committer.Date.Time
			}
		}

		emails[email].Commits[repoName] = append(emails[email].Commits[repoName], info)
		emails[email].CommitCount++
	}

	return emails, nil
}

func ghUserToInfo(u *gh.User) *UserInfo {
	info := &UserInfo{
		Login:       u.GetLogin(),
		Name:        u.GetName(),
		Email:       u.GetEmail(),
		Company:     u.GetCompany(),
		Location:    u.GetLocation(),
		Bio:         u.GetBio(),
		Blog:        u.GetBlog(),
		Twitter:     u.GetTwitterUsername(),
		Followers:   u.GetFollowers(),
		Following:   u.GetFollowing(),
		PublicRepos: u.GetPublicRepos(),
		PublicGists: u.GetPublicGists(),
		AvatarURL:   u.GetAvatarURL(),
	}
	if !u.GetCreatedAt().Time.IsZero() {
		info.CreatedAt = u.GetCreatedAt().Time
	}
	if !u.GetUpdatedAt().Time.IsZero() {
		info.UpdatedAt = u.GetUpdatedAt().Time
	}
	return info
}
