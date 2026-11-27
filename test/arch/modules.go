package arch

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const manifestFile = "module.json"

type Manifest struct {
	Description string   `json:"description"`
	Uses        []string `json:"uses"`
}

type Module struct {
	Name string
	Manifest
	Imports []string
	Owners  []string
}

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found")
		}
		dir = parent
	}
}

func goModulePath(root string) (string, error) {
	f, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if p, ok := strings.CutPrefix(scanner.Text(), "module "); ok {
			return strings.TrimSpace(p), nil
		}
	}
	return "", errors.New("module directive not found in go.mod")
}

func loadModules(root string) ([]Module, error) {
	modulePath, err := goModulePath(root)
	if err != nil {
		return nil, err
	}
	modulesDir := filepath.Join(root, "modules")
	entries, err := os.ReadDir(modulesDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	owners, err := loadOwners(root)
	if err != nil {
		return nil, err
	}

	var modules []Module
	prefix := modulePath + "/modules/"
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		m := Module{Name: entry.Name(), Owners: owners[entry.Name()]}
		dir := filepath.Join(modulesDir, m.Name)

		if m.Manifest, err = loadManifest(filepath.Join(dir, manifestFile)); err != nil {
			return nil, fmt.Errorf("module %s: %w", m.Name, err)
		}
		err := eachGoFile(dir, func(file *ast.File) {
			for _, spec := range file.Imports {
				importPath, _ := strconv.Unquote(spec.Path.Value)
				rest, ok := strings.CutPrefix(importPath, prefix)
				if !ok {
					continue
				}
				if other, _, _ := strings.Cut(rest, "/"); other != m.Name {
					m.Imports = append(m.Imports, other)
				}
			}
		})
		if err != nil {
			return nil, fmt.Errorf("module %s: %w", m.Name, err)
		}
		slices.Sort(m.Imports)
		m.Imports = slices.Compact(m.Imports)
		modules = append(modules, m)
	}
	return modules, nil
}

func loadManifest(path string) (Manifest, error) {
	var manifest Manifest
	raw, err := os.ReadFile(path)
	if err != nil {
		return manifest, fmt.Errorf("every module needs a %s: %w", manifestFile, err)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&manifest); err != nil {
		return manifest, fmt.Errorf("%s: %w", manifestFile, err)
	}
	return manifest, nil
}

func eachGoFile(dir string, visit func(*ast.File)) error {
	fset := token.NewFileSet()
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		visit(file)
		return nil
	})
}

func loadOwners(root string) (map[string][]string, error) {
	owners := map[string][]string{}
	f, err := os.Open(filepath.Join(root, ".github", "CODEOWNERS"))
	if errors.Is(err, fs.ErrNotExist) {
		return owners, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		name, ok := strings.CutPrefix(fields[0], "/modules/")
		if name, isDir := strings.CutSuffix(name, "/"); ok && isDir && !strings.Contains(name, "/") {
			owners[name] = fields[1:]
		}
	}
	return owners, scanner.Err()
}

func checkModules(modules []Module) []string {
	var problems []string
	report := func(m Module, format string, args ...any) {
		problems = append(problems, fmt.Sprintf("module %s: %s", m.Name, fmt.Sprintf(format, args...)))
	}
	known := map[string]Module{}
	for _, m := range modules {
		known[m.Name] = m
	}

	for _, m := range modules {
		if strings.TrimSpace(m.Description) == "" {
			report(m, "%s needs a description", manifestFile)
		}
		if len(m.Owners) == 0 {
			report(m, "no owner: add a \"/modules/%s/\" line to .github/CODEOWNERS", m.Name)
		}
		if !slices.IsSorted(m.Uses) || len(slices.Compact(slices.Clone(m.Uses))) != len(m.Uses) {
			report(m, "\"uses\" in %s must be sorted and free of duplicates", manifestFile)
		}
		for _, used := range m.Uses {
			switch _, exists := known[used]; {
			case used == m.Name:
				report(m, "lists itself in \"uses\"")
			case !exists:
				report(m, "declares a dependency on %q, which is not a module", used)
			case !slices.Contains(m.Imports, used):
				report(m, "declares a dependency on %q but never imports its contract: remove it from %s", used, manifestFile)
			}
		}
		for _, imported := range m.Imports {
			if !slices.Contains(m.Uses, imported) {
				report(m, "imports %q without declaring it: add %q to \"uses\" in %s", imported, imported, manifestFile)
			}
		}
	}

	if cycle := findCycle(known); cycle != nil {
		problems = append(problems, fmt.Sprintf(
			"modules depend on each other in a cycle: %s", strings.Join(cycle, " -> ")))
	}
	return problems
}

func findCycle(modules map[string]Module) []string {
	const (
		unvisited = iota
		inProgress
		done
	)
	state := map[string]int{}
	var path []string

	var visit func(name string) []string
	visit = func(name string) []string {
		switch state[name] {
		case done:
			return nil
		case inProgress:
			return append(slices.Clone(path[slices.Index(path, name):]), name)
		}
		state[name] = inProgress
		path = append(path, name)
		for _, used := range modules[name].Uses {
			if _, exists := modules[used]; !exists {
				continue
			}
			if cycle := visit(used); cycle != nil {
				return cycle
			}
		}
		path = path[:len(path)-1]
		state[name] = done
		return nil
	}

	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if cycle := visit(name); cycle != nil {
			return cycle
		}
	}
	return nil
}
