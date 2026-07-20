package scanner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ComposerJSON represents a composer.json file
type ComposerJSON struct {
	Name       string            `json:"name"`
	Require    map[string]string `json:"require"`
	RequireDev map[string]string `json:"require-dev"`
}

// ComposerResult holds parsed composer.json information
type ComposerResult struct {
	Name       string
	PHPVersion string
	// PHPVersionWarning is set when the detected version had to be adjusted
	// (e.g. floored to the minimum version supported by FrankenPHP).
	PHPVersionWarning string
	Extensions        []string
	HasSymfony        bool
	Packages          map[string]string
}

// ParseComposer parses the composer.json file
func (s *Scanner) ParseComposer() (*ComposerResult, error) {
	composerPath := filepath.Join(s.projectPath, "composer.json")
	data, err := os.ReadFile(composerPath)
	if err != nil {
		return nil, err
	}

	var composer ComposerJSON
	if err := json.Unmarshal(data, &composer); err != nil {
		return nil, err
	}

	result := &ComposerResult{
		Name:       composer.Name,
		Packages:   make(map[string]string),
		Extensions: []string{},
	}

	// Merge require and require-dev
	for pkg, version := range composer.Require {
		result.Packages[pkg] = version
	}
	for pkg, version := range composer.RequireDev {
		result.Packages[pkg] = version
	}

	// Extract PHP version
	if phpVersion, ok := composer.Require["php"]; ok {
		result.PHPVersion, result.PHPVersionWarning = extractPHPVersion(phpVersion)
	} else {
		result.PHPVersion = defaultPHPVersion
	}

	// Check for Symfony
	for pkg := range composer.Require {
		if strings.HasPrefix(pkg, "symfony/") {
			result.HasSymfony = true
			break
		}
	}

	// Extract PHP extensions
	for pkg := range composer.Require {
		if strings.HasPrefix(pkg, "ext-") {
			extName := strings.TrimPrefix(pkg, "ext-")
			result.Extensions = append(result.Extensions, extName)
		}
	}

	return result, nil
}

const (
	// defaultPHPVersion is used when no usable PHP 8.x version is found.
	defaultPHPVersion = "8.3"
	// minPHPMinor is the minimum 8.x minor with an official FrankenPHP image
	// (dunglas/frankenphp requires PHP >= 8.2).
	minPHPMinor = 2
)

// phpConstraintRegex captures an optional comparison operator and the minor
// component of each "8.x" version in a composer constraint.
var phpConstraintRegex = regexp.MustCompile(`(>=|<=|<|>|\^|~)?\s*8\.(\d+)`)

// extractPHPVersion extracts the highest PHP 8.x version allowed by the
// composer constraint. An exclusive upper bound ("<8.4") contributes the
// version just below it ("8.3"). Minors are compared numerically so "8.10"
// ranks above "8.9". The result is floored at 8.2 (the minimum version with
// a FrankenPHP image); a non-empty warning is returned when the constraint
// had to be adjusted or could not be interpreted.
func extractPHPVersion(constraint string) (string, string) {
	matches := phpConstraintRegex.FindAllStringSubmatch(constraint, -1)

	if len(matches) == 0 {
		if strings.TrimSpace(constraint) == "" {
			return defaultPHPVersion, ""
		}
		return defaultPHPVersion, fmt.Sprintf(
			"no PHP 8.x version found in composer constraint %q, using PHP %s",
			constraint, defaultPHPVersion)
	}

	highestMinor := -1
	for _, m := range matches {
		minor, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		if m[1] == "<" {
			// Exclusive upper bound: the highest allowed minor is one below.
			minor--
		}
		if minor > highestMinor {
			highestMinor = minor
		}
	}

	if highestMinor < minPHPMinor {
		return defaultPHPVersion, fmt.Sprintf(
			"composer constraint %q allows PHP 8.%d but FrankenPHP requires PHP >= 8.2, using PHP %s (set php.version in frankendeploy.yaml to override)",
			constraint, highestMinor, defaultPHPVersion)
	}

	return fmt.Sprintf("8.%d", highestMinor), ""
}

// GetPackageVersion returns the version constraint for a package
func (c *ComposerResult) GetPackageVersion(pkg string) string {
	return c.Packages[pkg]
}

// HasPackage checks if a package is required
func (c *ComposerResult) HasPackage(pkg string) bool {
	_, ok := c.Packages[pkg]
	return ok
}

// HasAnyPackage checks if any of the given packages is required
func (c *ComposerResult) HasAnyPackage(packages ...string) bool {
	for _, pkg := range packages {
		if c.HasPackage(pkg) {
			return true
		}
	}
	return false
}
