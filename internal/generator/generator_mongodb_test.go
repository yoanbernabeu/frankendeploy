package generator

import (
	"strings"
	"testing"

	"github.com/yoanbernabeu/frankendeploy/internal/config"
)

func TestComposeGenerator_GenerateDev_MongoDB(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name:    "test-app",
		PHP:     config.PHPConfig{Version: "8.3"},
		MongoDB: config.MongoConfig{Enabled: true},
	}
	gen := NewComposeGenerator(cfg)
	compose, err := gen.GenerateDev()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	for _, want := range []string{
		"mongodb/mongodb-community-server",
		"27017:27017",
		"--replSet",
		"rs0",
		"MONGO_INITDB_ROOT_USERNAME: app",
		"mongodb_data",
		"mongodb-init",
		"mongosh",
	} {
		if !strings.Contains(compose, want) {
			t.Errorf("compose-dev should contain %q", want)
		}
	}
}

func TestComposeGenerator_GenerateDev_MongoDBDisabled(t *testing.T) {
	cfg := &config.ProjectConfig{
		Name: "test-app",
		PHP:  config.PHPConfig{Version: "8.3"},
	}
	gen := NewComposeGenerator(cfg)
	compose, err := gen.GenerateDev()
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	if strings.Contains(compose, "mongodb/mongodb-community-server") {
		t.Error("compose-dev should not render a mongodb service when disabled")
	}
}
