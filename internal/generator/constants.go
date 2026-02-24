package generator

import "github.com/yoanbernabeu/frankendeploy/internal/constants"

// Centralized constants for all generated Docker artifacts.
// These values are used in templates via template functions and in Go code.
//
// Shared constants (AppPort, DefaultUID, DefaultGID, NetworkName) are sourced
// from internal/constants to avoid duplication.

const (
	// Ports — sourced from constants package
	AppPort         = constants.AppPort
	MetricsPort     = "2019"
	DevExternalPort = "8000"

	// User/Group IDs — sourced from constants package
	DefaultUID = constants.ContainerUID
	DefaultGID = constants.ContainerGID

	// Networking — sourced from constants package
	NetworkName = constants.NetworkName

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
