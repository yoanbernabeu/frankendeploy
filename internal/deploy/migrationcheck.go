package deploy

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/yoanbernabeu/frankendeploy/internal/ssh"
)

// MigrationCheckResult holds the result of migration directory check
type MigrationCheckResult struct {
	MigrationFilesCount int
	EntityFilesCount    int
	HasPotentialProblem bool
}

// CheckMigrationState checks if migrations exist when entities are present
// It runs docker exec commands to count files in migrations/ and src/Entity/
func CheckMigrationState(ctx context.Context, client ssh.Executor, containerName string) (*MigrationCheckResult, error) {
	result := &MigrationCheckResult{}

	// Count migration files (.php files in /app/migrations/)
	migrationCmd := fmt.Sprintf(
		"docker exec %s sh -c 'find /app/migrations -name \"*.php\" -type f 2>/dev/null | wc -l'",
		containerName,
	)
	migrationResult, err := client.Exec(ctx, migrationCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to check migrations: %w", err)
	}
	result.MigrationFilesCount = parseCount(migrationResult.Stdout)

	// Count entity files (.php files in /app/src/Entity/)
	entityCmd := fmt.Sprintf(
		"docker exec %s sh -c 'find /app/src/Entity -name \"*.php\" -type f 2>/dev/null | wc -l'",
		containerName,
	)
	entityResult, err := client.Exec(ctx, entityCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to check entities: %w", err)
	}
	result.EntityFilesCount = parseCount(entityResult.Stdout)

	// Determine if there's a potential problem
	// Problem: entities exist (> 0) but no migrations (== 0)
	result.HasPotentialProblem = result.EntityFilesCount > 0 && result.MigrationFilesCount == 0

	return result, nil
}

// parseCount extracts an integer from command output
func parseCount(output string) int {
	trimmed := strings.TrimSpace(output)
	count, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0
	}
	return count
}

// HasMigrationWarningBeenShown checks if the warning marker exists on the server
func HasMigrationWarningBeenShown(ctx context.Context, client ssh.Executor, appName string) bool {
	markerPath := getMigrationWarningMarkerPath(appName)
	cmd := fmt.Sprintf("test -f %s && echo 'exists'", markerPath)
	result, err := client.Exec(ctx, cmd)
	if err != nil {
		return false
	}
	return strings.TrimSpace(result.Stdout) == "exists"
}

// MarkMigrationWarningShown creates the warning marker file on the server
func MarkMigrationWarningShown(ctx context.Context, client ssh.Executor, appName string) error {
	markerPath := getMigrationWarningMarkerPath(appName)
	// Ensure directory exists and create marker file
	cmd := fmt.Sprintf("mkdir -p $(dirname %s) && touch %s", markerPath, markerPath)
	_, err := client.Exec(ctx, cmd)
	return err
}

// ClearMigrationWarningMarker removes the marker when the problem is resolved
func ClearMigrationWarningMarker(ctx context.Context, client ssh.Executor, appName string) error {
	markerPath := getMigrationWarningMarkerPath(appName)
	cmd := fmt.Sprintf("rm -f %s", markerPath)
	_, err := client.Exec(ctx, cmd)
	return err
}

// getMigrationWarningMarkerPath returns the path to the warning marker file
func getMigrationWarningMarkerPath(appName string) string {
	return filepath.Join("/opt/frankendeploy/apps", appName, "shared", ".migration_warning_shown")
}

// FormatMigrationWarning formats the warning message for display
func FormatMigrationWarning(result *MigrationCheckResult) string {
	var sb strings.Builder

	sb.WriteString("Warning: No database migrations found but entities exist!\n\n")
	sb.WriteString(fmt.Sprintf("   Entities found: %d files in src/Entity/\n", result.EntityFilesCount))
	sb.WriteString(fmt.Sprintf("   Migrations:     %d files in migrations/\n", result.MigrationFilesCount))
	sb.WriteString("\n")
	sb.WriteString("   This may cause 'no such table' errors at runtime.\n\n")
	sb.WriteString("   To fix this, run locally:\n")
	sb.WriteString("      php bin/console make:migration\n")
	sb.WriteString("      php bin/console doctrine:migrations:migrate\n")
	sb.WriteString("      git add migrations/\n")
	sb.WriteString("      git commit -m \"Add database migrations\"\n\n")
	sb.WriteString("   Then redeploy your application.\n")

	return sb.String()
}

// HasMigrationHook checks if the hooks list contains a Doctrine migration command
func HasMigrationHook(hooks []string) bool {
	for _, hook := range hooks {
		if strings.Contains(hook, "doctrine:migrations:migrate") ||
			strings.Contains(hook, "d:m:m") {
			return true
		}
	}
	return false
}
