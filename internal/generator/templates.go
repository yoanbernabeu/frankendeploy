package generator

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"sync"
	"text/template"
)

//go:embed templates/*
var templatesFS embed.FS

// TemplateLoader handles loading and executing embedded templates
type TemplateLoader struct {
	mu        sync.RWMutex
	templates map[string]*template.Template
}

// NewTemplateLoader creates a new template loader
func NewTemplateLoader() *TemplateLoader {
	return &TemplateLoader{
		templates: make(map[string]*template.Template),
	}
}

// LoadTemplate loads a template from the embedded filesystem
func (l *TemplateLoader) LoadTemplate(name string) (*template.Template, error) {
	// Fast path: read lock for cache hit
	l.mu.RLock()
	if tmpl, ok := l.templates[name]; ok {
		l.mu.RUnlock()
		return tmpl, nil
	}
	l.mu.RUnlock()

	// Slow path: write lock with double-check
	l.mu.Lock()
	defer l.mu.Unlock()

	if tmpl, ok := l.templates[name]; ok {
		return tmpl, nil
	}

	content, err := templatesFS.ReadFile("templates/" + name)
	if err != nil {
		return nil, fmt.Errorf("failed to load template %s: %w", name, err)
	}

	tmpl, err := template.New(name).Funcs(templateFuncs()).Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template %s: %w", name, err)
	}

	l.templates[name] = tmpl
	return tmpl, nil
}

// Execute executes a template with the given data
func (l *TemplateLoader) Execute(name string, data interface{}) (string, error) {
	tmpl, err := l.LoadTemplate(name)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", name, err)
	}

	return buf.String(), nil
}

// templateFuncs returns custom template functions
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"join": func(sep string, items []string) string {
			result := ""
			for i, item := range items {
				if i > 0 {
					result += sep
				}
				result += item
			}
			return result
		},
		"contains": func(slice []string, item string) bool {
			for _, s := range slice {
				if s == item {
					return true
				}
			}
			return false
		},
		"default": func(def, val interface{}) interface{} {
			if val == nil || val == "" {
				return def
			}
			return val
		},
		"yamlEscape": func(s string) string {
			replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
			return replacer.Replace(s)
		},
		"appPort":     func() string { return AppPort },
		"metricsPort": func() string { return MetricsPort },
		"devPort":     func() string { return DevExternalPort },
		"defaultUID":  func() string { return DefaultUID },
		"defaultGID":  func() string { return DefaultGID },
		"networkName": func() string { return NetworkName },
	}
}
