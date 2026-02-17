package classifier

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func defaultClassifier() *Classifier {
	return New(Rules{})
}

// --- Masked classification ---

func TestClassify_APIKey_LongValue_Masked(t *testing.T) {
	c := defaultClassifier()
	got := c.Classify("API_KEY", "longsecretvalue123", "staging")
	assert.True(t, got.Masked)
	assert.Equal(t, "env_var", got.VarType)
}

func TestClassify_APIKey_ShortValue_NotMasked(t *testing.T) {
	c := defaultClassifier()
	// value < 8 chars → not masked
	got := c.Classify("API_KEY", "short", "staging")
	assert.False(t, got.Masked)
}

func TestClassify_MaxTokens_NotMasked(t *testing.T) {
	// MAX_TOKENS matches masked pattern suffix TOKEN but is in exclude list
	c := defaultClassifier()
	got := c.Classify("MAX_TOKENS", "longenoughvalue123", "staging")
	assert.False(t, got.Masked)
}

func TestClassify_Timeout_NotMasked(t *testing.T) {
	c := defaultClassifier()
	got := c.Classify("REQUEST_TIMEOUT", "longenoughvalue123", "staging")
	assert.False(t, got.Masked)
}

func TestClassify_Port_NotMasked(t *testing.T) {
	c := defaultClassifier()
	got := c.Classify("DB_PORT", "longenoughvalue123", "staging")
	assert.False(t, got.Masked)
}

func TestClassify_DBPassword_Staging_MaskedNotProtected(t *testing.T) {
	c := defaultClassifier()
	got := c.Classify("DB_PASSWORD", "supersecretvalue", "staging")
	assert.True(t, got.Masked)
	assert.False(t, got.Protected)
}

func TestClassify_DBPassword_Production_MaskedAndProtected(t *testing.T) {
	c := defaultClassifier()
	got := c.Classify("DB_PASSWORD", "supersecretvalue", "production")
	assert.True(t, got.Masked)
	assert.True(t, got.Protected)
}

func TestClassify_LogLevel_Production_NeitherMaskedNorProtected(t *testing.T) {
	c := defaultClassifier()
	got := c.Classify("LOG_LEVEL", "info", "production")
	assert.False(t, got.Masked)
	assert.False(t, got.Protected)
}

func TestClassify_Secret_LongValue_Masked(t *testing.T) {
	c := defaultClassifier()
	got := c.Classify("MY_SECRET", "verylongsecretvalue", "staging")
	assert.True(t, got.Masked)
}

func TestClassify_DSN_LongValue_Masked(t *testing.T) {
	c := defaultClassifier()
	got := c.Classify("DATABASE_DSN", "postgres://user:pass@host/db", "staging")
	assert.True(t, got.Masked)
}

func TestClassify_Token_Multiline_NotMasked(t *testing.T) {
	// Multiline value → not eligible for masking (GitLab rejects multiline masked vars)
	c := defaultClassifier()
	got := c.Classify("AUTH_TOKEN", "line1\nline2\nline3", "staging")
	assert.False(t, got.Masked)
}

// --- File type classification ---

func TestClassify_PrivateKey_FileType(t *testing.T) {
	c := defaultClassifier()
	got := c.Classify("PRIVATE_KEY", "somevalue", "staging")
	assert.Equal(t, "file", got.VarType)
}

func TestClassify_CACert_FileType(t *testing.T) {
	c := defaultClassifier()
	got := c.Classify("CA_CERT", "somevalue", "staging")
	assert.Equal(t, "file", got.VarType)
}

func TestClassify_TLSPem_FileType(t *testing.T) {
	c := defaultClassifier()
	got := c.Classify("TLS_PEM", "somevalue", "staging")
	assert.Equal(t, "file", got.VarType)
}

func TestClassify_CertPath_NotFileType(t *testing.T) {
	// _PATH suffix is in file exclude list
	c := defaultClassifier()
	got := c.Classify("CERT_PATH", "somevalue", "staging")
	assert.Equal(t, "env_var", got.VarType)
}

func TestClassify_CertDir_NotFileType(t *testing.T) {
	c := defaultClassifier()
	got := c.Classify("CERT_DIR", "somevalue", "staging")
	assert.Equal(t, "env_var", got.VarType)
}

func TestClassify_CertURL_NotFileType(t *testing.T) {
	c := defaultClassifier()
	got := c.Classify("CERT_URL", "https://example.com", "staging")
	assert.Equal(t, "env_var", got.VarType)
}

func TestClassify_PEMValueDetected_FileType(t *testing.T) {
	// Value contains -----BEGIN → file type regardless of key
	c := defaultClassifier()
	got := c.Classify("MY_VAR", "-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----", "staging")
	assert.Equal(t, "file", got.VarType)
}

func TestClassify_PEMValue_TokenKey_FileType(t *testing.T) {
	// Even if key would be masked, PEM in value → file type (file takes precedence)
	c := defaultClassifier()
	got := c.Classify("AUTH_TOKEN", "-----BEGIN RSA PRIVATE KEY-----\nMIIEo...\n-----END RSA PRIVATE KEY-----", "staging")
	assert.Equal(t, "file", got.VarType)
}

// --- Custom rules ---

func TestClassify_CustomMaskedPattern(t *testing.T) {
	c := New(Rules{
		MaskedPatterns: []string{"CUSTOM_SECRET"},
	})
	got := c.Classify("MY_CUSTOM_SECRET", "longenoughvalue123", "staging")
	assert.True(t, got.Masked)
}

func TestClassify_CustomMaskedExclude(t *testing.T) {
	// Add built-in pattern but also exclude it via custom rule
	c := New(Rules{
		MaskedExclude: []string{"API_KEY"},
	})
	got := c.Classify("API_KEY", "longenoughvalue123", "staging")
	assert.False(t, got.Masked)
}

func TestClassify_CustomFilePattern(t *testing.T) {
	c := New(Rules{
		FilePatterns: []string{"_BUNDLE"},
	})
	got := c.Classify("APP_BUNDLE", "somevalue", "staging")
	assert.Equal(t, "file", got.VarType)
}

func TestClassify_CustomFileExclude(t *testing.T) {
	// _CERT is built-in file pattern, exclude it via user rule
	c := New(Rules{
		FileExclude: []string{"_CERT"},
	})
	got := c.Classify("CA_CERT", "somevalue", "staging")
	assert.Equal(t, "env_var", got.VarType)
}

// --- Table-driven comprehensive test ---

func TestClassify_TableDriven(t *testing.T) {
	c := defaultClassifier()

	tests := []struct {
		name        string
		key         string
		value       string
		env         string
		wantMasked  bool
		wantProtected bool
		wantVarType string
	}{
		{
			name:        "simple var no classification",
			key:         "APP_NAME",
			value:       "myapp",
			env:         "staging",
			wantMasked:  false,
			wantProtected: false,
			wantVarType: "env_var",
		},
		{
			name:        "token long enough",
			key:         "GITHUB_TOKEN",
			value:       "ghp_longsecrettoken123",
			env:         "staging",
			wantMasked:  true,
			wantProtected: false,
			wantVarType: "env_var",
		},
		{
			name:        "password production",
			key:         "DB_PASSWORD",
			value:       "supersecretpass",
			env:         "production",
			wantMasked:  true,
			wantProtected: true,
			wantVarType: "env_var",
		},
		{
			name:        "private key file type",
			key:         "RSA_PRIVATE_KEY",
			value:       "any value",
			env:         "staging",
			wantMasked:  false,
			wantProtected: false,
			wantVarType: "file",
		},
		{
			name:        "pem header in value",
			key:         "MY_CERT_DATA",
			value:       "-----BEGIN CERTIFICATE-----\ndata\n-----END CERTIFICATE-----",
			env:         "staging",
			wantMasked:  false,
			wantProtected: false,
			wantVarType: "file",
		},
		{
			name:        "max_tokens excluded",
			key:         "MAX_TOKENS",
			value:       "longvaluehere123",
			env:         "staging",
			wantMasked:  false,
			wantProtected: false,
			wantVarType: "env_var",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.Classify(tt.key, tt.value, tt.env)
			assert.Equal(t, tt.wantMasked, got.Masked, "masked")
			assert.Equal(t, tt.wantProtected, got.Protected, "protected")
			assert.Equal(t, tt.wantVarType, got.VarType, "var_type")
		})
	}
}

// --- Edge cases ---

func TestClassify_ExactlyEightChars_Masked(t *testing.T) {
	c := defaultClassifier()
	// exactly 8 chars → masked (>= 8)
	got := c.Classify("MY_SECRET", "12345678", "staging")
	assert.True(t, got.Masked)
}

func TestClassify_SevenChars_NotMasked(t *testing.T) {
	c := defaultClassifier()
	got := c.Classify("MY_SECRET", "1234567", "staging")
	assert.False(t, got.Masked)
}

func TestClassify_EmptyEnvironment_NotProtected(t *testing.T) {
	c := defaultClassifier()
	got := c.Classify("DB_PASSWORD", "supersecretvalue", "")
	assert.False(t, got.Protected)
}

func TestClassify_ProductionEnvCaseSensitive(t *testing.T) {
	c := defaultClassifier()
	// "Production" (capital P) is NOT "production" → not protected
	got := c.Classify("DB_PASSWORD", "supersecretvalue", "Production")
	assert.False(t, got.Protected)
}

func TestClassify_MatchCaseInsensitive_Masked(t *testing.T) {
	c := defaultClassifier()
	// key pattern matching is case-insensitive
	got := c.Classify("db_password", "supersecretvalue", "staging")
	assert.True(t, got.Masked)
}
