package security

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	// appNameRegex validates application names (DNS-compatible)
	// Allows: lowercase letters, numbers, hyphens (not at start/end)
	// Length: 1-63 characters
	appNameRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

	// serverNameRegex validates server configuration names
	// Allows: letters, numbers, underscores, hyphens
	// Length: 1-64 characters
	serverNameRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9_-]{0,62}[a-zA-Z0-9])?$`)

	// releaseRegex validates release/tag names
	// Allows: letters, numbers, dots, underscores, hyphens
	// Length: 1-128 characters
	releaseRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]{0,126}[a-zA-Z0-9])?$`)

	// unixUserRegex validates Unix usernames
	// Standard POSIX username rules
	// Length: 1-32 characters
	unixUserRegex = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

	// healthPathRegex validates health check paths
	// Allows: URL paths with alphanumeric, slashes, dots, underscores, hyphens
	// Does not allow double slashes or parent traversal (..)
	healthPathRegex = regexp.MustCompile(`^/([a-zA-Z0-9_.-]+(/[a-zA-Z0-9_.-]+)*)?$`)

	// logTailRegex validates --tail argument for docker logs
	// Allows: positive integers or "all"
	logTailRegex = regexp.MustCompile(`^([0-9]+|all)$`)

	// logSinceRegex validates --since argument for docker logs
	// Allows: durations (e.g., "2h", "30m", "1h30m") or timestamps
	logSinceRegex = regexp.MustCompile(`^([0-9]+[smhd])+$|^[0-9]{4}-[0-9]{2}-[0-9]{2}(T[0-9]{2}:[0-9]{2}:[0-9]{2})?$`)

	// envKeyRegex validates environment variable keys
	// Standard environment variable naming
	envKeyRegex = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

	// sharedDirRegex validates shared directory paths
	// Allows: alphanumeric, underscores, hyphens, dots, forward slashes (no ..)
	sharedDirRegex = regexp.MustCompile(`^[a-zA-Z0-9_.-]+(/[a-zA-Z0-9_.-]+)*$`)

	// sensitivePatterns used by SanitizeCommandForLog to mask secrets
	sensitiveLogPatterns = []string{
		"DATABASE_URL=",
		"POSTGRES_PASSWORD=",
		"MYSQL_PASSWORD=",
		"MYSQL_ROOT_PASSWORD=",
	}
)

// ValidateAppName validates an application name
// Application names must be DNS-compatible for Docker container naming
func ValidateAppName(name string) error {
	if name == "" {
		return fmt.Errorf("app name cannot be empty")
	}
	if len(name) > 63 {
		return fmt.Errorf("app name too long (max 63 characters)")
	}
	if !appNameRegex.MatchString(name) {
		return fmt.Errorf("app name must contain only lowercase letters, numbers, and hyphens (not at start/end)")
	}
	return nil
}

// ValidateServerName validates a server configuration name
func ValidateServerName(name string) error {
	if name == "" {
		return fmt.Errorf("server name cannot be empty")
	}
	if len(name) > 64 {
		return fmt.Errorf("server name too long (max 64 characters)")
	}
	if !serverNameRegex.MatchString(name) {
		return fmt.Errorf("server name must contain only letters, numbers, underscores, and hyphens")
	}
	return nil
}

// ValidateRelease validates a release/tag name
func ValidateRelease(release string) error {
	if release == "" {
		return fmt.Errorf("release name cannot be empty")
	}
	if len(release) > 128 {
		return fmt.Errorf("release name too long (max 128 characters)")
	}
	if !releaseRegex.MatchString(release) {
		return fmt.Errorf("release name must contain only letters, numbers, dots, underscores, and hyphens")
	}
	return nil
}

// ValidateUnixUser validates a Unix username
func ValidateUnixUser(user string) error {
	if user == "" {
		return fmt.Errorf("username cannot be empty")
	}
	if len(user) > 32 {
		return fmt.Errorf("username too long (max 32 characters)")
	}
	if !unixUserRegex.MatchString(user) {
		return fmt.Errorf("username must start with a lowercase letter or underscore, followed by lowercase letters, numbers, underscores, or hyphens")
	}
	return nil
}

// ValidateHealthPath validates a health check URL path
func ValidateHealthPath(path string) error {
	if path == "" {
		return nil // Empty path defaults to "/"
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("health path must start with /")
	}
	if len(path) > 2048 {
		return fmt.Errorf("health path too long (max 2048 characters)")
	}
	// Check for path traversal attempts
	if strings.Contains(path, "..") {
		return fmt.Errorf("health path cannot contain path traversal (..) sequences")
	}
	if !healthPathRegex.MatchString(path) {
		return fmt.Errorf("health path contains invalid characters")
	}
	return nil
}

// ValidateLogTail validates the --tail argument for docker logs
func ValidateLogTail(tail string) error {
	if tail == "" {
		return nil // Empty defaults to "100"
	}
	if !logTailRegex.MatchString(tail) {
		return fmt.Errorf("tail must be a positive integer or 'all'")
	}
	// Additional check: if numeric, ensure it's reasonable
	if tail != "all" {
		n, err := strconv.Atoi(tail)
		if err != nil {
			return fmt.Errorf("invalid tail value: %w", err)
		}
		if n < 0 {
			return fmt.Errorf("tail cannot be negative")
		}
		if n > 100000 {
			return fmt.Errorf("tail value too large (max 100000)")
		}
	}
	return nil
}

// ValidateLogSince validates the --since argument for docker logs
func ValidateLogSince(since string) error {
	if since == "" {
		return nil // Empty means no --since filter
	}
	if len(since) > 64 {
		return fmt.Errorf("since value too long")
	}
	if !logSinceRegex.MatchString(since) {
		return fmt.Errorf("since must be a duration (e.g., '2h', '30m') or timestamp (e.g., '2024-01-15')")
	}
	return nil
}

// ValidateEnvKey validates an environment variable key
func ValidateEnvKey(key string) error {
	if key == "" {
		return fmt.Errorf("environment variable key cannot be empty")
	}
	if len(key) > 256 {
		return fmt.Errorf("environment variable key too long (max 256 characters)")
	}
	if !envKeyRegex.MatchString(key) {
		return fmt.Errorf("environment variable key must start with a letter or underscore, followed by letters, numbers, or underscores")
	}
	return nil
}

// ValidateDockerCommand validates a command to be executed in a container
// This function checks for common shell injection patterns
func ValidateDockerCommand(command string) error {
	if command == "" {
		return fmt.Errorf("command cannot be empty")
	}

	// Check for dangerous shell metacharacters in suspicious contexts
	dangerousPatterns := []string{
		";",  // Command separator
		"&&", // Command chaining
		"||", // Command chaining
		"|",  // Pipe
		"`",  // Command substitution
		"$(", // Command substitution
		"${", // Variable expansion (could be dangerous)
		">",  // Redirect
		"<",  // Redirect
		"\n", // Newline (command injection)
		"\r", // Carriage return
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(command, pattern) {
			return fmt.Errorf("command contains potentially dangerous character sequence: %q", pattern)
		}
	}

	return nil
}

// ShellEscape escapes a string for safe use in shell commands by wrapping it
// in single quotes and escaping any internal single quotes using the POSIX
// pattern: ' → '\''
func ShellEscape(s string) string {
	// Replace single quotes with the POSIX escape sequence: end quote, escaped quote, start quote
	escaped := strings.ReplaceAll(s, "'", "'\\''")
	return "'" + escaped + "'"
}

// ValidateHook validates a deployment hook command.
// Hooks are executed inside a Docker container via docker exec, so they must
// not contain shell metacharacters that could break out of the intended command.
func ValidateHook(hook string) error {
	if hook == "" {
		return fmt.Errorf("hook command cannot be empty")
	}
	return ValidateDockerCommand(hook)
}

// ValidateSharedDir validates a shared directory path.
// Shared dirs must be relative paths without parent traversal.
func ValidateSharedDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("shared directory cannot be empty")
	}

	// Must be relative (no leading /)
	if strings.HasPrefix(dir, "/") {
		return fmt.Errorf("shared directory must be a relative path, got: %s", dir)
	}

	// No parent traversal
	if strings.Contains(dir, "..") {
		return fmt.Errorf("shared directory cannot contain path traversal (..): %s", dir)
	}

	// Only safe characters
	if !sharedDirRegex.MatchString(dir) {
		return fmt.Errorf("shared directory contains invalid characters: %s", dir)
	}

	return nil
}

// GenerateHeredocDelimiter generates a unique heredoc delimiter to prevent
// heredoc injection attacks. Uses crypto/rand for unpredictability.
func GenerateHeredocDelimiter(prefix string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random bytes for heredoc delimiter: %w", err)
	}
	return prefix + "_" + hex.EncodeToString(b), nil
}

// SanitizeCommandForLog masks sensitive values in commands before logging.
// This prevents secrets from leaking into verbose output or log files.
func SanitizeCommandForLog(cmd string) string {
	result := cmd

	// Mask sensitive environment variable values
	for _, pattern := range sensitiveLogPatterns {
		searchFrom := 0
		for {
			idx := strings.Index(result[searchFrom:], pattern)
			if idx == -1 {
				break
			}
			absIdx := searchFrom + idx
			// Find the end of the value (next space or end of string)
			valueStart := absIdx + len(pattern)
			valueEnd := findValueEnd(result, valueStart)
			masked := "****"
			result = result[:valueStart] + masked + result[valueEnd:]
			// Advance past the replacement to avoid infinite loop
			searchFrom = valueStart + len(masked)
		}
	}

	// Mask -p<password> pattern (MySQL password flag)
	result = maskMySQLPasswordFlag(result)

	return result
}

// findValueEnd finds where a shell value ends (handles quoted and unquoted values)
func findValueEnd(s string, start int) int {
	if start >= len(s) {
		return start
	}

	// Handle single-quoted value
	if s[start] == '\'' {
		end := strings.Index(s[start+1:], "'")
		if end == -1 {
			return len(s)
		}
		return start + end + 2
	}

	// Handle double-quoted value
	if s[start] == '"' {
		end := strings.Index(s[start+1:], "\"")
		if end == -1 {
			return len(s)
		}
		return start + end + 2
	}

	// Unquoted: find next whitespace
	for i := start; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' || s[i] == '\n' {
			return i
		}
	}
	return len(s)
}

// maskMySQLPasswordFlag masks -p<password> patterns in commands
func maskMySQLPasswordFlag(cmd string) string {
	// Match -p followed by non-space characters (the password)
	result := cmd
	for {
		idx := strings.Index(result, "-p")
		if idx == -1 {
			break
		}
		// Check it's actually a password flag, not -port or similar
		afterP := idx + 2
		if afterP >= len(result) {
			break
		}
		// -p followed by a space means separate argument, skip
		if result[afterP] == ' ' || result[afterP] == '-' {
			// Move past this -p to avoid infinite loop
			nextSpace := strings.IndexByte(result[afterP:], ' ')
			if nextSpace == -1 {
				break
			}
			idx = afterP + nextSpace
			continue
		}
		// Find end of password value
		valueEnd := afterP
		for valueEnd < len(result) && result[valueEnd] != ' ' && result[valueEnd] != '\t' {
			valueEnd++
		}
		result = result[:afterP] + "****" + result[valueEnd:]
		break // Only mask the first occurrence to avoid infinite loops
	}
	return result
}
