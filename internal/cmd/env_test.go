package cmd

// Tests for issue #56: --from-stdin secret input for env set.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stdinFrom returns an *os.File whose content is the given string (a real
// file, so term.IsTerminal reports false — the piped-stdin path).
func stdinFrom(t *testing.T, content string) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write stdin file: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open stdin file: %v", err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestResolveEnvSetInput_KeyValue(t *testing.T) {
	key, value, err := resolveEnvSetInput("APP_SECRET=abc=def", false, nil)
	if err != nil {
		t.Fatalf("resolveEnvSetInput() error = %v", err)
	}
	if key != "APP_SECRET" || value != "abc=def" {
		t.Errorf("got (%q, %q), want (APP_SECRET, abc=def)", key, value)
	}
}

func TestResolveEnvSetInput_MissingEquals(t *testing.T) {
	if _, _, err := resolveEnvSetInput("APP_SECRET", false, nil); err == nil {
		t.Error("expected error for KEY without =value and without --from-stdin")
	}
}

func TestResolveEnvSetInput_InvalidKey(t *testing.T) {
	if _, _, err := resolveEnvSetInput("BAD-KEY=x", false, nil); err == nil {
		t.Error("expected error for invalid key")
	}
}

func TestResolveEnvSetInput_FromStdin(t *testing.T) {
	stdin := stdinFrom(t, "s3cret-value\n")
	key, value, err := resolveEnvSetInput("APP_SECRET", true, stdin)
	if err != nil {
		t.Fatalf("resolveEnvSetInput() error = %v", err)
	}
	if key != "APP_SECRET" || value != "s3cret-value" {
		t.Errorf("got (%q, %q), want (APP_SECRET, s3cret-value)", key, value)
	}
}

func TestResolveEnvSetInput_FromStdinTrimsTrailingNewlinesOnly(t *testing.T) {
	stdin := stdinFrom(t, "  spaced value \r\n")
	_, value, err := resolveEnvSetInput("APP_SECRET", true, stdin)
	if err != nil {
		t.Fatalf("resolveEnvSetInput() error = %v", err)
	}
	if value != "  spaced value " {
		t.Errorf("only trailing newlines must be trimmed, got %q", value)
	}
}

func TestResolveEnvSetInput_FromStdinRejectsKeyValue(t *testing.T) {
	stdin := stdinFrom(t, "x\n")
	if _, _, err := resolveEnvSetInput("APP_SECRET=x", true, stdin); err == nil {
		t.Error("expected error when --from-stdin is combined with KEY=value")
	}
}

func TestResolveEnvSetInput_FromStdinEmptyValue(t *testing.T) {
	stdin := stdinFrom(t, "\n")
	_, _, err := resolveEnvSetInput("APP_SECRET", true, stdin)
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected empty-value error, got: %v", err)
	}
}

func TestResolveEnvSetInput_FromStdinInvalidKey(t *testing.T) {
	stdin := stdinFrom(t, "x\n")
	if _, _, err := resolveEnvSetInput("BAD KEY", true, stdin); err == nil {
		t.Error("expected error for invalid key with --from-stdin")
	}
}
