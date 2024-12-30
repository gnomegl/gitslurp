package scanner

import (
	"regexp"
)

type PatternGroup struct {
	Name        string
	Description string
	Patterns    []string
}

type Scanner struct {
	showInteresting bool
}

func NewScanner(showInteresting bool) *Scanner {
	return &Scanner{
		showInteresting: showInteresting,
	}
}

func (s *Scanner) ScanText(text string) []Match {
	var matches []Match

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

type Match struct {
	Type  string // "Secret" or "Interesting"
	Name  string // Pattern name
	Value string // The actual matched string
}

func (m *Match) Validate() bool {
	if _, ok := ValidationRules[m.Name]; !ok {
		return true // No validation rules defined
	}

	return true
}
