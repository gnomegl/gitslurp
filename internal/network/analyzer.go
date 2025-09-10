package network

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"git.sr.ht/~gnome/gitslurp/internal/models"
	"github.com/google/go-github/v57/github"
)

type NetworkAnalyzer struct {
	githubClient *github.Client
	cache        map[string]interface{}
	cacheMu      sync.RWMutex
}

func NewNetworkAnalyzer(client *github.Client) *NetworkAnalyzer {
	return &NetworkAnalyzer{
		githubClient: client,
		cache:        make(map[string]interface{}),
	}
}

type UserNetwork struct {
	Username           string
	Followers          []string
	Following          []string
	MutualConnections  []string
	Collaborators      map[string]*CollaboratorInfo
	CommitCoAuthors    map[string]*CoAuthorInfo
	Organizations      []string
	NetworkStrength    map[string]float64
}

type CollaboratorInfo struct {
	Username     string
	Repositories []string
	CommitCount  int
	FirstCollab  time.Time
	LastCollab   time.Time
	Strength     float64
}

type CoAuthorInfo struct {
	Name         string
	Email        string
	Repositories []string
	CommitCount  int
	FirstCommit  time.Time
	LastCommit   time.Time
}

func (n *NetworkAnalyzer) AnalyzeUserNetwork(ctx context.Context, username string) (*UserNetwork, error) {
	network := &UserNetwork{
		Username:        username,
		Collaborators:   make(map[string]*CollaboratorInfo),
		CommitCoAuthors: make(map[string]*CoAuthorInfo),
		NetworkStrength: make(map[string]float64),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	errChan := make(chan error, 4)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := n.fetchFollowers(ctx, username, network, &mu); err != nil {
			errChan <- err
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := n.fetchFollowing(ctx, username, network, &mu); err != nil {
			errChan <- err
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := n.fetchOrganizations(ctx, username, network, &mu); err != nil {
			errChan <- err
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := n.analyzeRepositoryCollaborations(ctx, username, network, &mu); err != nil {
			errChan <- err
		}
	}()

	wg.Wait()
	close(errChan)

	var errs []error
	for err := range errChan {
		if err != nil {
			errs = append(errs, err)
		}
	}

	n.calculateMutualConnections(network)
	n.calculateNetworkStrength(network)

	if len(errs) > 0 {
		return network, fmt.Errorf("partial network analysis completed with errors: %v", errs)
	}

	return network, nil
}

func (n *NetworkAnalyzer) fetchFollowers(ctx context.Context, username string, network *UserNetwork, mu *sync.Mutex) error {
	opt := &github.ListOptions{PerPage: 100}
	var allFollowers []string

	for {
		users, resp, err := n.githubClient.Users.ListFollowers(ctx, username, opt)
		if err != nil {
			return err
		}

		for _, user := range users {
			if user.Login != nil {
				allFollowers = append(allFollowers, *user.Login)
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	mu.Lock()
	network.Followers = allFollowers
	mu.Unlock()

	return nil
}

func (n *NetworkAnalyzer) fetchFollowing(ctx context.Context, username string, network *UserNetwork, mu *sync.Mutex) error {
	opt := &github.ListOptions{PerPage: 100}
	var allFollowing []string

	for {
		users, resp, err := n.githubClient.Users.ListFollowing(ctx, username, opt)
		if err != nil {
			return err
		}

		for _, user := range users {
			if user.Login != nil {
				allFollowing = append(allFollowing, *user.Login)
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	mu.Lock()
	network.Following = allFollowing
	mu.Unlock()

	return nil
}

func (n *NetworkAnalyzer) fetchOrganizations(ctx context.Context, username string, network *UserNetwork, mu *sync.Mutex) error {
	opt := &github.ListOptions{PerPage: 100}
	var allOrgs []string

	for {
		orgs, resp, err := n.githubClient.Organizations.List(ctx, username, opt)
		if err != nil {
			return err
		}

		for _, org := range orgs {
			if org.Login != nil {
				allOrgs = append(allOrgs, *org.Login)
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	mu.Lock()
	network.Organizations = allOrgs
	mu.Unlock()

	return nil
}

func (n *NetworkAnalyzer) analyzeRepositoryCollaborations(ctx context.Context, username string, network *UserNetwork, mu *sync.Mutex) error {
	opt := &github.RepositoryListOptions{
		Type:        "owner",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var allRepos []*github.Repository

	for {
		repos, resp, err := n.githubClient.Repositories.List(ctx, username, opt)
		if err != nil {
			return err
		}

		allRepos = append(allRepos, repos...)

		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	for _, repo := range allRepos {
		if repo.Name == nil {
			continue
		}

		collaborators, _, err := n.githubClient.Repositories.ListCollaborators(ctx, username, *repo.Name, nil)
		if err != nil {
			continue
		}

		for _, collab := range collaborators {
			if collab.Login == nil || *collab.Login == username {
				continue
			}

			mu.Lock()
			if _, exists := network.Collaborators[*collab.Login]; !exists {
				network.Collaborators[*collab.Login] = &CollaboratorInfo{
					Username:     *collab.Login,
					Repositories: []string{},
				}
			}
			network.Collaborators[*collab.Login].Repositories = append(
				network.Collaborators[*collab.Login].Repositories,
				*repo.Name,
			)
			mu.Unlock()
		}
	}

	return nil
}

func (n *NetworkAnalyzer) calculateMutualConnections(network *UserNetwork) {
	followersSet := make(map[string]bool)
	for _, f := range network.Followers {
		followersSet[f] = true
	}

	var mutual []string
	for _, f := range network.Following {
		if followersSet[f] {
			mutual = append(mutual, f)
		}
	}

	network.MutualConnections = mutual
}

func (n *NetworkAnalyzer) calculateNetworkStrength(network *UserNetwork) {
	for username, collab := range network.Collaborators {
		strength := float64(len(collab.Repositories)) * 0.3
		
		if contains(network.MutualConnections, username) {
			strength += 0.4
		} else if contains(network.Followers, username) || contains(network.Following, username) {
			strength += 0.2
		}

		if len(collab.Repositories) > 5 {
			strength += 0.3
		} else if len(collab.Repositories) > 2 {
			strength += 0.1
		}

		if strength > 1.0 {
			strength = 1.0
		}

		collab.Strength = strength
		network.NetworkStrength[username] = strength
	}
}

func (n *NetworkAnalyzer) ExtractCommitCoAuthors(commits []*models.CommitInfo) map[string]*CoAuthorInfo {
	coAuthors := make(map[string]*CoAuthorInfo)
	coAuthorPattern := regexp.MustCompile(`Co-authored-by:\s*([^<]+)<([^>]+)>`)

	for _, commit := range commits {
		matches := coAuthorPattern.FindAllStringSubmatch(commit.Message, -1)
		
		for _, match := range matches {
			if len(match) >= 3 {
				name := strings.TrimSpace(match[1])
				email := strings.TrimSpace(match[2])
				
				if email == commit.AuthorEmail {
					continue
				}

				if _, exists := coAuthors[email]; !exists {
					coAuthors[email] = &CoAuthorInfo{
						Name:         name,
						Email:        email,
						Repositories: []string{},
						FirstCommit:  commit.AuthorDate,
						LastCommit:   commit.AuthorDate,
					}
				}

				coAuthor := coAuthors[email]
				coAuthor.CommitCount++
				
				if !contains(coAuthor.Repositories, commit.RepoName) {
					coAuthor.Repositories = append(coAuthor.Repositories, commit.RepoName)
				}

				if commit.AuthorDate.Before(coAuthor.FirstCommit) {
					coAuthor.FirstCommit = commit.AuthorDate
				}
				if commit.AuthorDate.After(coAuthor.LastCommit) {
					coAuthor.LastCommit = commit.AuthorDate
				}
			}
		}
	}

	return coAuthors
}

type NetworkVisualizer struct {
	analyzer *NetworkAnalyzer
}

func NewNetworkVisualizer(analyzer *NetworkAnalyzer) *NetworkVisualizer {
	return &NetworkVisualizer{
		analyzer: analyzer,
	}
}

func (v *NetworkVisualizer) GenerateNetworkData(network *UserNetwork) *NetworkVisualization {
	viz := &NetworkVisualization{
		Nodes: make([]NetworkNode, 0),
		Edges: make([]NetworkEdge, 0),
	}

	viz.Nodes = append(viz.Nodes, NetworkNode{
		ID:    network.Username,
		Label: network.Username,
		Type:  "primary",
		Size:  30,
	})

	for _, follower := range network.Followers {
		if !v.nodeExists(viz.Nodes, follower) {
			viz.Nodes = append(viz.Nodes, NetworkNode{
				ID:    follower,
				Label: follower,
				Type:  "follower",
				Size:  10,
			})
		}
		viz.Edges = append(viz.Edges, NetworkEdge{
			From:   follower,
			To:     network.Username,
			Type:   "follows",
			Weight: 1,
		})
	}

	for _, following := range network.Following {
		if !v.nodeExists(viz.Nodes, following) {
			viz.Nodes = append(viz.Nodes, NetworkNode{
				ID:    following,
				Label: following,
				Type:  "following",
				Size:  10,
			})
		}
		viz.Edges = append(viz.Edges, NetworkEdge{
			From:   network.Username,
			To:     following,
			Type:   "follows",
			Weight: 1,
		})
	}

	for username, collab := range network.Collaborators {
		if !v.nodeExists(viz.Nodes, username) {
			viz.Nodes = append(viz.Nodes, NetworkNode{
				ID:    username,
				Label: username,
				Type:  "collaborator",
				Size:  15 + len(collab.Repositories),
			})
		}
		viz.Edges = append(viz.Edges, NetworkEdge{
			From:   network.Username,
			To:     username,
			Type:   "collaborates",
			Weight: len(collab.Repositories),
		})
	}

	return viz
}

func (v *NetworkVisualizer) nodeExists(nodes []NetworkNode, id string) bool {
	for _, node := range nodes {
		if node.ID == id {
			return true
		}
	}
	return false
}

type NetworkVisualization struct {
	Nodes []NetworkNode
	Edges []NetworkEdge
}

type NetworkNode struct {
	ID    string
	Label string
	Type  string
	Size  int
}

type NetworkEdge struct {
	From   string
	To     string
	Type   string
	Weight int
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

type CollaborationScorer struct{}

func NewCollaborationScorer() *CollaborationScorer {
	return &CollaborationScorer{}
}

func (s *CollaborationScorer) ScoreCollaborations(network *UserNetwork) []CollaborationScore {
	scores := make([]CollaborationScore, 0, len(network.Collaborators))

	for username, collab := range network.Collaborators {
		score := CollaborationScore{
			Username:     username,
			Repositories: collab.Repositories,
			Score:        0.0,
		}

		score.Score += float64(len(collab.Repositories)) * 0.2

		if contains(network.MutualConnections, username) {
			score.Score += 0.3
			score.IsMutual = true
		}

		if collab.CommitCount > 50 {
			score.Score += 0.3
		} else if collab.CommitCount > 20 {
			score.Score += 0.2
		} else if collab.CommitCount > 5 {
			score.Score += 0.1
		}

		for _, org := range network.Organizations {
			if strings.Contains(username, org) {
				score.Score += 0.1
				score.SharedOrg = true
				break
			}
		}

		if score.Score > 1.0 {
			score.Score = 1.0
		}

		scores = append(scores, score)
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	return scores
}

type CollaborationScore struct {
	Username     string
	Repositories []string
	Score        float64
	IsMutual     bool
	SharedOrg    bool
}
