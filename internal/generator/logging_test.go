package generator

// Tests for issue #53: default log rotation, optional resource limits, and
// related compose fixes.

import (
	"strings"
	"testing"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
)

func TestComposeProd_LogRotation(t *testing.T) {
	compose, err := NewComposeGenerator(minimalConfig()).GenerateProd()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	for _, want := range []string{"logging:", "max-size:", "max-file:"} {
		if !strings.Contains(compose, want) {
			t.Errorf("compose-prod should set log rotation (%q missing):\n%s", want, compose)
		}
	}
}

func TestComposeProd_ResourceLimits(t *testing.T) {
	cfg := minimalConfig()
	cfg.Deploy.MemoryLimit = "512m"
	cfg.Deploy.CPULimit = "1.5"

	compose, err := NewComposeGenerator(cfg).GenerateProd()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	if !strings.Contains(compose, "mem_limit: 512m") {
		t.Errorf("compose-prod should apply deploy.memory_limit:\n%s", compose)
	}
	if !strings.Contains(compose, "cpus: 1.5") {
		t.Errorf("compose-prod should apply deploy.cpu_limit:\n%s", compose)
	}
}

func TestComposeProd_NoLimitsByDefault(t *testing.T) {
	compose, err := NewComposeGenerator(minimalConfig()).GenerateProd()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	if strings.Contains(compose, "mem_limit") || strings.Contains(compose, "cpus:") {
		t.Errorf("no resource limit expected by default:\n%s", compose)
	}
}

func TestComposeProd_MySQLRandomRootPassword(t *testing.T) {
	managed := true
	cfg := minimalConfig()
	cfg.Database = config.DatabaseConfig{Driver: "mysql", Version: "8.0", Managed: &managed}

	compose, err := NewComposeGenerator(cfg).GenerateProd()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	if !strings.Contains(compose, "MYSQL_RANDOM_ROOT_PASSWORD") {
		t.Errorf("mysql root password must be random, not the app password:\n%s", compose)
	}
	if strings.Contains(compose, "MYSQL_ROOT_PASSWORD") {
		t.Errorf("the app password must not be reused as the root password:\n%s", compose)
	}
}

func TestComposeDev_DBPortsBoundToLocalhost(t *testing.T) {
	for _, driver := range []string{"pgsql", "mysql"} {
		t.Run(driver, func(t *testing.T) {
			cfg := minimalConfig()
			cfg.Database = config.DatabaseConfig{Driver: driver, Version: "16"}
			if driver == "mysql" {
				cfg.Database.Version = "8.0"
			}

			compose, err := NewComposeGenerator(cfg).GenerateDev()
			if err != nil {
				t.Fatalf("failed to generate: %v", err)
			}
			if !strings.Contains(compose, `"127.0.0.1:`) {
				t.Errorf("dev database port must bind to 127.0.0.1, not every interface:\n%s", compose)
			}
		})
	}
}

func TestComposeDev_Mailpit(t *testing.T) {
	cfg := minimalConfig()
	cfg.Mailer.Enabled = true

	compose, err := NewComposeGenerator(cfg).GenerateDev()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	if strings.Contains(compose, "mailhog") {
		t.Error("mailhog is archived and has no arm64 image: use mailpit")
	}
	if !strings.Contains(compose, "axllent/mailpit") {
		t.Errorf("dev mailer should be axllent/mailpit:\n%s", compose)
	}
}

func TestComposeDev_RabbitMQ4(t *testing.T) {
	cfg := minimalConfig()
	cfg.Messenger.Enabled = true

	compose, err := NewComposeGenerator(cfg).GenerateDev()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	if strings.Contains(compose, "rabbitmq:3") {
		t.Error("rabbitmq:3 is EOL: use rabbitmq:4")
	}
	if !strings.Contains(compose, "rabbitmq:4") {
		t.Errorf("dev messenger should use rabbitmq:4:\n%s", compose)
	}
}

func TestComposeDev_ServerNameNeverProdDomain(t *testing.T) {
	cfg := minimalConfig()
	cfg.Deploy.Domain = "www.production-domain.com"

	compose, err := NewComposeGenerator(cfg).GenerateDev()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	if strings.Contains(compose, "production-domain.com") {
		t.Errorf("dev SERVER_NAME must never default to the production domain (it triggers real Let's Encrypt attempts locally):\n%s", compose)
	}
	if !strings.Contains(compose, "localhost") {
		t.Errorf("dev SERVER_NAME should default to localhost:\n%s", compose)
	}
}

func TestComposeDev_LogRotation(t *testing.T) {
	compose, err := NewComposeGenerator(minimalConfig()).GenerateDev()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	if !strings.Contains(compose, "max-size:") {
		t.Errorf("compose-dev should set log rotation too:\n%s", compose)
	}
}
