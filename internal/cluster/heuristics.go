package cluster

import (
	"regexp"
	"strings"
)

// Parameter-detection patterns (never compiled per token).
var (
	reNumeric = regexp.MustCompile(`^-?(?:0[xX][0-9a-fA-F]+|\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)$`)
	reUUID    = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	reIP      = regexp.MustCompile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}(?::\d+)?$`)
	reISOTS   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}`)
	reBase64  = regexp.MustCompile(`^[A-Za-z0-9+/]{24,}={0,2}$`)
)

// hasDigit returns true if s contains at least one ASCII digit.
func hasDigit(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			return true
		}
	}
	return false
}

// looksParameterLike returns true if tok is a variable runtime value that hould not be used as a tree-routing key
// (numbers, UUIDs, IPs, timestamps, file paths).
func looksParameterLike(tok string) bool {
	return reNumeric.MatchString(tok) ||
		reUUID.MatchString(tok) ||
		reIP.MatchString(tok) ||
		reISOTS.MatchString(tok) ||
		strings.HasPrefix(tok, "/") ||
		reBase64.MatchString(tok)
}
