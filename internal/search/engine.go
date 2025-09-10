package search

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"git.sr.ht/~gnome/gitslurp/internal/cache"
	"git.sr.ht/~gnome/gitslurp/internal/domains"
	"git.sr.ht/~gnome/gitslurp/internal/email"
	"git.sr.ht/~gnome/gitslurp/internal/github"
	"git.sr.ht/~gnome/gitslurp/internal/network"
	"git.sr.ht/~gnome/gitslurp/internal/social"
	githubv3 "github.com/google/go-github/v57/github"
)

type SearchModule interface {
	Search(ctx context.Context, query string) (SearchResult, error)
	Name() string
}

type SearchResult struct {
	Module     string
	Query      string
	Results    []interface{}
	Count      int
	Error      error
	Timestamp  time.Time
	Confidence float64
}

type MultiModalSearchEngine struct {
	modules      []SearchModule
	cache        cache.Cache
	concurrency  int
	timeout      time.Duration
}

func NewMultiModalSearchEngine(githubClient *githubv3.Client) *MultiModalSearchEngine {
	memCache := cache.NewMemoryCache()
	
	return &MultiModalSearchEngine{
		modules: []SearchModule{
			NewGitHubSearchModule(githubClient),
			NewEmailSearchModule(),
			NewSocialSearchModule(),
			NewDomainSearchModule(),
		},
		cache:       memCache,
		concurrency: 4,
		timeout:     30 * time.Second,
	}
}

func (e *MultiModalSearchEngine) Search(ctx context.Context, query string) (*ComprehensiveSearchResult, error) {
	cacheKey := cache.NewCacheKeyBuilder().Add("search").Add(query).Build()
	
	if cached, found := e.cache.Get(cacheKey); found {
		if result, ok := cached.(*ComprehensiveSearchResult); ok {
			return result, nil
		}
	}

	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	resultsChan := make(chan SearchResult, len(e.modules))
	var wg sync.WaitGroup

	semaphore := make(chan struct{}, e.concurrency)

	for _, module := range e.modules {
		wg.Add(1)
		go func(m SearchModule) {
			defer wg.Done()
			
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result, err := m.Search(ctx, query)
			if err != nil {
				result.Error = err
			}
			result.Module = m.Name()
			result.Query = query
			result.Timestamp = time.Now()
			
			resultsChan <- result
		}(module)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	searchResults := make([]SearchResult, 0, len(e.modules))
	for result := range resultsChan {
		searchResults = append(searchResults, result)
	}

	comprehensive := e.correlateResults(query, searchResults)
	
	e.cache.Set(cacheKey, comprehensive, 15*time.Minute)
	
	return comprehensive, nil
}

func (e *MultiModalSearchEngine) correlateResults(query string, results []SearchResult) *ComprehensiveSearchResult {
	comprehensive := &ComprehensiveSearchResult{
		Query:            query,
		ModuleResults:    make(map[string]SearchResult),
		Correlations:     make(map[string][]string),
		Identifiers:      make(map[string][]string),
		ConfidenceScores: make(map[string]float64),
		Timestamp:        time.Now(),
	}

	for _, result := range results {
		comprehensive.ModuleResults[result.Module] = result
		
		identifiers := e.extractIdentifiers(result)
		for idType, ids := range identifiers {
			comprehensive.Identifiers[idType] = append(comprehensive.Identifiers[idType], ids...)
		}
	}

	comprehensive.Correlations = e.findCorrelations(comprehensive.Identifiers)
	comprehensive.OverallConfidence = e.calculateOverallConfidence(results)

	return comprehensive
}

func (e *MultiModalSearchEngine) extractIdentifiers(result SearchResult) map[string][]string {
	identifiers := make(map[string][]string)

	switch result.Module {
	case "github":
		for _, item := range result.Results {
			if user, ok := item.(map[string]interface{}); ok {
				if username, ok := user["login"].(string); ok {
					identifiers["usernames"] = append(identifiers["usernames"], username)
				}
				if email, ok := user["email"].(string); ok && email != "" {
					identifiers["emails"] = append(identifiers["emails"], email)
				}
			}
		}
	case "email":
		for _, item := range result.Results {
			if email, ok := item.(string); ok {
				identifiers["emails"] = append(identifiers["emails"], email)
			}
		}
	case "social":
		for _, item := range result.Results {
			if profile, ok := item.(map[string]interface{}); ok {
				if username, ok := profile["username"].(string); ok {
					identifiers["usernames"] = append(identifiers["usernames"], username)
				}
			}
		}
	case "domain":
		for _, item := range result.Results {
			if domain, ok := item.(string); ok {
				identifiers["domains"] = append(identifiers["domains"], domain)
			}
		}
	}

	return identifiers
}

func (e *MultiModalSearchEngine) findCorrelations(identifiers map[string][]string) map[string][]string {
	correlations := make(map[string][]string)

	for idType, ids := range identifiers {
		uniqueIds := make(map[string]bool)
		for _, id := range ids {
			uniqueIds[strings.ToLower(id)] = true
		}
		
		for id := range uniqueIds {
			correlations[id] = append(correlations[id], idType)
		}
	}

	return correlations
}

func (e *MultiModalSearchEngine) calculateOverallConfidence(results []SearchResult) float64 {
	if len(results) == 0 {
		return 0.0
	}

	totalConfidence := 0.0
	validResults := 0

	for _, result := range results {
		if result.Error == nil && result.Count > 0 {
			totalConfidence += result.Confidence
			validResults++
		}
	}

	if validResults == 0 {
		return 0.0
	}

	return totalConfidence / float64(validResults)
}

type ComprehensiveSearchResult struct {
	Query             string
	ModuleResults     map[string]SearchResult
	Correlations      map[string][]string
	Identifiers       map[string][]string
	ConfidenceScores  map[string]float64
	OverallConfidence float64
	Timestamp         time.Time
}

type GitHubSearchModule struct {
	client *githubv3.Client
}

func NewGitHubSearchModule(client *githubv3.Client) *GitHubSearchModule {
	return &GitHubSearchModule{client: client}
}

func (g *GitHubSearchModule) Name() string {
	return "github"
}

func (g *GitHubSearchModule) Search(ctx context.Context, query string) (SearchResult, error) {
	result := SearchResult{
		Module:    g.Name(),
		Query:     query,
		Results:   make([]interface{}, 0),
		Timestamp: time.Now(),
	}

	opts := &githubv3.SearchOptions{
		ListOptions: githubv3.ListOptions{PerPage: 30},
	}

	users, _, err := g.client.Search.Users(ctx, query, opts)
	if err != nil {
		result.Error = err
		return result, err
	}

	for _, user := range users.Users {
		userMap := make(map[string]interface{})
		if user.Login != nil {
			userMap["login"] = *user.Login
		}
		if user.Email != nil {
			userMap["email"] = *user.Email
		}
		if user.Name != nil {
			userMap["name"] = *user.Name
		}
		if user.Company != nil {
			userMap["company"] = *user.Company
		}
		if user.Location != nil {
			userMap["location"] = *user.Location
		}
		if user.Bio != nil {
			userMap["bio"] = *user.Bio
		}
		result.Results = append(result.Results, userMap)
	}

	result.Count = len(result.Results)
	if result.Count > 0 {
		result.Confidence = 1.0
	}

	return result, nil
}

type EmailSearchModule struct {
	generator *email.EmailGenerator
	validator *email.Validator
}

func NewEmailSearchModule() *EmailSearchModule {
	return &EmailSearchModule{
		generator: email.NewEmailGenerator(),
		validator: email.NewValidator(),
	}
}

func (e *EmailSearchModule) Name() string {
	return "email"
}

func (e *EmailSearchModule) Search(ctx context.Context, query string) (SearchResult, error) {
	result := SearchResult{
		Module:    e.Name(),
		Query:     query,
		Results:   make([]interface{}, 0),
		Timestamp: time.Now(),
	}

	parts := strings.Fields(query)
	var firstName, lastName, username string

	if len(parts) >= 2 {
		firstName = parts[0]
		lastName = parts[len(parts)-1]
	}
	username = strings.ToLower(strings.ReplaceAll(query, " ", ""))

	generatedEmails := e.generator.Generate(firstName, lastName, username, nil, true)
	
	validatedCount := 0
	for _, email := range generatedEmails {
		validationResult, err := e.validator.ValidateEmail(ctx, email)
		if err == nil && validationResult.Valid {
			result.Results = append(result.Results, email)
			validatedCount++
		}
	}

	result.Count = len(result.Results)
	if len(generatedEmails) > 0 {
		result.Confidence = float64(validatedCount) / float64(len(generatedEmails))
	}

	return result, nil
}

type SocialSearchModule struct {
	checker *social.Checker
}

func NewSocialSearchModule() *SocialSearchModule {
	return &SocialSearchModule{
		checker: social.NewChecker(),
	}
}

func (s *SocialSearchModule) Name() string {
	return "social"
}

func (s *SocialSearchModule) Search(ctx context.Context, query string) (SearchResult, error) {
	result := SearchResult{
		Module:    s.Name(),
		Query:     query,
		Results:   make([]interface{}, 0),
		Timestamp: time.Now(),
	}

	username := strings.ToLower(strings.ReplaceAll(query, " ", ""))
	checkResults := s.checker.CheckUsername(ctx, username)

	foundCount := 0
	for _, check := range checkResults {
		if check.Exists && check.Error == nil {
			profile := map[string]interface{}{
				"platform": check.Platform,
				"url":      check.URL,
				"username": username,
				"exists":   true,
			}
			result.Results = append(result.Results, profile)
			foundCount++
		}
	}

	result.Count = foundCount
	if len(checkResults) > 0 {
		result.Confidence = float64(foundCount) / float64(len(checkResults))
	}

	return result, nil
}

type DomainSearchModule struct {
	finder *domains.Finder
}

func NewDomainSearchModule() *DomainSearchModule {
	return &DomainSearchModule{
		finder: domains.NewFinder(),
	}
}

func (d *DomainSearchModule) Name() string {
	return "domain"
}

func (d *DomainSearchModule) Search(ctx context.Context, query string) (SearchResult, error) {
	result := SearchResult{
		Module:    d.Name(),
		Query:     query,
		Results:   make([]interface{}, 0),
		Timestamp: time.Now(),
	}

	username := strings.ToLower(strings.ReplaceAll(query, " ", ""))
	
	githubPages, err := d.finder.FindGitHubPages(ctx, username)
	if err == nil {
		for _, page := range githubPages {
			if page.Verified {
				result.Results = append(result.Results, page.Domain)
			}
		}
	}

	result.Count = len(result.Results)
	if result.Count > 0 {
		result.Confidence = 1.0
	}

	return result, nil
}

type SearchAggregator struct {
	engine *MultiModalSearchEngine
}

func NewSearchAggregator(githubClient *githubv3.Client) *SearchAggregator {
	return &SearchAggregator{
		engine: NewMultiModalSearchEngine(githubClient),
	}
}

func (a *SearchAggregator) AggregateSearchResults(ctx context.Context, queries []string) (*AggregatedSearchResult, error) {
	aggregated := &AggregatedSearchResult{
		Queries:           queries,
		CombinedResults:   make(map[string][]interface{}),
		UniqueIdentifiers: make(map[string]map[string]bool),
		Timestamp:         time.Now(),
	}

	for _, query := range queries {
		result, err := a.engine.Search(ctx, query)
		if err != nil {
			continue
		}

		for module, moduleResult := range result.ModuleResults {
			aggregated.CombinedResults[module] = append(
				aggregated.CombinedResults[module],
				moduleResult.Results...,
			)
		}

		for idType, ids := range result.Identifiers {
			if _, exists := aggregated.UniqueIdentifiers[idType]; !exists {
				aggregated.UniqueIdentifiers[idType] = make(map[string]bool)
			}
			for _, id := range ids {
				aggregated.UniqueIdentifiers[idType][strings.ToLower(id)] = true
			}
		}
	}

	return aggregated, nil
}

type AggregatedSearchResult struct {
	Queries           []string
	CombinedResults   map[string][]interface{}
	UniqueIdentifiers map[string]map[string]bool
	Timestamp         time.Time
}