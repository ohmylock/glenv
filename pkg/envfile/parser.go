package envfile

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// SkipReason describes why a line was skipped during parsing.
type SkipReason int

const (
	SkipBlank         SkipReason = iota + 1 // 1
	SkipComment                              // 2
	SkipPlaceholder                          // 3
	SkipInterpolation                        // 4
)

// Variable holds a parsed environment variable.
type Variable struct {
	Key   string
	Value string
	Line  int
}

// SkippedLine records a line that was intentionally skipped.
type SkippedLine struct {
	Line   int
	Key    string
	Reason SkipReason
}

// ParseResult holds the outcome of parsing a .env file.
type ParseResult struct {
	Variables []Variable
	Skipped   []SkippedLine
}

// placeholderPatterns lists case-insensitive substrings that indicate placeholder values.
var placeholderPatterns = []string{
	"your_",
	"change_me",
	"replace_with_",
}

// isPlaceholder returns true if the value appears to be a placeholder.
func isPlaceholder(value string) bool {
	lower := strings.ToLower(value)
	for _, p := range placeholderPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// isInterpolation returns true if the value contains shell variable interpolation.
func isInterpolation(value string) bool {
	return strings.Contains(value, "${")
}

// containsUnescapedInterpolation returns true if s contains an unescaped ${.
// A ${ preceded by an odd number of backslashes is considered escaped and
// does not count as interpolation.
func containsUnescapedInterpolation(s string) bool {
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '$' && s[i+1] == '{' {
			backslashes := 0
			for j := i - 1; j >= 0 && s[j] == '\\'; j-- {
				backslashes++
			}
			if backslashes%2 == 0 {
				return true
			}
		}
	}
	return false
}

// ParseFile opens the file at path and parses it as a .env file.
func ParseFile(path string) (*ParseResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("envfile: open %q: %w", path, err)
	}
	defer f.Close()
	return ParseReader(f)
}

// ParseReader parses a .env formatted stream from r.
//
// Supported syntax:
//   - KEY=VALUE          (unquoted)
//   - KEY="value"        (double-quoted, supports multiline)
//   - KEY='value'        (single-quoted)
//   - KEY=               (empty value)
//   - # comment          (skipped)
//   - blank lines        (skipped)
//   - export KEY=VALUE   (export prefix stripped)
//
// Values containing ${...} are skipped (interpolation).
// Values matching placeholder patterns are skipped.
func ParseReader(r io.Reader) (*ParseResult, error) {
	result := &ParseResult{}
	scanner := bufio.NewScanner(r)
	// Increase buffer to 1 MB to handle large values (certificates, base64 blobs).
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Strip "export " prefix
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "export ") {
			trimmed = strings.TrimPrefix(trimmed, "export ")
			trimmed = strings.TrimSpace(trimmed)
		}

		// Blank line
		if trimmed == "" {
			result.Skipped = append(result.Skipped, SkippedLine{Line: lineNum, Reason: SkipBlank})
			continue
		}

		// Comment line
		if strings.HasPrefix(trimmed, "#") {
			result.Skipped = append(result.Skipped, SkippedLine{Line: lineNum, Reason: SkipComment})
			continue
		}

		// Must contain '='
		eqIdx := strings.Index(trimmed, "=")
		if eqIdx < 0 {
			// Not a valid key=value line; skip silently
			continue
		}

		key := strings.TrimRight(trimmed[:eqIdx], " \t")
		if key == "" {
			continue
		}
		rawValue := trimmed[eqIdx+1:]

		// Check for opening quote to determine if multiline
		// dqProcessed is true when interpolation was already checked pre-unescape for double-quoted values.
		var value string
		dqProcessed := false
		if len(rawValue) > 0 && (rawValue[0] == '"' || rawValue[0] == '\'') {
			quote := rawValue[0]
			inner := rawValue[1:]

			// Check if the closing quote is on the same line.
			// For double-quoted values, skip escaped quotes.
			var closeIdx int
			if quote == '"' {
				closeIdx = findUnescapedQuote(inner)
			} else {
				closeIdx = strings.IndexByte(inner, quote)
			}
			if closeIdx >= 0 {
				// Single-line quoted value.
				raw := inner[:closeIdx]
				if quote == '"' {
					// Use unescaped interpolation check so \${LITERAL} is not
					// treated as interpolation (only unescaped ${ counts).
					if containsUnescapedInterpolation(raw) {
						result.Skipped = append(result.Skipped, SkippedLine{Line: lineNum, Key: key, Reason: SkipInterpolation})
						continue
					}
					value = unescapeDoubleQuoted(raw)
					dqProcessed = true
				} else {
					value = raw
				}
			} else if quote == '"' {
				// Multiline: accumulate lines until an unescaped closing "
				startLine := lineNum
				var sb strings.Builder
				sb.WriteString(inner)
				terminated := false
				for scanner.Scan() {
					lineNum++
					nextLine := scanner.Text()
					closeIdx = findUnescapedQuote(nextLine)
					if closeIdx >= 0 {
						sb.WriteByte('\n')
						sb.WriteString(nextLine[:closeIdx])
						terminated = true
						break
					}
					sb.WriteByte('\n')
					sb.WriteString(nextLine)
				}
				if !terminated {
					return nil, fmt.Errorf("envfile: line %d: unterminated double-quoted value for key %q", startLine, key)
				}
				raw := sb.String()
				// Use unescaped check for multiline too: \${LITERAL} is not interpolation.
				if containsUnescapedInterpolation(raw) {
					result.Skipped = append(result.Skipped, SkippedLine{Line: startLine, Key: key, Reason: SkipInterpolation})
					continue
				}
				value = unescapeDoubleQuoted(raw)
				dqProcessed = true
			} else {
				// Unterminated single-quoted value: fail fast (consistent with double-quoted).
				return nil, fmt.Errorf("envfile: line %d: unterminated single-quoted value for key %q", lineNum, key)
			}
		} else {
			value = rawValue
		}

		// Check for interpolation (unquoted and single-quoted values; double-quoted already checked pre-unescape).
		if !dqProcessed && isInterpolation(value) {
			result.Skipped = append(result.Skipped, SkippedLine{Line: lineNum, Key: key, Reason: SkipInterpolation})
			continue
		}

		// Check for placeholder
		if isPlaceholder(value) {
			result.Skipped = append(result.Skipped, SkippedLine{Line: lineNum, Key: key, Reason: SkipPlaceholder})
			continue
		}

		result.Variables = append(result.Variables, Variable{
			Key:   key,
			Value: value,
			Line:  lineNum,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("envfile: scan: %w", err)
	}

	// Deduplicate variables: last occurrence wins.
	seen := make(map[string]int, len(result.Variables))
	deduped := make([]Variable, 0, len(result.Variables))
	for _, v := range result.Variables {
		if idx, ok := seen[v.Key]; ok {
			deduped[idx] = v
		} else {
			seen[v.Key] = len(deduped)
			deduped = append(deduped, v)
		}
	}
	result.Variables = deduped

	return result, nil
}

// findUnescapedQuote returns the index of the first unescaped double-quote in s,
// or -1 if none is found. A quote preceded by a backslash is considered escaped.
func findUnescapedQuote(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			// Count consecutive preceding backslashes.
			backslashes := 0
			for j := i - 1; j >= 0 && s[j] == '\\'; j-- {
				backslashes++
			}
			if backslashes%2 == 0 {
				return i
			}
		}
	}
	return -1
}

// unescapeDoubleQuoted processes escape sequences inside a double-quoted value:
// \\ → \, \" → ", \$ → $, \n → newline, \r → carriage return
func unescapeDoubleQuoted(s string) string {
	return strings.NewReplacer(`\\`, `\`, `\"`, `"`, `\$`, `$`, `\n`, "\n", `\r`, "\r").Replace(s)
}
