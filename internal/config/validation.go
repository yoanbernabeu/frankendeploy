package config

import (
	"fmt"
	"regexp"
	"strings"
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
