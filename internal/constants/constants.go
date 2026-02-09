package constants

import (
	"path/filepath"
	"time"
)

// Base paths for FrankenDeploy on the server
const (
	BasePath    = "/opt/frankendeploy"
	AppsDir     = BasePath + "/apps"
	CaddyDir    = BasePath + "/caddy"
	CaddyAppsDir = CaddyDir + "/apps"
	CaddyLogsDir = CaddyDir + "/logs"
)

// Container configuration
const (
	ContainerUser = "1000:1000"
	ContainerUID  = "1000"
	ContainerGID  = "1000"
	AppPort       = "8080"
	NetworkName   = "frankendeploy"
)

// Health check defaults
const (
	PreHealthSleep     = 5 * time.Second
	HealthCheckTimeout = 30 * time.Second
	HealthCheckRetries = 5
	HealthCheckInterval = 2 * time.Second
)

// Deployment defaults
const (
	DefaultKeepReleases = 5
	DefaultCertEmail    = "admin@localhost"
)

// AppBasePath returns the base path for an application on the server.
func AppBasePath(name string) string {
	return filepath.Join(AppsDir, name)
}

// AppReleasePath returns the path for a specific release.
func AppReleasePath(name, tag string) string {
	return filepath.Join(AppsDir, name, "releases", tag)
}

// AppCurrentPath returns the current symlink path for an app.
func AppCurrentPath(name string) string {
	return filepath.Join(AppsDir, name, "current")
}

// AppSharedPath returns the shared directory path for an app.
func AppSharedPath(name string) string {
	return filepath.Join(AppsDir, name, "shared")
}

// CaddyAppConfig returns the Caddy config file path for an app.
func CaddyAppConfig(name string) string {
	return filepath.Join(CaddyAppsDir, name+".caddy")
}

// AppEnvFilePath returns the .env.local file path for an app.
func AppEnvFilePath(name string) string {
	return filepath.Join(AppsDir, name, "shared", ".env.local")
}
