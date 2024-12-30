package scanner

import (
	"regexp"
)

// PatternGroup represents a group of regex patterns with a name and description
type PatternGroup struct {
	Name        string
	Description string
	Patterns    []string
}

// Scanner handles secret and interesting string detection
type Scanner struct {
	showInteresting bool
}

// NewScanner creates a new scanner instance
func NewScanner(showInteresting bool) *Scanner {
	return &Scanner{
		showInteresting: showInteresting,
	}
}

// ScanText scans the given text for secrets and interesting strings
func (s *Scanner) ScanText(text string) []Match {
	var matches []Match

	// Scan for secrets
	for name, pattern := range SecretPatterns {
		re := regexp.MustCompile(pattern)
		found := re.FindAllString(text, -1)
		for _, match := range found {
			matches = append(matches, Match{
				Type:  "Secret",
				Name:  name,
				Value: match,
			})
		}
	}

	// If interesting strings are enabled, scan for those too
	if s.showInteresting {
		for _, pattern := range InterestingStrings {
			re := regexp.MustCompile(pattern)
			found := re.FindAllString(text, -1)
			for _, match := range found {
				matches = append(matches, Match{
					Type:  "Interesting",
					Name:  "Interesting String",
					Value: match,
				})
			}
		}
	}

	return matches
}

// Match represents a found secret or interesting string
type Match struct {
	Type  string // "Secret" or "Interesting"
	Name  string // Pattern name
	Value string // The actual matched string
}

// Validate checks if a match meets its validation rules
func (m *Match) Validate() bool {
	if _, ok := ValidationRules[m.Name]; !ok {
		return true // No validation rules defined
	}

	// TODO: Implement actual validation logic based on rules
	// For now, we just return true
	return true
}
