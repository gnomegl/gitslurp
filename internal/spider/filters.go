package spider

type Filters struct {
	MinRepos     int
	MinFollowers int
	MaxNodes     int
}

func (f *Filters) PassesUserFilter(followers, publicRepos int) bool {
	if f.MinFollowers > 0 && followers < f.MinFollowers {
		return false
	}
	if f.MinRepos > 0 && publicRepos < f.MinRepos {
		return false
	}
	return true
}

func (f *Filters) NodeLimitReached(currentCount int) bool {
	return f.MaxNodes > 0 && currentCount >= f.MaxNodes
}
