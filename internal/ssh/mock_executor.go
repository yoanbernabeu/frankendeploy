package ssh

import "context"

// MockExecutor is a test double that records commands and returns configured results.
type MockExecutor struct {
	ExecFunc       func(ctx context.Context, command string) (*ExecResult, error)
	ExecStreamFunc func(ctx context.Context, command string) error
	Commands       []string
}

// Exec records the command and delegates to ExecFunc.
func (m *MockExecutor) Exec(ctx context.Context, command string) (*ExecResult, error) {
	m.Commands = append(m.Commands, command)
	if m.ExecFunc != nil {
		return m.ExecFunc(ctx, command)
	}
	return &ExecResult{Stdout: "", Stderr: "", ExitCode: 0}, nil
}

// ExecStream records the command and delegates to ExecStreamFunc.
func (m *MockExecutor) ExecStream(ctx context.Context, command string) error {
	m.Commands = append(m.Commands, command)
	if m.ExecStreamFunc != nil {
		return m.ExecStreamFunc(ctx, command)
	}
	return nil
}

// Close is a no-op for the mock.
func (m *MockExecutor) Close() error {
	return nil
}
