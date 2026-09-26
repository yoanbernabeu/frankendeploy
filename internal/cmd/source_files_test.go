package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

// gitProject creates a Git repository with the given files, commits the
// ones listed in commit, and returns its path.
func gitProject(t *testing.T, files map[string]string, commit []string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	if len(commit) > 0 {
		run(append([]string{"add", "--"}, commit...)...)
		run("commit", "-q", "-m", "init")
	}
	return dir
}

func TestSourceFiles_RespectsGitignore(t *testing.T) {
	dir := gitProject(t, map[string]string{
		".gitignore":          "/.env.prod.local\n/config/secrets/prod/prod.decrypt.private.php\n/dump.sql\n/public/bundles/\n/Dockerfile\n",
		"composer.json":       "{}",
		"src/Kernel.php":      "<?php",
		"src/New.php":         "<?php // not committed yet, not ignored",
		".env.prod.local":     "APP_SECRET=leak",
		"dump.sql":            "INSERT",
		"public/bundles/x.js": "bundle",
		"Dockerfile":          "FROM x",
		"config/secrets/prod/prod.decrypt.private.php": "<?php // key",
	}, []string{".gitignore", "composer.json", "src/Kernel.php"})

	files := sourceFiles(context.Background(), dir)

	for _, want := range []string{"composer.json", "src/Kernel.php", "src/New.php", "Dockerfile", "public/bundles/x.js"} {
		if !slices.Contains(files, want) {
			t.Errorf("expected %s to be transferred, got %v", want, files)
		}
	}
	for _, leak := range []string{".env.prod.local", "dump.sql", "config/secrets/prod/prod.decrypt.private.php"} {
		if slices.Contains(files, leak) {
			t.Errorf("%s is ignored by Git and must stay local, got %v", leak, files)
		}
	}
}

func TestSourceFiles_DeletedCommittedFileSkipped(t *testing.T) {
	dir := gitProject(t, map[string]string{"a.php": "a", "b.php": "b"}, []string{"a.php", "b.php"})
	if err := os.Remove(filepath.Join(dir, "b.php")); err != nil {
		t.Fatal(err)
	}

	if files := sourceFiles(context.Background(), dir); slices.Contains(files, "b.php") {
		t.Errorf("a committed file deleted locally must not be listed, got %v", files)
	}
}

func TestSourceFiles_NotAGitRepository(t *testing.T) {
	if files := sourceFiles(context.Background(), t.TempDir()); files != nil {
		t.Errorf("expected nil (walk fallback) outside Git, got %v", files)
	}
}

func TestSeedableSharedFiles(t *testing.T) {
	dir := gitProject(t, map[string]string{
		".gitignore":             "/data/local.sqlite\n",
		"data/database.sqlite":   "demo",
		"data/fixtures/a.json":   "{}",
		"data/changed.json":      "v1",
		"data/local.sqlite":      "dev accounts",
		"public/uploads/old.png": "png",
	}, []string{".gitignore", "data/database.sqlite", "data/fixtures/a.json", "data/changed.json", "public/uploads/old.png"})
	if err := os.WriteFile(filepath.Join(dir, "data", "changed.json"), []byte("v2"), 0644); err != nil {
		t.Fatal(err)
	}

	seeds, modified := seedableSharedFiles(context.Background(), dir, []string{"data", "var/log"})

	got := seeds["data"]
	slices.Sort(got)
	if want := []string{"database.sqlite", "fixtures/a.json"}; !slices.Equal(got, want) {
		t.Errorf("data seeds = %v, want %v (ignored and modified files excluded)", got, want)
	}
	if !slices.Equal(modified, []string{"data/changed.json"}) {
		t.Errorf("modified = %v, want [data/changed.json]", modified)
	}
	if _, ok := seeds["var/log"]; ok {
		t.Error("a shared dir without committed files must not be seeded")
	}
}

func TestSeedSharedDirs_OnlyEmptyDirs(t *testing.T) {
	mock := &ssh.MockExecutor{}
	mock.ExecFunc = func(ctx context.Context, command string) (*ssh.ExecResult, error) {
		// shared/data is empty, shared/uploads already holds files
		if strings.HasPrefix(command, "test -z") && strings.Contains(command, "uploads") {
			return &ssh.ExecResult{ExitCode: 1}, nil
		}
		if strings.HasPrefix(command, "docker run --rm") {
			return &ssh.ExecResult{Stdout: "1\n"}, nil
		}
		return &ssh.ExecResult{}, nil
	}
	seeds := map[string][]string{"data": {"database.sqlite"}, "uploads": {"logo.png"}}

	seedSharedDirs(context.Background(), mock, "app:1", "/opt/frankendeploy/apps/app/shared", seeds, nil)

	runs := commandsContaining(mock, "docker run --rm")
	if len(runs) != 1 {
		t.Fatalf("expected exactly one seeding run, got %v", runs)
	}
	for _, want := range []string{"/opt/frankendeploy/apps/app/shared/data", "database.sqlite", "app:1", "--user"} {
		if !strings.Contains(runs[0], want) {
			t.Errorf("seeding command missing %q: %s", want, runs[0])
		}
	}
	if strings.Contains(runs[0], "logo.png") {
		t.Error("a shared dir that already holds data must never be seeded")
	}
}
