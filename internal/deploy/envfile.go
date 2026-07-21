package deploy

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/yoanbernabeu/frankendeploy/internal/constants"
	"github.com/yoanbernabeu/frankendeploy/internal/security"
	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

// ParseEnvContent parses .env file content into a map. Double-quoted values
// are unquoted and unescaped (\" and \\), matching BuildEnvContent output.
func ParseEnvContent(content string) map[string]string {
	vars := make(map[string]string)

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			value = value[1 : len(value)-1]
			value = strings.ReplaceAll(value, `\"`, `"`)
			value = strings.ReplaceAll(value, `\\`, `\`)
		} else {
			value = strings.Trim(value, "'")
		}
		vars[key] = value
	}

	return vars
}

// BuildEnvContent builds .env file content from a map. Keys are sorted so the
// file is stable across writes (clean diffs), and values needing quoting are
// double-quoted with backslashes and double quotes escaped.
func BuildEnvContent(vars map[string]string) string {
	keys := make([]string, 0, len(vars))
	for key := range vars {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, key := range keys {
		value := vars[key]
		if strings.ContainsAny(value, " \t\n\"'\\") {
			value = strings.ReplaceAll(value, `\`, `\\`)
			value = strings.ReplaceAll(value, `"`, `\"`)
			value = `"` + value + `"`
		}
		fmt.Fprintf(&sb, "%s=%s\n", key, value)
	}
	return sb.String()
}

// ReadEnvVars reads and parses the app's .env.local file on the server.
// A missing file yields an empty map.
func ReadEnvVars(ctx context.Context, client ssh.Executor, appName string) (map[string]string, error) {
	envFile := constants.AppEnvFilePath(appName)
	result, err := client.Exec(ctx, fmt.Sprintf("cat %s 2>/dev/null || echo ''", envFile))
	if err != nil {
		return nil, fmt.Errorf("failed to read env file: %w", err)
	}
	return ParseEnvContent(result.Stdout), nil
}

// WriteEnvVars writes the app's .env.local file on the server. This is the
// single write path for env files: it creates the directory, writes the
// sorted content, then enforces container-user ownership and 0600
// permissions so secrets are never left world-readable.
func WriteEnvVars(ctx context.Context, client ssh.Executor, appName string, vars map[string]string) error {
	envFile := constants.AppEnvFilePath(appName)

	if _, err := client.Exec(ctx, fmt.Sprintf("mkdir -p $(dirname %s)", envFile)); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	content := BuildEnvContent(vars)
	delim, err := security.GenerateHeredocDelimiter("ENVEOF")
	if err != nil {
		return fmt.Errorf("failed to generate delimiter: %w", err)
	}
	writeCmd := fmt.Sprintf("cat > %s << '%s'\n%s%s", envFile, delim, content, delim)
	if _, err := client.Exec(ctx, writeCmd); err != nil {
		return fmt.Errorf("failed to write env file: %w", err)
	}

	// Never leave secrets world-readable: enforce ownership and 0600 on every
	// write. Failures are non-critical (sudo may be unavailable) — the
	// deploy-time fixSharedPermissions pass retries the same commands.
	if _, err := client.Exec(ctx, fmt.Sprintf("sudo chown %s %s 2>/dev/null || true", constants.ContainerUser, envFile)); err != nil {
		_ = err
	}
	if _, err := client.Exec(ctx, fmt.Sprintf("sudo chmod 600 %s 2>/dev/null || true", envFile)); err != nil {
		_ = err
	}

	return nil
}
