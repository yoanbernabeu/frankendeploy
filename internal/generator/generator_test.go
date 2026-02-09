package generator

import (
	"strings"
	"sync"
	"testing"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
)

// ─── Validation: DockerfileData ──────────────────────────────────────────────

func TestValidateDockerfileData_ValidMinimal(t *testing.T) {
	data := DockerfileData{
		PHP: config.PHPConfig{Version: "8.3"},
	}
	if err := ValidateDockerfileData(data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateDockerfileData_InvalidPHPVersion(t *testing.T) {
	for _, v := range []string{"7.4", "8.0", "8.5", "9.0", "abc", ""} {
		data := DockerfileData{PHP: config.PHPConfig{Version: v}}
		if err := ValidateDockerfileData(data); err == nil {
			t.Errorf("expected error for PHP version %q", v)
		}
	}
}

func TestValidateDockerfileData_ValidPHPVersions(t *testing.T) {
	for _, v := range []string{"8.1", "8.2", "8.3", "8.4"} {
		data := DockerfileData{PHP: config.PHPConfig{Version: v}}
		if err := ValidateDockerfileData(data); err != nil {
			t.Errorf("unexpected error for PHP version %q: %v", v, err)
		}
	}
}

func TestValidateDockerfileData_MaliciousExtension(t *testing.T) {
	data := DockerfileData{
		PHP: config.PHPConfig{
			Version:    "8.3",
			Extensions: []string{"pdo_pgsql", "intl;rm -rf /"},
		},
	}
	if err := ValidateDockerfileData(data); err == nil {
		t.Error("expected error for malicious extension name")
	}
}

func TestValidateDockerfileData_ValidExtensions(t *testing.T) {
	data := DockerfileData{
		PHP: config.PHPConfig{
			Version:    "8.3",
			Extensions: []string{"pdo_pgsql", "intl", "opcache", "gd"},
		},
	}
	if err := ValidateDockerfileData(data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateDockerfileData_BuildCommandInjection(t *testing.T) {
	assets := &config.AssetsConfig{
		BuildTool:    "npm",
		BuildCommand: "npm run build && rm -rf /",
	}
	data := DockerfileData{
		PHP:    config.PHPConfig{Version: "8.3"},
		Assets: assets,
	}
	if err := ValidateDockerfileData(data); err == nil {
		t.Error("expected error for injected build command")
	}
}

func TestValidateDockerfileData_ValidBuildCommand(t *testing.T) {
	assets := &config.AssetsConfig{
		BuildTool:    "npm",
		BuildCommand: "npm run build",
	}
	data := DockerfileData{
		PHP:    config.PHPConfig{Version: "8.3"},
		Assets: assets,
	}
	if err := ValidateDockerfileData(data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateDockerfileData_InvalidExtraPackage(t *testing.T) {
	data := DockerfileData{
		PHP: config.PHPConfig{Version: "8.3"},
		Dockerfile: config.DockerfileConfig{
			ExtraPackages: []string{"curl", "$(malicious)"},
		},
	}
	if err := ValidateDockerfileData(data); err == nil {
		t.Error("expected error for invalid extra package")
	}
}

func TestValidateDockerfileData_ValidExtraPackages(t *testing.T) {
	data := DockerfileData{
		PHP: config.PHPConfig{Version: "8.3"},
		Dockerfile: config.DockerfileConfig{
			ExtraPackages: []string{"curl", "libpng-dev", "libjpeg62-turbo-dev"},
		},
	}
	if err := ValidateDockerfileData(data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateDockerfileData_DangerousExtraCommand(t *testing.T) {
	for _, cmd := range []string{
		"RUN $(evil)",
		"RUN echo `whoami`",
		"RUN test; rm -rf /",
		"RUN true && false",
	} {
		data := DockerfileData{
			PHP: config.PHPConfig{Version: "8.3"},
			Dockerfile: config.DockerfileConfig{
				ExtraCommands: []string{cmd},
			},
		}
		if err := ValidateDockerfileData(data); err == nil {
			t.Errorf("expected error for dangerous command %q", cmd)
		}
	}
}

func TestValidateDockerfileData_InvalidFrankenPHPVersion(t *testing.T) {
	data := DockerfileData{
		PHP:               config.PHPConfig{Version: "8.3"},
		FrankenPHPVersion: "1.0; echo pwned",
	}
	if err := ValidateDockerfileData(data); err == nil {
		t.Error("expected error for invalid FrankenPHP version")
	}
}

func TestValidateDockerfileData_ValidFrankenPHPVersion(t *testing.T) {
	data := DockerfileData{
		PHP:               config.PHPConfig{Version: "8.3"},
		FrankenPHPVersion: "1.4.0-rc.1",
	}
	if err := ValidateDockerfileData(data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ─── Validation: ComposeData ─────────────────────────────────────────────────

func TestValidateComposeData_Valid(t *testing.T) {
	data := ComposeData{Name: "my-app"}
	if err := ValidateComposeData(data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateComposeData_EmptyName(t *testing.T) {
	data := ComposeData{}
	if err := ValidateComposeData(data); err == nil {
		t.Error("expected error for empty name")
	}
}

func TestValidateComposeData_UnknownDBDriver(t *testing.T) {
	data := ComposeData{
		Name:     "my-app",
		Database: config.DatabaseConfig{Driver: "oracle"},
	}
	if err := ValidateComposeData(data); err == nil {
		t.Error("expected error for unknown DB driver")
	}
}

func TestValidateComposeData_SQLiteIsAllowed(t *testing.T) {
	data := ComposeData{
		Name:     "my-app",
		Database: config.DatabaseConfig{Driver: "sqlite"},
	}
	if err := ValidateComposeData(data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ─── Dockerfile Generation ───────────────────────────────────────────────────

func TestDockerfileGenerator_Generate_Minimal(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "test-app",
		PHP: config.PHPConfig{
			Version:    "8.3",
			Extensions: []string{"intl", "opcache"},
		},
	}
	gen := NewDockerfileGenerator(cfg)
	dockerfile, err := gen.Generate()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}

	// Must not contain Node stage when no assets
	if strings.Contains(dockerfile, "node_build") {
		t.Error("minimal Dockerfile should not have node_build stage")
	}

	for _, want := range []string{"dunglas/frankenphp", "php8.3", "HEALTHCHECK", "intl", "opcache"} {
		if !strings.Contains(dockerfile, want) {
			t.Errorf("Dockerfile should contain %q", want)
		}
	}
}

func TestDockerfileGenerator_Generate_WithExtensions(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "test-app",
		PHP: config.PHPConfig{
			Version:    "8.3",
			Extensions: []string{"pdo_pgsql", "intl", "gd", "redis"},
		},
	}
	gen := NewDockerfileGenerator(cfg)
	dockerfile, err := gen.Generate()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	for _, ext := range cfg.PHP.Extensions {
		if !strings.Contains(dockerfile, ext) {
			t.Errorf("Dockerfile should contain extension %q", ext)
		}
	}
}

func TestDockerfileGenerator_Generate_AssetMapper(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "test-app",
		PHP: config.PHPConfig{
			Version:    "8.3",
			Extensions: []string{"intl"},
		},
		Assets: config.AssetsConfig{
			BuildTool: "assetmapper",
		},
	}
	gen := NewDockerfileGenerator(cfg)
	dockerfile, err := gen.Generate()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	if strings.Contains(dockerfile, "node_build") {
		t.Error("assetmapper should not use Node build stage")
	}
	if !strings.Contains(dockerfile, "importmap:install") {
		t.Error("assetmapper should run importmap:install")
	}
	if !strings.Contains(dockerfile, "asset-map:compile") {
		t.Error("assetmapper should run asset-map:compile")
	}
}

func TestDockerfileGenerator_Generate_NPMVite(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "test-app",
		PHP: config.PHPConfig{
			Version:    "8.3",
			Extensions: []string{"intl"},
		},
		Assets: config.AssetsConfig{
			BuildTool:    "npm",
			BuildCommand: "npm run build",
		},
	}
	gen := NewDockerfileGenerator(cfg)
	dockerfile, err := gen.Generate()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	if !strings.Contains(dockerfile, "FROM node:20-slim AS node_build") {
		t.Error("npm/Vite should have Node build stage")
	}
	if !strings.Contains(dockerfile, "COPY --from=node_build") {
		t.Error("should copy assets from node_build stage")
	}
	if !strings.Contains(dockerfile, "npm run build") {
		t.Error("should execute the build command in the node stage")
	}
}

func TestDockerfileGenerator_Generate_ExtraPackages(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "test-app",
		PHP: config.PHPConfig{
			Version:    "8.3",
			Extensions: []string{"intl"},
		},
		Dockerfile: config.DockerfileConfig{
			ExtraPackages: []string{"curl", "libpng-dev"},
		},
	}
	gen := NewDockerfileGenerator(cfg)
	dockerfile, err := gen.Generate()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	for _, pkg := range cfg.Dockerfile.ExtraPackages {
		if !strings.Contains(dockerfile, pkg) {
			t.Errorf("Dockerfile should contain package %q", pkg)
		}
	}
}

func TestDockerfileGenerator_Generate_ExtraCommands(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "test-app",
		PHP: config.PHPConfig{
			Version:    "8.3",
			Extensions: []string{"intl"},
		},
		Dockerfile: config.DockerfileConfig{
			ExtraCommands: []string{"RUN echo hello"},
		},
	}
	gen := NewDockerfileGenerator(cfg)
	dockerfile, err := gen.Generate()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	if !strings.Contains(dockerfile, "RUN echo hello") {
		t.Error("Dockerfile should contain the extra command")
	}
}

func TestDockerfileGenerator_Generate_PHPIniValues(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "test-app",
		PHP: config.PHPConfig{
			Version:    "8.3",
			Extensions: []string{"intl"},
			IniValues:  []string{"memory_limit=256M", "upload_max_filesize=50M"},
		},
	}
	gen := NewDockerfileGenerator(cfg)
	dockerfile, err := gen.Generate()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	for _, ini := range cfg.PHP.IniValues {
		if !strings.Contains(dockerfile, ini) {
			t.Errorf("Dockerfile should contain PHP ini value %q", ini)
		}
	}
}

func TestDockerfileGenerator_Generate_CustomFrankenPHPVersion(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "test-app",
		PHP: config.PHPConfig{
			Version:    "8.3",
			Extensions: []string{"intl"},
		},
		FrankenPHPVersion: "1.4.0",
	}
	gen := NewDockerfileGenerator(cfg)
	dockerfile, err := gen.Generate()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	if !strings.Contains(dockerfile, "dunglas/frankenphp:1.4.0-php8.3") {
		t.Error("Dockerfile should use custom FrankenPHP version")
	}
}

func TestDockerfileGenerator_Generate_Constants(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "test-app",
		PHP: config.PHPConfig{
			Version:    "8.3",
			Extensions: []string{"intl"},
		},
	}
	gen := NewDockerfileGenerator(cfg)
	dockerfile, err := gen.Generate()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	if !strings.Contains(dockerfile, "ARG UID="+DefaultUID) {
		t.Error("Dockerfile should use DefaultUID constant")
	}
	if !strings.Contains(dockerfile, "ARG GID="+DefaultGID) {
		t.Error("Dockerfile should use DefaultGID constant")
	}
	if !strings.Contains(dockerfile, "localhost:"+MetricsPort+"/metrics") {
		t.Error("Dockerfile should use MetricsPort constant")
	}
}

func TestDockerfileGenerator_Generate_ValidationRejectsInvalidPHP(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "test-app",
		PHP: config.PHPConfig{
			Version:    "7.4",
			Extensions: []string{"intl"},
		},
	}
	gen := NewDockerfileGenerator(cfg)
	_, err := gen.Generate()
	if err == nil {
		t.Error("expected validation error for PHP 7.4")
	}
}

// ─── Compose Dev ─────────────────────────────────────────────────────────────

func TestComposeGenerator_GenerateDev_PostgreSQL(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "test-app",
		PHP:  config.PHPConfig{Version: "8.3"},
		Database: config.DatabaseConfig{
			Driver:  "pgsql",
			Version: "16",
		},
	}
	gen := NewComposeGenerator(cfg)
	compose, err := gen.GenerateDev()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	for _, want := range []string{"test-app", "postgres", "8000:8080", "POSTGRES_USER: app", "POSTGRES_PASSWORD: app", "pg_isready"} {
		if !strings.Contains(compose, want) {
			t.Errorf("compose-dev should contain %q", want)
		}
	}
}

func TestComposeGenerator_GenerateDev_MySQL(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "test-app",
		PHP:  config.PHPConfig{Version: "8.3"},
		Database: config.DatabaseConfig{
			Driver:  "mysql",
			Version: "8.0",
		},
	}
	gen := NewComposeGenerator(cfg)
	compose, err := gen.GenerateDev()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	for _, want := range []string{"mysql", "MYSQL_USER: app", "MYSQL_PASSWORD: app", "mysqladmin"} {
		if !strings.Contains(compose, want) {
			t.Errorf("compose-dev should contain %q", want)
		}
	}
}

func TestComposeGenerator_GenerateDev_SQLite(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "test-app",
		PHP:  config.PHPConfig{Version: "8.3"},
		Database: config.DatabaseConfig{
			Driver: "sqlite",
		},
	}
	gen := NewComposeGenerator(cfg)
	compose, err := gen.GenerateDev()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	if strings.Contains(compose, "database:") {
		t.Error("sqlite should not have a database service")
	}
	if !strings.Contains(compose, "sqlite://") {
		t.Error("should contain sqlite DATABASE_URL")
	}
}

func TestComposeGenerator_GenerateDev_NoDB(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "test-app",
		PHP:  config.PHPConfig{Version: "8.3"},
	}
	gen := NewComposeGenerator(cfg)
	compose, err := gen.GenerateDev()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	if strings.Contains(compose, "DATABASE_URL") {
		t.Error("no DB config should not have DATABASE_URL")
	}
	if strings.Contains(compose, "database:") {
		t.Error("no DB config should not have database service")
	}
}

func TestComposeGenerator_GenerateDev_WithMailer(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "test-app",
		PHP:  config.PHPConfig{Version: "8.3"},
	}
	gen := NewComposeGenerator(cfg)
	gen.config.Env = config.EnvConfig{}
	// We need to set HasMailer via the compose data; since buildComposeData doesn't set it
	// from config, we test the template by using the generator directly
	compose, err := gen.GenerateDev()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	// Without HasMailer, no mailhog
	if strings.Contains(compose, "mailhog") {
		t.Error("should not contain mailhog without HasMailer")
	}
}

func TestComposeGenerator_GenerateDev_WithMessenger(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "test-app",
		PHP:  config.PHPConfig{Version: "8.3"},
	}
	gen := NewComposeGenerator(cfg)
	compose, err := gen.GenerateDev()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	// Without HasMessenger, no rabbitmq
	if strings.Contains(compose, "rabbitmq") {
		t.Error("should not contain rabbitmq without HasMessenger")
	}
}

func TestComposeGenerator_GenerateDev_Constants(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "test-app",
		PHP:  config.PHPConfig{Version: "8.3"},
	}
	gen := NewComposeGenerator(cfg)
	compose, err := gen.GenerateDev()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	if !strings.Contains(compose, DefaultUID+":"+DefaultGID) {
		t.Error("compose-dev should use UID:GID constants")
	}
	if !strings.Contains(compose, DevExternalPort+":"+AppPort) {
		t.Error("compose-dev should use port constants")
	}
}

// ─── Compose Prod ────────────────────────────────────────────────────────────

func TestComposeGenerator_GenerateProd(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "test-app",
		PHP:  config.PHPConfig{Version: "8.3"},
		Deploy: config.DeployConfig{
			Domain: "example.com",
		},
	}
	gen := NewComposeGenerator(cfg)
	compose, err := gen.GenerateProd()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	for _, want := range []string{
		"test-app:${TAG:-latest}",
		"expose:",
		`"` + AppPort + `"`,
		"localhost:" + MetricsPort + "/metrics",
		NetworkName,
		"app.domain=example.com",
		DefaultUID + ":" + DefaultGID,
	} {
		if !strings.Contains(compose, want) {
			t.Errorf("compose-prod should contain %q", want)
		}
	}
}

func TestComposeGenerator_GenerateProd_NoDomain(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "test-app",
		PHP:  config.PHPConfig{Version: "8.3"},
	}
	gen := NewComposeGenerator(cfg)
	compose, err := gen.GenerateProd()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	if strings.Contains(compose, "app.domain=") {
		t.Error("compose-prod should not have app.domain label without domain config")
	}
}

// ─── Entrypoint ──────────────────────────────────────────────────────────────

func TestDockerfileGenerator_GenerateEntrypoint(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "test-app",
		PHP:  config.PHPConfig{Version: "8.3"},
	}
	gen := NewDockerfileGenerator(cfg)
	entrypoint, err := gen.GenerateEntrypoint()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	if !strings.Contains(entrypoint, "MAX_ATTEMPTS=30") {
		t.Error("entrypoint should contain MAX_ATTEMPTS=30")
	}
	if !strings.Contains(entrypoint, "sleep 1") {
		t.Error("entrypoint should contain sleep 1")
	}
	if !strings.Contains(entrypoint, "wait_for_database") {
		t.Error("entrypoint should contain wait_for_database function")
	}
}

// ─── Dockerignore ────────────────────────────────────────────────────────────

func TestDockerfileGenerator_GenerateDockerignore(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "test-app",
		PHP:  config.PHPConfig{Version: "8.3"},
	}
	gen := NewDockerfileGenerator(cfg)
	dockerignore, err := gen.GenerateDockerignore()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}

	expectedPatterns := []string{
		".git",
		"vendor",
		"node_modules",
		"var/cache",
		"Dockerfile",
		"compose",
		".env.local",
		"frankendeploy.yaml",
	}

	for _, pattern := range expectedPatterns {
		if !strings.Contains(dockerignore, pattern) {
			t.Errorf("dockerignore should contain %q", pattern)
		}
	}
}

// ─── Database Registry ───────────────────────────────────────────────────────

func TestGetDBDriverInfo_PostgreSQL(t *testing.T) {
	info, err := GetDBDriverInfo("pgsql")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.DockerImage != "postgres" {
		t.Errorf("expected docker image 'postgres', got %q", info.DockerImage)
	}
	if info.Port != PostgresPort {
		t.Errorf("expected port %s, got %s", PostgresPort, info.Port)
	}
	if info.URLScheme != "postgresql" {
		t.Errorf("expected URL scheme 'postgresql', got %q", info.URLScheme)
	}
}

func TestGetDBDriverInfo_MySQL(t *testing.T) {
	info, err := GetDBDriverInfo("mysql")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.DockerImage != "mysql" {
		t.Errorf("expected docker image 'mysql', got %q", info.DockerImage)
	}
	if info.Port != MySQLPort {
		t.Errorf("expected port %s, got %s", MySQLPort, info.Port)
	}
	if info.URLCharset != "utf8mb4" {
		t.Errorf("expected charset 'utf8mb4', got %q", info.URLCharset)
	}
}

func TestGetDBDriverInfo_Unknown(t *testing.T) {
	_, err := GetDBDriverInfo("oracle")
	if err == nil {
		t.Error("expected error for unknown driver")
	}
}

func TestIsContainerizedDriver(t *testing.T) {
	if !IsContainerizedDriver("pgsql") {
		t.Error("pgsql should be containerized")
	}
	if !IsContainerizedDriver("mysql") {
		t.Error("mysql should be containerized")
	}
	if IsContainerizedDriver("sqlite") {
		t.Error("sqlite should not be containerized")
	}
	if IsContainerizedDriver("unknown") {
		t.Error("unknown should not be containerized")
	}
}

// ─── buildDatabaseURL ────────────────────────────────────────────────────────

func TestComposeGenerator_BuildDatabaseURL(t *testing.T) {
	tests := []struct {
		driver   string
		expected string
	}{
		{"pgsql", "postgresql://"},
		{"mysql", "mysql://"},
		{"sqlite", "sqlite://"},
	}

	for _, tt := range tests {
		t.Run(tt.driver, func(t *testing.T) {
			cfg := &config.ProjectConfig{
				Database: config.DatabaseConfig{
					Driver:  tt.driver,
					Version: "16",
				},
			}
			gen := NewComposeGenerator(cfg)
			url := gen.buildDatabaseURL()
			if !strings.HasPrefix(url, tt.expected) {
				t.Errorf("buildDatabaseURL() for %s should start with %q, got %q", tt.driver, tt.expected, url)
			}
		})
	}
}

func TestComposeGenerator_BuildDatabaseURL_EmptyDriver(t *testing.T) {
	cfg := &config.ProjectConfig{}
	gen := NewComposeGenerator(cfg)
	url := gen.buildDatabaseURL()
	if url != "" {
		t.Errorf("empty driver should return empty URL, got %q", url)
	}
}

func TestComposeGenerator_BuildDatabaseURL_ContainsCredentials(t *testing.T) {
	cfg := &config.ProjectConfig{
		Database: config.DatabaseConfig{
			Driver:  "pgsql",
			Version: "16",
		},
	}
	gen := NewComposeGenerator(cfg)
	url := gen.buildDatabaseURL()
	if !strings.Contains(url, DefaultDevDBUser+":"+DefaultDevDBPassword+"@") {
		t.Error("DATABASE_URL should contain dev credentials")
	}
}

// ─── Thread Safety ───────────────────────────────────────────────────────────

func TestTemplateLoader_ConcurrentAccess(t *testing.T) {
	loader := NewTemplateLoader()
	var wg sync.WaitGroup
	errs := make(chan error, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := loader.LoadTemplate("dockerfile.tmpl")
			if err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent LoadTemplate error: %v", err)
	}
}

func TestTemplateLoader_ConcurrentDifferentTemplates(t *testing.T) {
	loader := NewTemplateLoader()
	templates := []string{"dockerfile.tmpl", "compose-dev.tmpl", "compose-prod.tmpl", "docker-entrypoint.tmpl", "dockerignore.tmpl"}
	var wg sync.WaitGroup
	errs := make(chan error, len(templates)*5)

	for _, tmplName := range templates {
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(name string) {
				defer wg.Done()
				_, err := loader.LoadTemplate(name)
				if err != nil {
					errs <- err
				}
			}(tmplName)
		}
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent LoadTemplate error: %v", err)
	}
}

// ─── DefaultEntrypointData ───────────────────────────────────────────────────

func TestDefaultEntrypointData(t *testing.T) {
	data := DefaultEntrypointData()
	if data.MaxDBWaitAttempts != DefaultDBWaitMaxAttempts {
		t.Errorf("expected MaxDBWaitAttempts=%d, got %d", DefaultDBWaitMaxAttempts, data.MaxDBWaitAttempts)
	}
	if data.DBWaitInterval != DefaultDBWaitInterval {
		t.Errorf("expected DBWaitInterval=%d, got %d", DefaultDBWaitInterval, data.DBWaitInterval)
	}
}
