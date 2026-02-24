package generator

import "fmt"

// DBDriverInfo holds configuration for a database driver used in Docker generation and deployment.
type DBDriverInfo struct {
	DockerImage    string
	DefaultVersion string
	ImageSuffix    string // suffix appended after version for deploy images (e.g., "-alpine")
	Port           string
	URLScheme      string
	URLCharset     string
	HealthcheckCmd []string
	DataVolumePath string
	PHPExtension   string

	// BuildEnvArgs returns Docker env flags (-e ...) for the database container.
	// Arguments (user, password, dbName) should be pre-escaped for shell safety.
	BuildEnvArgs func(user, password, dbName string) string

	// BuildHealthCmd returns a command to check if the database is ready.
	// Arguments (containerName, user, password) should be pre-escaped for shell safety.
	BuildHealthCmd func(containerName, user, password string) string
}

// FullImage returns the Docker image reference with version and optional suffix.
func (d DBDriverInfo) FullImage(version string) string {
	if version == "" {
		version = d.DefaultVersion
	}
	return fmt.Sprintf("%s:%s%s", d.DockerImage, version, d.ImageSuffix)
}

// BuildDatabaseURL constructs a Symfony-compatible DATABASE_URL.
func (d DBDriverInfo) BuildDatabaseURL(user, password, host, dbName, version string) string {
	if version == "" {
		version = d.DefaultVersion
	}
	return fmt.Sprintf("%s://%s:%s@%s:%s/%s?serverVersion=%s&charset=%s",
		d.URLScheme, user, password, host, d.Port, dbName, version, d.URLCharset)
}

var dbDriverRegistry = map[string]DBDriverInfo{
	"pgsql": {
		DockerImage:    "postgres",
		DefaultVersion: "16",
		ImageSuffix:    "-alpine",
		Port:           PostgresPort,
		URLScheme:      "postgresql",
		URLCharset:     "utf8",
		HealthcheckCmd: []string{"CMD-SHELL", "pg_isready -U " + DefaultDevDBUser + " -d " + DefaultDevDBName},
		DataVolumePath: "/var/lib/postgresql/data",
		PHPExtension:   "pdo_pgsql",
		BuildEnvArgs: func(user, password, dbName string) string {
			return fmt.Sprintf("-e POSTGRES_USER=%s -e POSTGRES_PASSWORD=%s -e POSTGRES_DB=%s",
				user, password, dbName)
		},
		BuildHealthCmd: func(containerName, user, _ string) string {
			return fmt.Sprintf("docker exec %s pg_isready -U %s", containerName, user)
		},
	},
	"mysql": {
		DockerImage:    "mysql",
		DefaultVersion: "8.0",
		ImageSuffix:    "",
		Port:           MySQLPort,
		URLScheme:      "mysql",
		URLCharset:     "utf8mb4",
		HealthcheckCmd: []string{"CMD", "mysqladmin", "ping", "-h", "localhost"},
		DataVolumePath: "/var/lib/mysql",
		PHPExtension:   "pdo_mysql",
		BuildEnvArgs: func(user, password, dbName string) string {
			return fmt.Sprintf("-e MYSQL_ROOT_PASSWORD=%s -e MYSQL_USER=%s -e MYSQL_PASSWORD=%s -e MYSQL_DATABASE=%s",
				password, user, password, dbName)
		},
		BuildHealthCmd: func(containerName, user, password string) string {
			return fmt.Sprintf("docker exec %s mysqladmin ping -u%s -p%s --silent",
				containerName, user, password)
		},
	},
}

// GetDBDriverInfo returns the driver info for the given driver name.
func GetDBDriverInfo(driver string) (DBDriverInfo, error) {
	info, ok := dbDriverRegistry[driver]
	if !ok {
		return DBDriverInfo{}, fmt.Errorf("unknown database driver: %q", driver)
	}
	return info, nil
}

// IsContainerizedDriver returns true if the driver runs in a container (not file-based like sqlite).
func IsContainerizedDriver(driver string) bool {
	_, ok := dbDriverRegistry[driver]
	return ok
}
