package version

import (
	"fmt"
	"regexp"
	"strings"
)

var releaseRE = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+$`)

func Normalize(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("version value is empty")
	}
	if !releaseRE.MatchString(trimmed) {
		return "", fmt.Errorf("invalid Bottlerocket version-lock value %q", value)
	}
	if strings.HasPrefix(trimmed, "v") {
		return trimmed, nil
	}
	return "v" + trimmed, nil
}

func Equal(left, right string) bool {
	left = strings.TrimPrefix(strings.TrimSpace(left), "v")
	right = strings.TrimPrefix(strings.TrimSpace(right), "v")
	return left == right
}
