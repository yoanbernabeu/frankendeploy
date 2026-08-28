package ssh

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestIsExcluded(t *testing.T) {
	excludes := []string{".git", "node_modules", "vendor", "var", ".env.local"}
	tests := []struct {
		path string
		want bool
	}{
		{".git", true},
		{".git/config", true},
		{"node_modules/left-pad/index.js", true},
		{"vendor/autoload.php", true},
		{"var/cache/prod", true},
		{".env.local", true},
		{".env", false},
		{".env.local.dist", false}, // exact or dir-prefix only
		{"src/Kernel.php", false},
		{"public/vendor.css", false}, // "vendor" must not match mid-path
		{"assets/var.js", false},
		{"config/packages/framework.yaml", false},
	}
	for _, tt := range tests {
		if got := isExcluded(tt.path, excludes); got != tt.want {
			t.Errorf("isExcluded(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestCopyWithProgress_ReportsProgress(t *testing.T) {
	src := strings.NewReader(strings.Repeat("x", 600*1024)) // > 2 chunks
	var dst bytes.Buffer

	var calls []int64
	err := copyWithProgress(context.Background(), &dst, src, 600*1024, func(written, total int64) {
		calls = append(calls, written)
		if total != 600*1024 {
			t.Errorf("unexpected total %d", total)
		}
	})
	if err != nil {
		t.Fatalf("copyWithProgress: %v", err)
	}
	if dst.Len() != 600*1024 {
		t.Errorf("expected %d bytes copied, got %d", 600*1024, dst.Len())
	}
	if len(calls) < 2 {
		t.Errorf("expected progress callbacks per chunk, got %d", len(calls))
	}
	if calls[len(calls)-1] != 600*1024 {
		t.Errorf("last progress call must report the full size, got %d", calls[len(calls)-1])
	}
}

func TestCopyWithProgress_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	src := strings.NewReader("data")
	var dst bytes.Buffer
	err := copyWithProgress(ctx, &dst, src, 4, nil)
	if err == nil {
		t.Fatal("expected context error")
	}
	if dst.Len() != 0 {
		t.Errorf("nothing should be copied after cancellation, got %d bytes", dst.Len())
	}
}
