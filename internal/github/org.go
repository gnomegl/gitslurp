package github

import (
	"context"

	"github.com/google/go-github/v57/github"
)

type RepoContributors struct {
	RepoName     string
	Contributors []*ContributorInfo
}

type ContributorInfo struct {
	Login          string
	Commits        int
	HasWriteAccess bool
}

func AnalyzeOrgRepositories(ctx context.Context, client *github.Client, orgName string) ([]*RepoContributors, error) {
	opt := &github.RepositoryListByOrgOptions{
		Type:        "public",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var allRepoContributors []*RepoContributors

	for {
		repos, resp, err := client.Repositories.ListByOrg(ctx, orgName, opt)
		if err != nil {
			return nil, err
		}

		for _, repo := range repos {
			repoContribs := &RepoContributors{
				RepoName: *repo.Name,
			}

			contributors, _, err := client.Repositories.ListContributors(ctx, orgName, *repo.Name, &github.ListContributorsOptions{
				ListOptions: github.ListOptions{PerPage: 100},
			})
			if err != nil {
				continue
			}

			// check for collaborators with write access
			collaborators, _, err := client.Repositories.ListCollaborators(ctx, orgName, *repo.Name, &github.ListCollaboratorsOptions{
				ListOptions: github.ListOptions{PerPage: 100},
			})
			if err != nil {
				continue
			}

			// make a map of collaborators with write access
			writeAccessMap := make(map[string]bool)
			for _, collab := range collaborators {
				// check if the collaborator has write access (direct push)
				if collab.Permissions != nil && (collab.Permissions["push"] || collab.Permissions["admin"]) {
					writeAccessMap[*collab.Login] = true
				}
			}

			for _, contributor := range contributors {
				if contributor.Login == nil || contributor.Contributions == nil {
					continue
				}

				repoContribs.Contributors = append(repoContribs.Contributors, &ContributorInfo{
					Login:          *contributor.Login,
					Commits:        *contributor.Contributions,
					HasWriteAccess: writeAccessMap[*contributor.Login],
				})
			}

			allRepoContributors = append(allRepoContributors, repoContribs)
		}

		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return allRepoContributors, nil
}
