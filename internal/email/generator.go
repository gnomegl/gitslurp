package email

import (
	"fmt"
	"strings"
)

var DefaultDomains = []string{
	"gmail.com",
	"yahoo.com",
	"outlook.com",
	"hotmail.com",
	"protonmail.com",
	"icloud.com",
	"mail.com",
	"aol.com",
	"fastmail.com",
	"yandex.com",
}

type EmailGenerator struct {
	domains []string
}

func NewEmailGenerator() *EmailGenerator {
	return &EmailGenerator{
		domains: DefaultDomains,
	}
}

func (eg *EmailGenerator) Generate(firstName, lastName, username string, customDomains []string, permuteDefault bool) []string {
	emails := make(map[string]bool)
	domains := customDomains

	if permuteDefault {
		domains = append(domains, eg.domains...)
	}

	if len(domains) == 0 {
		domains = []string{"gmail.com"}
	}

	for _, domain := range domains {
		for _, email := range eg.generatePermutations(firstName, lastName, username, domain) {
			emails[strings.ToLower(email)] = true
		}
	}

	result := make([]string, 0, len(emails))
	for email := range emails {
		result = append(result, email)
	}

	return result
}

func (eg *EmailGenerator) generatePermutations(firstName, lastName, username, domain string) []string {
	var emails []string

	firstName = strings.ToLower(firstName)
	lastName = strings.ToLower(lastName)
	username = strings.ToLower(username)

	if firstName != "" && lastName != "" {
		emails = append(emails,
			fmt.Sprintf("%s.%s@%s", firstName, lastName, domain),
			fmt.Sprintf("%s%s@%s", firstName, lastName, domain),
			fmt.Sprintf("%s_%s@%s", firstName, lastName, domain),
			fmt.Sprintf("%s-%s@%s", firstName, lastName, domain),
			fmt.Sprintf("%s%s@%s", firstName[:1], lastName, domain),
			fmt.Sprintf("%s%s@%s", firstName, lastName[:1], domain),
			fmt.Sprintf("%s.%s@%s", lastName, firstName, domain),
			fmt.Sprintf("%s%s@%s", lastName, firstName, domain),
			fmt.Sprintf("%s_%s@%s", lastName, firstName, domain),
			fmt.Sprintf("%s-%s@%s", lastName, firstName, domain),
			fmt.Sprintf("%s%s@%s", lastName[:1], firstName, domain),
			fmt.Sprintf("%s%s@%s", lastName, firstName[:1], domain),
		)

		if len(firstName) > 1 {
			emails = append(emails,
				fmt.Sprintf("%s.%s@%s", firstName[:2], lastName, domain),
				fmt.Sprintf("%s%s@%s", firstName[:2], lastName, domain),
			)
		}

		if len(lastName) > 1 {
			emails = append(emails,
				fmt.Sprintf("%s.%s@%s", firstName, lastName[:2], domain),
				fmt.Sprintf("%s%s@%s", firstName, lastName[:2], domain),
			)
		}
	}

	if username != "" {
		emails = append(emails,
			fmt.Sprintf("%s@%s", username, domain),
		)

		if firstName != "" {
			emails = append(emails,
				fmt.Sprintf("%s.%s@%s", username, firstName, domain),
				fmt.Sprintf("%s.%s@%s", firstName, username, domain),
			)
		}

		if lastName != "" {
			emails = append(emails,
				fmt.Sprintf("%s.%s@%s", username, lastName, domain),
				fmt.Sprintf("%s.%s@%s", lastName, username, domain),
			)
		}
	}

	return emails
}

func (eg *EmailGenerator) GenerateFromGitAuthor(name, email string) []string {
	emails := make(map[string]bool)

	parts := strings.Fields(name)
	if len(parts) >= 2 {
		firstName := parts[0]
		lastName := parts[len(parts)-1]

		username := ""
		if email != "" {
			if idx := strings.Index(email, "@"); idx > 0 {
				username = email[:idx]
			}
		}

		domain := ""
		if email != "" {
			if idx := strings.Index(email, "@"); idx > 0 && idx < len(email)-1 {
				domain = email[idx+1:]
			}
		}

		if domain != "" {
			for _, generated := range eg.generatePermutations(firstName, lastName, username, domain) {
				emails[strings.ToLower(generated)] = true
			}
		}

		for _, defaultDomain := range eg.domains {
			for _, generated := range eg.generatePermutations(firstName, lastName, username, defaultDomain) {
				emails[strings.ToLower(generated)] = true
			}
		}
	}

	if email != "" {
		emails[strings.ToLower(email)] = true
	}

	result := make([]string, 0, len(emails))
	for email := range emails {
		result = append(result, email)
	}

	return result
}

func ExtractDomainFromEmail(email string) string {
	if idx := strings.Index(email, "@"); idx > 0 && idx < len(email)-1 {
		return email[idx+1:]
	}
	return ""
}

func ExtractUsernameFromEmail(email string) string {
	if idx := strings.Index(email, "@"); idx > 0 {
		return email[:idx]
	}
	return ""
}

func IsValidEmailFormat(email string) bool {
	if email == "" {
		return false
	}

	atCount := strings.Count(email, "@")
	if atCount != 1 {
		return false
	}

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}

	local, domain := parts[0], parts[1]

	if local == "" || domain == "" {
		return false
	}

	if strings.Contains(domain, ".") == false {
		return false
	}

	if strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") {
		return false
	}

	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return false
	}

	return true
}