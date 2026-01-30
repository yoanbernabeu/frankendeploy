package scanner

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
)

// PackageJSON represents a package.json file
type PackageJSON struct {
	Name    string            `json:"name"`
	Scripts map[string]string `json:"scripts"`
	DevDeps map[string]string `json:"devDependencies"`
	Deps    map[string]string `json:"dependencies"`
}

// DetectAssets detects the asset build configuration
func (s *Scanner) DetectAssets() (*config.AssetsConfig, error) {
	assetsConfig := &config.AssetsConfig{}

	// Check for AssetMapper (Symfony native, no build needed)
	if s.hasAssetMapper() {
		assetsConfig.BuildTool = "assetmapper"
		assetsConfig.OutputDir = "public/assets"
		return assetsConfig, nil
	}

	// Check for package.json
	packageJSON, err := s.parsePackageJSON()
	if err != nil {
		// No package.json, no frontend build
		return nil, nil
	}

	// Detect build tool
	if s.hasVite(packageJSON) {
		assetsConfig.BuildTool = "npm"
		assetsConfig.BuildCommand = s.getBuildCommand(packageJSON)
		assetsConfig.OutputDir = "public/build"
		return assetsConfig, nil
	}

	if s.hasWebpackEncore(packageJSON) {
		assetsConfig.BuildTool = "npm"
		assetsConfig.BuildCommand = s.getBuildCommand(packageJSON)
		assetsConfig.OutputDir = "public/build"
		return assetsConfig, nil
	}

	// Generic npm/yarn setup
	if packageJSON.Scripts["build"] != "" {
		assetsConfig.BuildTool = s.detectPackageManager()
		assetsConfig.BuildCommand = assetsConfig.BuildTool + " run build"
		assetsConfig.OutputDir = "public/build"
		return assetsConfig, nil
	}

	return nil, nil
}

// parsePackageJSON parses the package.json file
func (s *Scanner) parsePackageJSON() (*PackageJSON, error) {
	packagePath := filepath.Join(s.projectPath, "package.json")
	data, err := os.ReadFile(packagePath)
	if err != nil {
		return nil, err
	}

	var pkg PackageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}

	return &pkg, nil
}

// hasAssetMapper checks if Symfony AssetMapper is used
func (s *Scanner) hasAssetMapper() bool {
	// Check for importmap.php
	importmapPath := filepath.Join(s.projectPath, "importmap.php")
	if _, err := os.Stat(importmapPath); err == nil {
		return true
	}

	// Check for asset_mapper.yaml config
	assetMapperPath := filepath.Join(s.projectPath, "config", "packages", "asset_mapper.yaml")
	if _, err := os.Stat(assetMapperPath); err == nil {
		return true
	}

	return false
}

// hasVite checks if Vite is used
func (s *Scanner) hasVite(pkg *PackageJSON) bool {
	if pkg == nil {
		return false
	}

	// Check for vite in dependencies
	if _, ok := pkg.DevDeps["vite"]; ok {
		return true
	}
	if _, ok := pkg.Deps["vite"]; ok {
		return true
	}

	// Check for vite.config.js
	viteConfigPath := filepath.Join(s.projectPath, "vite.config.js")
	if _, err := os.Stat(viteConfigPath); err == nil {
		return true
	}

	viteConfigTsPath := filepath.Join(s.projectPath, "vite.config.ts")
	if _, err := os.Stat(viteConfigTsPath); err == nil {
		return true
	}

	return false
}

// hasWebpackEncore checks if Webpack Encore is used
func (s *Scanner) hasWebpackEncore(pkg *PackageJSON) bool {
	if pkg == nil {
		return false
	}

	// Check for @symfony/webpack-encore in dependencies
	if _, ok := pkg.DevDeps["@symfony/webpack-encore"]; ok {
		return true
	}
	if _, ok := pkg.Deps["@symfony/webpack-encore"]; ok {
		return true
	}

	// Check for webpack.config.js
	webpackConfigPath := filepath.Join(s.projectPath, "webpack.config.js")
	if _, err := os.Stat(webpackConfigPath); err == nil {
		return true
	}

	return false
}

// getBuildCommand returns the appropriate build command
func (s *Scanner) getBuildCommand(pkg *PackageJSON) string {
	pm := s.detectPackageManager()

	// Check for prod script first
	if _, ok := pkg.Scripts["build:prod"]; ok {
		return pm + " run build:prod"
	}
	if _, ok := pkg.Scripts["build"]; ok {
		return pm + " run build"
	}

	return pm + " run build"
}

// detectPackageManager detects whether npm, yarn, or pnpm is used
func (s *Scanner) detectPackageManager() string {
	// Check for lock files
	if _, err := os.Stat(filepath.Join(s.projectPath, "pnpm-lock.yaml")); err == nil {
		return "pnpm"
	}
	if _, err := os.Stat(filepath.Join(s.projectPath, "yarn.lock")); err == nil {
		return "yarn"
	}
	// Default to npm
	return "npm"
}
