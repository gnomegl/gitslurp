package service

import (
	"context"
	"sort"

	"github.com/fatih/color"
	gh "github.com/google/go-github/v57/github"
)

type RepoEventProcessor struct {
	client *gh.Client
	target string
}

func NewRepoEventProcessor(client *gh.Client, target string) *RepoEventProcessor {
	return &RepoEventProcessor{
		client: client,
		target: target,
	}
}

func (p *RepoEventProcessor) Process(ctx context.Context, repos []*gh.Repository, showStargazers, showForkers bool) error {
	stargazers := make(map[string]struct{})
	forkers := make(map[string]struct{})

	opts := &gh.ListOptions{
		PerPage: 100,
	}

	for _, repo := range repos {
		if showStargazers {
			if err := p.collectStargazers(ctx, repo, stargazers, opts); err != nil {
				continue
			}
		}

		if showForkers {
			if err := p.collectForkers(ctx, repo, forkers, opts); err != nil {
				continue
			}
		}
	}

	orchestrator := &Orchestrator{}

	if showForkers {
		forkersList := sortedKeys(forkers)
		if err := orchestrator.outputEventList(forkersList, p.target+"_forkers.txt", "Repository Forkers:", "🔱"); err != nil {
			return err
		}
	}

	if showStargazers {
		stargazersList := sortedKeys(stargazers)
		if err := orchestrator.outputEventList(stargazersList, p.target+"_stargazers.txt", "Repository Stargazers:", "⭐"); err != nil {
			return err
		}
	}

	return nil
}

func (p *RepoEventProcessor) collectStargazers(ctx context.Context, repo *gh.Repository, stargazers map[string]struct{}, opts *gh.ListOptions) error {
	stargazerList, _, err := p.client.Activity.ListStargazers(ctx, repo.GetOwner().GetLogin(), repo.GetName(), opts)
	if err != nil {
		color.Yellow("[!]  Warning: Could not fetch stargazers for %s: %v", repo.GetFullName(), err)
		return err
	}
	for _, stargazer := range stargazerList {
		stargazers[stargazer.User.GetLogin()] = struct{}{}
	}
	return nil
}

func (p *RepoEventProcessor) collectForkers(ctx context.Context, repo *gh.Repository, forkers map[string]struct{}, opts *gh.ListOptions) error {
	forks, _, err := p.client.Repositories.ListForks(ctx, repo.GetOwner().GetLogin(), repo.GetName(), &gh.RepositoryListForksOptions{
		ListOptions: *opts,
	})
	if err != nil {
		color.Yellow("[!]  Warning: Could not fetch forks for %s: %v", repo.GetFullName(), err)
		return err
	}
	for _, fork := range forks {
		forkers[fork.GetOwner().GetLogin()] = struct{}{}
	}
	return nil
}

func sortedKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}