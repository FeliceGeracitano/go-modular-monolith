package arch

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDependencyRules(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	modulePath, err := goModulePath(root)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	named := map[string]bool{}

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
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
		t.Fatal("no imports were checked")
	}
}

func TestModules(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	modules, err := loadModules(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range checkModules(modules) {
		t.Error(problem)
	}
}

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
		want    string
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
			problems := strings.Join(checkModules(tt.modules), "\n")
			switch {
			case tt.want == "" && problems != "":
				t.Fatalf("unexpected problems: %s", problems)
			case tt.want != "" && !strings.Contains(problems, tt.want):
				t.Fatalf("problems = %q, want one containing %q", problems, tt.want)
			}
		})
	}
}

func TestRules(t *testing.T) {
	tests := []struct {
		from, to string
		allowed  bool
	}{
		{"app", "modules/orders", true},
		{".", "app", true},
		{"test/e2e", "app", true},

		{"pkg/outbox", "pkg/event", true},
		{"pkg/httpx", "pkg/apperr", true},
		{"pkg/httpx", "modules/orders/ordersapi", false},
		{"pkg/config", "app", false},

		{"modules/orders/ordersapi", "pkg/event", true},
		{"modules/orders/ordersapi", "modules/orders/internal", false},
		{"modules/orders/ordersapi", "modules/customers/customersapi", false},

		{"modules/orders/internal", "pkg/httpx", true},
		{"modules/orders/internal", "modules/customers/customersapi", true},
		{"modules/orders/internal", "modules/customers/internal", false},
		{"modules/orders", "modules/customers", false},
		{"modules/orders", "app", false},
	}
	for _, tt := range tests {
		reason := checkInternal(classify(tt.from), classify(tt.to))
		if got := reason == ""; got != tt.allowed {
			t.Errorf("%s -> %s: allowed = %v, want %v (%s)", tt.from, tt.to, got, tt.allowed, reason)
		}
	}

	if checkName("modules/orders/internal/utils") == "" || checkName("common") == "" {
		t.Error("junk-drawer package names must be rejected")
	}
	if checkName("pkg/httpx") != "" {
		t.Error("purposeful package names must be accepted")
	}
	if checkThirdParty(classify("modules/orders/ordersapi"), "go.opentelemetry.io/otel") == "" {
		t.Error("contracts must not import third-party packages")
	}
	if checkThirdParty(classify("modules/orders/ordersapi"), "github.com/google/uuid") != "" {
		t.Error("allow-listed packages must be importable from contracts")
	}
}

func isStdlib(importPath string) bool {
	first, _, _ := strings.Cut(importPath, "/")
	return !strings.Contains(first, ".")
}
