package classifier

import (
	"slices"
	"strings"
)

// Classification holds the result of classifying a GitLab CI/CD variable.
type Classification struct {
	Masked    bool
	Protected bool
	// VarType is "env_var" or "file".
	VarType string
}

// Rules holds user-supplied pattern overrides that are merged with built-in rules.
type Rules struct {
	MaskedPatterns []string
	MaskedExclude  []string
	FilePatterns   []string
	FileExclude    []string
}

// Classifier classifies variables using merged built-in and user rules.
type Classifier struct {
	maskedPatterns []string
	maskedExclude  []string
	filePatterns   []string
	fileExclude    []string
}

// Built-in patterns (case-insensitive substring matching against uppercase key).
var (
	builtinMaskedPatterns = []string{"_TOKEN", "SECRET", "PASSWORD", "API_KEY", "DSN"}
	builtinMaskedExclude  = []string{"MAX_TOKENS", "TIMEOUT", "PORT"}
	builtinFilePatterns   = []string{"PRIVATE_KEY", "_CERT", "_PEM"}
	builtinFileExclude    = []string{"_PATH", "_DIR", "_URL"}
)

// New creates a Classifier by merging built-in rules with user-provided rules.
// User rules are appended to built-in rules (both patterns and excludes).
// All patterns are pre-normalized to uppercase for case-insensitive matching.
func New(userRules Rules) *Classifier {
	c := &Classifier{
		maskedPatterns: toUpper(slices.Concat(builtinMaskedPatterns, userRules.MaskedPatterns)),
		maskedExclude:  toUpper(slices.Concat(builtinMaskedExclude, userRules.MaskedExclude)),
		filePatterns:   toUpper(slices.Concat(builtinFilePatterns, userRules.FilePatterns)),
		fileExclude:    toUpper(slices.Concat(builtinFileExclude, userRules.FileExclude)),
	}
	return c
}

// NewEmpty creates a Classifier with no patterns at all (not even built-ins).
// Use this when auto-classification must be fully disabled.
func NewEmpty() *Classifier {
	return &Classifier{}
}

// toUpper returns a new slice with all strings converted to uppercase.
func toUpper(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = strings.ToUpper(s)
	}
	return out
}

// Classify determines the classification of a variable given its key, value, and
// deployment environment.
func (c *Classifier) Classify(key, value, environment string) Classification {
	cl := Classification{VarType: "env_var"}

	// File type check takes priority over masked.
	if c.matchesFile(key, value) {
		cl.VarType = "file"
		// File variables are never masked (GitLab handles them differently),
		// but they can still be protected.
		if environment == "production" {
			cl.Protected = true
		}
		return cl
	}

	// Masked: key matches secret pattern AND value is maskable by GitLab.
	// GitLab masked variables must be >=8 chars, single-line, and contain only
	// characters from the set: a-zA-Z0-9 and @:.~
	if c.matchesMasked(key) && isMaskable(value) {
		cl.Masked = true
	}

	// Protected: production environment AND key matches secret patterns.
	if environment == "production" && c.matchesMasked(key) {
		cl.Protected = true
	}

	return cl
}

// isMaskable checks if a value can be masked by GitLab.
// GitLab requires: >=8 chars, single-line with no spaces, and only chars from
// [a-zA-Z0-9_:@-.+~=/] (alphanumeric plus @, :, ., ~, _, -, +, =, /).
func isMaskable(value string) bool {
	if len(value) < 8 {
		return false
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == ':' || r == '@' || r == '-' || r == '+' || r == '.' || r == '~' || r == '=' || r == '/':
		default:
			return false
		}
	}
	return true
}

// matchesMasked returns true if the key matches a masked pattern and is NOT in
// the exclude list. Exclude is checked first (exclude-first logic).
func (c *Classifier) matchesMasked(key string) bool {
	upper := strings.ToUpper(key)
	for _, excl := range c.maskedExclude {
		if strings.Contains(upper, excl) {
			return false
		}
	}
	for _, pat := range c.maskedPatterns {
		if strings.Contains(upper, pat) {
			return true
		}
	}
	return false
}

// matchesFile returns true if the key matches a file pattern (and is NOT excluded)
// OR if the value contains a PEM header (only when patterns are configured).
func (c *Classifier) matchesFile(key, value string) bool {
	// PEM detection in value: only when the classifier has file patterns.
	// NewEmpty() sets no patterns, so --no-auto-classify fully disables this too.
	if len(c.filePatterns) > 0 && strings.Contains(value, "-----BEGIN") {
		return true
	}

	upper := strings.ToUpper(key)
	for _, excl := range c.fileExclude {
		if strings.Contains(upper, excl) {
			return false
		}
	}
	for _, pat := range c.filePatterns {
		if strings.Contains(upper, pat) {
			return true
		}
	}
	return false
}
