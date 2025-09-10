package social

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type DiscoveryResult struct {
	Username     string
	Platform     string
	ProfileURL   string
	DisplayName  string
	Bio          string
	AvatarURL    string
	Followers    int
	Following    int
	PublicRepos  int
	Verified     bool
	CreatedAt    time.Time
	LastActivity time.Time
	Metadata     map[string]interface{}
}

type Discovery struct {
	httpClient *http.Client
	cache      map[string]*DiscoveryResult
	cacheMu    sync.RWMutex
}

func NewDiscovery() *Discovery {
	return &Discovery{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		cache: make(map[string]*DiscoveryResult),
	}
}

func (d *Discovery) DiscoverProfile(ctx context.Context, platform, username string) (*DiscoveryResult, error) {
	cacheKey := fmt.Sprintf("%s:%s", platform, username)
	
	d.cacheMu.RLock()
	if cached, exists := d.cache[cacheKey]; exists {
		d.cacheMu.RUnlock()
		return cached, nil
	}
	d.cacheMu.RUnlock()

	var result *DiscoveryResult
	var err error

	switch strings.ToLower(platform) {
	case "github":
		result, err = d.discoverGitHub(ctx, username)
	case "gitlab":
		result, err = d.discoverGitLab(ctx, username)
	case "twitter":
		result, err = d.discoverTwitter(ctx, username)
	default:
		return nil, fmt.Errorf("unsupported platform: %s", platform)
	}

	if err == nil && result != nil {
		d.cacheMu.Lock()
		d.cache[cacheKey] = result
		d.cacheMu.Unlock()
	}

	return result, err
}

func (d *Discovery) discoverGitHub(ctx context.Context, username string) (*DiscoveryResult, error) {
	url := fmt.Sprintf("https://api.github.com/users/%s", username)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var data struct {
		Login       string    `json:"login"`
		Name        string    `json:"name"`
		Bio         string    `json:"bio"`
		AvatarURL   string    `json:"avatar_url"`
		Followers   int       `json:"followers"`
		Following   int       `json:"following"`
		PublicRepos int       `json:"public_repos"`
		CreatedAt   time.Time `json:"created_at"`
		UpdatedAt   time.Time `json:"updated_at"`
		Email       string    `json:"email"`
		Location    string    `json:"location"`
		Company     string    `json:"company"`
		Blog        string    `json:"blog"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	result := &DiscoveryResult{
		Username:     data.Login,
		Platform:     "GitHub",
		ProfileURL:   fmt.Sprintf("https://github.com/%s", data.Login),
		DisplayName:  data.Name,
		Bio:          data.Bio,
		AvatarURL:    data.AvatarURL,
		Followers:    data.Followers,
		Following:    data.Following,
		PublicRepos:  data.PublicRepos,
		CreatedAt:    data.CreatedAt,
		LastActivity: data.UpdatedAt,
		Metadata: map[string]interface{}{
			"email":    data.Email,
			"location": data.Location,
			"company":  data.Company,
			"blog":     data.Blog,
		},
	}

	return result, nil
}

func (d *Discovery) discoverGitLab(ctx context.Context, username string) (*DiscoveryResult, error) {
	url := fmt.Sprintf("https://gitlab.com/api/v4/users?username=%s", username)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitLab API returned status %d", resp.StatusCode)
	}

	var users []struct {
		ID            int       `json:"id"`
		Username      string    `json:"username"`
		Name          string    `json:"name"`
		Bio           string    `json:"bio"`
		AvatarURL     string    `json:"avatar_url"`
		WebURL        string    `json:"web_url"`
		CreatedAt     time.Time `json:"created_at"`
		PublicEmail   string    `json:"public_email"`
		Location      string    `json:"location"`
		Organization  string    `json:"organization"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return nil, err
	}

	if len(users) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	user := users[0]
	result := &DiscoveryResult{
		Username:    user.Username,
		Platform:    "GitLab",
		ProfileURL:  user.WebURL,
		DisplayName: user.Name,
		Bio:         user.Bio,
		AvatarURL:   user.AvatarURL,
		CreatedAt:   user.CreatedAt,
		Metadata: map[string]interface{}{
			"id":           user.ID,
			"public_email": user.PublicEmail,
			"location":     user.Location,
			"organization": user.Organization,
		},
	}

	return result, nil
}

func (d *Discovery) discoverTwitter(ctx context.Context, username string) (*DiscoveryResult, error) {
	return &DiscoveryResult{
		Username:   username,
		Platform:   "Twitter",
		ProfileURL: fmt.Sprintf("https://twitter.com/%s", username),
		Metadata:   map[string]interface{}{},
	}, nil
}

type CrossPlatformAnalyzer struct {
	discovery *Discovery
	checker   *Checker
}

func NewCrossPlatformAnalyzer() *CrossPlatformAnalyzer {
	return &CrossPlatformAnalyzer{
		discovery: NewDiscovery(),
		checker:   NewChecker(),
	}
}

func (a *CrossPlatformAnalyzer) AnalyzeUsername(ctx context.Context, username string) (*UsernameAnalysis, error) {
	analysis := &UsernameAnalysis{
		Username:          username,
		PlatformProfiles:  make(map[string]*DiscoveryResult),
		DiscoveredEmails:  make([]string, 0),
		RelatedUsernames:  make([]string, 0),
		CommonAttributes:  make(map[string][]string),
	}

	checkResults := a.checker.CheckUsername(ctx, username)
	
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, check := range checkResults {
		if check.Exists && check.Error == nil {
			wg.Add(1)
			go func(platformName string) {
				defer wg.Done()
				
				if profile, err := a.discovery.DiscoverProfile(ctx, platformName, username); err == nil {
					mu.Lock()
					analysis.PlatformProfiles[platformName] = profile
					
					if email, ok := profile.Metadata["email"].(string); ok && email != "" {
						analysis.DiscoveredEmails = append(analysis.DiscoveredEmails, email)
					}
					mu.Unlock()
				}
			}(check.Platform)
		}
	}

	wg.Wait()

	a.extractCommonAttributes(analysis)
	
	return analysis, nil
}

func (a *CrossPlatformAnalyzer) extractCommonAttributes(analysis *UsernameAnalysis) {
	names := make(map[string]int)
	locations := make(map[string]int)
	companies := make(map[string]int)

	for _, profile := range analysis.PlatformProfiles {
		if profile.DisplayName != "" {
			names[profile.DisplayName]++
		}
		
		if location, ok := profile.Metadata["location"].(string); ok && location != "" {
			locations[location]++
		}
		
		if company, ok := profile.Metadata["company"].(string); ok && company != "" {
			companies[company]++
		}
	}

	if len(names) > 0 {
		analysis.CommonAttributes["names"] = extractKeys(names)
	}
	if len(locations) > 0 {
		analysis.CommonAttributes["locations"] = extractKeys(locations)
	}
	if len(companies) > 0 {
		analysis.CommonAttributes["companies"] = extractKeys(companies)
	}
}

func extractKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

type UsernameAnalysis struct {
	Username         string
	PlatformProfiles map[string]*DiscoveryResult
	DiscoveredEmails []string
	RelatedUsernames []string
	CommonAttributes map[string][]string
}

func (a *UsernameAnalysis) GetActiveProfiles() []string {
	active := make([]string, 0)
	for platform, profile := range a.PlatformProfiles {
		if profile != nil {
			active = append(active, platform)
		}
	}
	return active
}

func (a *UsernameAnalysis) GetProfileURLs() map[string]string {
	urls := make(map[string]string)
	for platform, profile := range a.PlatformProfiles {
		if profile != nil {
			urls[platform] = profile.ProfileURL
		}
	}
	return urls
}