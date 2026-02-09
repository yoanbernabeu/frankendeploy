package generator

import "fmt"

// DBDriverInfo holds configuration for a database driver used in Docker generation.
type DBDriverInfo struct {
	DockerImage    string
	DefaultVersion string
	Port           string
	URLScheme      string
	URLCharset     string
	HealthcheckCmd []string
	DataVolumePath string
	PHPExtension   string
}

var dbDriverRegistry = map[string]DBDriverInfo{
	"pgsql": {
		DockerImage:    "postgres",
		DefaultVersion: "16",
		Port:           PostgresPort,
		URLScheme:      "postgresql",
		URLCharset:     "utf8",
		HealthcheckCmd: []string{"CMD-SHELL", "pg_isready -U " + DefaultDevDBUser + " -d " + DefaultDevDBName},
		DataVolumePath: "/var/lib/postgresql/data",
		PHPExtension:   "pdo_pgsql",
	},
	"mysql": {
		DockerImage:    "mysql",
		DefaultVersion: "8.0",
		Port:           MySQLPort,
		URLScheme:      "mysql",
		URLCharset:     "utf8mb4",
		HealthcheckCmd: []string{"CMD", "mysqladmin", "ping", "-h", "localhost"},
		DataVolumePath: "/var/lib/mysql",
		PHPExtension:   "pdo_mysql",
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
