package caddy

import (
	"strings"
	"testing"
)

func TestGenerateAppConfig_DoesNotEmitTLSInternal(t *testing.T) {
	// The `tls internal` branch was unreachable since AppConfigFromProject
	// always left TLS as the default. The field and branch were removed —
	// this test guards against accidental reintroduction.
	gen := NewConfigGenerator()
	out, err := gen.GenerateAppConfig(AppConfig{
		Name:   "myapp",
		Domain: "example.com",
		Port:   8080,
	})
	if err != nil {
		t.Fatalf("GenerateAppConfig: %v", err)
	}
	if strings.Contains(out, "tls internal") {
		t.Errorf("generated config should not contain 'tls internal':\n%s", out)
	}
	wantFragments := []string{
		"example.com {",
		"reverse_proxy myapp:8080",
		"encode zstd gzip",
	}
	for _, want := range wantFragments {
		if !strings.Contains(out, want) {
			t.Errorf("generated config missing %q\n%s", want, out)
		}
	}
}
