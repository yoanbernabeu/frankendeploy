package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra/doc"
	"github.com/yoanbernabeu/frankendeploy/internal/cmd"
)

func main() {
	outputDir := "./docs/src/content/docs/commands"

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Custom file prepender for Astro frontmatter
	filePrepender := func(filename string) string {
		name := filepath.Base(filename)
		name = strings.TrimSuffix(name, filepath.Ext(name))
		title := strings.ReplaceAll(name, "_", " ")
		return `---
title: "` + title + `"
---

`
	}

	// Custom link handler for internal links
	linkHandler := func(name string) string {
		base := strings.TrimSuffix(name, filepath.Ext(name))
		return "/frankendeploy/commands/" + strings.ToLower(base) + "/"
	}

	// Generate markdown documentation
	rootCmd := cmd.GetRootCmd()
	err := doc.GenMarkdownTreeCustom(rootCmd, outputDir, filePrepender, linkHandler)
	if err != nil {
		log.Fatalf("Failed to generate documentation: %v", err)
	}

	log.Printf("Documentation generated in %s", outputDir)
}
