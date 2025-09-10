package email

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type ValidationResult struct {
	Email       string
	Valid       bool
	Methods     map[string]MethodResult
	Confidence  float64
	LastChecked time.Time
}

type MethodResult struct {
	Found  bool
	Error  error
	Data   map[string]interface{}
}

type Validator struct {
	httpClient *http.Client
	dnsTimeout time.Duration
}

func NewValidator() *Validator {
	return &Validator{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		dnsTimeout: 5 * time.Second,
	}
}

func (v *Validator) ValidateEmail(ctx context.Context, email string) (*ValidationResult, error) {
	if !IsValidEmailFormat(email) {
		return &ValidationResult{
			Email:      email,
			Valid:      false,
			Confidence: 0.0,
		}, nil
	}

	result := &ValidationResult{
		Email:       email,
		Methods:     make(map[string]MethodResult),
		LastChecked: time.Now(),
	}

	methods := []struct {
		name string
		fn   func(context.Context, string) MethodResult
	}{
		{"gravatar", v.checkGravatar},
		{"mx_records", v.checkMXRecords},
		{"github", v.checkGitHub},
	}

	validCount := 0
	totalMethods := 0

	for _, method := range methods {
		methodResult := method.fn(ctx, email)
		result.Methods[method.name] = methodResult
		
		if methodResult.Error == nil {
			totalMethods++
			if methodResult.Found {
				validCount++
			}
		}
	}

	if totalMethods > 0 {
		result.Confidence = float64(validCount) / float64(totalMethods)
		result.Valid = validCount > 0
	}

	return result, nil
}

func (v *Validator) checkGravatar(ctx context.Context, email string) MethodResult {
	hash := md5.Sum([]byte(strings.ToLower(strings.TrimSpace(email))))
	url := fmt.Sprintf("https://www.gravatar.com/avatar/%x?d=404", hash)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return MethodResult{Error: err}
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return MethodResult{Error: err}
	}
	defer resp.Body.Close()

	return MethodResult{
		Found: resp.StatusCode == http.StatusOK,
		Data: map[string]interface{}{
			"status_code": resp.StatusCode,
			"avatar_url":  url,
		},
	}
}

func (v *Validator) checkMXRecords(ctx context.Context, email string) MethodResult {
	domain := ExtractDomainFromEmail(email)
	if domain == "" {
		return MethodResult{Error: fmt.Errorf("invalid email format")}
	}

	resolver := &net.Resolver{}
	ctx, cancel := context.WithTimeout(ctx, v.dnsTimeout)
	defer cancel()

	mxRecords, err := resolver.LookupMX(ctx, domain)
	if err != nil {
		return MethodResult{Error: err}
	}

	records := make([]string, 0, len(mxRecords))
	for _, mx := range mxRecords {
		records = append(records, mx.Host)
	}

	return MethodResult{
		Found: len(mxRecords) > 0,
		Data: map[string]interface{}{
			"mx_records": records,
			"count":      len(mxRecords),
		},
	}
}

func (v *Validator) checkGitHub(ctx context.Context, email string) MethodResult {
	url := fmt.Sprintf("https://api.github.com/search/users?q=%s+in:email", email)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return MethodResult{Error: err}
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return MethodResult{Error: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return MethodResult{
			Found: false,
			Data: map[string]interface{}{
				"status_code": resp.StatusCode,
			},
		}
	}

	var result struct {
		TotalCount int `json:"total_count"`
		Items      []struct {
			Login string `json:"login"`
			ID    int    `json:"id"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return MethodResult{Error: err}
	}

	users := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		users = append(users, item.Login)
	}

	return MethodResult{
		Found: result.TotalCount > 0,
		Data: map[string]interface{}{
			"total_count": result.TotalCount,
			"users":       users,
		},
	}
}

func (v *Validator) ValidateBatch(ctx context.Context, emails []string) map[string]*ValidationResult {
	results := make(map[string]*ValidationResult)
	
	for _, email := range emails {
		if result, err := v.ValidateEmail(ctx, email); err == nil {
			results[email] = result
		}
	}

	return results
}

type EmailScorer struct {
	weights map[string]float64
}

func NewEmailScorer() *EmailScorer {
	return &EmailScorer{
		weights: map[string]float64{
			"gravatar":   0.3,
			"mx_records": 0.2,
			"github":     0.5,
		},
	}
}

func (s *EmailScorer) Score(result *ValidationResult) float64 {
	if result == nil || len(result.Methods) == 0 {
		return 0.0
	}

	totalScore := 0.0
	totalWeight := 0.0

	for method, weight := range s.weights {
		if methodResult, exists := result.Methods[method]; exists && methodResult.Error == nil {
			if methodResult.Found {
				totalScore += weight
			}
			totalWeight += weight
		}
	}

	if totalWeight == 0 {
		return 0.0
	}

	return totalScore / totalWeight
}

func (s *EmailScorer) RankEmails(results map[string]*ValidationResult) []string {
	type emailScore struct {
		email string
		score float64
	}

	scores := make([]emailScore, 0, len(results))
	for email, result := range results {
		scores = append(scores, emailScore{
			email: email,
			score: s.Score(result),
		})
	}

	for i := 0; i < len(scores); i++ {
		for j := i + 1; j < len(scores); j++ {
			if scores[j].score > scores[i].score {
				scores[i], scores[j] = scores[j], scores[i]
			}
		}
	}

	ranked := make([]string, len(scores))
	for i, s := range scores {
		ranked[i] = s.email
	}

	return ranked
}