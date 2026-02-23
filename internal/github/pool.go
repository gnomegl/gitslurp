package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	gh "github.com/google/go-github/v57/github"
	"golang.org/x/oauth2"
)

type ManagedClient struct {
	Client    *gh.Client
	Token     string
	Proxy     string
	remaining int
	resetAt   time.Time
	mu        sync.Mutex
}

func (mc *ManagedClient) UpdateRateLimit(remaining int, resetAt time.Time) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.remaining = remaining
	mc.resetAt = resetAt
}

func (mc *ManagedClient) Remaining() int {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return mc.remaining
}

func (mc *ManagedClient) ResetAt() time.Time {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return mc.resetAt
}

type ClientPool struct {
	clients []*ManagedClient
	mu      sync.Mutex
}

func NewClientPool(tokens []string, proxies []string) (*ClientPool, error) {
	if len(tokens) == 0 {
		client := gh.NewClient(nil)
		return &ClientPool{
			clients: []*ManagedClient{{
				Client:    client,
				remaining: 60,
			}},
		}, nil
	}

	pool := &ClientPool{
		clients: make([]*ManagedClient, 0, len(tokens)),
	}

	for i, token := range tokens {
		var proxyURL string
		if i < len(proxies) {
			proxyURL = proxies[i]
		}

		client, err := createClientWithProxy(token, proxyURL)
		if err != nil {
			return nil, fmt.Errorf("failed to create client for token %d: %v", i+1, err)
		}

		pool.clients = append(pool.clients, &ManagedClient{
			Client:    client,
			Token:     token,
			Proxy:     proxyURL,
			remaining: 5000,
		})
	}

	return pool, nil
}

func createClientWithProxy(token, proxyURL string) (*gh.Client, error) {
	transport := &http.Transport{}

	if proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL %q: %v", proxyURL, err)
		}
		transport.Proxy = http.ProxyURL(parsed)
	}

	var httpClient *http.Client
	if token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		httpClient = &http.Client{
			Transport: &oauth2.Transport{
				Source: ts,
				Base:   transport,
			},
		}
	} else {
		httpClient = &http.Client{Transport: transport}
	}

	return gh.NewClient(httpClient), nil
}

func (p *ClientPool) GetClient() *ManagedClient {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.clients) == 1 {
		return p.clients[0]
	}

	var best *ManagedClient
	bestRemaining := -1

	for _, mc := range p.clients {
		mc.mu.Lock()
		rem := mc.remaining
		mc.mu.Unlock()

		if rem > bestRemaining {
			bestRemaining = rem
			best = mc
		}
	}

	if bestRemaining < 100 {
		var earliest *ManagedClient
		earliestReset := time.Now().Add(24 * time.Hour)

		for _, mc := range p.clients {
			mc.mu.Lock()
			reset := mc.resetAt
			mc.mu.Unlock()

			if reset.Before(earliestReset) {
				earliestReset = reset
				earliest = mc
			}
		}

		if earliest != nil {
			return earliest
		}
	}

	return best
}

func (p *ClientPool) PrimaryToken() string {
	if len(p.clients) == 0 {
		return ""
	}
	return p.clients[0].Token
}

func (p *ClientPool) Size() int {
	return len(p.clients)
}

func (p *ClientPool) AllClients() []*ManagedClient {
	return p.clients
}

func (p *ClientPool) DisplayPoolRateLimit(ctx context.Context) {
	if p.Size() <= 1 {
		DisplayRateLimit(ctx, p.clients[0].Client)
		return
	}

	fmt.Println()
	fmt.Println(strings.Repeat("-", 50))
	color.Cyan("Token Pool Rate Limits (%d tokens):", p.Size())

	for i, mc := range p.clients {
		rateLimitInfo, err := GetRateLimit(ctx, mc.Client)
		if err != nil {
			color.Yellow("  Token %d: Could not fetch rate limit: %v", i+1, err)
			continue
		}

		percentage := float64(rateLimitInfo.Remaining) / float64(rateLimitInfo.Limit) * 100
		label := fmt.Sprintf("  Token %d", i+1)
		if mc.Proxy != "" {
			label += " (proxied)"
		}

		if percentage > 50 {
			color.Green("%s: %d/%d (%.1f%%)", label, rateLimitInfo.Remaining, rateLimitInfo.Limit, percentage)
		} else if percentage > 20 {
			color.Yellow("%s: %d/%d (%.1f%%)", label, rateLimitInfo.Remaining, rateLimitInfo.Limit, percentage)
		} else {
			color.Red("%s: %d/%d (%.1f%%)", label, rateLimitInfo.Remaining, rateLimitInfo.Limit, percentage)
		}
	}
}
