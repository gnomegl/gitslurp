package display

import (
	"strings"

	"github.com/gnomegl/gitslurp/internal/models"
	gh "github.com/google/go-github/v57/github"
)

type UserMatcher struct {
	identifiers map[string]bool
	targetNames map[string]bool
}

func NewUserMatcher(username, lookupEmail string, user *gh.User) *UserMatcher {
	identifiers := buildUserIdentifiers(username, lookupEmail, user)
	return &UserMatcher{
		identifiers: identifiers,
		targetNames: make(map[string]bool),
	}
}

func (m *UserMatcher) IsTargetUser(email string, details *models.EmailDetails) bool {
	if m.identifiers[email] {
		return true
	}

	for name := range details.Names {
		if m.identifiers[name] {
			return true
		}
	}

	return false
}

func (m *UserMatcher) HasMatchingNames(names []string) bool {
	for _, name := range names {
		nameParts := strings.FieldsFunc(name, func(c rune) bool {
			return c == ' ' || c == ','
		})
		for _, part := range nameParts {
			part = strings.TrimSpace(part)
			if m.targetNames[part] {
				return true
			}
		}
	}
	return false
}

func buildUserIdentifiers(username, lookupEmail string, user *gh.User) map[string]bool {
	identifiers := make(map[string]bool)

	if username != "" {
		identifiers[username] = true
	}
	if lookupEmail != "" {
		identifiers[lookupEmail] = true
	}

	if user != nil {
		if login := user.GetLogin(); login != "" {
			identifiers[login] = true
		}
		if name := user.GetName(); name != "" {
			identifiers[name] = true
		}
		if email := user.GetEmail(); email != "" {
			identifiers[email] = true
		}
	}

	return identifiers
}

func extractTargetUserNames(emails map[string]*models.EmailDetails, userIdentifiers map[string]bool) map[string]bool {
	targetNames := make(map[string]bool)

	for email, details := range emails {
		isTargetUser := userIdentifiers[email]
		if !isTargetUser {
			for name := range details.Names {
				if userIdentifiers[name] {
					isTargetUser = true
					break
				}
			}
		}

		if isTargetUser {
			for name := range details.Names {
				nameParts := strings.FieldsFunc(name, func(c rune) bool {
					return c == ' ' || c == ','
				})
				for _, part := range nameParts {
					part = strings.TrimSpace(part)
					if part != "" {
						targetNames[part] = true
					}
				}
			}
		}
	}

	return targetNames
}

func extractNames(details *models.EmailDetails) []string {
	names := make([]string, 0, len(details.Names))
	for name := range details.Names {
		names = append(names, name)
	}
	return names
}

