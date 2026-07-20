package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

// writeProjectFile writes a file inside the temp project, creating parent directories.
func writeProjectFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	fullPath := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("failed to create dir for %s: %v", relPath, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", relPath, err)
	}
}

// TestScanner_Scan_APIPlatformWithoutPackageJSON reproduces the panic reported in #39:
// a pure API Platform project (no package.json, no AssetMapper) must scan successfully.
func TestScanner_Scan_APIPlatformWithoutPackageJSON(t *testing.T) {
	tempDir := t.TempDir()

	writeProjectFile(t, tempDir, "composer.json", `{
		"require": {
			"php": ">=8.2",
			"symfony/framework-bundle": "^7.0",
			"api-platform/core": "^3.2",
			"doctrine/orm": "^2.17"
		}
	}`)
	writeProjectFile(t, tempDir, ".env", "DATABASE_URL=postgresql://app:pass@127.0.0.1:5432/app?serverVersion=16\n")
	writeProjectFile(t, tempDir, "config/packages/doctrine.yaml", "doctrine:\n  dbal:\n    url: '%env(resolve:DATABASE_URL)%'\n")

	result, err := New(tempDir).Scan()
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}

	if !result.IsSymfony {
		t.Error("expected IsSymfony to be true")
	}
	if result.Assets.BuildTool != "" {
		t.Errorf("expected no asset build tool, got %q", result.Assets.BuildTool)
	}
	if !result.HasDoctrine {
		t.Error("expected HasDoctrine to be true")
	}
	if result.Database.Driver != "pgsql" {
		t.Errorf("expected pgsql database driver, got %q", result.Database.Driver)
	}
	if !result.HasAPIPlatform {
		t.Error("expected HasAPIPlatform to be true")
	}

	cfg := New(tempDir).ToProjectConfig(result, "apiproject")
	if cfg.Deploy.HealthcheckPath != "/api" {
		t.Errorf("expected healthcheck path /api for API Platform project, got %q", cfg.Deploy.HealthcheckPath)
	}
}

// TestScanner_Scan_APIPlatform4SymfonyPackage covers the API Platform 4 split
// packages, where Symfony projects require api-platform/symfony instead of
// api-platform/core.
func TestScanner_Scan_APIPlatform4SymfonyPackage(t *testing.T) {
	tempDir := t.TempDir()

	writeProjectFile(t, tempDir, "composer.json", `{
		"require": {
			"php": ">=8.2",
			"symfony/framework-bundle": "^7.1",
			"api-platform/symfony": "^4.0"
		}
	}`)

	result, err := New(tempDir).Scan()
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}
	if !result.HasAPIPlatform {
		t.Error("expected HasAPIPlatform to be true for api-platform/symfony")
	}
	if cfg := New(tempDir).ToProjectConfig(result, "app"); cfg.Deploy.HealthcheckPath != "/api" {
		t.Errorf("expected healthcheck path /api, got %q", cfg.Deploy.HealthcheckPath)
	}
}

// TestScanner_Scan_PackageJSONWithoutBuildScript covers the second nil-return path of
// DetectAssets: a package.json that has no build script must not break Scan.
func TestScanner_Scan_PackageJSONWithoutBuildScript(t *testing.T) {
	tempDir := t.TempDir()

	writeProjectFile(t, tempDir, "composer.json", `{
		"require": {
			"php": ">=8.2",
			"symfony/framework-bundle": "^7.0"
		}
	}`)
	writeProjectFile(t, tempDir, "package.json", `{
		"name": "app",
		"scripts": {"lint": "eslint ."}
	}`)

	result, err := New(tempDir).Scan()
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}
	if result.Assets.BuildTool != "" {
		t.Errorf("expected no asset build tool, got %q", result.Assets.BuildTool)
	}
	if result.HasAPIPlatform {
		t.Error("expected HasAPIPlatform to be false without api-platform packages")
	}
	// Non-API Platform projects keep the default healthcheck path
	if cfg := New(tempDir).ToProjectConfig(result, "app"); cfg.Deploy.HealthcheckPath != "/" {
		t.Errorf("expected default healthcheck path /, got %q", cfg.Deploy.HealthcheckPath)
	}
}

// TestScanner_Scan_AssetMapper covers the AssetMapper path end to end.
func TestScanner_Scan_AssetMapper(t *testing.T) {
	tempDir := t.TempDir()

	writeProjectFile(t, tempDir, "composer.json", `{
		"require": {
			"php": ">=8.2",
			"symfony/framework-bundle": "^7.0",
			"symfony/asset-mapper": "^7.0"
		}
	}`)
	writeProjectFile(t, tempDir, "importmap.php", "<?php return [];\n")

	result, err := New(tempDir).Scan()
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}
	if result.Assets.BuildTool != "assetmapper" {
		t.Errorf("expected assetmapper build tool, got %q", result.Assets.BuildTool)
	}
}

// TestScanner_Scan_WebpackEncore covers a classic webapp with a frontend build.
func TestScanner_Scan_WebpackEncore(t *testing.T) {
	tempDir := t.TempDir()

	writeProjectFile(t, tempDir, "composer.json", `{
		"require": {
			"php": ">=8.2",
			"symfony/framework-bundle": "^6.4"
		}
	}`)
	writeProjectFile(t, tempDir, "package.json", `{
		"name": "app",
		"devDependencies": {"@symfony/webpack-encore": "^4.0"},
		"scripts": {"build": "encore production"}
	}`)

	result, err := New(tempDir).Scan()
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}
	if result.Assets.BuildTool != "npm" {
		t.Errorf("expected npm build tool, got %q", result.Assets.BuildTool)
	}
	if result.Assets.BuildCommand == "" {
		t.Error("expected a build command for Webpack Encore project")
	}
}

// TestScanner_DetectAssets_NeverReturnsNilConfig locks the contract relied on by Scan().
func TestScanner_DetectAssets_NeverReturnsNilConfig(t *testing.T) {
	// Case 1: empty project (no package.json at all)
	s := New(t.TempDir())
	cfg, err := s.DetectAssets()
	if err != nil {
		t.Fatalf("DetectAssets() failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("DetectAssets() returned nil config for project without package.json")
	}

	// Case 2: package.json without build script
	tempDir := t.TempDir()
	writeProjectFile(t, tempDir, "package.json", `{"name": "app", "scripts": {}}`)
	cfg, err = New(tempDir).DetectAssets()
	if err != nil {
		t.Fatalf("DetectAssets() failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("DetectAssets() returned nil config for package.json without build script")
	}
}
