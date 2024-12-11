package scanner

import "regexp"

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)aws_access_key.*=.*`),
	regexp.MustCompile(`(?i)aws_secret.*=.*`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`(?i)private_key.*=.*`),
	regexp.MustCompile(`(?i)secret.*=.*[0-9a-zA-Z]{16,}`),
	regexp.MustCompile(`(?i)password.*=.*[0-9a-zA-Z]{8,}`),
	regexp.MustCompile(`(?i)token.*=.*[0-9a-zA-Z]{8,}`),
	regexp.MustCompile(`-----BEGIN ((RSA|DSA|EC|PGP|OPENSSH) )?PRIVATE KEY( BLOCK)?-----`),
	regexp.MustCompile(`(?i)github[_\-\.]?token.*=.*[0-9a-zA-Z]{35,40}`),
	regexp.MustCompile(`(?i)api[_\-\.]?key.*=.*[0-9a-zA-Z]{16,}`),
}

func CheckForSecrets(content string) []string {
	var secrets []string
	for _, pattern := range secretPatterns {
		matches := pattern.FindAllString(content, -1)
		secrets = append(secrets, matches...)
	}
	return secrets
}
