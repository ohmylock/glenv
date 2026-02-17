package classifier

import "strings"

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
func New(userRules Rules) *Classifier {
	c := &Classifier{
		maskedPatterns: append(builtinMaskedPatterns, userRules.MaskedPatterns...),
		maskedExclude:  append(builtinMaskedExclude, userRules.MaskedExclude...),
		filePatterns:   append(builtinFilePatterns, userRules.FilePatterns...),
		fileExclude:    append(builtinFileExclude, userRules.FileExclude...),
	}
	return c
}

// Classify determines the classification of a variable given its key, value, and
// deployment environment.
func (c *Classifier) Classify(key, value, environment string) Classification {
	cl := Classification{VarType: "env_var"}

	// File type check takes priority over masked.
	if c.matchesFile(key, value) {
		cl.VarType = "file"
		// File variables are never masked (GitLab handles them differently).
		return cl
	}

	// Masked: key matches secret pattern AND value >= 8 chars AND single-line.
	if c.matchesMasked(key) && len(value) >= 8 && !strings.Contains(value, "\n") {
		cl.Masked = true
	}

	// Protected: production environment AND key matches secret patterns.
	if environment == "production" && c.matchesMasked(key) {
		cl.Protected = true
	}

	return cl
}

// matchesMasked returns true if the key matches a masked pattern and is NOT in
// the exclude list. Exclude is checked first (exclude-first logic).
func (c *Classifier) matchesMasked(key string) bool {
	upper := strings.ToUpper(key)
	for _, excl := range c.maskedExclude {
		if strings.Contains(upper, strings.ToUpper(excl)) {
			return false
		}
	}
	for _, pat := range c.maskedPatterns {
		if strings.Contains(upper, strings.ToUpper(pat)) {
			return true
		}
	}
	return false
}

// matchesFile returns true if the key matches a file pattern (and is NOT excluded)
// OR if the value contains a PEM header.
func (c *Classifier) matchesFile(key, value string) bool {
	// PEM detection in value is always a file, regardless of key.
	if strings.Contains(value, "-----BEGIN") {
		return true
	}

	upper := strings.ToUpper(key)
	for _, excl := range c.fileExclude {
		if strings.Contains(upper, strings.ToUpper(excl)) {
			return false
		}
	}
	for _, pat := range c.filePatterns {
		if strings.Contains(upper, strings.ToUpper(pat)) {
			return true
		}
	}
	return false
}
