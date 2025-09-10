package social

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Platform struct {
	Name        string
	URLTemplate string
	CheckFunc   func(ctx context.Context, client *http.Client, url string) (bool, error)
}

var DefaultPlatforms = []Platform{
	{
		Name:        "Twitter",
		URLTemplate: "https://twitter.com/%s",
		CheckFunc:   checkHTTPStatus,
	},
	{
		Name:        "Instagram",
		URLTemplate: "https://instagram.com/%s",
		CheckFunc:   checkHTTPStatus,
	},
	{
		Name:        "LinkedIn",
		URLTemplate: "https://linkedin.com/in/%s",
		CheckFunc:   checkHTTPStatus,
	},
	{
		Name:        "Reddit",
		URLTemplate: "https://reddit.com/user/%s",
		CheckFunc:   checkHTTPStatus,
	},
	{
		Name:        "Medium",
		URLTemplate: "https://medium.com/@%s",
		CheckFunc:   checkHTTPStatus,
	},
	{
		Name:        "Dev.to",
		URLTemplate: "https://dev.to/%s",
		CheckFunc:   checkHTTPStatus,
	},
	{
		Name:        "StackOverflow",
		URLTemplate: "https://stackoverflow.com/users/%s",
		CheckFunc:   checkHTTPStatus,
	},
	{
		Name:        "GitLab",
		URLTemplate: "https://gitlab.com/%s",
		CheckFunc:   checkHTTPStatus,
	},
	{
		Name:        "Bitbucket",
		URLTemplate: "https://bitbucket.org/%s",
		CheckFunc:   checkHTTPStatus,
	},
	{
		Name:        "Keybase",
		URLTemplate: "https://keybase.io/%s",
		CheckFunc:   checkHTTPStatus,
	},
	{
		Name:        "HackerNews",
		URLTemplate: "https://news.ycombinator.com/user?id=%s",
		CheckFunc:   checkHTTPStatus,
	},
	{
		Name:        "Docker Hub",
		URLTemplate: "https://hub.docker.com/u/%s",
		CheckFunc:   checkHTTPStatus,
	},
}

type CheckResult struct {
	Platform  string
	URL       string
	Exists    bool
	Error     error
	Timestamp time.Time
}

type Checker struct {
	client      *http.Client
	platforms   []Platform
	concurrency int
	userAgent   string
}

func NewChecker() *Checker {
	return &Checker{
		client: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
		platforms:   DefaultPlatforms,
		concurrency: 5,
		userAgent:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
	}
}

func (c *Checker) CheckUsername(ctx context.Context, username string) []CheckResult {
	results := make([]CheckResult, 0, len(c.platforms))
	resultsChan := make(chan CheckResult, len(c.platforms))

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, c.concurrency)

	for _, platform := range c.platforms {
		wg.Add(1)
		go func(p Platform) {
			defer wg.Done()
			
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			url := fmt.Sprintf(p.URLTemplate, username)
			exists, err := c.checkPlatform(ctx, p, url)
			
			resultsChan <- CheckResult{
				Platform:  p.Name,
				URL:       url,
				Exists:    exists,
				Error:     err,
				Timestamp: time.Now(),
			}
		}(platform)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	for result := range resultsChan {
		results = append(results, result)
	}

	return results
}

func (c *Checker) checkPlatform(ctx context.Context, platform Platform, url string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, err
	}

	req.Header.Set("User-Agent", c.userAgent)

	return platform.CheckFunc(ctx, c.client, url)
}

func checkHTTPStatus(ctx context.Context, client *http.Client, url string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusFound, nil
}

func (c *Checker) AddCustomPlatform(platform Platform) {
	c.platforms = append(c.platforms, platform)
}

func (c *Checker) SetConcurrency(n int) {
	if n > 0 {
		c.concurrency = n
	}
}

func (c *Checker) SetUserAgent(ua string) {
	c.userAgent = ua
}

type UserAgentRotator struct {
	agents []string
	index  int
	mu     sync.Mutex
}

func NewUserAgentRotator() *UserAgentRotator {
	return &UserAgentRotator{
		agents: []string{
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
			"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:91.0) Gecko/20100101",
		},
	}
}

func (r *UserAgentRotator) Next() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	agent := r.agents[r.index]
	r.index = (r.index + 1) % len(r.agents)
	return agent
}

type ProfileAggregator struct {
	checker *Checker
}

func NewProfileAggregator() *ProfileAggregator {
	return &ProfileAggregator{
		checker: NewChecker(),
	}
}

func (a *ProfileAggregator) AggregateProfiles(ctx context.Context, usernames []string) map[string][]CheckResult {
	results := make(map[string][]CheckResult)
	mu := sync.Mutex{}

	var wg sync.WaitGroup
	for _, username := range usernames {
		wg.Add(1)
		go func(user string) {
			defer wg.Done()
			
			userResults := a.checker.CheckUsername(ctx, user)
			
			mu.Lock()
			results[user] = userResults
			mu.Unlock()
		}(username)
	}

	wg.Wait()
	return results
}

func FilterExistingProfiles(results []CheckResult) []CheckResult {
	existing := make([]CheckResult, 0)
	for _, result := range results {
		if result.Exists && result.Error == nil {
			existing = append(existing, result)
		}
	}
	return existing
}