package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gnomegl/gitslurp/v2/internal/models"
	"github.com/gnomegl/gitslurp/v2/internal/scanner"
	"github.com/gnomegl/gitslurp/v2/internal/utils"
)

type CodebergProvider struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewCodebergProvider(token string) *CodebergProvider {
	return &CodebergProvider{
		baseURL:    "https://codeberg.org/api/v1",
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *CodebergProvider) Name() Platform { return Codeberg }

func (c *CodebergProvider) doRequest(ctx context.Context, method, path string) ([]byte, int, error) {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}

	resp, err := c.httpClient.Do(req)
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

type giteaUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	FullName  string `json:"full_name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
	Location  string `json:"location"`
	Website   string `json:"website"`
	Bio       string `json:"description"`
	Followers int    `json:"followers_count"`
	Following int    `json:"following_count"`
	Created   string `json:"created"`
}

type giteaOrg struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Website     string `json:"website"`
	Location    string `json:"location"`
	AvatarURL   string `json:"avatar_url"`
}

type giteaRepo struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Fork     bool   `json:"fork"`
	HTMLURL  string `json:"html_url"`
	Owner    struct {
		Login string `json:"login"`
	} `json:"owner"`
}

type giteaCommit struct {
	SHA     string `json:"sha"`
	HTMLURL string `json:"html_url"`
	Commit  struct {
		Message string `json:"message"`
		Author  struct {
			Name  string `json:"name"`
			Email string `json:"email"`
			Date  string `json:"date"`
		} `json:"author"`
		Committer struct {
			Name  string `json:"name"`
			Email string `json:"email"`
			Date  string `json:"date"`
		} `json:"committer"`
	} `json:"commit"`
	Author *struct {
		Login string `json:"login"`
	} `json:"author"`
}

type giteaCommitDetail struct {
	SHA     string `json:"sha"`
	HTMLURL string `json:"html_url"`
	Commit  struct {
		Message string `json:"message"`
	} `json:"commit"`
	Files []struct {
		Filename string `json:"filename"`
		Patch    string `json:"patch"`
	} `json:"files"`
}

func (c *CodebergProvider) GetUser(ctx context.Context, username string) (*UserInfo, error) {
	body, status, err := c.doRequest(ctx, "GET", "/users/"+username)
	if err != nil {
		return nil, err
	}
	if status == 404 {
		return nil, fmt.Errorf("user not found: %s", username)
	}
	if status != 200 {
		return nil, fmt.Errorf("codeberg API error: %d", status)
	}

	var u giteaUser
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, err
	}

	info := &UserInfo{
		Login:     u.Login,
		Name:      u.FullName,
		Email:     u.Email,
		Location:  u.Location,
		Bio:       u.Bio,
		Blog:      u.Website,
		Followers: u.Followers,
		Following: u.Following,
		AvatarURL: u.AvatarURL,
	}
	if t, err := time.Parse(time.RFC3339, u.Created); err == nil {
		info.CreatedAt = t
	}

	repos, _ := c.ListUserRepos(ctx, username, true)
	info.PublicRepos = len(repos)

	return info, nil
}

func (c *CodebergProvider) IsOrganization(ctx context.Context, name string) (bool, error) {
	_, status, err := c.doRequest(ctx, "GET", "/orgs/"+name)
	if err != nil {
		return false, err
	}
	return status == 200, nil
}

func (c *CodebergProvider) UserExists(ctx context.Context, username string) (bool, error) {
	_, status, err := c.doRequest(ctx, "GET", "/users/"+username)
	if err != nil {
		return false, err
	}
	return status == 200, nil
}

func (c *CodebergProvider) ListUserRepos(ctx context.Context, username string, includeForks bool) ([]*Repository, error) {
	var allRepos []*Repository
	page := 1

	for {
		path := fmt.Sprintf("/users/%s/repos?page=%d&limit=50", username, page)
		body, status, err := c.doRequest(ctx, "GET", path)
		if err != nil {
			return nil, err
		}
		if status != 200 {
			return nil, fmt.Errorf("codeberg API error: %d", status)
		}

		var repos []giteaRepo
		if err := json.Unmarshal(body, &repos); err != nil {
			return nil, err
		}
		if len(repos) == 0 {
			break
		}

		for _, r := range repos {
			if !includeForks && r.Fork {
				continue
			}
			allRepos = append(allRepos, &Repository{
				Owner:    r.Owner.Login,
				Name:     r.Name,
				FullName: r.FullName,
				Fork:     r.Fork,
				HTMLURL:  r.HTMLURL,
			})
		}

		if len(repos) < 50 {
			break
		}
		page++
	}
	return allRepos, nil
}

func (c *CodebergProvider) ListOrgRepos(ctx context.Context, orgName string) ([]*Repository, error) {
	var allRepos []*Repository
	page := 1

	for {
		path := fmt.Sprintf("/orgs/%s/repos?page=%d&limit=50", orgName, page)
		body, status, err := c.doRequest(ctx, "GET", path)
		if err != nil {
			return nil, err
		}
		if status != 200 {
			return nil, fmt.Errorf("codeberg API error: %d", status)
		}

		var repos []giteaRepo
		if err := json.Unmarshal(body, &repos); err != nil {
			return nil, err
		}
		if len(repos) == 0 {
			break
		}

		for _, r := range repos {
			allRepos = append(allRepos, &Repository{
				Owner:    r.Owner.Login,
				Name:     r.Name,
				FullName: r.FullName,
				Fork:     r.Fork,
				HTMLURL:  r.HTMLURL,
			})
		}

		if len(repos) < 50 {
			break
		}
		page++
	}
	return allRepos, nil
}

func (c *CodebergProvider) ListCommits(ctx context.Context, owner, repo string, cfg ScanConfig) ([]models.CommitInfo, error) {
	perPage := cfg.PerPage
	if perPage > 50 {
		perPage = 50
	}
	if cfg.QuickMode {
		perPage = 50
	}

	var allCommits []models.CommitInfo
	page := 1

	for {
		path := fmt.Sprintf("/repos/%s/%s/commits?page=%d&limit=%d", owner, repo, page, perPage)
		body, status, err := c.doRequest(ctx, "GET", path)
		if err != nil {
			return nil, err
		}
		if status == 409 {
			return nil, fmt.Errorf("repository is empty")
		}
		if status != 200 {
			return nil, fmt.Errorf("codeberg API error: %d", status)
		}

		var commits []giteaCommit
		if err := json.Unmarshal(body, &commits); err != nil {
			return nil, err
		}
		if len(commits) == 0 {
			break
		}

		for _, gc := range commits {
			info := models.CommitInfo{
				Hash:           gc.SHA,
				URL:            gc.HTMLURL,
				AuthorName:     gc.Commit.Author.Name,
				AuthorEmail:    gc.Commit.Author.Email,
				CommitterName:  gc.Commit.Committer.Name,
				CommitterEmail: gc.Commit.Committer.Email,
				Message:        gc.Commit.Message,
				RepoName:       fmt.Sprintf("%s/%s", owner, repo),
			}

			if gc.Author != nil {
				info.AuthorLogin = gc.Author.Login
			}

			if t, err := time.Parse(time.RFC3339, gc.Commit.Author.Date); err == nil {
				info.AuthorDate = t
			}
			if t, err := time.Parse(time.RFC3339, gc.Commit.Committer.Date); err == nil {
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

				files, _, err := c.GetCommitDetail(ctx, owner, repo, gc.SHA)
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

func (c *CodebergProvider) GetCommitDetail(ctx context.Context, owner, repo, sha string) ([]CommitFile, string, error) {
	path := fmt.Sprintf("/repos/%s/%s/git/commits/%s", owner, repo, sha)
	body, status, err := c.doRequest(ctx, "GET", path)
	if err != nil {
		return nil, "", err
	}
	if status != 200 {
		return nil, "", fmt.Errorf("codeberg API error: %d", status)
	}

	var detail giteaCommitDetail
	if err := json.Unmarshal(body, &detail); err != nil {
		return nil, "", err
	}

	var files []CommitFile
	for _, f := range detail.Files {
		files = append(files, CommitFile{
			Filename: f.Filename,
			Patch:    f.Patch,
		})
	}
	return files, detail.Commit.Message, nil
}

func (c *CodebergProvider) SearchCommitsByUser(ctx context.Context, username string, cfg ScanConfig) (map[string]*models.EmailDetails, error) {
	return make(map[string]*models.EmailDetails), nil
}
