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
	SkipBlank         SkipReason = iota + 1
	SkipComment       SkipReason = iota
	SkipPlaceholder   SkipReason = iota
	SkipInterpolation SkipReason = iota
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

		key := trimmed[:eqIdx]
		rawValue := trimmed[eqIdx+1:]

		// Check for opening quote to determine if multiline
		var value string
		if len(rawValue) > 0 && (rawValue[0] == '"' || rawValue[0] == '\'') {
			quote := rawValue[0]
			inner := rawValue[1:]

			// Check if the closing quote is on the same line
			closeIdx := strings.IndexByte(inner, quote)
			if closeIdx >= 0 {
				// Single-line quoted value
				value = inner[:closeIdx]
			} else if quote == '"' {
				// Multiline: accumulate lines until closing "
				var sb strings.Builder
				sb.WriteString(inner)
				found := false
				for scanner.Scan() {
					lineNum++
					nextLine := scanner.Text()
					closeIdx = strings.IndexByte(nextLine, '"')
					if closeIdx >= 0 {
						// Closing quote found
						sb.WriteByte('\n')
						sb.WriteString(nextLine[:closeIdx])
						found = true
						break
					}
					sb.WriteByte('\n')
					sb.WriteString(nextLine)
				}
				if !found {
					// EOF without closing quote; take what we have
				}
				value = sb.String()
			} else {
				// Single-quoted multiline not supported; treat remainder as value
				value = inner
			}
		} else {
			value = rawValue
		}

		// Check for interpolation
		if isInterpolation(value) {
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

	return result, nil
}
