package archtest

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
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// ManifestFile is the name of the file every module declares itself in.
const ManifestFile = "module.json"

// Manifest is the content of a module's module.json.
type Manifest struct {
	// Description says, in a sentence, what the module is responsible for.
	Description string `json:"description"`
	// Uses lists the other modules whose contract this module may import.
	// A dependency that is not declared here does not compile past the
	// architecture tests, so adding one is always a visible, reviewed change.
	Uses []string `json:"uses"`
}

// Module is a business module as found on disk: what it declares in its
// manifest and what its code actually does.
type Module struct {
	Name string
	Manifest
	// Imports are the other modules whose contract the code imports.
	Imports []string
	// Owners come from .github/CODEOWNERS.
	Owners []string
	// Routes are the HTTP routes the module registers.
	Routes []string
	// Publishes are the integration events defined in the module's contract.
	Publishes []string
	// Subscribes are the events of other modules this module handles.
	Subscribes []string
}

var (
	routePattern = regexp.MustCompile(`^(GET|POST|PUT|PATCH|DELETE) /\S*$`)
	eventPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*\.[a-z0-9_]+\.v[0-9]+$`)
)

// RepoRoot returns the directory containing go.mod, searching upwards from
// the working directory.
func RepoRoot() (string, error) {
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

// ModulePath returns the Go module path declared in root's go.mod.
func ModulePath(root string) (string, error) {
	f, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if modulePath, ok := strings.CutPrefix(scanner.Text(), "module "); ok {
			return strings.TrimSpace(modulePath), nil
		}
	}
	return "", errors.New("module directive not found in go.mod")
}

// LoadModules reads every module under internal/modules, sorted by name.
func LoadModules(root string) ([]Module, error) {
	modulePath, err := ModulePath(root)
	if err != nil {
		return nil, err
	}
	modulesDir := filepath.Join(root, "internal", "modules")
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
	events := map[string]map[string]string{} // module -> constant name -> event name
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		m := Module{Name: entry.Name(), Owners: owners[entry.Name()]}
		dir := filepath.Join(modulesDir, m.Name)

		if m.Manifest, err = loadManifest(filepath.Join(dir, ManifestFile)); err != nil {
			return nil, fmt.Errorf("module %s: %w", m.Name, err)
		}
		if events[m.Name], err = contractEvents(filepath.Join(dir, m.Name+"api")); err != nil {
			return nil, fmt.Errorf("module %s: %w", m.Name, err)
		}
		for _, name := range events[m.Name] {
			m.Publishes = append(m.Publishes, name)
		}
		slices.Sort(m.Publishes)
		modules = append(modules, m)
	}

	// Second pass: what each module's code does with the others.
	for i := range modules {
		m := &modules[i]
		prefix := modulePath + "/internal/modules/"
		err := eachGoFile(filepath.Join(modulesDir, m.Name), func(file *ast.File) {
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
			ast.Inspect(file, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.BasicLit:
					if n.Kind == token.STRING {
						if value, err := strconv.Unquote(n.Value); err == nil && routePattern.MatchString(value) {
							m.Routes = append(m.Routes, value)
						}
					}
				case *ast.SelectorExpr:
					// ordersapi.OrderPlacedName, referenced from another module.
					pkg, ok := n.X.(*ast.Ident)
					if !ok {
						return true
					}
					other, isContract := strings.CutSuffix(pkg.Name, "api")
					if event, found := events[other][n.Sel.Name]; isContract && found && other != m.Name {
						m.Subscribes = append(m.Subscribes, event)
					}
				}
				return true
			})
		})
		if err != nil {
			return nil, fmt.Errorf("module %s: %w", m.Name, err)
		}
		m.Imports = sortedUnique(m.Imports)
		m.Routes = sortedUnique(m.Routes)
		m.Subscribes = sortedUnique(m.Subscribes)
	}
	return modules, nil
}

func loadManifest(path string) (Manifest, error) {
	var manifest Manifest
	raw, err := os.ReadFile(path)
	if err != nil {
		return manifest, fmt.Errorf("every module needs a %s: %w", ManifestFile, err)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&manifest); err != nil {
		return manifest, fmt.Errorf("%s: %w", ManifestFile, err)
	}
	return manifest, nil
}

// contractEvents returns the event names defined as string constants in a
// module's contract package, keyed by constant name.
func contractEvents(dir string) (map[string]string, error) {
	events := map[string]string{}
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return events, nil
	}
	err := eachGoFile(dir, func(file *ast.File) {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				value := spec.(*ast.ValueSpec)
				for i, name := range value.Names {
					if i >= len(value.Values) {
						continue
					}
					lit, ok := value.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					if event, err := strconv.Unquote(lit.Value); err == nil && eventPattern.MatchString(event) {
						events[name.Name] = event
					}
				}
			}
		}
	})
	return events, err
}

// eachGoFile parses every non-test Go file under dir.
func eachGoFile(dir string, visit func(*ast.File)) error {
	fset := token.NewFileSet()
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		visit(file)
		return nil
	})
}

// loadOwners reads .github/CODEOWNERS and returns the owners of each module
// that has its own "/internal/modules/<name>/" line.
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
		name, ok := strings.CutPrefix(fields[0], "/internal/modules/")
		if name, isDir := strings.CutSuffix(name, "/"); ok && isDir && !strings.Contains(name, "/") {
			owners[name] = fields[1:]
		}
	}
	return owners, scanner.Err()
}

func sortedUnique(values []string) []string {
	slices.Sort(values)
	return slices.Compact(values)
}

// CheckModules reports every way the modules break the rules about
// manifests, declared dependencies and ownership.
func CheckModules(modules []Module) []string {
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
			report(m, "%s needs a description", ManifestFile)
		}
		if len(m.Owners) == 0 {
			report(m, "no owner: add a \"/internal/modules/%s/\" line to .github/CODEOWNERS", m.Name)
		}
		if !slices.IsSorted(m.Uses) || len(slices.Compact(slices.Clone(m.Uses))) != len(m.Uses) {
			report(m, "\"uses\" in %s must be sorted and free of duplicates", ManifestFile)
		}
		for _, used := range m.Uses {
			switch _, exists := known[used]; {
			case used == m.Name:
				report(m, "lists itself in \"uses\"")
			case !exists:
				report(m, "declares a dependency on %q, which is not a module", used)
			case !slices.Contains(m.Imports, used):
				report(m, "declares a dependency on %q but never imports its contract: remove it from %s", used, ManifestFile)
			}
		}
		for _, imported := range m.Imports {
			if !slices.Contains(m.Uses, imported) {
				report(m, "imports the contract of %q without declaring it: add %q to \"uses\" in %s, or remove the dependency",
					imported, imported, ManifestFile)
			}
		}
	}

	if cycle := findCycle(known); cycle != nil {
		problems = append(problems, fmt.Sprintf(
			"modules depend on each other in a cycle: %s. Turn one direction into an event, or move the workflow into a module above both",
			strings.Join(cycle, " -> ")))
	}
	return problems
}

// findCycle returns one dependency cycle among the declared "uses", or nil.
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
