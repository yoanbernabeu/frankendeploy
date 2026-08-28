package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
)

// extractSQLitePath extracts the file path from a SQLite DATABASE_URL
func extractSQLitePath(url string) string {
	// Remove the sqlite:// prefix (case-insensitive)
	path := url
	lowerURL := strings.ToLower(url)
	if strings.HasPrefix(lowerURL, "sqlite://") {
		path = url[9:] // Keep original case for the path
	} else if strings.HasPrefix(lowerURL, "sqlite:") {
		path = url[7:]
	}

	// Handle Symfony's %kernel.project_dir% placeholder
	path = strings.ReplaceAll(path, "%kernel.project_dir%", "")

	// Remove leading slashes
	path = strings.TrimLeft(path, "/")

	// Default to var/data.db if empty
	if path == "" {
		return "var/data.db"
	}

	return path
}

// getSQLiteDirectory returns the directory containing the SQLite file
func getSQLiteDirectory(path string) string {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return ""
	}
	return dir
}

// DetectDatabase detects the database configuration from the project.
// Returns the config, optional warning messages, and an error.
func (s *Scanner) DetectDatabase() (*config.DatabaseConfig, []string, error) {
	dbConfig := &config.DatabaseConfig{}

	// First, check doctrine.yaml for explicit driver
	if doctrineConfig, err := s.GetDoctrineConfig(); err == nil {
		if driver := doctrineConfig.Doctrine.DBAL.Driver; driver != "" {
			dbConfig.Driver = config.NormalizeDBDriver(driver)
			dbConfig.Version = getDefaultVersion(dbConfig.Driver)
			// SQLite doesn't support managed mode (file-based database)
			if dbConfig.Driver != "sqlite" {
				managed := true
				dbConfig.Managed = &managed
			}
			return dbConfig, nil, nil
		}
	}

	// Check .env + .env.local for DATABASE_URL (.env.local overrides,
	// as Symfony does at runtime — reading only .env silently detects
	// the wrong database)
	if env, err := s.GetMergedEnv(); err == nil {
		if dbURL, ok := env["DATABASE_URL"]; ok {
			driver, version := parseDBURL(dbURL)
			if driver != "" {
				var warnings []string
				if w := s.envDivergenceWarning(driver); w != "" {
					warnings = append(warnings, w)
				}
				dbConfig.Driver = driver
				dbConfig.Version = version
				// SQLite: extract path and don't set managed
				if driver == "sqlite" {
					dbConfig.Path = extractSQLitePath(dbURL)
				} else {
					managed := true
					dbConfig.Managed = &managed
				}
				return dbConfig, warnings, nil
			}
		}
	}

	// Check composer.json for database packages
	composer, err := s.ParseComposer()
	if err != nil {
		return nil, nil, err
	}

	// Detect from installed packages
	if composer.HasAnyPackage("doctrine/dbal", "doctrine/orm") {
		// Check for driver-specific packages
		if composer.HasPackage("ext-pdo_pgsql") || s.hasExtInPlatform("pdo_pgsql") {
			dbConfig.Driver = "pgsql"
			dbConfig.Version = "16"
		} else if composer.HasPackage("ext-pdo_mysql") || s.hasExtInPlatform("pdo_mysql") {
			dbConfig.Driver = "mysql"
			dbConfig.Version = "8.0"
		} else {
			// Default to PostgreSQL as it's recommended for production
			dbConfig.Driver = "pgsql"
			dbConfig.Version = "16"
			managed := true
			dbConfig.Managed = &managed
			return dbConfig, []string{"no database driver explicitly configured, defaulting to PostgreSQL. Set DATABASE_URL in .env or configure doctrine.yaml to silence this warning"}, nil
		}
		managed := true
		dbConfig.Managed = &managed
		return dbConfig, nil, nil
	}

	// No database detected
	return nil, nil, nil
}

// envDivergenceWarning returns a warning when .env and .env.local declare
// DATABASE_URLs with different drivers (the .env.local one wins).
func (s *Scanner) envDivergenceWarning(effectiveDriver string) string {
	base, baseErr := s.GetEnvFile(".env")
	local, localErr := s.GetEnvFile(".env.local")
	if baseErr != nil || localErr != nil {
		return ""
	}
	baseURL, okBase := base["DATABASE_URL"]
	localURL, okLocal := local["DATABASE_URL"]
	if !okBase || !okLocal {
		return ""
	}
	baseDriver, _ := parseDBURL(baseURL)
	localDriver, _ := parseDBURL(localURL)
	if baseDriver != "" && localDriver != "" && baseDriver != localDriver {
		return fmt.Sprintf("DATABASE_URL differs between .env (%s) and .env.local (%s): using .env.local (%s), which is what Symfony does at runtime",
			baseDriver, localDriver, effectiveDriver)
	}
	return ""
}

// serverVersionRegex matches the serverVersion query parameter, including
// MariaDB forms ("10.11.2-MariaDB", "mariadb-10.11.2").
var serverVersionRegex = regexp.MustCompile(`serverversion=([a-z0-9.-]+)`)

// parseDBURL extracts driver and version from DATABASE_URL
func parseDBURL(url string) (string, string) {
	// Format: driver://user:pass@host:port/dbname?serverVersion=X
	url = strings.ToLower(url)

	var driver string
	switch {
	case strings.HasPrefix(url, "postgresql://"), strings.HasPrefix(url, "postgres://"), strings.HasPrefix(url, "pgsql://"):
		driver = "pgsql"
	case strings.HasPrefix(url, "mariadb://"):
		driver = "mariadb"
	case strings.HasPrefix(url, "mysql://"), strings.HasPrefix(url, "mysql2://"):
		driver = "mysql"
	case strings.HasPrefix(url, "sqlite://"), strings.HasPrefix(url, "sqlite:"):
		driver = "sqlite"
	default:
		return "", ""
	}

	// Try to extract version from serverVersion parameter. A MariaDB
	// serverVersion silently produced a nonexistent mysql:X image before:
	// detect it and switch to the mariadb driver/image.
	version := ""
	if matches := serverVersionRegex.FindStringSubmatch(url); len(matches) > 1 {
		v := matches[1]
		switch {
		case strings.HasSuffix(v, "-mariadb"):
			driver = "mariadb"
			v = strings.TrimSuffix(v, "-mariadb")
		case strings.HasPrefix(v, "mariadb-"):
			driver = "mariadb"
			v = strings.TrimPrefix(v, "mariadb-")
		}
		version = v
	}
	if version == "" {
		version = getDefaultVersion(driver)
	}

	return driver, version
}

// getDefaultVersion returns the default version for a database driver
func getDefaultVersion(driver string) string {
	switch driver {
	case "pgsql":
		return "16"
	case "mysql":
		return "8.0"
	case "mariadb":
		return "11"
	case "sqlite":
		return "3"
	default:
		return ""
	}
}

// hasExtInPlatform checks if an extension is defined in composer.json platform
func (s *Scanner) hasExtInPlatform(ext string) bool {
	composerPath := filepath.Join(s.projectPath, "composer.json")
	data, err := os.ReadFile(composerPath)
	if err != nil {
		return false
	}

	// Simple check for ext in platform config
	return strings.Contains(string(data), `"ext-`+ext+`"`)
}
