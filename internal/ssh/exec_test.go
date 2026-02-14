package ssh

import (
	"errors"
	"testing"
)

func TestExecResult_Err_Zero(t *testing.T) {
	r := &ExecResult{ExitCode: 0, Stdout: "ok", Stderr: ""}
	if r.Err() != nil {
		t.Error("expected nil error for exit code 0")
	}
}

func TestExecResult_Err_NonZero(t *testing.T) {
	r := &ExecResult{ExitCode: 1, Stderr: "something went wrong"}
	err := r.Err()
	if err == nil {
		t.Fatal("expected error for exit code 1")
	}

	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatal("expected *CommandError")
	}
	if cmdErr.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", cmdErr.ExitCode)
	}
	if cmdErr.Stderr != "something went wrong" {
		t.Errorf("unexpected stderr: %q", cmdErr.Stderr)
	}
}

func TestCommandError_ErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		err      CommandError
		expected string
	}{
		{
			name:     "with stderr",
			err:      CommandError{ExitCode: 1, Stderr: "file not found"},
			expected: "command failed (exit 1): file not found",
		},
		{
			name:     "without stderr",
			err:      CommandError{ExitCode: 127, Stderr: ""},
			expected: "command failed (exit 127)",
		},
		{
			name:     "stderr with whitespace",
			err:      CommandError{ExitCode: 2, Stderr: "  error  \n"},
			expected: "command failed (exit 2): error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestCommandError_ErrorsAs(t *testing.T) {
	r := &ExecResult{ExitCode: 42, Stderr: "oops"}
	err := r.Err()

	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatal("errors.As should work with *CommandError")
	}
	if cmdErr.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", cmdErr.ExitCode)
	}
}
