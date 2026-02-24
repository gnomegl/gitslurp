package spider

import (
	"context"
	"strings"

	"github.com/gnomegl/gitslurp/internal/github"
	gh "github.com/google/go-github/v57/github"
)

type RelationFetcher struct {
	pool *github.ClientPool
}

func NewRelationFetcher(pool *github.ClientPool) *RelationFetcher {
	return &RelationFetcher{pool: pool}
}

type DiscoveredRelation struct {
	Login string
	Type  string
	Repo  string
}

func (rf *RelationFetcher) FetchFollowing(ctx context.Context, login string) ([]DiscoveredRelation, error) {
	var relations []DiscoveredRelation
	mc := rf.pool.GetClient()
	opts := &gh.ListOptions{PerPage: 100}

	for {
		users, resp, err := mc.Client.Users.ListFollowing(ctx, login, opts)
		if resp != nil {
			mc.UpdateRateLimit(resp.Rate.Remaining, resp.Rate.Reset.Time)
		}
		if err != nil {
			return relations, err
		}
		for _, u := range users {
			relations = append(relations, DiscoveredRelation{
				Login: u.GetLogin(),
				Type:  "follows",
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return relations, nil
}

func (rf *RelationFetcher) FetchFollowers(ctx context.Context, login string) ([]DiscoveredRelation, error) {
	var relations []DiscoveredRelation
	mc := rf.pool.GetClient()
	opts := &gh.ListOptions{PerPage: 100}

	for {
		users, resp, err := mc.Client.Users.ListFollowers(ctx, login, opts)
		if resp != nil {
			mc.UpdateRateLimit(resp.Rate.Remaining, resp.Rate.Reset.Time)
		}
		if err != nil {
			return relations, err
		}
		for _, u := range users {
			relations = append(relations, DiscoveredRelation{
				Login: u.GetLogin(),
				Type:  "follower",
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return relations, nil
}

func (rf *RelationFetcher) FetchStarredRepoOwners(ctx context.Context, login string) ([]DiscoveredRelation, error) {
	var relations []DiscoveredRelation
	mc := rf.pool.GetClient()
	opts := &gh.ActivityListStarredOptions{
		ListOptions: gh.ListOptions{PerPage: 100},
	}

	for {
		starred, resp, err := mc.Client.Activity.ListStarred(ctx, login, opts)
		if resp != nil {
			mc.UpdateRateLimit(resp.Rate.Remaining, resp.Rate.Reset.Time)
		}
		if err != nil {
			return relations, err
		}
		for _, s := range starred {
			owner := s.GetRepository().GetOwner().GetLogin()
			if owner != "" && owner != login {
				relations = append(relations, DiscoveredRelation{
					Login: owner,
					Type:  "starred",
					Repo:  s.GetRepository().GetFullName(),
				})
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return relations, nil
}

func (rf *RelationFetcher) FetchRepoStargazers(ctx context.Context, owner, repo string) ([]DiscoveredRelation, error) {
	var relations []DiscoveredRelation
	mc := rf.pool.GetClient()
	opts := &gh.ListOptions{PerPage: 100}

	stargazers, resp, err := mc.Client.Activity.ListStargazers(ctx, owner, repo, opts)
	if resp != nil {
		mc.UpdateRateLimit(resp.Rate.Remaining, resp.Rate.Reset.Time)
	}
	if err != nil {
		return relations, err
	}
	for _, s := range stargazers {
		login := s.User.GetLogin()
		if login != "" && login != owner {
			relations = append(relations, DiscoveredRelation{
				Login: login,
				Type:  "stargazer",
				Repo:  owner + "/" + repo,
			})
		}
	}
	return relations, nil
}

func (rf *RelationFetcher) FetchRepoWatchers(ctx context.Context, owner, repo string) ([]DiscoveredRelation, error) {
	var relations []DiscoveredRelation
	mc := rf.pool.GetClient()
	opts := &gh.ListOptions{PerPage: 100}

	watchers, resp, err := mc.Client.Activity.ListWatchers(ctx, owner, repo, opts)
	if resp != nil {
		mc.UpdateRateLimit(resp.Rate.Remaining, resp.Rate.Reset.Time)
	}
	if err != nil {
		return relations, err
	}
	for _, w := range watchers {
		login := w.GetLogin()
		if login != "" && login != owner {
			relations = append(relations, DiscoveredRelation{
				Login: login,
				Type:  "watcher",
				Repo:  owner + "/" + repo,
			})
		}
	}
	return relations, nil
}

func (rf *RelationFetcher) FetchRepoCommitters(ctx context.Context, owner, repo string) ([]DiscoveredRelation, error) {
	var relations []DiscoveredRelation
	mc := rf.pool.GetClient()
	opts := &gh.CommitsListOptions{
		ListOptions: gh.ListOptions{PerPage: 100},
	}

	commits, resp, err := mc.Client.Repositories.ListCommits(ctx, owner, repo, opts)
	if resp != nil {
		mc.UpdateRateLimit(resp.Rate.Remaining, resp.Rate.Reset.Time)
	}
	if err != nil {
		return relations, err
	}

	seen := make(map[string]bool)
	for _, c := range commits {
		if c.Author != nil {
			login := c.Author.GetLogin()
			if login != "" && login != owner && !seen[login] {
				seen[login] = true
				relations = append(relations, DiscoveredRelation{
					Login: login,
					Type:  "commit",
					Repo:  owner + "/" + repo,
				})
			}
		}
	}
	return relations, nil
}

func (rf *RelationFetcher) FetchIssueParticipants(ctx context.Context, owner, repo string) ([]DiscoveredRelation, error) {
	var relations []DiscoveredRelation
	mc := rf.pool.GetClient()

	issues, resp, err := mc.Client.Issues.ListByRepo(ctx, owner, repo, &gh.IssueListByRepoOptions{
		State:       "all",
		Sort:        "updated",
		ListOptions: gh.ListOptions{PerPage: 30},
	})
	if resp != nil {
		mc.UpdateRateLimit(resp.Rate.Remaining, resp.Rate.Reset.Time)
	}
	if err != nil {
		return relations, err
	}

	seen := make(map[string]bool)
	for _, issue := range issues {
		if issue.IsPullRequest() {
			continue
		}

		if issue.User != nil {
			login := issue.User.GetLogin()
			if login != "" && login != owner && !seen[login] {
				seen[login] = true
				relations = append(relations, DiscoveredRelation{
					Login: login,
					Type:  "issue",
					Repo:  owner + "/" + repo,
				})
			}
		}

		for _, assignee := range issue.Assignees {
			login := assignee.GetLogin()
			if login != "" && login != owner && !seen[login] {
				seen[login] = true
				relations = append(relations, DiscoveredRelation{
					Login: login,
					Type:  "issue",
					Repo:  owner + "/" + repo,
				})
			}
		}
	}
	return relations, nil
}

func (rf *RelationFetcher) FetchUserRepos(ctx context.Context, login string) ([]string, error) {
	var repoNames []string
	mc := rf.pool.GetClient()
	opts := &gh.RepositoryListByUserOptions{
		Type:        "owner",
		Sort:        "updated",
		ListOptions: gh.ListOptions{PerPage: 100},
	}

	repos, resp, err := mc.Client.Repositories.ListByUser(ctx, login, opts)
	if resp != nil {
		mc.UpdateRateLimit(resp.Rate.Remaining, resp.Rate.Reset.Time)
	}
	if err != nil {
		return nil, err
	}

	for _, r := range repos {
		if !r.GetFork() {
			repoNames = append(repoNames, r.GetName())
		}
	}
	return repoNames, nil
}

func (rf *RelationFetcher) FetchUserProfile(ctx context.Context, login string) (*Node, error) {
	mc := rf.pool.GetClient()
	user, resp, err := mc.Client.Users.Get(ctx, login)
	if resp != nil {
		mc.UpdateRateLimit(resp.Rate.Remaining, resp.Rate.Reset.Time)
	}
	if err != nil {
		return nil, err
	}

	return &Node{
		Login:       user.GetLogin(),
		Name:        user.GetName(),
		AvatarURL:   user.GetAvatarURL(),
		Followers:   user.GetFollowers(),
		Following:   user.GetFollowing(),
		PublicRepos: user.GetPublicRepos(),
		Company:     user.GetCompany(),
		Location:    user.GetLocation(),
		Bio:         strings.ReplaceAll(user.GetBio(), "\n", " "),
	}, nil
}
