package cluster

import "strings"

// Tokenize splits s on whitespace, keeps spans as single tokens, and strips trailing punctuation (,.;:) from each token
func Tokenize(s string) []string {
	var tokens []string
	var cur strings.Builder
	inQuote := false

	flush := func() {
		if cur.Len() == 0 {
			return
		}
		tok := strings.TrimRight(cur.String(), ",.;:")
		if tok != "" {
			tokens = append(tokens, tok)
		}
		cur.Reset()
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQuote {
			cur.WriteByte(c)
			if c == '"' {
				inQuote = false
				flush()
			}
		} else if c == '"' {
			flush()
			inQuote = true
			cur.WriteByte(c)
		} else if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			flush()
		} else {
			cur.WriteByte(c)
		}
	}
	flush()
	return tokens
}
