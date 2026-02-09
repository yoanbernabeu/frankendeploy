package config

// ProjectConfig represents the frankendeploy.yaml configuration
type ProjectConfig struct {
	Name              string           `yaml:"name"`
	FrankenPHPVersion string           `yaml:"frankenphp_version,omitempty"`
	PHP               PHPConfig        `yaml:"php"`
	Database          DatabaseConfig   `yaml:"database,omitempty"`
	Assets            AssetsConfig     `yaml:"assets,omitempty"`
	Messenger         MessengerConfig  `yaml:"messenger,omitempty"`
	Mailer            MailerConfig     `yaml:"mailer,omitempty"`
	Dockerfile        DockerfileConfig `yaml:"dockerfile,omitempty"`
	Deploy            DeployConfig     `yaml:"deploy,omitempty"`
	Env               EnvConfig        `yaml:"env,omitempty"`
}

// PHPConfig holds PHP-specific configuration
type PHPConfig struct {
	Version    string   `yaml:"version"`
	Extensions []string `yaml:"extensions,omitempty"`
	IniValues  []string `yaml:"ini_values,omitempty"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Driver  string `yaml:"driver,omitempty"`
	Version string `yaml:"version,omitempty"`
	Host    string `yaml:"host,omitempty"`
	Port    int    `yaml:"port,omitempty"`
	Name    string `yaml:"name,omitempty"`
	// Path: file path for SQLite database (relative to project root)
	Path string `yaml:"path,omitempty"`
	// Managed: if true, FrankenDeploy creates a DB container in production
	// If false, expects external DATABASE_URL in .env.local
	// Note: SQLite does not support managed mode (file-based database)
	Managed *bool `yaml:"managed,omitempty"`
}

// AssetsConfig holds asset build configuration
type AssetsConfig struct {
	BuildTool    string `yaml:"build_tool,omitempty"`
	BuildCommand string `yaml:"build_command,omitempty"`
	OutputDir    string `yaml:"output_dir,omitempty"`
}

// MessengerConfig holds Symfony Messenger worker configuration
type MessengerConfig struct {
	Enabled    bool     `yaml:"enabled,omitempty"`
	Workers    int      `yaml:"workers,omitempty"`
	Transports []string `yaml:"transports,omitempty"`
}

// MailerConfig holds Symfony Mailer configuration
type MailerConfig struct {
	Enabled bool `yaml:"enabled,omitempty"`
}

// DockerfileConfig holds Dockerfile customization options
type DockerfileConfig struct {
	ExtraPackages []string `yaml:"extra_packages,omitempty"`
	ExtraCommands []string `yaml:"extra_commands,omitempty"`
}

// DeployConfig holds deployment configuration
type DeployConfig struct {
	Domain          string   `yaml:"domain,omitempty"`
	HealthcheckPath string   `yaml:"healthcheck_path,omitempty"`
	HealthcheckHost string   `yaml:"healthcheck_host,omitempty"`
	KeepReleases    int      `yaml:"keep_releases,omitempty"`
	SharedFiles     []string `yaml:"shared_files,omitempty"`
	SharedDirs      []string `yaml:"shared_dirs,omitempty"`
	Hooks           Hooks    `yaml:"hooks,omitempty"`
}

// Hooks holds deployment hook commands
type Hooks struct {
	PreDeploy  []string `yaml:"pre_deploy,omitempty"`
	PostDeploy []string `yaml:"post_deploy,omitempty"`
}

// EnvConfig holds environment variable configuration
type EnvConfig struct {
	Dev  map[string]string `yaml:"dev,omitempty"`
	Prod map[string]string `yaml:"prod,omitempty"`
}

// GlobalConfig represents the global ~/.config/frankendeploy/config.yaml
type GlobalConfig struct {
	Servers     map[string]ServerConfig `yaml:"servers"`
	DefaultUser string                  `yaml:"default_user,omitempty"`
	DefaultPort int                     `yaml:"default_port,omitempty"`
}

// ServerConfig represents a configured server
type ServerConfig struct {
	Name        string            `yaml:"name,omitempty"`
	Host        string            `yaml:"host"`
	User        string            `yaml:"user"`
	Port        int               `yaml:"port,omitempty"`
	KeyPath     string            `yaml:"key_path,omitempty"`
	Apps        map[string]string `yaml:"apps,omitempty"`
	RemoteBuild *bool             `yaml:"remote_build,omitempty"`
}

// AppConfig represents a deployed application on a server
type AppConfig struct {
	Name           string `yaml:"name"`
	Domain         string `yaml:"domain"`
	Path           string `yaml:"path"`
	CurrentRelease string `yaml:"current_release,omitempty"`
}

// ScanResult holds the result of project scanning
type ScanResult struct {
	IsSymfony     bool
	PHPVersion    string
	PHPExtensions []string
	Database      DatabaseConfig
	Assets        AssetsConfig
	HasDoctrine   bool
	HasMessenger  bool
	HasMailer     bool
	Framework     string
}

// DefaultProjectConfig returns a default project configuration
func DefaultProjectConfig() *ProjectConfig {
	return &ProjectConfig{
		PHP: PHPConfig{
			Version: "8.3",
			Extensions: []string{
				"intl",
				"opcache",
				"zip",
			},
		},
		Deploy: DeployConfig{
			HealthcheckPath: "/",
			KeepReleases:    5,
			SharedDirs:      []string{"var/log", "var/sessions"},
			SharedFiles:     []string{".env.local"},
		},
	}
}

// DefaultGlobalConfig returns a default global configuration
func DefaultGlobalConfig() *GlobalConfig {
	return &GlobalConfig{
		Servers:     make(map[string]ServerConfig),
		DefaultUser: "deploy",
		DefaultPort: 22,
	}
}
