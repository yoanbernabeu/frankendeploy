package generator

// Tests for issue #47: per-driver dump commands in the DB registry.

import (
	"strings"
	"testing"
)

func TestBuildDumpCmd_Postgres(t *testing.T) {
	info, err := GetDBDriverInfo("pgsql")
	if err != nil {
		t.Fatalf("GetDBDriverInfo() error = %v", err)
	}

	cmd := info.BuildDumpCmd("myapp-db", "'myapp'", "'pw'", "'myapp'")
	for _, want := range []string{"docker exec", "myapp-db", "pg_dump", "--clean", "--if-exists"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("pgsql dump command missing %q: %s", want, cmd)
		}
	}
	if strings.Contains(cmd, "'pw'") {
		t.Error("pg_dump inside the container uses the local socket: the password must not appear on the command line")
	}
}

func TestBuildDumpCmd_MySQL(t *testing.T) {
	info, err := GetDBDriverInfo("mysql")
	if err != nil {
		t.Fatalf("GetDBDriverInfo() error = %v", err)
	}

	cmd := info.BuildDumpCmd("myapp-db", "'myapp'", "'pw'", "'myapp'")
	for _, want := range []string{"docker exec", "myapp-db", "mysqldump", "--single-transaction"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("mysql dump command missing %q: %s", want, cmd)
		}
	}
}
