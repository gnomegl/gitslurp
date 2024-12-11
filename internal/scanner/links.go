package scanner

import (
	"regexp"
	"sort"
)

var urlPattern = regexp.MustCompile(`https?://[^\s<>"]+|www\.[^\s<>"]+`)

func ExtractLinks(content string) []string {
	matches := urlPattern.FindAllString(content, -1)
	uniqueLinks := make(map[string]struct{})
	for _, link := range matches {
		uniqueLinks[link] = struct{}{}
	}

	links := make([]string, 0, len(uniqueLinks))
	for link := range uniqueLinks {
		links = append(links, link)
	}
	sort.Strings(links)
	return links
}
