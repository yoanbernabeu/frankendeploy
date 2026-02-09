package generator

import (
	"fmt"
	"net/url"
	"os"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
)

// composeContext indicates whether we are generating a dev or prod compose file.
type composeContext int

const (
	composeContextDev composeContext = iota
	composeContextProd
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
	data, err := g.buildComposeData(composeContextDev)
	if err != nil {
		return "", fmt.Errorf("failed to build compose data: %w", err)
	}
	if err := ValidateComposeData(data); err != nil {
		return "", fmt.Errorf("validation failed: %w", err)
	}
	return g.loader.Execute("compose-dev.tmpl", data)
}

// GenerateProd generates docker-compose.prod.yaml for production
func (g *ComposeGenerator) GenerateProd() (string, error) {
	data, err := g.buildComposeData(composeContextProd)
	if err != nil {
		return "", fmt.Errorf("failed to build compose data: %w", err)
	}
	if err := ValidateComposeData(data); err != nil {
		return "", fmt.Errorf("validation failed: %w", err)
	}
	return g.loader.Execute("compose-prod.tmpl", data)
}

// buildComposeData builds the data for compose templates
func (g *ComposeGenerator) buildComposeData(ctx composeContext) (ComposeData, error) {
	data := ComposeData{
		Name:      g.config.Name,
		ImageName: g.config.Name,
		PHP:       g.config.PHP,
		Database:  g.config.Database,
		Assets:    g.config.Assets,
		Deploy:    g.config.Deploy,
		Env:       g.config.Env,
	}

	if ctx == composeContextDev {
		data.DevDBUser = DefaultDevDBUser
		data.DevDBPassword = DefaultDevDBPassword
		data.DevDBName = DefaultDevDBName
		data.HasMailer = g.config.Mailer.Enabled
		data.HasMessenger = g.config.Messenger.Enabled

		if g.config.Database.Driver != "" {
			dbURL, err := g.buildDatabaseURL()
			if err != nil {
				return ComposeData{}, err
			}
			data.DatabaseURL = dbURL
		}
	}

	return data, nil
}

// buildDatabaseURL builds the database URL for docker-compose
func (g *ComposeGenerator) buildDatabaseURL() (string, error) {
	driver := g.config.Database.Driver

	if driver == "" {
		return "", nil
	}

	if driver == "sqlite" {
		path := g.config.Database.Path
		if path == "" {
			path = "var/data.db"
		}
		return fmt.Sprintf("sqlite://%%kernel.project_dir%%/%s", path), nil
	}

	info, err := GetDBDriverInfo(driver)
	if err != nil {
		return "", fmt.Errorf("cannot build DATABASE_URL: %w", err)
	}

	host := g.config.Database.Host
	if host == "" {
		host = "database"
	}

	port := fmt.Sprintf("%d", g.config.Database.Port)
	if g.config.Database.Port == 0 {
		port = info.Port
	}

	dbName := g.config.Database.Name
	if dbName == "" {
		dbName = DefaultDevDBName
	}

	version := url.QueryEscape(g.config.Database.Version)

	return fmt.Sprintf("%s://%s:%s@%s:%s/%s?serverVersion=%s&charset=%s",
		info.URLScheme,
		DefaultDevDBUser, DefaultDevDBPassword,
		host, port,
		dbName,
		version,
		info.URLCharset,
	), nil
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
