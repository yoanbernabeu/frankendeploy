package generator

// Centralized constants for all generated Docker artifacts.
// These values are used in templates via template functions and in Go code.

const (
	// Ports
	AppPort         = "8080"
	MetricsPort     = "2019"
	DevExternalPort = "8000"

	// User/Group IDs for non-root container execution
	DefaultUID = "1000"
	DefaultGID = "1000"

	// Networking
	NetworkName = "frankendeploy"

	// Database wait defaults (entrypoint script)
	DefaultDBWaitMaxAttempts = 30
	DefaultDBWaitInterval    = 1

	// Default dev database credentials
	DefaultDevDBUser     = "app"
	DefaultDevDBPassword = "app"
	DefaultDevDBName     = "app"

	// Database ports
	PostgresPort = "5432"
	MySQLPort    = "3306"
)
