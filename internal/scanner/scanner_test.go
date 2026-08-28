package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
)

func TestScanner_IsSymfonyProject(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	// Test without composer.json
	s := New(tempDir)
	if s.IsSymfonyProject() {
		t.Error("expected false for non-Symfony project")
	}

	// Create a basic composer.json with Symfony
	composerContent := `{
		"require": {
			"php": ">=8.1",
			"symfony/framework-bundle": "^6.4"
		}
	}`
	err := os.WriteFile(filepath.Join(tempDir, "composer.json"), []byte(composerContent), 0644)
	if err != nil {
		t.Fatalf("failed to create composer.json: %v", err)
	}

	// Test with Symfony project
	s = New(tempDir)
	if !s.IsSymfonyProject() {
		t.Error("expected true for Symfony project")
	}
}

func TestScanner_DetectPackageManager(t *testing.T) {
	tempDir := t.TempDir()
	s := New(tempDir)

	// Default should be npm
	if s.detectPackageManager() != "npm" {
		t.Error("expected npm as default package manager")
	}

	// Create yarn.lock
	err := os.WriteFile(filepath.Join(tempDir, "yarn.lock"), []byte{}, 0644)
	if err != nil {
		t.Fatalf("failed to create yarn.lock: %v", err)
	}

	if s.detectPackageManager() != "yarn" {
		t.Error("expected yarn when yarn.lock exists")
	}

	// Create pnpm-lock.yaml (should take precedence)
	err = os.WriteFile(filepath.Join(tempDir, "pnpm-lock.yaml"), []byte{}, 0644)
	if err != nil {
		t.Fatalf("failed to create pnpm-lock.yaml: %v", err)
	}

	if s.detectPackageManager() != "pnpm" {
		t.Error("expected pnpm when pnpm-lock.yaml exists")
	}
}

func TestEnhanceExtensions_Dedup(t *testing.T) {
	s := New(t.TempDir())
	result := &config.ScanResult{
		Database: config.DatabaseConfig{Driver: "pgsql"},
	}

	// Input with duplicates: pdo_pgsql appears in composer AND would be added by enhanceExtensions
	input := []string{"intl", "opcache", "zip", "pdo_pgsql", "intl", "zip"}
	got := s.enhanceExtensions(input, result, nil, messengerTransportInfo{})

	// Check no duplicates
	seen := make(map[string]bool)
	for _, ext := range got {
		if seen[ext] {
			t.Errorf("duplicate extension found: %q", ext)
		}
		seen[ext] = true
	}

	// Ensure pdo_pgsql is present
	if !seen["pdo_pgsql"] {
		t.Error("expected pdo_pgsql in result")
	}
}

func TestExtractPHPVersion(t *testing.T) {
	tests := []struct {
		constraint  string
		expected    string
		wantWarning bool
	}{
		// FrankenPHP requires PHP >= 8.2: versions below are floored to the
		// default with a warning (a stock Symfony 6.4 skeleton declares >=8.1).
		{">=8.1", "8.3", true},
		{"^8.0", "8.3", true},
		{"^8.2", "8.2", false},
		{"~8.3", "8.3", false},
		{"8.2.*", "8.2", false},
		// "<8.4" is an exclusive upper bound: the highest allowed version is 8.3.
		{">=8.1 <8.4", "8.3", false},
		{"^8.5", "8.5", false},
		{">=8.10", "8.10", false},
		// Guards the numeric-comparison fix: lexicographically "8.9" > "8.11"
		// ('9' > '1' at the same position) and "8.2" > "8.10" ('2' > '1').
		// The minor version must be compared as a number.
		{">=8.9 <8.11", "8.10", false},
		{">=8.2 <8.10", "8.9", false},
		{">=7.4", "8.3", true}, // No 8.x found, defaults with a warning
		{"", "8.3", false},
	}

	for _, tt := range tests {
		t.Run(tt.constraint, func(t *testing.T) {
			result, warning := extractPHPVersion(tt.constraint)
			if result != tt.expected {
				t.Errorf("extractPHPVersion(%q) = %q, want %q", tt.constraint, result, tt.expected)
			}
			if tt.wantWarning && warning == "" {
				t.Errorf("extractPHPVersion(%q): expected a warning, got none", tt.constraint)
			}
			if !tt.wantWarning && warning != "" {
				t.Errorf("extractPHPVersion(%q): unexpected warning %q", tt.constraint, warning)
			}
		})
	}
}

// TestScanner_Scan_PHPVersionFloorWarning verifies that the floor warning
// reaches ScanResult.Warnings (what `frankendeploy init` displays).
func TestScanner_Scan_PHPVersionFloorWarning(t *testing.T) {
	tempDir := t.TempDir()
	composerContent := `{
		"require": {
			"php": ">=8.1",
			"symfony/framework-bundle": "^6.4"
		}
	}`
	if err := os.WriteFile(filepath.Join(tempDir, "composer.json"), []byte(composerContent), 0644); err != nil {
		t.Fatalf("failed to create composer.json: %v", err)
	}

	result, err := New(tempDir).Scan()
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}
	if result.PHPVersion != "8.3" {
		t.Errorf("expected PHP version floored to 8.3, got %q", result.PHPVersion)
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "FrankenPHP requires PHP >= 8.2") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a floor warning in ScanResult.Warnings, got %v", result.Warnings)
	}
}

func TestGetEnvFile_InlineComments(t *testing.T) {
	tempDir := t.TempDir()

	envContent := `# Full line comment
APP_ENV=prod
APP_SECRET=abc123 # this is a comment
DATABASE_URL="postgresql://user:pass#word@localhost/db" # connection string
QUOTED_HASH='value#with#hashes'
NO_SPACE_HASH=value#notacomment
EMPTY_VALUE=
`
	if err := os.WriteFile(filepath.Join(tempDir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	s := New(tempDir)
	env, err := s.GetEnvFile(".env")
	if err != nil {
		t.Fatalf("GetEnvFile() error = %v", err)
	}

	tests := []struct {
		key      string
		expected string
	}{
		{"APP_ENV", "prod"},
		{"APP_SECRET", "abc123"},
		{"DATABASE_URL", "postgresql://user:pass#word@localhost/db"},
		{"QUOTED_HASH", "value#with#hashes"},
		{"NO_SPACE_HASH", "value#notacomment"},
		{"EMPTY_VALUE", ""},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got, ok := env[tt.key]
			if !ok {
				t.Fatalf("key %q not found in env", tt.key)
			}
			if got != tt.expected {
				t.Errorf("env[%q] = %q, want %q", tt.key, got, tt.expected)
			}
		})
	}
}

func TestScan_FrankenPHPRuntime_EnablesWorker(t *testing.T) {
	tempDir := t.TempDir()
	composer := `{
		"require": {
			"php": ">=8.3",
			"symfony/framework-bundle": "^7.1",
			"runtime/frankenphp-symfony": "^0.2"
		}
	}`
	if err := os.WriteFile(filepath.Join(tempDir, "composer.json"), []byte(composer), 0644); err != nil {
		t.Fatal(err)
	}

	s := New(tempDir)
	result, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if !result.HasFrankenPHPRuntime {
		t.Error("expected HasFrankenPHPRuntime to be true")
	}

	cfg := s.ToProjectConfig(result, "myapp")
	if !cfg.FrankenPHP.Worker {
		t.Error("expected FrankenPHP.Worker to be enabled when runtime/frankenphp-symfony is present")
	}

	foundWarning := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "worker") {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Errorf("expected a warning announcing worker mode, got: %v", result.Warnings)
	}
}

func TestScan_NoFrankenPHPRuntime_WorkerDisabled(t *testing.T) {
	tempDir := t.TempDir()
	composer := `{
		"require": {
			"php": ">=8.3",
			"symfony/framework-bundle": "^7.1"
		}
	}`
	if err := os.WriteFile(filepath.Join(tempDir, "composer.json"), []byte(composer), 0644); err != nil {
		t.Fatal(err)
	}

	s := New(tempDir)
	result, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if result.HasFrankenPHPRuntime {
		t.Error("expected HasFrankenPHPRuntime to be false")
	}
	cfg := s.ToProjectConfig(result, "myapp")
	if cfg.FrankenPHP.Worker {
		t.Error("worker mode must stay opt-in without the runtime package")
	}
}

// --- Issue #59: smarter detection ---

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

const minimalSymfonyComposer = `{"require": {"php": ">=8.3", "symfony/framework-bundle": "^7.1"}}`

func TestGetMergedEnv_EnvLocalOverrides(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".env", "DATABASE_URL=postgresql://app:x@127.0.0.1:5432/app?serverVersion=16\nAPP_ENV=dev\n")
	writeFile(t, dir, ".env.local", "DATABASE_URL=mysql://app:x@127.0.0.1:3306/app?serverVersion=8.0\n")

	env, err := New(dir).GetMergedEnv()
	if err != nil {
		t.Fatalf("GetMergedEnv: %v", err)
	}
	if !strings.HasPrefix(env["DATABASE_URL"], "mysql://") {
		t.Errorf(".env.local must override .env, got %q", env["DATABASE_URL"])
	}
	if env["APP_ENV"] != "dev" {
		t.Errorf(".env values not overridden must survive, got %q", env["APP_ENV"])
	}
}

func TestDetectDatabase_EnvLocalOverridesEnv_WithWarning(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "composer.json", minimalSymfonyComposer)
	writeFile(t, dir, ".env", "DATABASE_URL=postgresql://app:x@127.0.0.1:5432/app?serverVersion=16\n")
	writeFile(t, dir, ".env.local", "DATABASE_URL=mysql://app:x@127.0.0.1:3306/app?serverVersion=8.0\n")

	db, warnings, err := New(dir).DetectDatabase()
	if err != nil {
		t.Fatalf("DetectDatabase: %v", err)
	}
	if db == nil || db.Driver != "mysql" {
		t.Fatalf("expected mysql from .env.local, got %+v", db)
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, ".env.local") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a divergence warning mentioning .env.local, got %v", warnings)
	}
}

func TestParseDBURL_MariaDBAndAltSchemes(t *testing.T) {
	tests := []struct {
		url         string
		wantDriver  string
		wantVersion string
	}{
		{"mysql://app:x@db:3306/app?serverVersion=10.11.2-MariaDB", "mariadb", "10.11.2"},
		{"mysql://app:x@db:3306/app?serverVersion=mariadb-10.6", "mariadb", "10.6"},
		{"mariadb://app:x@db:3306/app", "mariadb", "11"},
		{"mysql2://app:x@db:3306/app?serverVersion=8.0", "mysql", "8.0"},
		{"pgsql://app:x@db:5432/app", "pgsql", "16"},
		{"postgresql://app:x@db:5432/app?serverVersion=16", "pgsql", "16"},
	}
	for _, tt := range tests {
		driver, version := parseDBURL(tt.url)
		if driver != tt.wantDriver || version != tt.wantVersion {
			t.Errorf("parseDBURL(%q) = (%q, %q), want (%q, %q)", tt.url, driver, version, tt.wantDriver, tt.wantVersion)
		}
	}
}

func TestScan_InferredExtensionsAnnounced(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "composer.json", `{
		"require": {
			"php": ">=8.3",
			"symfony/framework-bundle": "^7.1",
			"liip/imagine-bundle": "^2.12",
			"phpoffice/phpspreadsheet": "^2.0"
		}
	}`)

	result, err := New(dir).Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	for _, want := range []string{"gd", "zip"} {
		found := false
		for _, ext := range result.PHPExtensions {
			if ext == want {
				found = true
			}
		}
		if !found {
			t.Errorf("expected inferred extension %q, got %v", want, result.PHPExtensions)
		}
	}
	warned := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "gd") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("inferred extensions must be announced, warnings: %v", result.Warnings)
	}
}

func TestScan_MessengerTransportsFromYaml(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "composer.json", minimalSymfonyComposer)
	writeFile(t, dir, ".env", "MESSENGER_TRANSPORT_DSN=doctrine://default\n")
	writeFile(t, dir, "config/packages/messenger.yaml", `
framework:
    messenger:
        failure_transport: failed
        transports:
            async: '%env(MESSENGER_TRANSPORT_DSN)%'
            failed: 'doctrine://default?queue_name=failed'
            sync_bus: 'sync://'
        routing:
            App\Message\Foo: async
`)

	s := New(dir)
	result, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	cfg := s.ToProjectConfig(result, "myapp")
	if len(cfg.Messenger.Transports) != 1 || cfg.Messenger.Transports[0] != "async" {
		t.Errorf("expected only [async] (no failed, no sync), got %v", cfg.Messenger.Transports)
	}
	// doctrine transport: no amqp extension
	for _, ext := range result.PHPExtensions {
		if ext == "amqp" {
			t.Errorf("amqp extension must not be added for a doctrine transport, got %v", result.PHPExtensions)
		}
	}
	// pcntl for graceful worker shutdown
	foundPcntl := false
	for _, ext := range result.PHPExtensions {
		if ext == "pcntl" {
			foundPcntl = true
		}
	}
	if !foundPcntl {
		t.Errorf("expected pcntl for messenger graceful shutdown, got %v", result.PHPExtensions)
	}
}

func TestScan_MessengerAMQPTransport(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "composer.json", minimalSymfonyComposer)
	writeFile(t, dir, ".env", "MESSENGER_TRANSPORT_DSN=amqp://guest:guest@rabbitmq:5672/%2f/messages\n")
	writeFile(t, dir, "config/packages/messenger.yaml", `
framework:
    messenger:
        transports:
            async: '%env(MESSENGER_TRANSPORT_DSN)%'
`)

	result, err := New(dir).Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	found := false
	for _, ext := range result.PHPExtensions {
		if ext == "amqp" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected amqp extension for amqp transport, got %v", result.PHPExtensions)
	}
}

func TestScan_SchedulerAddsTransport(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "composer.json", `{
		"require": {
			"php": ">=8.3",
			"symfony/framework-bundle": "^7.1",
			"symfony/scheduler": "^7.1"
		}
	}`)

	s := New(dir)
	result, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if !result.HasScheduler {
		t.Error("expected HasScheduler")
	}
	cfg := s.ToProjectConfig(result, "myapp")
	if !cfg.Messenger.Enabled {
		t.Error("scheduler requires a running messenger worker")
	}
	found := false
	for _, tr := range cfg.Messenger.Transports {
		if tr == "scheduler_default" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected scheduler_default transport, got %v", cfg.Messenger.Transports)
	}
}

func TestDetectAssets_ViteWithPnpm(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "composer.json", minimalSymfonyComposer)
	writeFile(t, dir, "package.json", `{"scripts": {"build": "vite build"}, "devDependencies": {"vite": "^5.0"}}`)
	writeFile(t, dir, "pnpm-lock.yaml", "lockfileVersion: 9\n")

	assets, err := New(dir).DetectAssets()
	if err != nil {
		t.Fatalf("DetectAssets: %v", err)
	}
	if assets.BuildTool != "pnpm" {
		t.Errorf("BuildTool must follow the lockfile (pnpm), got %q", assets.BuildTool)
	}
	if !strings.HasPrefix(assets.BuildCommand, "pnpm ") {
		t.Errorf("BuildCommand must use pnpm, got %q", assets.BuildCommand)
	}
}

func TestDetectAssets_PentatrionViteBundle(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "composer.json", `{
		"require": {
			"php": ">=8.3",
			"symfony/framework-bundle": "^7.1",
			"pentatrion/vite-bundle": "^8.0"
		}
	}`)
	writeFile(t, dir, "package.json", `{"scripts": {"build": "vite build"}}`)

	assets, err := New(dir).DetectAssets()
	if err != nil {
		t.Fatalf("DetectAssets: %v", err)
	}
	if assets.BuildTool == "" || assets.BuildTool == "assetmapper" {
		t.Errorf("pentatrion/vite-bundle must trigger a JS build, got %q", assets.BuildTool)
	}
}

func TestDetectAssets_TailwindBundle(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "composer.json", `{
		"require": {
			"php": ">=8.3",
			"symfony/framework-bundle": "^7.1",
			"symfonycasts/tailwind-bundle": "^0.6"
		}
	}`)
	writeFile(t, dir, "importmap.php", "<?php return [];\n")

	assets, err := New(dir).DetectAssets()
	if err != nil {
		t.Fatalf("DetectAssets: %v", err)
	}
	if assets.BuildTool != "assetmapper" {
		t.Fatalf("expected assetmapper, got %q", assets.BuildTool)
	}
	if !assets.Tailwind {
		t.Error("expected Tailwind to be detected (site would deploy without CSS)")
	}
}

func TestIsSymfonyProject_RequiresFrameworkBundle(t *testing.T) {
	dir := t.TempDir()
	// A Laravel project pulls symfony/console transitively
	writeFile(t, dir, "composer.json", `{
		"require": {
			"php": ">=8.3",
			"laravel/framework": "^11.0",
			"symfony/console": "^7.0"
		}
	}`)

	if New(dir).IsSymfonyProject() {
		t.Error("a project without symfony/framework-bundle must not be detected as Symfony")
	}
}
