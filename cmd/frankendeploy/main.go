package main

import (
	"os"

	"github.com/yoanbernabeu/frankendeploy/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
