package envfile

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseReader_SimpleKeyValue(t *testing.T) {
	input := "KEY=VALUE\n"
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Variables, 1)
	assert.Equal(t, "KEY", result.Variables[0].Key)
	assert.Equal(t, "VALUE", result.Variables[0].Value)
	assert.Equal(t, 1, result.Variables[0].Line)
}

func TestParseReader_DoubleQuoted(t *testing.T) {
	input := `KEY="value with spaces"` + "\n"
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Variables, 1)
	assert.Equal(t, "value with spaces", result.Variables[0].Value)
}

func TestParseReader_SingleQuoted(t *testing.T) {
	input := "KEY='value'\n"
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Variables, 1)
	assert.Equal(t, "value", result.Variables[0].Value)
}

func TestParseReader_EmptyValue(t *testing.T) {
	input := "KEY=\n"
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Variables, 1)
	assert.Equal(t, "KEY", result.Variables[0].Key)
	assert.Equal(t, "", result.Variables[0].Value)
}

func TestParseReader_EmptyValueDoubleQuoted(t *testing.T) {
	input := `KEY=""` + "\n"
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Variables, 1)
	assert.Equal(t, "", result.Variables[0].Value)
}

func TestParseReader_CommentLine(t *testing.T) {
	input := "# this is a comment\nKEY=VALUE\n"
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Variables, 1)
	require.Len(t, result.Skipped, 1)
	assert.Equal(t, SkipComment, result.Skipped[0].Reason)
	assert.Equal(t, 1, result.Skipped[0].Line)
}

func TestParseReader_BlankLine(t *testing.T) {
	input := "\nKEY=VALUE\n"
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Variables, 1)
	require.Len(t, result.Skipped, 1)
	assert.Equal(t, SkipBlank, result.Skipped[0].Reason)
}

func TestParseReader_PlaceholderSkip(t *testing.T) {
	tests := []struct {
		name  string
		input string
		key   string
	}{
		{"your_ prefix in value", "KEY=your_value_here\n", "KEY"},
		{"CHANGE_ME in value", "KEY=CHANGE_ME\n", "KEY"},
		{"REPLACE_WITH_ in value", "KEY=REPLACE_WITH_something\n", "KEY"},
		{"your_ prefix case insensitive", "KEY=YOUR_VALUE\n", "KEY"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseReader(strings.NewReader(tt.input))
			require.NoError(t, err)
			assert.Empty(t, result.Variables)
			require.Len(t, result.Skipped, 1)
			assert.Equal(t, SkipPlaceholder, result.Skipped[0].Reason)
			assert.Equal(t, tt.key, result.Skipped[0].Key)
		})
	}
}

func TestParseReader_InterpolationSkip(t *testing.T) {
	input := "KEY=${OTHER_VAR}\n"
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	assert.Empty(t, result.Variables)
	require.Len(t, result.Skipped, 1)
	assert.Equal(t, SkipInterpolation, result.Skipped[0].Reason)
	assert.Equal(t, "KEY", result.Skipped[0].Key)
}

func TestParseReader_InterpolationSkip_Partial(t *testing.T) {
	// Value contains ${...} anywhere
	input := "KEY=prefix_${VAR}_suffix\n"
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	assert.Empty(t, result.Variables)
	require.Len(t, result.Skipped, 1)
	assert.Equal(t, SkipInterpolation, result.Skipped[0].Reason)
}

func TestParseReader_MultilineDoubleQuoted(t *testing.T) {
	input := "CERT=\"-----BEGIN CERTIFICATE-----\nMIIBkTCB+wIJ\n-----END CERTIFICATE-----\"\n"
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Variables, 1)
	assert.Equal(t, "CERT", result.Variables[0].Key)
	assert.Contains(t, result.Variables[0].Value, "-----BEGIN CERTIFICATE-----")
	assert.Contains(t, result.Variables[0].Value, "-----END CERTIFICATE-----")
	assert.Contains(t, result.Variables[0].Value, "\n")
}

func TestParseReader_ValueContainingHash(t *testing.T) {
	// Hash in value is NOT a comment — kept as-is
	input := "KEY=value#with#hash\n"
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Variables, 1)
	assert.Equal(t, "value#with#hash", result.Variables[0].Value)
}

func TestParseReader_ValueWithEqualSign(t *testing.T) {
	// Value may contain = sign (split on first = only)
	input := "KEY=val=ue\n"
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Variables, 1)
	assert.Equal(t, "val=ue", result.Variables[0].Value)
}

func TestParseReader_MultipleVars(t *testing.T) {
	input := "A=1\nB=2\nC=3\n"
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Variables, 3)
	assert.Equal(t, "A", result.Variables[0].Key)
	assert.Equal(t, "B", result.Variables[1].Key)
	assert.Equal(t, "C", result.Variables[2].Key)
}

func TestParseReader_LineNumbers(t *testing.T) {
	input := "# comment\n\nKEY=VALUE\n"
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Variables, 1)
	assert.Equal(t, 3, result.Variables[0].Line)
}

func TestParseReader_NoTrailingNewline(t *testing.T) {
	input := "KEY=VALUE"
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Variables, 1)
	assert.Equal(t, "VALUE", result.Variables[0].Value)
}

func TestParseReader_WhitespaceAroundEquals(t *testing.T) {
	// Trailing whitespace on unquoted values is stripped (line-level TrimSpace)
	// but leading space after = is preserved in the raw value split.
	input := "KEY= VALUE\n"
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Variables, 1)
	assert.Equal(t, " VALUE", result.Variables[0].Value)
}

func TestParseReader_TrailingWhitespace_Stripped(t *testing.T) {
	// Trailing whitespace on unquoted values must be stripped by TrimSpace.
	input := "KEY=VALUE   \n"
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Variables, 1)
	assert.Equal(t, "VALUE", result.Variables[0].Value)
}

func TestParseReader_QuotedValueWithNewlines_MultilineMode(t *testing.T) {
	// PEM block in double quotes
	input := `PRIVATE_KEY="-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA1234567890
-----END RSA PRIVATE KEY-----"
`
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Variables, 1)
	assert.Equal(t, "PRIVATE_KEY", result.Variables[0].Key)
	assert.Contains(t, result.Variables[0].Value, "-----BEGIN RSA PRIVATE KEY-----")
	assert.Contains(t, result.Variables[0].Value, "-----END RSA PRIVATE KEY-----")
}

func TestParseReader_ExportPrefix(t *testing.T) {
	// Lines starting with "export " should strip the prefix
	input := "export KEY=VALUE\n"
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Variables, 1)
	assert.Equal(t, "KEY", result.Variables[0].Key)
	assert.Equal(t, "VALUE", result.Variables[0].Value)
}

func TestParseReader_InvalidLine_NoEquals(t *testing.T) {
	// Line without = sign is skipped
	input := "INVALID_LINE\nKEY=VALUE\n"
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Variables, 1)
	assert.Equal(t, "KEY", result.Variables[0].Key)
}

// Integration test using a temp file
func TestParseFile_Integration(t *testing.T) {
	content := `# Database config
DB_HOST=localhost
DB_PORT=5432
DB_PASSWORD=supersecretpassword123

# Placeholders — these should be skipped
API_KEY=CHANGE_ME
TOKEN=your_token_here

# Interpolation — skip
COMPUTED=${DB_HOST}

# Multiline cert
TLS_CERT="-----BEGIN CERTIFICATE-----
MIIB...
-----END CERTIFICATE-----"

SIMPLE=value
`
	tmpFile, err := os.CreateTemp("", "test.env")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	result, err := ParseFile(tmpFile.Name())
	require.NoError(t, err)

	// Should have: DB_HOST, DB_PORT, DB_PASSWORD, TLS_CERT, SIMPLE
	keys := make(map[string]string)
	for _, v := range result.Variables {
		keys[v.Key] = v.Value
	}
	assert.Equal(t, "localhost", keys["DB_HOST"])
	assert.Equal(t, "5432", keys["DB_PORT"])
	assert.Equal(t, "supersecretpassword123", keys["DB_PASSWORD"])
	assert.Equal(t, "value", keys["SIMPLE"])
	assert.Contains(t, keys["TLS_CERT"], "-----BEGIN CERTIFICATE-----")

	// API_KEY=CHANGE_ME and TOKEN=your_token_here should be skipped
	_, apiKeyFound := keys["API_KEY"]
	assert.False(t, apiKeyFound, "placeholder should be skipped")
	_, tokenFound := keys["TOKEN"]
	assert.False(t, tokenFound, "placeholder should be skipped")

	// COMPUTED=${DB_HOST} should be skipped
	_, computedFound := keys["COMPUTED"]
	assert.False(t, computedFound, "interpolation should be skipped")

	// Check skipped entries
	skippedReasons := make(map[string]SkipReason)
	for _, s := range result.Skipped {
		if s.Key != "" {
			skippedReasons[s.Key] = s.Reason
		}
	}
	assert.Equal(t, SkipPlaceholder, skippedReasons["API_KEY"])
	assert.Equal(t, SkipPlaceholder, skippedReasons["TOKEN"])
	assert.Equal(t, SkipInterpolation, skippedReasons["COMPUTED"])
}

func TestParseFile_NotFound(t *testing.T) {
	_, err := ParseFile("/nonexistent/path/file.env")
	assert.Error(t, err)
}

func TestParseReader_EmptyInput(t *testing.T) {
	result, err := ParseReader(strings.NewReader(""))
	require.NoError(t, err)
	assert.Empty(t, result.Variables)
	assert.Empty(t, result.Skipped)
}

// Table-driven comprehensive test
func TestParseReader_TableDriven(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantVarCount  int
		wantSkipCount int
		wantKey       string
		wantValue     string
		wantSkipReason SkipReason
	}{
		{
			name:         "simple unquoted",
			input:        "FOO=bar\n",
			wantVarCount: 1,
			wantKey:      "FOO",
			wantValue:    "bar",
		},
		{
			name:         "double quoted with spaces",
			input:        `FOO="hello world"` + "\n",
			wantVarCount: 1,
			wantKey:      "FOO",
			wantValue:    "hello world",
		},
		{
			name:         "single quoted",
			input:        "FOO='bar baz'\n",
			wantVarCount: 1,
			wantKey:      "FOO",
			wantValue:    "bar baz",
		},
		{
			name:         "empty value",
			input:        "FOO=\n",
			wantVarCount: 1,
			wantKey:      "FOO",
			wantValue:    "",
		},
		{
			name:          "comment skipped",
			input:         "# comment\n",
			wantVarCount:  0,
			wantSkipCount: 1,
			wantSkipReason: SkipComment,
		},
		{
			name:          "blank line skipped",
			input:         "\n",
			wantVarCount:  0,
			wantSkipCount: 1,
			wantSkipReason: SkipBlank,
		},
		{
			name:          "placeholder CHANGE_ME skipped",
			input:         "KEY=CHANGE_ME\n",
			wantVarCount:  0,
			wantSkipCount: 1,
			wantSkipReason: SkipPlaceholder,
		},
		{
			name:          "interpolation skipped",
			input:         "KEY=${OTHER}\n",
			wantVarCount:  0,
			wantSkipCount: 1,
			wantSkipReason: SkipInterpolation,
		},
		{
			name:         "hash in value kept",
			input:        "KEY=color#red\n",
			wantVarCount: 1,
			wantKey:      "KEY",
			wantValue:    "color#red",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseReader(strings.NewReader(tt.input))
			require.NoError(t, err)
			assert.Len(t, result.Variables, tt.wantVarCount)
			if tt.wantSkipCount > 0 {
				assert.Len(t, result.Skipped, tt.wantSkipCount)
				if tt.wantSkipReason != 0 {
					assert.Equal(t, tt.wantSkipReason, result.Skipped[0].Reason)
				}
			}
			if tt.wantVarCount > 0 && tt.wantKey != "" {
				assert.Equal(t, tt.wantKey, result.Variables[0].Key)
				assert.Equal(t, tt.wantValue, result.Variables[0].Value)
			}
		})
	}
}

// --- Escaped interpolation in double-quoted values ---

func TestParseReader_EscapedDollar_NotSkipped(t *testing.T) {
	// \${LITERAL} inside double quotes is an escaped dollar, not interpolation.
	input := `KEY="\${LITERAL}"` + "\n"
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Variables, 1, "escaped \\$ should not trigger interpolation skip")
	assert.Equal(t, "${LITERAL}", result.Variables[0].Value)
}

func TestParseReader_UnescapedDollar_Skipped(t *testing.T) {
	// ${VAR} inside double quotes is real interpolation and should be skipped.
	input := `KEY="${VAR}"` + "\n"
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	assert.Empty(t, result.Variables)
	require.Len(t, result.Skipped, 1)
	assert.Equal(t, SkipInterpolation, result.Skipped[0].Reason)
}

func TestParseReader_MixedEscapedAndUnescaped_Skipped(t *testing.T) {
	// Value has both \${ESCAPED} and ${REAL} — the unescaped one means skip.
	input := `KEY="\${A} ${B}"` + "\n"
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	assert.Empty(t, result.Variables)
	require.Len(t, result.Skipped, 1)
	assert.Equal(t, SkipInterpolation, result.Skipped[0].Reason)
}

func TestParseReader_EscapedCarriageReturn_Roundtrip(t *testing.T) {
	// \r inside a double-quoted value must be decoded to CR so that
	// export → import is a lossless round-trip.
	input := "KEY=\"value\\rwith\\rcr\"\n"
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Variables, 1)
	assert.Equal(t, "value\rwith\rcr", result.Variables[0].Value)
}

// --- Unterminated single-quoted values ---

func TestParseReader_UnterminatedSingleQuote_Error(t *testing.T) {
	input := "KEY='hello world\n"
	_, err := ParseReader(strings.NewReader(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unterminated single-quoted value")
	assert.Contains(t, err.Error(), "KEY")
}

func TestParseReader_UnterminatedDoubleQuote_Error(t *testing.T) {
	input := "KEY=\"hello world\n"
	_, err := ParseReader(strings.NewReader(input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unterminated double-quoted value")
	assert.Contains(t, err.Error(), "KEY")
}

func TestParseReader_DuplicateKey_LastWins(t *testing.T) {
	input := "KEY=first\nKEY=second\n"
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, result.Variables, 1)
	assert.Equal(t, "second", result.Variables[0].Value)
	assert.Equal(t, 2, result.Variables[0].Line)
}

func TestParseReader_SingleQuotedInterpolation_Skipped(t *testing.T) {
	// Single-quoted values with ${} are treated as interpolation and skipped,
	// matching the unquoted value behavior (isInterpolation check).
	input := "KEY='${VAR}'\n"
	result, err := ParseReader(strings.NewReader(input))
	require.NoError(t, err)
	assert.Empty(t, result.Variables)
	require.Len(t, result.Skipped, 1)
	assert.Equal(t, "KEY", result.Skipped[0].Key)
}
