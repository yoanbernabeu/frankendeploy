package ssh

import "context"

// Executor abstracts remote command execution for testability.
type Executor interface {
	Exec(ctx context.Context, command string) (*ExecResult, error)
	ExecStream(ctx context.Context, command string) error
	Close() error
}
