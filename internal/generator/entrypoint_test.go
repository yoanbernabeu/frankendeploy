package generator

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// renderEntrypointScript renders the entrypoint template and strips the final
// `main "$@"` call so the script can be sourced in tests without executing.
func renderEntrypointScript(t *testing.T, attempts, interval int) string {
	t.Helper()
	loader := NewTemplateLoader()
	out, err := loader.Execute("docker-entrypoint.tmpl", EntrypointData{
		MaxDBWaitAttempts: attempts,
		DBWaitInterval:    interval,
	})
	if err != nil {
		t.Fatalf("failed to render entrypoint template: %v", err)
	}
	return strings.Replace(out, "\nmain \"$@\"", "", 1)
}

// runWaitForDatabase sources the rendered entrypoint with a fake `nc` on PATH
// and runs wait_for_database with the given DATABASE_URL. It returns the nc
// invocations ("host port" lines), the combined output, and the exit error.
func runWaitForDatabase(t *testing.T, databaseURL string, ncExit int) (ncCalls []string, output string, runErr error) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}

	dir := t.TempDir()

	scriptPath := filepath.Join(dir, "entrypoint.sh")
	// Small attempts and zero interval keep failure cases fast.
	if err := os.WriteFile(scriptPath, []byte(renderEntrypointScript(t, 2, 0)), 0755); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	ncLog := filepath.Join(dir, "nc.log")
	fakeNc := "#!/bin/sh\necho \"$*\" >> \"" + ncLog + "\"\nexit " + map[bool]string{true: "0", false: "1"}[ncExit == 0] + "\n"
	if err := os.WriteFile(filepath.Join(binDir, "nc"), []byte(fakeNc), 0755); err != nil {
		t.Fatalf("failed to write fake nc: %v", err)
	}

	cmd := exec.Command("sh", "-c", ". \""+scriptPath+"\"; wait_for_database")
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+":"+os.Getenv("PATH"),
		"DATABASE_URL="+databaseURL,
	)
	outBytes, err := cmd.CombinedOutput()

	if data, readErr := os.ReadFile(ncLog); readErr == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if line != "" {
				ncCalls = append(ncCalls, line)
			}
		}
	}
	return ncCalls, string(outBytes), err
}

func TestEntrypoint_WaitForDatabase_HostAndPort(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		wantNcCall string // "" means nc must NOT be called
	}{
		// The #43 crash-loop: no explicit port must fall back to the scheme default.
		{"postgresql without port", "postgresql://user:pass@db/app?serverVersion=16", "-z db 5432"},
		{"mysql without port", "mysql://user:pass@db/app", "-z db 3306"},
		{"postgresql with port", "postgresql://user:pass@db:5433/app", "-z db 5433"},
		{"mysql with port", "mysql://user:pass@db:3307/app", "-z db 3307"},
		// Doctrine also accepts these schemes.
		{"postgres scheme", "postgres://user:pass@db/app", "-z db 5432"},
		{"pgsql scheme", "pgsql://user:pass@db:5432/app", "-z db 5432"},
		{"mariadb scheme", "mariadb://user:pass@db/app?serverVersion=10.11.2-MariaDB", "-z db 3306"},
		{"mysql2 scheme", "mysql2://user:pass@db/app", "-z db 3306"},
		// Credentials containing '@' or ':' must not break host extraction.
		{"password with @", "postgresql://user:p%40ss@db:5432/app", "-z db 5432"},
		{"password with raw @", "postgresql://user:p@ss@db/app", "-z db 5432"},
		// Nothing to wait for.
		{"sqlite", "sqlite:///%kernel.project_dir%/var/data.db", ""},
		{"localhost is skipped", "postgresql://user:pass@localhost:5432/app", ""},
		{"empty url", "", ""},
		{"unknown scheme", "redis://redis:6379", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ncCalls, output, err := runWaitForDatabase(t, tt.url, 0)
			if err != nil {
				t.Fatalf("wait_for_database failed: %v\noutput: %s", err, output)
			}
			if tt.wantNcCall == "" {
				if len(ncCalls) != 0 {
					t.Errorf("expected no nc call, got %v", ncCalls)
				}
				return
			}
			if len(ncCalls) == 0 {
				t.Fatalf("expected nc call %q, nc was never called\noutput: %s", tt.wantNcCall, output)
			}
			if ncCalls[0] != tt.wantNcCall {
				t.Errorf("nc called with %q, want %q", ncCalls[0], tt.wantNcCall)
			}
		})
	}
}

func TestEntrypoint_WaitForDatabase_UnreachableDatabaseFails(t *testing.T) {
	_, output, err := runWaitForDatabase(t, "postgresql://user:pass@db:5432/app", 1)
	if err == nil {
		t.Fatal("expected wait_for_database to exit non-zero when the database is unreachable")
	}
	if !strings.Contains(output, "Database not available") {
		t.Errorf("expected 'Database not available' in output, got: %s", output)
	}
}
