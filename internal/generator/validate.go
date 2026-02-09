package generator

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/yoanbernabeu/frankendeploy/internal/security"
)

var (
	phpVersionRegex      = regexp.MustCompile(`^8\.[1-4]$`)
	phpExtensionRegex    = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
	extraPackageRegex    = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._+-]*$`)
	frankenPHPVersionRgx = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
)

// shellInjectionPatterns are sequences blocked in extra commands.
var shellInjectionPatterns = []string{"$(", "`", ";", "&&", "||", "|", ">", "<", "\n", "\r"}

// ValidateDockerfileData validates inputs before Dockerfile generation.
func ValidateDockerfileData(data DockerfileData) error {
	if !phpVersionRegex.MatchString(data.PHP.Version) {
		return fmt.Errorf("invalid PHP version %q: must match 8.1–8.4", data.PHP.Version)
	}

	for _, ext := range data.PHP.Extensions {
		if !phpExtensionRegex.MatchString(ext) {
			return fmt.Errorf("invalid PHP extension name %q: only alphanumeric and underscores allowed", ext)
		}
	}

	if data.Assets != nil && data.Assets.BuildCommand != "" {
		if err := security.ValidateDockerCommand(data.Assets.BuildCommand); err != nil {
			return fmt.Errorf("invalid build command: %w", err)
		}
	}

	for _, pkg := range data.Dockerfile.ExtraPackages {
		if !extraPackageRegex.MatchString(pkg) {
			return fmt.Errorf("invalid extra package name %q: only alphanumeric, dots, plus, and hyphens allowed", pkg)
		}
	}

	for _, cmd := range data.Dockerfile.ExtraCommands {
		for _, pattern := range shellInjectionPatterns {
			if strings.Contains(cmd, pattern) {
				return fmt.Errorf("extra command contains potentially dangerous sequence %q", pattern)
			}
		}
	}

	if data.FrankenPHPVersion != "" && !frankenPHPVersionRgx.MatchString(data.FrankenPHPVersion) {
		return fmt.Errorf("invalid FrankenPHP version %q: only alphanumeric, dots, underscores, and hyphens allowed", data.FrankenPHPVersion)
	}

	return nil
}

// ValidateComposeData validates inputs before compose file generation.
func ValidateComposeData(data ComposeData) error {
	if data.Name == "" {
		return fmt.Errorf("compose data: name is required")
	}

	if data.Database.Driver != "" && data.Database.Driver != "sqlite" {
		if _, err := GetDBDriverInfo(data.Database.Driver); err != nil {
			return fmt.Errorf("compose data: %w", err)
		}
		if data.Database.Version == "" {
			return fmt.Errorf("compose data: database version is required for driver %q", data.Database.Driver)
		}
	}

	return nil
}
