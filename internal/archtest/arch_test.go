package archtest

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestDependencyRules parses every Go file in the repository and fails on
// any import that crosses a boundary it should not.
func TestDependencyRules(t *testing.T) {
	root := repoRoot(t)
	modulePath := goModulePath(t, root)
	fset := token.NewFileSet()
	checked := 0
	named := map[string]bool{} // packages already reported by checkName

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); path != root && (strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		from := classify(filepath.ToSlash(rel))
		if reason := checkName(from.path); reason != "" && !named[from.path] {
			named[from.path] = true
			t.Errorf("package %s: %s", from.path, reason)
		}

		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			checked++

			var reason string
			switch {
			case importPath == modulePath || strings.HasPrefix(importPath, modulePath+"/"):
				to := classify(strings.TrimPrefix(strings.TrimPrefix(importPath, modulePath), "/"))
				reason = checkInternal(from, to)
			case isStdlib(importPath):
			default:
				reason = checkThirdParty(from, importPath)
			}
			if reason != "" {
				t.Errorf("%s\n\timports %s\n\t%s", fset.Position(spec.Pos()), importPath, reason)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("no imports were checked: the test is not looking at the repository")
	}
}

// TestModules checks what every module declares about itself: a manifest
// with a description, an owner, and exactly the dependencies its code uses,
// with no cycles between modules.
func TestModules(t *testing.T) {
	modules, err := LoadModules(repoRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range CheckModules(modules) {
		t.Error(problem)
	}
}

// TestModuleMapIsUpToDate fails when docs/MODULES.md no longer matches the
// code. Regenerate it with `make docs`.
func TestModuleMapIsUpToDate(t *testing.T) {
	root := repoRoot(t)
	modules, err := LoadModules(root)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ModuleMapFile)))
	if err != nil {
		t.Fatalf("%v (generate it with `make docs`)", err)
	}
	if string(committed) != RenderModuleMap(modules) {
		t.Fatalf("%s is out of date: run `make docs` and commit the result", ModuleMapFile)
	}
}

// TestCheckModules pins the module rules against deliberately broken input.
func TestCheckModules(t *testing.T) {
	module := func(name string, uses, imports []string) Module {
		return Module{
			Name:     name,
			Manifest: Manifest{Description: "does things", Uses: uses},
			Imports:  imports,
			Owners:   []string{"@someone"},
		}
	}
	tests := []struct {
		name    string
		modules []Module
		want    string // substring of the expected problem; "" means none
	}{
		{"declared and used", []Module{
			module("orders", []string{"customers"}, []string{"customers"}),
			module("customers", nil, nil),
		}, ""},
		{"undeclared dependency", []Module{
			module("orders", nil, []string{"customers"}),
			module("customers", nil, nil),
		}, "without declaring it"},
		{"stale declaration", []Module{
			module("orders", []string{"customers"}, nil),
			module("customers", nil, nil),
		}, "never imports its contract"},
		{"unknown module", []Module{
			module("orders", []string{"ghosts"}, []string{"ghosts"}),
		}, "is not a module"},
		{"cycle", []Module{
			module("orders", []string{"customers"}, []string{"customers"}),
			module("customers", []string{"orders"}, []string{"orders"}),
		}, "customers -> orders -> customers"},
		{"no owner", []Module{{Name: "orders", Manifest: Manifest{Description: "does things"}}}, "no owner"},
		{"no description", []Module{{Name: "orders", Owners: []string{"@someone"}}}, "needs a description"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			problems := strings.Join(CheckModules(tt.modules), "\n")
			switch {
			case tt.want == "" && problems != "":
				t.Fatalf("unexpected problems: %s", problems)
			case tt.want != "" && !strings.Contains(problems, tt.want):
				t.Fatalf("problems = %q, want one containing %q", problems, tt.want)
			}
		})
	}
}

// TestRules pins the rules themselves, so that loosening one is a visible,
// reviewed change rather than an accident.
func TestRules(t *testing.T) {
	tests := []struct {
		from, to string
		allowed  bool
	}{
		// The composition root sees everything.
		{"internal/bootstrap", "internal/modules/orders", true},
		{"cmd/server", "internal/bootstrap", true},

		// Shared code never depends on business modules.
		{"internal/kernel/money", "internal/kernel/apperr", true},
		{"internal/kernel/money", "internal/platform/postgres", false},
		{"internal/platform/outbox", "internal/kernel/event", true},
		{"internal/platform/httpx", "internal/modules/orders/ordersapi", false},

		// Contracts are leaves.
		{"internal/modules/orders/ordersapi", "internal/kernel/event", true},
		{"internal/modules/orders/ordersapi", "internal/modules/orders/internal/domain", false},
		{"internal/modules/orders/ordersapi", "internal/modules/customers/customersapi", false},

		// Inside a module, dependencies point inwards.
		{"internal/modules/orders/internal/domain", "internal/kernel/money", true},
		{"internal/modules/orders/internal/domain", "internal/modules/orders/internal/app", false},
		{"internal/modules/orders/internal/domain", "internal/platform/postgres", false},
		{"internal/modules/orders/internal/app", "internal/modules/orders/internal/domain", true},
		{"internal/modules/orders/internal/app", "internal/modules/orders/ordersapi", true},
		{"internal/modules/orders/internal/app", "internal/modules/orders/internal/adapters/pgstore", false},
		{"internal/modules/orders/internal/app", "internal/platform/postgres", false},
		{"internal/modules/orders/internal/app", "internal/modules/customers/customersapi", false},
		{"internal/modules/orders/internal/adapters/pgstore", "internal/platform/postgres", true},
		{"internal/modules/orders/internal/adapters/pgstore", "internal/modules/orders/internal/domain", true},

		// Across modules, only contracts are visible.
		{"internal/modules/orders/internal/adapters/customersacl", "internal/modules/customers/customersapi", true},
		{"internal/modules/orders", "internal/modules/customers/customersapi", true},
		{"internal/modules/orders", "internal/modules/customers", false},
		{"internal/modules/orders/internal/adapters/pgstore", "internal/modules/customers/internal/domain", false},
		{"internal/modules/orders", "internal/bootstrap", false},
	}
	for _, tt := range tests {
		reason := checkInternal(classify(tt.from), classify(tt.to))
		if got := reason == ""; got != tt.allowed {
			t.Errorf("%s -> %s: allowed = %v, want %v (%s)", tt.from, tt.to, got, tt.allowed, reason)
		}
	}

	if checkName("internal/modules/orders/internal/utils") == "" || checkName("internal/common") == "" {
		t.Error("junk-drawer package names must be rejected")
	}
	if checkName("internal/platform/httpx") != "" {
		t.Error("purposeful package names must be accepted")
	}

	if checkThirdParty(classify("internal/modules/orders/internal/app"), "github.com/jackc/pgx/v5") == "" {
		t.Error("application code must not be allowed to import a database driver")
	}
	if checkThirdParty(classify("internal/modules/orders/internal/adapters/pgstore"), "github.com/jackc/pgx/v5") != "" {
		t.Error("adapters must be allowed to import third-party packages")
	}
	if checkThirdParty(classify("internal/kernel/id"), "github.com/google/uuid") != "" {
		t.Error("allow-listed packages must be importable from the kernel")
	}
}

// isStdlib relies on the convention that only standard library import paths
// have no dot in their first element.
func isStdlib(importPath string) bool {
	first, _, _ := strings.Cut(importPath, "/")
	return !strings.Contains(first, ".")
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := RepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func goModulePath(t *testing.T, root string) string {
	t.Helper()
	modulePath, err := ModulePath(root)
	if err != nil {
		t.Fatal(err)
	}
	return modulePath
}
