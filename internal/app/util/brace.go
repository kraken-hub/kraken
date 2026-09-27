package util

import (
	"strconv"
	"strings"
)

// expandBraces performs shell-like brace expansion on s. It supports:
//   - alternation: {foo,bar} → "foo", "bar"
//   - numeric ranges: {1..16} → "1", "2", …, "16"
//
// Patterns may appear anywhere in the string and may be combined with a prefix/suffix.
func expandBraces(s string) []string {
	open := strings.Index(s, "{")
	if open == -1 {
		return []string{s}
	}

	// Find the matching closing brace.
	depth := 0
	close := -1
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				close = i
			}
		}
		if close != -1 {
			break
		}
	}
	if close == -1 {
		return []string{s} // unmatched — treat as literal
	}

	prefix := s[:open]
	inner := s[open+1 : close]
	suffix := s[close+1:]

	suffixes := expandBraces(suffix)
	middles := braceAlternatives(inner)

	result := make([]string, 0, len(middles)*len(suffixes))
	for _, m := range middles {
		for _, suf := range suffixes {
			result = append(result, prefix+m+suf)
		}
	}
	return result
}

// braceAlternatives returns the expanded list of alternatives for the content
// inside a brace expression.
func braceAlternatives(inner string) []string {
	alts := splitTopLevelComma(inner)

	// Single token — check for a numeric range like {1..16}.
	if len(alts) == 1 {
		if from, to, ok := parseNumericRange(alts[0]); ok {
			return numericSequence(from, to)
		}
		return expandBraces(alts[0])
	}

	var result []string
	for _, alt := range alts {
		result = append(result, expandBraces(alt)...)
	}
	return result
}

// splitTopLevelComma splits s by comma, ignoring commas inside {} pairs.
func splitTopLevelComma(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}

// parseNumericRange parses "N..M" into (from, to, true).
func parseNumericRange(s string) (int, int, bool) {
	i := strings.Index(s, "..")
	if i == -1 {
		return 0, 0, false
	}
	from, err1 := strconv.Atoi(strings.TrimSpace(s[:i]))
	to, err2 := strconv.Atoi(strings.TrimSpace(s[i+2:]))
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return from, to, true
}

// numericSequence returns string integers from..to inclusive.
func numericSequence(from, to int) []string {
	step := 1
	if from > to {
		step = -1
	}
	n := to - from
	if n < 0 {
		n = -n
	}
	result := make([]string, 0, n+1)
	for v := from; ; v += step {
		result = append(result, strconv.Itoa(v))
		if v == to {
			break
		}
	}
	return result
}
