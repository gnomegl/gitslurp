package display

import (
	"strings"

	"github.com/fatih/color"
)

type SecretDisplayer struct {
	secretsShown  map[string]bool
	patternsShown map[string]bool
}

func NewSecretDisplayer() *SecretDisplayer {
	return &SecretDisplayer{
		secretsShown:  make(map[string]bool),
		patternsShown: make(map[string]bool),
	}
}

func displaySecretLine(secret string) {
	if strings.HasPrefix(secret, "INTERESTING:") || strings.HasPrefix(secret, "PATTERN:") {
		color.Yellow("      PATTERN: %s", secret)
	} else {
		color.Red("      SECRET: %s", secret)
	}
}
