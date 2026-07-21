package deploy

// Tests for issue #56: unified env-file writer with strict permissions.

import (
	"context"
	"strings"
	"testing"

	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

func TestBuildEnvContent_SortedKeys(t *testing.T) {
	vars := map[string]string{
		"ZULU":  "1",
		"ALPHA": "2",
		"MIKE":  "3",
	}

	content := BuildEnvContent(vars)
	alpha := strings.Index(content, "ALPHA=")
	mike := strings.Index(content, "MIKE=")
	zulu := strings.Index(content, "ZULU=")
	if alpha == -1 || mike == -1 || zulu == -1 {
		t.Fatalf("missing keys in content: %q", content)
	}
	if !(alpha < mike && mike < zulu) {
		t.Errorf("keys must be sorted for stable diffs, got: %q", content)
	}

	// Deterministic across calls
	if BuildEnvContent(vars) != content {
		t.Error("BuildEnvContent must be deterministic")
	}
}

func TestBuildEnvContent_EscapesQuotes(t *testing.T) {
	vars := map[string]string{"MESSAGE": `he said "hello" to me`}

	content := BuildEnvContent(vars)
	if !strings.Contains(content, `MESSAGE="he said \"hello\" to me"`) {
		t.Errorf("double quotes must be escaped, got: %q", content)
	}
}

func TestBuildEnvContent_EscapesBackslashes(t *testing.T) {
	vars := map[string]string{"WINPATH": `C:\Users\app "x"`}

	content := BuildEnvContent(vars)
	if !strings.Contains(content, `WINPATH="C:\\Users\\app \"x\""`) {
		t.Errorf("backslashes must be escaped inside quoted values, got: %q", content)
	}
}

func TestBuildEnvContent_SimpleValueUnquoted(t *testing.T) {
	content := BuildEnvContent(map[string]string{"APP_ENV": "prod"})
	if !strings.Contains(content, "APP_ENV=prod\n") {
		t.Errorf("simple values should stay unquoted, got: %q", content)
	}
}

func TestParseEnvContent_RoundTrip(t *testing.T) {
	original := map[string]string{
		"SIMPLE":    "value",
		"SPACED":    "a value with spaces",
		"QUOTED":    `he said "hello"`,
		"BACKSLASH": `C:\path with "quote"`,
		"URL":       "postgresql://user:pass@host:5432/db?serverVersion=16",
	}

	parsed := ParseEnvContent(BuildEnvContent(original))
	for key, want := range original {
		if got, ok := parsed[key]; !ok || got != want {
			t.Errorf("round trip %s: got %q, want %q", key, parsed[key], want)
		}
	}
}

func TestParseEnvContent_SkipsCommentsAndBlanks(t *testing.T) {
	content := "# comment\n\nKEY=value\n  # indented comment\nOTHER=x\n"
	vars := ParseEnvContent(content)
	if len(vars) != 2 || vars["KEY"] != "value" || vars["OTHER"] != "x" {
		t.Errorf("unexpected parse result: %v", vars)
	}
}

func TestWriteEnvVars_PermissionsAndOwnership(t *testing.T) {
	mock := &ssh.MockExecutor{}

	err := WriteEnvVars(context.Background(), mock, "myapp", map[string]string{"APP_SECRET": "s3cret"})
	if err != nil {
		t.Fatalf("WriteEnvVars() error = %v", err)
	}

	all := strings.Join(mock.Commands, "\n---\n")
	if !strings.Contains(all, "mkdir -p") {
		t.Error("WriteEnvVars should ensure the directory exists")
	}
	if !strings.Contains(all, "APP_SECRET=s3cret") {
		t.Error("WriteEnvVars should write the variables")
	}
	if !strings.Contains(all, "chmod 600") {
		t.Error("WriteEnvVars must chmod 600 the env file: secrets must not be world-readable")
	}
	if !strings.Contains(all, "chown") {
		t.Error("WriteEnvVars should chown the env file for the container user")
	}

	// chmod must come after the write
	writeIdx, chmodIdx := -1, -1
	for i, c := range mock.Commands {
		if strings.Contains(c, "APP_SECRET") {
			writeIdx = i
		}
		if strings.Contains(c, "chmod 600") {
			chmodIdx = i
		}
	}
	if chmodIdx < writeIdx {
		t.Error("chmod 600 must happen after the file write")
	}
}

func TestReadEnvVars_ParsesRemoteFile(t *testing.T) {
	mock := &ssh.MockExecutor{
		ExecFunc: func(ctx context.Context, command string) (*ssh.ExecResult, error) {
			return &ssh.ExecResult{Stdout: "KEY=value\nOTHER=\"a b\"\n", ExitCode: 0}, nil
		},
	}

	vars, err := ReadEnvVars(context.Background(), mock, "myapp")
	if err != nil {
		t.Fatalf("ReadEnvVars() error = %v", err)
	}
	if vars["KEY"] != "value" || vars["OTHER"] != "a b" {
		t.Errorf("unexpected vars: %v", vars)
	}
}
