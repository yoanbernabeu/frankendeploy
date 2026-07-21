package deploy

// Tests for issue #47: automatic database backup before migration hooks.

import (
	"context"
	"strings"
	"testing"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

func managedPgConfig() *config.ProjectConfig {
	managed := true
	return &config.ProjectConfig{
		Name: "myapp",
		Database: config.DatabaseConfig{
			Driver:  "pgsql",
			Version: "16",
			Managed: &managed,
		},
		Deploy: config.DeployConfig{KeepReleases: 3},
	}
}

const testDatabaseURL = "postgresql://myapp:hex1234@myapp-db:5432/myapp?serverVersion=16&charset=utf8"

func TestParseDatabaseURL(t *testing.T) {
	user, password, dbName, err := parseDatabaseURL(testDatabaseURL)
	if err != nil {
		t.Fatalf("parseDatabaseURL() error = %v", err)
	}
	if user != "myapp" || password != "hex1234" || dbName != "myapp" {
		t.Errorf("got (%q, %q, %q), want (myapp, hex1234, myapp)", user, password, dbName)
	}
}

func TestParseDatabaseURL_Underscored(t *testing.T) {
	_, _, dbName, err := parseDatabaseURL("mysql://my_app:pw@my-app-db:3306/my_app?serverVersion=8.0")
	if err != nil {
		t.Fatalf("parseDatabaseURL() error = %v", err)
	}
	if dbName != "my_app" {
		t.Errorf("dbName = %q, want my_app", dbName)
	}
}

func TestParseDatabaseURL_Invalid(t *testing.T) {
	for _, bad := range []string{"", "not-a-url", "postgresql://host/"} {
		if _, _, _, err := parseDatabaseURL(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestBackupManagedDatabase_Success(t *testing.T) {
	mock := &ssh.MockExecutor{}

	path, err := BackupManagedDatabase(context.Background(), mock, managedPgConfig(), testDatabaseURL, "20260721-120000")
	if err != nil {
		t.Fatalf("BackupManagedDatabase() error = %v", err)
	}

	if !strings.Contains(path, "/opt/frankendeploy/apps/myapp/shared/backups/") {
		t.Errorf("backup path should live in shared/backups, got %q", path)
	}
	if !strings.Contains(path, "20260721-120000") {
		t.Errorf("backup path should embed the deploy tag, got %q", path)
	}
	if !strings.HasSuffix(path, ".sql.gz") {
		t.Errorf("backup should be gzipped SQL, got %q", path)
	}

	all := strings.Join(mock.Commands, "\n---\n")
	if !strings.Contains(all, "mkdir -p") {
		t.Error("should create the backups directory")
	}
	if !strings.Contains(all, "pg_dump") || !strings.Contains(all, "gzip") {
		t.Errorf("should run pg_dump piped to gzip, commands:\n%s", all)
	}
	if !strings.Contains(all, "myapp-db") {
		t.Error("dump should target the managed DB container")
	}
	if !strings.Contains(all, "chmod 600") {
		t.Error("backup file must be chmod 600 (it contains all the data)")
	}
	// Retention: keep_releases=3
	if !strings.Contains(all, "tail -n +4") {
		t.Errorf("retention should keep 3 backups (tail -n +4), commands:\n%s", all)
	}
}

func TestBackupManagedDatabase_MySQLUsesSingleTransaction(t *testing.T) {
	managed := true
	cfg := &config.ProjectConfig{
		Name: "myapp",
		Database: config.DatabaseConfig{
			Driver:  "mysql",
			Version: "8.0",
			Managed: &managed,
		},
	}

	mock := &ssh.MockExecutor{}
	_, err := BackupManagedDatabase(context.Background(), mock, cfg,
		"mysql://myapp:pw@myapp-db:3306/myapp?serverVersion=8.0&charset=utf8mb4", "tag1")
	if err != nil {
		t.Fatalf("BackupManagedDatabase() error = %v", err)
	}

	all := strings.Join(mock.Commands, "\n")
	if !strings.Contains(all, "mysqldump") || !strings.Contains(all, "--single-transaction") {
		t.Errorf("mysql backup should use mysqldump --single-transaction, commands:\n%s", all)
	}
}

func TestBackupManagedDatabase_DumpFailure(t *testing.T) {
	mock := &ssh.MockExecutor{
		ExecFunc: func(ctx context.Context, command string) (*ssh.ExecResult, error) {
			if strings.Contains(command, "pg_dump") {
				return &ssh.ExecResult{ExitCode: 1, Stderr: "connection refused"}, nil
			}
			return &ssh.ExecResult{ExitCode: 0}, nil
		},
	}

	if _, err := BackupManagedDatabase(context.Background(), mock, managedPgConfig(), testDatabaseURL, "tag1"); err == nil {
		t.Fatal("expected error when the dump command fails")
	}
}

func TestBackupManagedDatabase_EmptyDumpFails(t *testing.T) {
	mock := &ssh.MockExecutor{
		ExecFunc: func(ctx context.Context, command string) (*ssh.ExecResult, error) {
			if strings.Contains(command, "test -s") {
				return &ssh.ExecResult{ExitCode: 1}, nil
			}
			return &ssh.ExecResult{ExitCode: 0}, nil
		},
	}

	if _, err := BackupManagedDatabase(context.Background(), mock, managedPgConfig(), testDatabaseURL, "tag1"); err == nil {
		t.Fatal("expected error when the dump file is empty")
	}
}

func TestBackupManagedDatabase_DefaultRetention(t *testing.T) {
	cfg := managedPgConfig()
	cfg.Deploy.KeepReleases = 0 // unset → default

	mock := &ssh.MockExecutor{}
	if _, err := BackupManagedDatabase(context.Background(), mock, cfg, testDatabaseURL, "tag1"); err != nil {
		t.Fatalf("BackupManagedDatabase() error = %v", err)
	}

	all := strings.Join(mock.Commands, "\n")
	if !strings.Contains(all, "tail -n +6") {
		t.Errorf("default retention should keep %d backups, commands:\n%s", 5, all)
	}
}
