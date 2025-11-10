//go:build ignore

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func main() {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Fprintln(os.Stderr, "Cannot get current file info")
		os.Exit(1)
	}
	baseDir := filepath.Dir(currentFile)

	run := func(name string, arg ...string) error {
		cmd := exec.Command(name, arg...)
		cmd.Env = append(os.Environ(), "REDOCLY_TELEMETRY=off")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Dir = baseDir
		return cmd.Run()
	}

	// Bundle openapi spec with redocly cli
	if err := run("bunx", "@redocly/cli@2.2.2", "bundle", "./api.yaml", "--output", "./.build/openapi.yaml"); err != nil {
		fmt.Fprintf(os.Stderr, "Error running openapi-format: %v\n", err)
		os.Exit(1)
	}

	// Build models with oapi-codegen
	if err := run("go", "tool", "oapi-codegen", "-config", "./generate_cfg.yaml", "./.build/openapi.yaml"); err != nil {
		fmt.Fprintf(os.Stderr, "Error running oapi-codegen: %v\n", err)
		os.Exit(1)
	}

	// Remove the intermediate file
	if err := os.RemoveAll(filepath.Join(baseDir, ".build")); err != nil {
		fmt.Fprintf(os.Stderr, "Error removing ./.build/: %v\n", err)
		os.Exit(1)
	}
}
