package config

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/yoanbernabeu/frankendeploy/internal/security"
)

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationErrors holds multiple validation errors
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	var msgs []string
	for _, err := range e {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

// HasErrors returns true if there are validation errors
func (e ValidationErrors) HasErrors() bool {
	return len(e) > 0
}

// ValidateProjectConfig validates the project configuration
func ValidateProjectConfig(config *ProjectConfig) ValidationErrors {
	var errors ValidationErrors

	if config.Name == "" {
		errors = append(errors, ValidationError{
			Field:   "name",
			Message: "project name is required",
		})
	} else if !isValidProjectName(config.Name) {
		errors = append(errors, ValidationError{
			Field:   "name",
			Message: "project name must contain only lowercase letters, numbers, and hyphens",
		})
	}

	if config.PHP.Version == "" {
		errors = append(errors, ValidationError{
			Field:   "php.version",
			Message: "PHP version is required",
		})
	} else if !isValidPHPVersion(config.PHP.Version) {
		errors = append(errors, ValidationError{
			Field:   "php.version",
			Message: "invalid PHP version (must be 8.1, 8.2, 8.3, or 8.4)",
		})
	}

	if config.Database.Driver != "" {
		if !isValidDatabaseDriver(config.Database.Driver) {
			errors = append(errors, ValidationError{
				Field:   "database.driver",
				Message: "unsupported database driver (use pgsql, mysql, or sqlite)",
			})
		}

		if config.Database.Version != "" && !isValidDatabaseVersion(config.Database.Version) {
			errors = append(errors, ValidationError{
				Field:   "database.version",
				Message: "invalid database version format (expected numeric version like 16, 8.0, or 3.39.4)",
			})
		}

		// SQLite cannot use managed mode (file-based database, not a container)
		if isSQLiteDriver(config.Database.Driver) {
			if config.Database.Managed != nil && *config.Database.Managed {
				errors = append(errors, ValidationError{
					Field:   "database.managed",
					Message: "SQLite does not support managed mode. SQLite is a file-based database and cannot run as a container. Remove 'managed: true' from your configuration. The SQLite database directory should be in 'deploy.shared_dirs' for persistence",
				})
			}
		}
	}

	for _, ext := range config.PHP.Extensions {
		if !isValidExtensionName(ext) {
			errors = append(errors, ValidationError{
				Field:   "php.extensions",
				Message: fmt.Sprintf("invalid extension name %q: must contain only letters, numbers, and underscores", ext),
			})
		}
	}

	if config.Deploy.Domain != "" && !isValidDomain(config.Deploy.Domain) {
		errors = append(errors, ValidationError{
			Field:   "deploy.domain",
			Message: "invalid domain name",
		})
	}

	if config.Deploy.HealthcheckPath != "" {
		if err := security.ValidateHealthPath(config.Deploy.HealthcheckPath); err != nil {
			errors = append(errors, ValidationError{
				Field:   "deploy.healthcheck_path",
				Message: err.Error(),
			})
		}
	}

	if config.Messenger.Enabled && config.Messenger.Workers < 1 {
		errors = append(errors, ValidationError{
			Field:   "messenger.workers",
			Message: "workers must be greater than 0 when messenger is enabled",
		})
	}

	if config.Deploy.KeepReleases < 0 {
		errors = append(errors, ValidationError{
			Field:   "deploy.keep_releases",
			Message: "keep_releases must be a positive number",
		})
	}

	return errors
}

// ValidateServerConfig validates a server configuration
func ValidateServerConfig(config *ServerConfig) ValidationErrors {
	var errors ValidationErrors

	if config.Host == "" {
		errors = append(errors, ValidationError{
			Field:   "host",
			Message: "server host is required",
		})
	}

	if config.User == "" {
		errors = append(errors, ValidationError{
			Field:   "user",
			Message: "server user is required",
		})
	}

	if config.Port < 1 || config.Port > 65535 {
		errors = append(errors, ValidationError{
			Field:   "port",
			Message: "port must be between 1 and 65535",
		})
	}

	return errors
}

func isValidProjectName(name string) bool {
	matched, _ := regexp.MatchString(`^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]$`, name)
	return matched
}

func isValidPHPVersion(version string) bool {
	validVersions := []string{"8.1", "8.2", "8.3", "8.4"}
	for _, v := range validVersions {
		if version == v {
			return true
		}
	}
	return false
}

func isValidDatabaseDriver(driver string) bool {
	validDrivers := []string{"pgsql", "mysql", "sqlite", "pdo_pgsql", "pdo_mysql", "pdo_sqlite"}
	for _, d := range validDrivers {
		if driver == d {
			return true
		}
	}
	return false
}

func isSQLiteDriver(driver string) bool {
	return driver == "sqlite" || driver == "pdo_sqlite"
}

func isValidExtensionName(name string) bool {
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_]+$`, name)
	return matched
}

func isValidDatabaseVersion(version string) bool {
	matched, _ := regexp.MatchString(`^[0-9]+(\.[0-9]+)*$`, version)
	return matched
}

func isValidDomain(domain string) bool {
	if len(domain) > 253 {
		return false
	}
	// DNS hostname validation: labels separated by dots
	matched, _ := regexp.MatchString(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`, domain)
	return matched
}
