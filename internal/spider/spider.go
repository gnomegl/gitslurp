package spider

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/gnomegl/gitslurp/internal/github"
	"github.com/schollz/progressbar/v3"
)

type SpiderConfig struct {
	Depth        int
	MaxNodes     int
	MinRepos     int
	MinFollowers int
	MaxWorkers   int
	OutputFile   string
}

type Spider struct {
	pool    *github.ClientPool
	config  SpiderConfig
	graph   *Graph
	filters *Filters
	fetcher *RelationFetcher
	limiter *time.Ticker
}

func NewSpider(pool *github.ClientPool, cfg SpiderConfig) *Spider {
	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = 5
	}
	if cfg.MaxNodes <= 0 {
		cfg.MaxNodes = 500
	}
	if cfg.Depth <= 0 {
		cfg.Depth = 1
	}
	if cfg.Depth > 5 {
		cfg.Depth = 5
	}

	return &Spider{
		pool:   pool,
		config: cfg,
		graph:  NewGraph(),
		filters: &Filters{
			MinRepos:     cfg.MinRepos,
			MinFollowers: cfg.MinFollowers,
			MaxNodes:     cfg.MaxNodes,
		},
		fetcher: NewRelationFetcher(pool),
		limiter: time.NewTicker(100 * time.Millisecond),
	}
}

func (s *Spider) Run(ctx context.Context, seedLogin string) error {
	defer s.limiter.Stop()

	color.Cyan("Starting social graph spider for: %s", seedLogin)
	fmt.Printf("  Depth: %d | Max nodes: %d | Workers: %d\n", s.config.Depth, s.config.MaxNodes, s.config.MaxWorkers)
	if s.config.MinFollowers > 0 || s.config.MinRepos > 0 {
		fmt.Printf("  Filters: min-followers=%d min-repos=%d\n", s.config.MinFollowers, s.config.MinRepos)
	}
	fmt.Println()

	seedNode, err := s.fetcher.FetchUserProfile(ctx, seedLogin)
	if err != nil {
		return fmt.Errorf("failed to fetch seed user profile: %v", err)
	}
	seedNode.Depth = 0
	s.graph.AddNode(seedNode)

	currentLevel := []string{seedLogin}

	for depth := 0; depth < s.config.Depth; depth++ {
		if len(currentLevel) == 0 {
			color.Yellow("[!] No users to process at depth %d, stopping", depth+1)
			break
		}

		if s.filters.NodeLimitReached(s.graph.NodeCount()) {
			color.Yellow("[!] Node limit reached (%d), stopping", s.config.MaxNodes)
			break
		}

		color.Blue("\nDepth %d/%d - Processing %d users...", depth+1, s.config.Depth, len(currentLevel))

		nextLevel := s.processLevel(ctx, currentLevel, depth+1)
		currentLevel = nextLevel

		color.Green("[+] Depth %d complete: %d nodes, %d edges",
			depth+1, s.graph.NodeCount(), s.graph.EdgeCount())
	}

	outputPath := s.config.OutputFile
	if outputPath == "" {
		outputPath = seedLogin + "_graph.gexf"
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer f.Close()

	if err := WriteGEXF(f, s.graph, seedLogin); err != nil {
		return fmt.Errorf("failed to write GEXF: %v", err)
	}

	fmt.Println()
	color.Green("[+] Social graph complete:")
	fmt.Printf("  Nodes: %d\n", s.graph.NodeCount())
	fmt.Printf("  Edges: %d\n", s.graph.EdgeCount())
	fmt.Printf("  Output: %s\n", outputPath)

	s.printEdgeTypeSummary()

	return nil
}

func (s *Spider) processLevel(ctx context.Context, logins []string, nextDepth int) []string {
	type discoveryResult struct {
		login     string
		relations []DiscoveredRelation
	}

	resultsChan := make(chan discoveryResult, len(logins)*10)
	sem := make(chan struct{}, s.config.MaxWorkers)
	var wg sync.WaitGroup

	bar := progressbar.NewOptions(len(logins),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(10),
		progressbar.OptionSetDescription("[cyan]Enumerating relationships[reset]"),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]#[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: "-",
			BarStart:      "[",
			BarEnd:        "]",
		}))

	for _, login := range logins {
		if s.filters.NodeLimitReached(s.graph.NodeCount()) {
			break
		}

		wg.Add(1)
		go func(login string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			relations := s.enumerateUser(ctx, login)
			resultsChan <- discoveryResult{login: login, relations: relations}
			bar.Add(1)
		}(login)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	newUsers := make(map[string]bool)
	for result := range resultsChan {
		for _, rel := range result.relations {
			if rel.Type == "follower" || rel.Type == "stargazer" || rel.Type == "watcher" {
				s.graph.AddEdge(rel.Login, result.login, rel.Type, rel.Repo)
			} else {
				s.graph.AddEdge(result.login, rel.Login, rel.Type, rel.Repo)
			}

			if !s.graph.HasNode(rel.Login) && !s.filters.NodeLimitReached(s.graph.NodeCount()) {
				newUsers[rel.Login] = true
			}
		}
	}

	bar.Finish()

	if len(newUsers) == 0 {
		return nil
	}

	color.Blue("Fetching profiles for %d new users...", len(newUsers))

	profileBar := progressbar.NewOptions(len(newUsers),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(10),
		progressbar.OptionSetDescription("[cyan]Fetching profiles[reset]"),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]#[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: "-",
			BarStart:      "[",
			BarEnd:        "]",
		}))

	var nextLevel []string
	var profileMu sync.Mutex
	var profileWg sync.WaitGroup
	profileSem := make(chan struct{}, s.config.MaxWorkers)

	for login := range newUsers {
		if s.filters.NodeLimitReached(s.graph.NodeCount()) {
			break
		}

		profileWg.Add(1)
		go func(login string) {
			defer profileWg.Done()
			profileSem <- struct{}{}
			defer func() { <-profileSem }()

			<-s.limiter.C
			node, err := s.fetcher.FetchUserProfile(ctx, login)
			if err != nil {
				profileBar.Add(1)
				return
			}

			if !s.filters.PassesUserFilter(node.Followers, node.PublicRepos) {
				profileBar.Add(1)
				return
			}

			node.Depth = nextDepth

			profileMu.Lock()
			added := s.graph.AddNode(node)
			if added {
				nextLevel = append(nextLevel, login)
			}
			profileMu.Unlock()

			profileBar.Add(1)
		}(login)
	}

	profileWg.Wait()
	profileBar.Finish()

	return nextLevel
}

func (s *Spider) enumerateUser(ctx context.Context, login string) []DiscoveredRelation {
	type relResult struct {
		relations []DiscoveredRelation
	}

	ch := make(chan relResult, 7)
	var wg sync.WaitGroup

	fetch := func(fn func() ([]DiscoveredRelation, error)) {
		defer wg.Done()
		<-s.limiter.C
		rels, err := fn()
		if err != nil {
			ch <- relResult{}
			return
		}
		ch <- relResult{relations: rels}
	}

	wg.Add(2)
	go fetch(func() ([]DiscoveredRelation, error) { return s.fetcher.FetchFollowing(ctx, login) })
	go fetch(func() ([]DiscoveredRelation, error) { return s.fetcher.FetchFollowers(ctx, login) })

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-s.limiter.C
		repos, err := s.fetcher.FetchUserRepos(ctx, login)
		if err != nil || len(repos) == 0 {
			return
		}

		maxRepos := 10
		if len(repos) < maxRepos {
			maxRepos = len(repos)
		}

		var repoWg sync.WaitGroup
		for _, repo := range repos[:maxRepos] {
			repoWg.Add(1)
			go func(repo string) {
				defer repoWg.Done()

				<-s.limiter.C
				stargazers, err := s.fetcher.FetchRepoStargazers(ctx, login, repo)
				if err == nil {
					ch <- relResult{relations: stargazers}
				}

				<-s.limiter.C
				watchers, err := s.fetcher.FetchRepoWatchers(ctx, login, repo)
				if err == nil {
					ch <- relResult{relations: watchers}
				}

				<-s.limiter.C
				committers, err := s.fetcher.FetchRepoCommitters(ctx, login, repo)
				if err == nil {
					ch <- relResult{relations: committers}
				}

				<-s.limiter.C
				participants, err := s.fetcher.FetchIssueParticipants(ctx, login, repo)
				if err == nil {
					ch <- relResult{relations: participants}
				}
			}(repo)
		}

		repoWg.Wait()
	}()

	wg.Add(1)
	go fetch(func() ([]DiscoveredRelation, error) { return s.fetcher.FetchStarredRepoOwners(ctx, login) })

	go func() {
		wg.Wait()
		close(ch)
	}()

	var all []DiscoveredRelation
	for result := range ch {
		all = append(all, result.relations...)
	}
	return all
}

func (s *Spider) printEdgeTypeSummary() {
	s.graph.mu.RLock()
	defer s.graph.mu.RUnlock()

	typeCounts := make(map[string]int)
	for _, edge := range s.graph.Edges {
		typeCounts[edge.Type]++
	}

	fmt.Println("\n  Edge types:")
	for edgeType, count := range typeCounts {
		fmt.Printf("    %s: %d\n", edgeType, count)
	}
}
