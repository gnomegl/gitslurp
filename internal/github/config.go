package github

// Config holds configuration for GitHub operations
type Config struct {
	MaxRepos              int
	MaxGists              int
	MaxCommits            int
	ShowInteresting       bool
	MaxConcurrentRequests int
	PerPage               int
	SkipNodeModules       bool
	QuickMode             bool
	TimestampAnalysis     bool
	IncludeForks          bool
}

// DefaultConfig returns a default configuration
func DefaultConfig() Config {
	return Config{
		MaxRepos:              100,
		MaxGists:              100,
		MaxCommits:            100,
		ShowInteresting:       false,
		MaxConcurrentRequests: 5,
		PerPage:               100,
		SkipNodeModules:       true,
		QuickMode:             false,
		TimestampAnalysis:     false,
		IncludeForks:          false,
	}
}
