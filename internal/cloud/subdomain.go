package cloud

import (
	"regexp"
	"strings"
)

const subdomainMaxLen = 50

var nonKebab = regexp.MustCompile(`[^a-z0-9]+`)

// Subdomain converts a name to a DNS-safe subdomain (lowercase kebab-case),
// limited to 50 chars, falling back to "project" if empty.
//
// If override is non-empty (after trimming whitespace), it's used (also
// sanitized) instead of name.
func Subdomain(name, override string) string {
	source := name
	if strings.TrimSpace(override) != "" {
		source = override
	}
	s := strings.ToLower(source)
	s = nonKebab.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > subdomainMaxLen {
		s = s[:subdomainMaxLen]
		s = strings.TrimRight(s, "-")
	}
	if s == "" {
		return "project"
	}
	return s
}
