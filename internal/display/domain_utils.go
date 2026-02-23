package display

import (
	"net/url"
	"strings"
)

func extractDomainFromWebsite(website string) string {
	if website == "" {
		return ""
	}

	if !strings.HasPrefix(website, "http://") && !strings.HasPrefix(website, "https://") {
		website = "https://" + website
	}

	parsedURL, err := url.Parse(website)
	if err != nil {
		return ""
	}

	domain := parsedURL.Hostname()
	domain = strings.TrimPrefix(domain, "www.")

	return domain
}

func extractBaseDomain(domain string) string {
	domain = strings.ToLower(domain)

	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return domain
	}

	twoLevelTLDs := map[string]bool{
		"co.uk": true, "co.jp": true, "co.nz": true, "co.za": true,
		"com.au": true, "com.br": true, "com.cn": true, "com.mx": true,
		"ac.uk": true, "gov.uk": true, "org.uk": true,
	}

	if len(parts) >= 3 {
		lastTwo := parts[len(parts)-2] + "." + parts[len(parts)-1]
		if twoLevelTLDs[lastTwo] {
			if len(parts) >= 3 {
				return parts[len(parts)-3]
			}
		}
	}

	return parts[len(parts)-2]
}

func isOrganizationEmail(email, orgDomain string) bool {
	if orgDomain == "" || email == "" {
		return false
	}

	if !strings.Contains(email, "@") {
		return false
	}

	emailDomain := strings.Split(email, "@")[1]
	emailDomain = strings.ToLower(emailDomain)
	orgDomain = strings.ToLower(orgDomain)

	if emailDomain == orgDomain {
		return true
	}

	emailBase := extractBaseDomain(emailDomain)
	orgBase := extractBaseDomain(orgDomain)

	return emailBase == orgBase && emailBase != "" && orgBase != ""
}

