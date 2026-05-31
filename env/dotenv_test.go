package env

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDotEnvFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write dotenv file: %v", err)
	}
	return path
}

func preserveEnv(t *testing.T, keys ...string) {
	t.Helper()
	original := make(map[string]*string, len(keys))
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			v := value
			original[key] = &v
		}
	}
	t.Cleanup(func() {
		for _, key := range keys {
			if value, ok := original[key]; ok {
				_ = os.Setenv(key, *value)
				continue
			}
			_ = os.Unsetenv(key)
		}
	})
}

func TestLoadDotEnvSupportsCommonSyntax(t *testing.T) {
	const (
		keyPlain       = "GOKIT_ENV_TEST_PLAIN"
		keyExport      = "GOKIT_ENV_TEST_EXPORT"
		keyDouble      = "GOKIT_ENV_TEST_DOUBLE"
		keySingle      = "GOKIT_ENV_TEST_SINGLE"
		keyInline      = "GOKIT_ENV_TEST_INLINE"
		keyEmpty       = "GOKIT_ENV_TEST_EMPTY"
		keyHashLiteral = "GOKIT_ENV_TEST_HASH_LITERAL"
	)
	preserveEnv(t, keyPlain, keyExport, keyDouble, keySingle, keyInline, keyEmpty, keyHashLiteral)

	path := writeDotEnvFile(t, strings.Join([]string{
		"# comment",
		keyPlain + "=plain-value",
		"export " + keyExport + "=exported",
		keyDouble + "=\"line1\\nline2\\tindent\\\"quote\"",
		keySingle + "='single quoted value'",
		keyInline + "=trim me # this is a comment",
		keyEmpty + "=    # empty value with inline comment",
		keyHashLiteral + "=#not-a-comment",
	}, "\n"))

	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv returned error: %v", err)
	}

	assertEnvValue(t, keyPlain, "plain-value")
	assertEnvValue(t, keyExport, "exported")
	assertEnvValue(t, keyDouble, "line1\nline2\tindent\"quote")
	assertEnvValue(t, keySingle, "single quoted value")
	assertEnvValue(t, keyInline, "trim me")
	assertEnvValue(t, keyEmpty, "")
	assertEnvValue(t, keyHashLiteral, "#not-a-comment")
}

func TestLoadDotEnvDoesNotOverrideExistingByDefault(t *testing.T) {
	const key = "GOKIT_ENV_TEST_NO_OVERRIDE"
	preserveEnv(t, key)
	t.Setenv(key, "already-set")

	path := writeDotEnvFile(t, key+"=from-file\n")
	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv returned error: %v", err)
	}

	assertEnvValue(t, key, "already-set")
}

func TestLoadDotEnvWithOptionsCanOverrideExisting(t *testing.T) {
	const key = "GOKIT_ENV_TEST_OVERRIDE"
	preserveEnv(t, key)
	t.Setenv(key, "already-set")

	path := writeDotEnvFile(t, key+"=from-file\n")
	if err := LoadDotEnvWithOptions(path, LoadOptions{OverrideExisting: true}); err != nil {
		t.Fatalf("LoadDotEnvWithOptions returned error: %v", err)
	}

	assertEnvValue(t, key, "from-file")
}

func TestLoadDotEnvReturnsParseErrorWithLineNumber(t *testing.T) {
	path := writeDotEnvFile(t, strings.Join([]string{
		"GOOD_KEY=value",
		"BROKEN_LINE",
	}, "\n"))

	err := LoadDotEnv(path)
	if err == nil {
		t.Fatal("expected parse error")
	}

	var parseErr *ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if parseErr.Line != 2 {
		t.Fatalf("parse error line = %d, want 2", parseErr.Line)
	}
	if !strings.Contains(parseErr.Error(), "line 2") {
		t.Fatalf("unexpected error text: %v", parseErr)
	}
}

func TestLoadDotEnvRejectsTrailingContentAfterQuotedValue(t *testing.T) {
	path := writeDotEnvFile(t, "BAD=\"value\" trailing\n")

	err := LoadDotEnv(path)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "unexpected trailing content") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertEnvValue(t *testing.T, key, want string) {
	t.Helper()
	got, ok := os.LookupEnv(key)
	if !ok {
		t.Fatalf("expected %s to be set", key)
	}
	if got != want {
		t.Fatalf("%s = %q, want %q", key, got, want)
	}
}
