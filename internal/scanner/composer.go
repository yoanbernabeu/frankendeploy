package scanner

import (
	"encoding/json"
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
	Extensions []string
	HasSymfony bool
	Packages   map[string]string
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
		result.PHPVersion = extractPHPVersion(phpVersion)
	} else {
		result.PHPVersion = "8.3" // Default
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

// extractPHPVersion extracts a clean PHP version from composer constraint.
// Compares matches numerically so "8.10" is recognised as higher than "8.9"
// (string comparison would incorrectly rank "8.9" above "8.10").
func extractPHPVersion(constraint string) string {
	re := regexp.MustCompile(`8\.\d+`)
	matches := re.FindAllString(constraint, -1)

	if len(matches) == 0 {
		return "8.3" // Default to latest stable
	}

	highest := matches[0]
	highestMinor := phpMinor(highest)
	for _, m := range matches[1:] {
		if phpMinor(m) > highestMinor {
			highest = m
			highestMinor = phpMinor(m)
		}
	}

	return highest
}

// phpMinor returns the minor-version component of a "8.x" string, or -1 if
// the string does not follow that format.
func phpMinor(v string) int {
	parts := strings.SplitN(v, ".", 2)
	if len(parts) != 2 {
		return -1
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil {
		return -1
	}
	return n
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
