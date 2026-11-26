// Command modulemap regenerates docs/MODULES.md from the code.
//
//	go run ./tools/modulemap     (or: make docs)
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/archtest"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "modulemap:", err)
		os.Exit(1)
	}
}

func run() error {
	root, err := archtest.RepoRoot()
	if err != nil {
		return err
	}
	modules, err := archtest.LoadModules(root)
	if err != nil {
		return err
	}
	path := filepath.Join(root, filepath.FromSlash(archtest.ModuleMapFile))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(archtest.RenderModuleMap(modules)), 0o644); err != nil {
		return err
	}
	fmt.Println("wrote", archtest.ModuleMapFile)
	return nil
}
