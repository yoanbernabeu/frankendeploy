package scanner

import (
	"testing"
)

func TestScanner_HasMongoDB(t *testing.T) {
	tests := []struct {
		name     string
		composer string
		env      string
		want     bool
	}{
		{
			name:     "mongodb library",
			composer: `{"require":{"mongodb/mongodb":"^1.0"}}`,
			want:     true,
		},
		{
			name:     "doctrine mongodb odm",
			composer: `{"require":{"doctrine/mongodb-odm":"^1.0"}}`,
			want:     true,
		},
		{
			name:     "doctrine mongodb odm bundle",
			composer: `{"require":{"doctrine/mongodb-odm-bundle":"^5.0"}}`,
			want:     true,
		},
		{
			name:     "laravel mongodb",
			composer: `{"require":{"mongodb/laravel-mongodb":"^5.0"}}`,
			want:     true,
		},
		{
			name:     "ext-mongodb extension in require",
			composer: `{"require":{"ext-mongodb":"*"}}`,
			want:     true,
		},
		{
			name:     "ext-mongodb extension in platform",
			composer: `{"require":{"php":">=8.2"},"config":{"platform":{"ext-mongodb":"1.19.0"}}}`,
			want:     true,
		},
		{
			name: "MONGODB_URI in env",
			env:  "MONGODB_URI=mongodb://user:pass@127.0.0.1:27017/db\n",
			want: true,
		},
		{
			name:     "no mongodb",
			composer: `{"require":{"doctrine/orm":"^2.17"}}`,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.composer != "" {
				writeProjectFile(t, dir, "composer.json", tt.composer)
			}
			if tt.env != "" {
				writeProjectFile(t, dir, ".env", tt.env)
			}
			if got := New(dir).HasMongoDB(); got != tt.want {
				t.Errorf("HasMongoDB() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScanner_HasMongoDB_AddsExtensionAndEnabled(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "composer.json", `{"require":{"symfony/framework-bundle":"^7.0","mongodb/mongodb":"^1.0"}}`)

	result, err := New(dir).Scan()
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}
	if !result.HasMongoDB {
		t.Error("expected HasMongoDB to be true")
	}
	if !hasExtension(result.PHPExtensions, "mongodb") {
		t.Errorf("expected a mongodb extension, got %v", result.PHPExtensions)
	}

	cfg := New(dir).ToProjectConfig(result, "mongoapp")
	if !cfg.MongoDB.Enabled {
		t.Error("expected cfg.MongoDB.Enabled to be true")
	}
	if cfg.MongoDB.Managed {
		t.Error("expected cfg.MongoDB.Managed to be false by default (external MONGODB_URI)")
	}
}

func hasExtension(exts []string, want string) bool {
	for _, e := range exts {
		if e == want {
			return true
		}
	}
	return false
}
