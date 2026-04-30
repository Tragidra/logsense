package normalize

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"
)

// Pre-compiled masking patterns — never compiled per event.
var (
	reNum     = regexp.MustCompile(`^-?\d+(\.\d+)?([eE][+-]?\d+)?$`)
	reUUID    = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	reIP      = regexp.MustCompile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}(?::\d+)?$`)
	reLongHex = regexp.MustCompile(`(?i)^[0-9a-f]{8,}$`)
	reISOTS   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}`)
)

// Fingerprint returns a stable hex hash of the masked token sequence of message.
// Tokens matching numbers, UUIDs, IPs, long hex strings, or ISO timestamps are replaced with "<*>",
// so semantically identical log lines share a fingerprint.
func Fingerprint(message string) string {
	tokens := tokenize(message)
	parts := make([]string, 0, len(tokens))
	for _, t := range tokens {
		switch {
		case reNum.MatchString(t):
			parts = append(parts, "<*>")
		case reUUID.MatchString(t):
			parts = append(parts, "<*>")
		case reIP.MatchString(t):
			parts = append(parts, "<*>")
		case reLongHex.MatchString(t):
			parts = append(parts, "<*>")
		case reISOTS.MatchString(t):
			parts = append(parts, "<*>")
		default:
			parts = append(parts, t)
		}
	}
	h := fnv.New64a()
	h.Write([]byte(strings.Join(parts, " ")))
	return fmt.Sprintf("%016x", h.Sum64())
}

// tokenize splits s on whitespace, keeping double-quoted substrings as single tokens.
func tokenize(s string) []string {
	var tokens []string
	var cur strings.Builder
	inQuote := false

	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQuote {
			cur.WriteByte(c)
			if c == '"' {
				inQuote = false
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		} else if c == '"' {
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
			inQuote = true
			cur.WriteByte(c)
		} else if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		} else {
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}
