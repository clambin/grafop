package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"
	kongyaml "github.com/alecthomas/kong-yaml"
)

func main() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	var cli CLI
	ctx := kong.Parse(&cli,
		kong.Configuration(kongyaml.Loader, filepath.Join(homeDir, ".grafop", "config.yaml")),
	)
	if err = ctx.Run(&cli); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to run: %v\n", err)
		os.Exit(1)
	}
}
