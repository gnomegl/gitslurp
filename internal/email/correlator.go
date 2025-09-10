package email

import (
	"strings"
	"time"

	"git.sr.ht/~gnome/gitslurp/internal/models"
)

type EmailCorrelation struct {
	Email           string
	LinkedUsernames []string
	LinkedNames     []string
	CommitCount     int
	Repositories    map[string]int
	FirstSeen       time.Time
	LastSeen        time.Time
	Confidence      float64
}

type Correlator struct {
	correlations map[string]*EmailCorrelation
}

func NewCorrelator() *Correlator {
	return &Correlator{
		correlations: make(map[string]*EmailCorrelation),
	}
}

func (c *Correlator) AddCommit(commit *models.CommitInfo) {
	email := strings.ToLower(commit.Email)
	
	if _, exists := c.correlations[email]; !exists {
		c.correlations[email] = &EmailCorrelation{
			Email:           email,
			LinkedUsernames: make([]string, 0),
			LinkedNames:     make([]string, 0),
			Repositories:    make(map[string]int),
			FirstSeen:       commit.Date,
			LastSeen:        commit.Date,
		}
	}

	corr := c.correlations[email]
	corr.CommitCount++

	if commit.RepoName != "" {
		corr.Repositories[commit.RepoName]++
	}

	if commit.Name != "" && !contains(corr.LinkedNames, commit.Name) {
		corr.LinkedNames = append(corr.LinkedNames, commit.Name)
	}

	if commit.Date.Before(corr.FirstSeen) {
		corr.FirstSeen = commit.Date
	}
	if commit.Date.After(corr.LastSeen) {
		corr.LastSeen = commit.Date
	}
}

func (c *Correlator) LinkUsername(email, username string) {
	email = strings.ToLower(email)
	
	if corr, exists := c.correlations[email]; exists {
		if !contains(corr.LinkedUsernames, username) {
			corr.LinkedUsernames = append(corr.LinkedUsernames, username)
		}
	}
}

func (c *Correlator) GetCorrelation(email string) (*EmailCorrelation, bool) {
	corr, exists := c.correlations[strings.ToLower(email)]
	return corr, exists
}

func (c *Correlator) GetAllCorrelations() []*EmailCorrelation {
	results := make([]*EmailCorrelation, 0, len(c.correlations))
	for _, corr := range c.correlations {
		results = append(results, corr)
	}
	return results
}

func (c *Correlator) FindRelatedEmails(targetEmail string) []*EmailCorrelation {
	targetEmail = strings.ToLower(targetEmail)
	target, exists := c.correlations[targetEmail]
	if !exists {
		return nil
	}

	related := make([]*EmailCorrelation, 0)

	for email, corr := range c.correlations {
		if email == targetEmail {
			continue
		}

		if hasOverlap(target.LinkedNames, corr.LinkedNames) ||
			hasOverlap(target.LinkedUsernames, corr.LinkedUsernames) ||
			hasRepoOverlap(target.Repositories, corr.Repositories) {
			related = append(related, corr)
		}
	}

	return related
}

func (c *Correlator) CalculateConfidence(email string) float64 {
	corr, exists := c.correlations[strings.ToLower(email)]
	if !exists {
		return 0.0
	}

	confidence := 0.0

	if corr.CommitCount > 10 {
		confidence += 0.2
	} else if corr.CommitCount > 5 {
		confidence += 0.1
	}

	if len(corr.LinkedUsernames) > 0 {
		confidence += 0.3
	}

	if len(corr.LinkedNames) == 1 {
		confidence += 0.2
	}

	if len(corr.Repositories) > 3 {
		confidence += 0.2
	}

	duration := corr.LastSeen.Sub(corr.FirstSeen)
	if duration > 365*24*time.Hour {
		confidence += 0.1
	}

	if confidence > 1.0 {
		confidence = 1.0
	}

	corr.Confidence = confidence
	return confidence
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func hasOverlap(slice1, slice2 []string) bool {
	for _, s1 := range slice1 {
		for _, s2 := range slice2 {
			if s1 == s2 {
				return true
			}
		}
	}
	return false
}

func hasRepoOverlap(repos1, repos2 map[string]int) bool {
	for repo := range repos1 {
		if _, exists := repos2[repo]; exists {
			return true
		}
	}
	return false
}

type EmailAnalyzer struct {
	generator  *EmailGenerator
	validator  *Validator
	correlator *Correlator
}

func NewEmailAnalyzer() *EmailAnalyzer {
	return &EmailAnalyzer{
		generator:  NewEmailGenerator(),
		validator:  NewValidator(),
		correlator: NewCorrelator(),
	}
}

func (a *EmailAnalyzer) AnalyzeUser(username string, commits []*models.CommitInfo) *UserEmailAnalysis {
	analysis := &UserEmailAnalysis{
		Username:         username,
		DiscoveredEmails: make(map[string]*EmailInfo),
		GeneratedEmails:  make([]string, 0),
		ValidatedEmails:  make(map[string]*ValidationResult),
	}

	emailSet := make(map[string]bool)
	for _, commit := range commits {
		a.correlator.AddCommit(commit)
		emailSet[strings.ToLower(commit.Email)] = true
	}

	for email := range emailSet {
		corr, _ := a.correlator.GetCorrelation(email)
		analysis.DiscoveredEmails[email] = &EmailInfo{
			Email:       email,
			CommitCount: corr.CommitCount,
			FirstSeen:   corr.FirstSeen,
			LastSeen:    corr.LastSeen,
			Names:       corr.LinkedNames,
		}
	}

	return analysis
}

type UserEmailAnalysis struct {
	Username         string
	DiscoveredEmails map[string]*EmailInfo
	GeneratedEmails  []string
	ValidatedEmails  map[string]*ValidationResult
}

type EmailInfo struct {
	Email       string
	CommitCount int
	FirstSeen   time.Time
	LastSeen    time.Time
	Names       []string
}