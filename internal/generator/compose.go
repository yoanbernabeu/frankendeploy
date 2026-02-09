package generator

import (
	"fmt"
	"os"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
)

// ComposeGenerator generates docker-compose files
type ComposeGenerator struct {
	loader *TemplateLoader
	config *config.ProjectConfig
}

// NewComposeGenerator creates a new compose generator
func NewComposeGenerator(cfg *config.ProjectConfig) *ComposeGenerator {
	return &ComposeGenerator{
		loader: NewTemplateLoader(),
		config: cfg,
	}
}

// ComposeData holds data for docker-compose templates
type ComposeData struct {
	Name          string
	ImageName     string
	PHP           config.PHPConfig
	Database      config.DatabaseConfig
	DatabaseURL   string
	Assets        config.AssetsConfig
	Deploy        config.DeployConfig
	Env           config.EnvConfig
	HasMailer     bool
	HasMessenger  bool
	DevDBUser     string
	DevDBPassword string
	DevDBName     string
}

// GenerateDev generates docker-compose.yaml for development
func (g *ComposeGenerator) GenerateDev() (string, error) {
	data := g.buildComposeData()
	if err := ValidateComposeData(data); err != nil {
		return "", fmt.Errorf("validation failed: %w", err)
	}
	return g.loader.Execute("compose-dev.tmpl", data)
}

// GenerateProd generates docker-compose.prod.yaml for production
func (g *ComposeGenerator) GenerateProd() (string, error) {
	data := g.buildComposeData()
	if err := ValidateComposeData(data); err != nil {
		return "", fmt.Errorf("validation failed: %w", err)
	}
	return g.loader.Execute("compose-prod.tmpl", data)
}

// buildComposeData builds the data for compose templates
func (g *ComposeGenerator) buildComposeData() ComposeData {
	data := ComposeData{
		Name:          g.config.Name,
		ImageName:     g.config.Name,
		PHP:           g.config.PHP,
		Database:      g.config.Database,
		Assets:        g.config.Assets,
		Deploy:        g.config.Deploy,
		Env:           g.config.Env,
		DevDBUser:     DefaultDevDBUser,
		DevDBPassword: DefaultDevDBPassword,
		DevDBName:     DefaultDevDBName,
	}

	// Generate DATABASE_URL for dev
	data.DatabaseURL = g.buildDatabaseURL()

	return data
}

// buildDatabaseURL builds the database URL for docker-compose
func (g *ComposeGenerator) buildDatabaseURL() string {
	driver := g.config.Database.Driver

	if driver == "sqlite" {
		return "sqlite:///%kernel.project_dir%/var/data.db"
	}

	info, err := GetDBDriverInfo(driver)
	if err != nil {
		return ""
	}

	return fmt.Sprintf("%s://%s:%s@database:%s/%s?serverVersion=%s&charset=%s",
		info.URLScheme,
		DefaultDevDBUser, DefaultDevDBPassword,
		info.Port,
		DefaultDevDBName,
		g.config.Database.Version,
		info.URLCharset,
	)
}

// WriteDevCompose writes compose.yaml for development
func (g *ComposeGenerator) WriteDevCompose(path string) error {
	if path == "" {
		path = "compose.yaml"
	}

	content, err := g.GenerateDev()
	if err != nil {
		return fmt.Errorf("failed to generate compose.yaml: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write compose.yaml: %w", err)
	}

	return nil
}

// WriteProdCompose writes compose.prod.yaml for production
func (g *ComposeGenerator) WriteProdCompose(path string) error {
	if path == "" {
		path = "compose.prod.yaml"
	}

	content, err := g.GenerateProd()
	if err != nil {
		return fmt.Errorf("failed to generate compose.prod.yaml: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write compose.prod.yaml: %w", err)
	}

	return nil
}

// WriteAll writes all compose files
func (g *ComposeGenerator) WriteAll() error {
	if err := g.WriteDevCompose(""); err != nil {
		return err
	}
	return g.WriteProdCompose("")
}
