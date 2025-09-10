package domains

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

var GitHubPagesIPs = []string{
	"185.199.108.153",
	"185.199.109.153",
	"185.199.110.153",
	"185.199.111.153",
}

type DomainInfo struct {
	Domain      string
	Type        string
	URL         string
	Verified    bool
	IPs         []string
	CNAME       string
	Repository  string
	LastChecked time.Time
}

type Finder struct {
	httpClient  *http.Client
	resolver    *net.Resolver
	dnsTimeout  time.Duration
}

func NewFinder() *Finder {
	return &Finder{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		resolver: &net.Resolver{
			PreferGo: true,
		},
		dnsTimeout: 5 * time.Second,
	}
}

func (f *Finder) FindGitHubPages(ctx context.Context, username string) ([]*DomainInfo, error) {
	potentialDomains := []string{
		fmt.Sprintf("%s.github.io", strings.ToLower(username)),
		fmt.Sprintf("%s.github.com", strings.ToLower(username)),
	}

	var results []*DomainInfo
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, domain := range potentialDomains {
		wg.Add(1)
		go func(d string) {
			defer wg.Done()
			
			if info := f.checkGitHubPagesDomain(ctx, d); info != nil {
				mu.Lock()
				results = append(results, info)
				mu.Unlock()
			}
		}(domain)
	}

	wg.Wait()
	return results, nil
}

func (f *Finder) checkGitHubPagesDomain(ctx context.Context, domain string) *DomainInfo {
	ctx, cancel := context.WithTimeout(ctx, f.dnsTimeout)
	defer cancel()

	ips, err := f.resolver.LookupIPAddr(ctx, domain)
	if err != nil {
		return nil
	}

	ipStrings := make([]string, 0, len(ips))
	isGitHubPages := false

	for _, ip := range ips {
		ipStr := ip.IP.String()
		ipStrings = append(ipStrings, ipStr)
		
		for _, ghIP := range GitHubPagesIPs {
			if ipStr == ghIP {
				isGitHubPages = true
				break
			}
		}
	}

	if !isGitHubPages {
		return nil
	}

	info := &DomainInfo{
		Domain:      domain,
		Type:        "github_pages",
		URL:         fmt.Sprintf("https://%s", domain),
		IPs:         ipStrings,
		LastChecked: time.Now(),
	}

	if f.checkSiteAccessible(ctx, info.URL) {
		info.Verified = true
	}

	cname, _ := f.resolver.LookupCNAME(ctx, domain)
	if cname != "" && cname != domain {
		info.CNAME = cname
	}

	return info
}

func (f *Finder) checkSiteAccessible(ctx context.Context, url string) bool {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusMovedPermanently
}

func (f *Finder) FindCustomDomains(ctx context.Context, repositories []string) ([]*DomainInfo, error) {
	var results []*DomainInfo
	var mu sync.Mutex
	var wg sync.WaitGroup

	semaphore := make(chan struct{}, 5)

	for _, repo := range repositories {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()
			
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			if domains := f.checkRepositoryForDomains(ctx, r); len(domains) > 0 {
				mu.Lock()
				results = append(results, domains...)
				mu.Unlock()
			}
		}(repo)
	}

	wg.Wait()
	return results, nil
}

func (f *Finder) checkRepositoryForDomains(ctx context.Context, repoPath string) []*DomainInfo {
	cnameURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/gh-pages/CNAME", repoPath)
	
	domains := make([]*DomainInfo, 0)
	
	if cnameDomain := f.fetchCNAME(ctx, cnameURL); cnameDomain != "" {
		info := &DomainInfo{
			Domain:      cnameDomain,
			Type:        "custom_domain",
			URL:         fmt.Sprintf("https://%s", cnameDomain),
			Repository:  repoPath,
			LastChecked: time.Now(),
		}
		
		if f.checkSiteAccessible(ctx, info.URL) {
			info.Verified = true
		}
		
		domains = append(domains, info)
	}

	mainCNAME := fmt.Sprintf("https://raw.githubusercontent.com/%s/main/CNAME", repoPath)
	if mainDomain := f.fetchCNAME(ctx, mainCNAME); mainDomain != "" && mainDomain != cnameDomain {
		info := &DomainInfo{
			Domain:      mainDomain,
			Type:        "custom_domain",
			URL:         fmt.Sprintf("https://%s", mainDomain),
			Repository:  repoPath,
			LastChecked: time.Now(),
		}
		
		if f.checkSiteAccessible(ctx, info.URL) {
			info.Verified = true
		}
		
		domains = append(domains, info)
	}

	return domains
}

func (f *Finder) fetchCNAME(ctx context.Context, url string) string {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return ""
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	buf := make([]byte, 256)
	n, _ := resp.Body.Read(buf)
	if n > 0 {
		domain := strings.TrimSpace(string(buf[:n]))
		return strings.Split(domain, "\n")[0]
	}

	return ""
}

type SubdomainEnumerator struct {
	finder *Finder
}

func NewSubdomainEnumerator() *SubdomainEnumerator {
	return &SubdomainEnumerator{
		finder: NewFinder(),
	}
}

func (e *SubdomainEnumerator) EnumerateCommon(ctx context.Context, baseDomain string) []string {
	commonPrefixes := []string{
		"www", "blog", "dev", "api", "docs", "admin",
		"app", "dashboard", "portal", "test", "staging",
		"cdn", "static", "assets", "images", "download",
		"ftp", "mail", "email", "smtp", "pop", "imap",
		"vpn", "remote", "secure", "private", "internal",
	}

	var found []string
	var mu sync.Mutex
	var wg sync.WaitGroup

	semaphore := make(chan struct{}, 10)

	for _, prefix := range commonPrefixes {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			subdomain := fmt.Sprintf("%s.%s", p, baseDomain)
			if e.checkDomainExists(ctx, subdomain) {
				mu.Lock()
				found = append(found, subdomain)
				mu.Unlock()
			}
		}(prefix)
	}

	wg.Wait()
	return found
}

func (e *SubdomainEnumerator) checkDomainExists(ctx context.Context, domain string) bool {
	ctx, cancel := context.WithTimeout(ctx, e.finder.dnsTimeout)
	defer cancel()

	ips, err := e.finder.resolver.LookupIPAddr(ctx, domain)
	return err == nil && len(ips) > 0
}

type DomainAnalyzer struct {
	finder     *Finder
	enumerator *SubdomainEnumerator
}

func NewDomainAnalyzer() *DomainAnalyzer {
	return &DomainAnalyzer{
		finder:     NewFinder(),
		enumerator: NewSubdomainEnumerator(),
	}
}

func (a *DomainAnalyzer) AnalyzeUser(ctx context.Context, username string, repositories []string) (*UserDomainAnalysis, error) {
	analysis := &UserDomainAnalysis{
		Username:       username,
		GitHubPages:    make([]*DomainInfo, 0),
		CustomDomains:  make([]*DomainInfo, 0),
		Subdomains:     make(map[string][]string),
	}

	ghPages, _ := a.finder.FindGitHubPages(ctx, username)
	analysis.GitHubPages = ghPages

	customDomains, _ := a.finder.FindCustomDomains(ctx, repositories)
	analysis.CustomDomains = customDomains

	allDomains := make([]string, 0)
	for _, info := range ghPages {
		allDomains = append(allDomains, info.Domain)
	}
	for _, info := range customDomains {
		allDomains = append(allDomains, info.Domain)
	}

	for _, domain := range allDomains {
		subdomains := a.enumerator.EnumerateCommon(ctx, domain)
		if len(subdomains) > 0 {
			analysis.Subdomains[domain] = subdomains
		}
	}

	return analysis, nil
}

type UserDomainAnalysis struct {
	Username      string
	GitHubPages   []*DomainInfo
	CustomDomains []*DomainInfo
	Subdomains    map[string][]string
}

func (a *UserDomainAnalysis) GetAllDomains() []string {
	domains := make(map[string]bool)
	
	for _, info := range a.GitHubPages {
		domains[info.Domain] = true
	}
	
	for _, info := range a.CustomDomains {
		domains[info.Domain] = true
	}
	
	for domain, subs := range a.Subdomains {
		domains[domain] = true
		for _, sub := range subs {
			domains[sub] = true
		}
	}
	
	result := make([]string, 0, len(domains))
	for domain := range domains {
		result = append(result, domain)
	}
	
	return result
}