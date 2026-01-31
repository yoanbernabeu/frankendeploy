package scanner

import (
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

// DetectDatabase detects the database configuration from the project
func (s *Scanner) DetectDatabase() (*config.DatabaseConfig, error) {
	dbConfig := &config.DatabaseConfig{}

	// First, check doctrine.yaml for explicit driver
	if doctrineConfig, err := s.GetDoctrineConfig(); err == nil {
		if driver := doctrineConfig.Doctrine.DBAL.Driver; driver != "" {
			dbConfig.Driver = normalizeDriver(driver)
			dbConfig.Version = getDefaultVersion(dbConfig.Driver)
			// SQLite doesn't support managed mode (file-based database)
			if dbConfig.Driver != "sqlite" {
				managed := true
				dbConfig.Managed = &managed
			}
			return dbConfig, nil
		}
	}

	// Check .env for DATABASE_URL
	if env, err := s.GetEnvFile(".env"); err == nil {
		if dbURL, ok := env["DATABASE_URL"]; ok {
			driver, version := parseDBURL(dbURL)
			if driver != "" {
				dbConfig.Driver = driver
				dbConfig.Version = version
				// SQLite: extract path and don't set managed
				if driver == "sqlite" {
					dbConfig.Path = extractSQLitePath(dbURL)
				} else {
					managed := true
					dbConfig.Managed = &managed
				}
				return dbConfig, nil
			}
		}
	}

	// Check composer.json for database packages
	composer, err := s.ParseComposer()
	if err != nil {
		return nil, err
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
		}
		managed := true
		dbConfig.Managed = &managed
		return dbConfig, nil
	}

	// No database detected
	return nil, nil
}

// parseDBURL extracts driver and version from DATABASE_URL
func parseDBURL(url string) (string, string) {
	// Format: driver://user:pass@host:port/dbname?serverVersion=X
	url = strings.ToLower(url)

	var driver string
	if strings.HasPrefix(url, "postgresql://") || strings.HasPrefix(url, "postgres://") {
		driver = "pgsql"
	} else if strings.HasPrefix(url, "mysql://") {
		driver = "mysql"
	} else if strings.HasPrefix(url, "sqlite://") {
		driver = "sqlite"
	} else {
		return "", ""
	}

	// Try to extract version from serverVersion parameter
	version := getDefaultVersion(driver)
	re := regexp.MustCompile(`serverversion=([0-9.]+)`)
	if matches := re.FindStringSubmatch(url); len(matches) > 1 {
		version = matches[1]
	}

	return driver, version
}

// normalizeDriver normalizes the database driver name
func normalizeDriver(driver string) string {
	switch strings.ToLower(driver) {
	case "pdo_pgsql", "postgresql", "postgres", "pgsql":
		return "pgsql"
	case "pdo_mysql", "mysql", "mysqli":
		return "mysql"
	case "pdo_sqlite", "sqlite", "sqlite3":
		return "sqlite"
	default:
		return driver
	}
}

// getDefaultVersion returns the default version for a database driver
func getDefaultVersion(driver string) string {
	switch driver {
	case "pgsql":
		return "16"
	case "mysql":
		return "8.0"
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
