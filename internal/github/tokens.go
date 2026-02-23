package github

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func ReadTokenFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open token file: %v", err)
	}
	defer f.Close()

	var tokens []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		tokens = append(tokens, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading token file: %v", err)
	}

	if len(tokens) == 0 {
		return nil, fmt.Errorf("token file is empty: %s", path)
	}

	return tokens, nil
}

func ReadProxyFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open proxy file: %v", err)
	}
	defer f.Close()

	var proxies []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.Contains(line, "://") {
			line = "http://" + line
		}
		proxies = append(proxies, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading proxy file: %v", err)
	}

	if len(proxies) == 0 {
		return nil, fmt.Errorf("proxy file is empty: %s", path)
	}

	return proxies, nil
}
