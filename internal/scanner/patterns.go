package scanner

// SecretPatterns contains regex patterns for detecting various types of secrets
var SecretPatterns = map[string]string{
	// AWS Access Keys
	"AWS Access Key": `\b((?:AKIA|ABIA|ACCA)[A-Z0-9]{16})\b`,

	// GitHub Tokens
	"GitHub Token": `\b((?:ghp|gho|ghu|ghs|ghr|github_pat)_[a-zA-Z0-9_]{36,255})\b`,

	// Private Keys
	"Private Key": `(?i)-----\s*?BEGIN[ A-Z0-9_-]*?PRIVATE KEY\s*?-----[\s\S]*?----\s*?END[ A-Z0-9_-]*? PRIVATE KEY\s*?-----`,

	// Generic API Keys/Secrets
	"Generic Secret": `(pass|token|cred|secret|key)(\b[\x21-\x7e]{16,64}\b)`,

	// Stripe API Keys
	"Stripe Key": `[rs]k_live_[a-zA-Z0-9]{20,247}`,

	// Slack Tokens
	"Slack Bot Token":               `xoxb\-[0-9]{10,13}\-[0-9]{10,13}[a-zA-Z0-9\-]*`,
	"Slack User Token":              `xoxp\-[0-9]{10,13}\-[0-9]{10,13}[a-zA-Z0-9\-]*`,
	"Slack Workspace Access Token":  `xoxa\-[0-9]{10,13}\-[0-9]{10,13}[a-zA-Z0-9\-]*`,
	"Slack Workspace Refresh Token": `xoxr\-[0-9]{10,13}\-[0-9]{10,13}[a-zA-Z0-9\-]*`,

	// Azure Storage
	"Azure Storage Account Name": `(?i:Account[_.-]?Name|Storage[_.-]?(?:Account|Name))(?:.|\s){0,20}?\b([a-z0-9]{3,24})\b|([a-z0-9]{3,24})(?i:\.blob\.core\.windows\.net)`,
	"Azure Storage Key":          `(?i:(?:Access|Account|Storage)[_.-]?Key)(?:.|\s){0,25}?([a-zA-Z0-9+\/-]{86,88}={0,2})`,

	// GCP Service Account Key
	"GCP Service Account": `\{[^{]+auth_provider_x509_cert_url[^}]+\}`,

	// MongoDB Connection Strings
	"MongoDB URI": `\b(mongodb(?:\+srv)?://(?P<username>\S{3,50}):(?P<password>\S{3,88})@(?P<host>[-.%\w]+(?::\d{1,5})?(?:,[-.%\w]+(?::\d{1,5})?)*)(?:/(?P<authdb>[\w-]+)?(?P<options>\?\w+=[\w@/.$-]+(?:&(?:amp;)?\w+=[\w@/.$-]+)*)?)?)(?:\b|$)`,

	// PostgreSQL Connection Strings
	"PostgreSQL URI": `\b(?i)(postgres(?:ql)?)://\S+\b`,
}

// InterestingStrings contains regex patterns for common false positives that might be interesting
// visible with the --interesting flag
var InterestingStrings = []string{
	// UUIDs
	`[0-9A-Fa-f]{8}(?:-[0-9A-Fa-f]{4}){3}-[0-9A-Fa-f]{12}`,
	// UUIDv4
	`[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-4[a-fA-F0-9]{3}-[8|9|aA|bB][a-fA-F0-9]{3}-[a-fA-F0-9]{12}`,
	// Issue tracker IDs
	`[A-Z]{2,6}\-[0-9]{2,6}`,
	// Hex encoded hashes
	`\b[A-Fa-f0-9]{64}\b`,
	// URLs
	`https?:\/\/(www\.)?[-a-zA-Z0-9@:%._\+~#=]{1,256}\.[a-zA-Z0-9()]{1,6}\b([-a-zA-Z0-9()@:%_\+.~#?&//=]*)`,
	// File paths
	`\b([/]{0,1}([\w]+[/])+[\w\.]*)\b`,
	// MAC addresses
	`([0-9A-F]{2}[:-]){5}([0-9A-F]{2})`,
	// IP addresses
	`[\d]{1,3}\.[\d]{1,3}\.[\d]{1,3}\.[\d]{1,3}`,
	// Hex encodings
	`[A-Fa-f0-9x]{2}:[A-Fa-f0-9x]{2}:[A-Fa-f0-9x]{2}`,
	// Placeholder passwords
	`^[xX]+|\*+$`,
}

// ValidationRules contains additional validation rules for each secret type
var ValidationRules = map[string][]string{
	"AWS Access Key": {
		"Must be 20 characters long",
		"Must start with AKIA, ABIA, or ACCA",
		"Must have high entropy",
	},
	"GitHub Token": {
		"Must start with ghp_, gho_, ghu_, ghs_, ghr_, or github_pat_",
		"Must be between 36 and 255 characters",
	},
	"Private Key": {
		"Must be a valid PEM format",
		"Must contain BEGIN and END markers",
	},
	"Generic Secret": {
		"Must be between 16 and 64 characters",
		"Must have high entropy",
		"Must not be base64 decodable",
	},
	"Stripe Key": {
		"Must start with sk_live_ or rk_live_",
		"Must be between 20 and 247 characters",
	},
	"Slack Bot Token": {
		"Must start with xoxb-",
		"Must match pattern of two 10-13 digit segments",
	},
	"Slack User Token": {
		"Must start with xoxp-",
		"Must match pattern of two 10-13 digit segments",
	},
	"Slack Workspace Access Token": {
		"Must start with xoxa-",
		"Must match pattern of two 10-13 digit segments",
	},
	"Slack Workspace Refresh Token": {
		"Must start with xoxr-",
		"Must match pattern of two 10-13 digit segments",
	},
	"Azure Storage Account Name": {
		"Must be between 3 and 24 characters",
		"Must be lowercase alphanumeric",
		"Must not be a test account name",
	},
	"Azure Storage Key": {
		"Must be between 86 and 88 characters",
		"Must be base64 encoded",
		"Must not be a test key",
	},
	"GCP Service Account": {
		"Must be valid JSON",
		"Must contain auth_provider_x509_cert_url",
		"Must contain valid service account email",
		"Must not be a test service account",
	},
	"MongoDB URI": {
		"Must be valid MongoDB connection string format",
		"Username must be between 3 and 50 characters",
		"Password must be between 3 and 88 characters",
		"Must not contain placeholder passwords",
	},
	"PostgreSQL URI": {
		"Must be valid PostgreSQL connection string format",
		"Must contain username and password",
		"Must not be a localhost or loopback connection",
		"Must specify port or use default 5432",
	},
}
