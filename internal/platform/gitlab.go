package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gnomegl/gitslurp/v2/internal/models"
	"github.com/gnomegl/gitslurp/v2/internal/scanner"
	"github.com/gnomegl/gitslurp/v2/internal/utils"
)

type GitLabProvider struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewGitLabProvider(token string) *GitLabProvider {
	return &GitLabProvider{
		baseURL:    "https://gitlab.com/api/v4",
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (g *GitLabProvider) Name() Platform { return GitLab }

func (g *GitLabProvider) doRequest(ctx context.Context, method, path string) ([]byte, int, error) {
	reqURL := g.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	if g.token != "" {
		req.Header.Set("PRIVATE-TOKEN", g.token)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

type glUser struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	Email     string `json:"public_email"`
	AvatarURL string `json:"avatar_url"`
	WebURL    string `json:"web_url"`
	Bio       string `json:"bio"`
	Location  string `json:"location"`
	Website   string `json:"website_url"`
	Twitter   string `json:"twitter"`
	Followers int    `json:"followers"`
	Following int    `json:"following"`
	CreatedAt string `json:"created_at"`
}

type glGroup struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	FullPath    string `json:"full_path"`
	Description string `json:"description"`
	WebURL      string `json:"web_url"`
	AvatarURL   string `json:"avatar_url"`
}

type glProject struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	PathWithNS    string `json:"path_with_namespace"`
	WebURL        string `json:"web_url"`
	ForkedFromPrj *struct {
		ID int64 `json:"id"`
	} `json:"forked_from_project"`
	Namespace struct {
		Path string `json:"path"`
	} `json:"namespace"`
}

type glCommit struct {
	ID             string   `json:"id"`
	ShortID        string   `json:"short_id"`
	Title          string   `json:"title"`
	Message        string   `json:"message"`
	AuthorName     string   `json:"author_name"`
	AuthorEmail    string   `json:"author_email"`
	CommitterName  string   `json:"committer_name"`
	CommitterEmail string   `json:"committer_email"`
	AuthoredDate   string   `json:"authored_date"`
	CommittedDate  string   `json:"committed_date"`
	WebURL         string   `json:"web_url"`
	ParentIDs      []string `json:"parent_ids"`
}

type glDiff struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
	Diff    string `json:"diff"`
}

func (g *GitLabProvider) GetUser(ctx context.Context, username string) (*UserInfo, error) {
	body, status, err := g.doRequest(ctx, "GET", "/users?username="+url.QueryEscape(username))
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("gitlab API error: %d", status)
	}

	var users []glUser
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("user not found: %s", username)
	}

	u := users[0]

	body2, status2, err := g.doRequest(ctx, "GET", fmt.Sprintf("/users/%d", u.ID))
	if err == nil && status2 == 200 {
		var fullUser glUser
		if err := json.Unmarshal(body2, &fullUser); err == nil {
			u = fullUser
		}
	}

	info := &UserInfo{
		Login:     u.Username,
		Name:      u.Name,
		Email:     u.Email,
		Location:  u.Location,
		Bio:       u.Bio,
		Blog:      u.Website,
		Twitter:   u.Twitter,
		Followers: u.Followers,
		Following: u.Following,
		AvatarURL: u.AvatarURL,
	}
	if t, err := time.Parse(time.RFC3339, u.CreatedAt); err == nil {
		info.CreatedAt = t
	} else if t, err := time.Parse("2006-01-02T15:04:05.000Z", u.CreatedAt); err == nil {
		info.CreatedAt = t
	}

	projects, _ := g.ListUserRepos(ctx, username, true)
	info.PublicRepos = len(projects)

	return info, nil
}

func (g *GitLabProvider) IsOrganization(ctx context.Context, name string) (bool, error) {
	_, status, err := g.doRequest(ctx, "GET", "/groups/"+url.PathEscape(name))
	if err != nil {
		return false, err
	}
	return status == 200, nil
}

func (g *GitLabProvider) UserExists(ctx context.Context, username string) (bool, error) {
	body, status, err := g.doRequest(ctx, "GET", "/users?username="+url.QueryEscape(username))
	if err != nil {
		return false, err
	}
	if status != 200 {
		return false, nil
	}
	var users []glUser
	if err := json.Unmarshal(body, &users); err != nil {
		return false, nil
	}
	return len(users) > 0, nil
}

func (g *GitLabProvider) resolveUserID(ctx context.Context, username string) (int64, error) {
	body, status, err := g.doRequest(ctx, "GET", "/users?username="+url.QueryEscape(username))
	if err != nil {
		return 0, err
	}
	if status != 200 {
		return 0, fmt.Errorf("gitlab API error: %d", status)
	}
	var users []glUser
	if err := json.Unmarshal(body, &users); err != nil {
		return 0, err
	}
	if len(users) == 0 {
		return 0, fmt.Errorf("user not found: %s", username)
	}
	return users[0].ID, nil
}

func (g *GitLabProvider) ListUserRepos(ctx context.Context, username string, includeForks bool) ([]*Repository, error) {
	userID, err := g.resolveUserID(ctx, username)
	if err != nil {
		return nil, err
	}

	var allRepos []*Repository
	page := 1

	for {
		path := fmt.Sprintf("/users/%d/projects?page=%d&per_page=100&visibility=public", userID, page)
		body, status, err := g.doRequest(ctx, "GET", path)
		if err != nil {
			return nil, err
		}
		if status != 200 {
			return nil, fmt.Errorf("gitlab API error: %d", status)
		}

		var projects []glProject
		if err := json.Unmarshal(body, &projects); err != nil {
			return nil, err
		}
		if len(projects) == 0 {
			break
		}

		for _, p := range projects {
			isFork := p.ForkedFromPrj != nil
			if !includeForks && isFork {
				continue
			}
			allRepos = append(allRepos, &Repository{
				Owner:    p.Namespace.Path,
				Name:     p.Path,
				FullName: p.PathWithNS,
				Fork:     isFork,
				HTMLURL:  p.WebURL,
			})
		}

		if len(projects) < 100 {
			break
		}
		page++
	}
	return allRepos, nil
}

func (g *GitLabProvider) ListOrgRepos(ctx context.Context, orgName string) ([]*Repository, error) {
	var allRepos []*Repository
	page := 1

	for {
		path := fmt.Sprintf("/groups/%s/projects?page=%d&per_page=100&visibility=public&include_subgroups=true",
			url.PathEscape(orgName), page)
		body, status, err := g.doRequest(ctx, "GET", path)
		if err != nil {
			return nil, err
		}
		if status != 200 {
			return nil, fmt.Errorf("gitlab API error: %d", status)
		}

		var projects []glProject
		if err := json.Unmarshal(body, &projects); err != nil {
			return nil, err
		}
		if len(projects) == 0 {
			break
		}

		for _, p := range projects {
			allRepos = append(allRepos, &Repository{
				Owner:    p.Namespace.Path,
				Name:     p.Path,
				FullName: p.PathWithNS,
				Fork:     p.ForkedFromPrj != nil,
				HTMLURL:  p.WebURL,
			})
		}

		if len(projects) < 100 {
			break
		}
		page++
	}
	return allRepos, nil
}

func (g *GitLabProvider) ListCommits(ctx context.Context, owner, repo string, cfg ScanConfig) ([]models.CommitInfo, error) {
	projectPath := url.PathEscape(owner + "/" + repo)
	perPage := cfg.PerPage
	if perPage > 100 {
		perPage = 100
	}
	if cfg.QuickMode {
		perPage = 50
	}

	var allCommits []models.CommitInfo
	page := 1

	for {
		path := fmt.Sprintf("/projects/%s/repository/commits?page=%d&per_page=%d&with_stats=false",
			projectPath, page, perPage)
		body, status, err := g.doRequest(ctx, "GET", path)
		if err != nil {
			return nil, err
		}
		if status == 404 {
			return nil, fmt.Errorf("project not found: %s/%s", owner, repo)
		}
		if status != 200 {
			return nil, fmt.Errorf("gitlab API error: %d", status)
		}

		var commits []glCommit
		if err := json.Unmarshal(body, &commits); err != nil {
			return nil, err
		}
		if len(commits) == 0 {
			break
		}

		for _, gc := range commits {
			info := models.CommitInfo{
				Hash:           gc.ID,
				URL:            gc.WebURL,
				AuthorName:     gc.AuthorName,
				AuthorEmail:    gc.AuthorEmail,
				CommitterName:  gc.CommitterName,
				CommitterEmail: gc.CommitterEmail,
				Message:        gc.Message,
				RepoName:       fmt.Sprintf("%s/%s", owner, repo),
			}

			if t, err := time.Parse(time.RFC3339, gc.AuthoredDate); err == nil {
				info.AuthorDate = t
			} else if t, err := time.Parse("2006-01-02T15:04:05.000Z", gc.AuthoredDate); err == nil {
				info.AuthorDate = t
			} else if t, err := time.Parse("2006-01-02T15:04:05.000-07:00", gc.AuthoredDate); err == nil {
				info.AuthorDate = t
			} else if t, err := time.Parse("2006-01-02T15:04:05.000+00:00", gc.AuthoredDate); err == nil {
				info.AuthorDate = t
			}

			if t, err := time.Parse(time.RFC3339, gc.CommittedDate); err == nil {
				info.CommitterDate = t
			} else if t, err := time.Parse("2006-01-02T15:04:05.000Z", gc.CommittedDate); err == nil {
				info.CommitterDate = t
			} else if t, err := time.Parse("2006-01-02T15:04:05.000-07:00", gc.CommittedDate); err == nil {
				info.CommitterDate = t
			} else if t, err := time.Parse("2006-01-02T15:04:05.000+00:00", gc.CommittedDate); err == nil {
				info.CommitterDate = t
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

				files, _, err := g.GetCommitDetail(ctx, owner, repo, gc.ID)
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

		if len(commits) < perPage || cfg.QuickMode {
			break
		}
		page++
	}
	return allCommits, nil
}

func (g *GitLabProvider) GetCommitDetail(ctx context.Context, owner, repo, sha string) ([]CommitFile, string, error) {
	projectPath := url.PathEscape(owner + "/" + repo)

	diffPath := fmt.Sprintf("/projects/%s/repository/commits/%s/diff", projectPath, sha)
	body, status, err := g.doRequest(ctx, "GET", diffPath)
	if err != nil {
		return nil, "", err
	}
	if status != 200 {
		return nil, "", fmt.Errorf("gitlab API error: %d", status)
	}

	var diffs []glDiff
	if err := json.Unmarshal(body, &diffs); err != nil {
		return nil, "", err
	}

	var files []CommitFile
	for _, d := range diffs {
		filename := d.NewPath
		if filename == "" {
			filename = d.OldPath
		}
		files = append(files, CommitFile{
			Filename: filename,
			Patch:    d.Diff,
		})
	}
	return files, "", nil
}

func (g *GitLabProvider) SearchCommitsByUser(ctx context.Context, username string, cfg ScanConfig) (map[string]*models.EmailDetails, error) {
	return make(map[string]*models.EmailDetails), nil
}
