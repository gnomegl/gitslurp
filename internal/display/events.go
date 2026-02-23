package display

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

func OutputEventList(list []string, filename, header, emoji string) error {
	if len(list) == 0 {
		fmt.Println("\n" + strings.Replace(header, ":", "", 1) + " - None found")
		return nil
	}

	content := strings.Join(list, "\n")

	if len(list) > 50 {
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %v", filename, err)
		}
		fmt.Printf("\n%s exceeds 50 entries, written to %s\n", strings.Replace(header, ":", "", 1), filename)
	} else {
		fmt.Println("\n" + header)
		for _, item := range list {
			fmt.Printf("  %s\n", item)
		}
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %v", filename, err)
		}
	}

	return nil
}

func SortedKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func HandleNoEmails(isOrg bool, username string, repoCount int) error {
	if isOrg {
		if repoCount > 0 {
			fmt.Println("\nAll commits in this organization's repositories are anonymous")
			return nil
		}
		return fmt.Errorf("no repositories found for organization: %s", username)
	}
	return fmt.Errorf("no commits or gists found for user: %s", username)
}
