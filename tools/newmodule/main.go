// Command newmodule scaffolds a business module and wires it into the
// application.
//
//	go run ./tools/newmodule -name billing     (or: make module name=billing)
//
// It creates internal/modules/<name> with the standard layout, registers the
// module in internal/bootstrap/modules.go, gives it an owner in
// .github/CODEOWNERS and regenerates docs/MODULES.md. The result compiles,
// passes the architecture tests and serves GET /api/v1/<name>/status.
package main

import (
	"bytes"
	"embed"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"text/template"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/archtest"
)

//go:embed templates/*.tmpl
var templates embed.FS

// files maps each template to the file it produces inside the module.
var files = map[string]string{
	"module.json.tmpl":     "module.json",
	"module.go.tmpl":       "module.go",
	"contract.go.tmpl":     "{{.Name}}api/{{.Name}}api.go",
	"migration.sql.tmpl":   "migrations/00001_init.sql",
	"service.go.tmpl":      "internal/app/service.go",
	"service_test.go.tmpl": "internal/app/service_test.go",
	"store.go.tmpl":        "internal/adapters/pgstore/store.go",
	"handler.go.tmpl":      "internal/adapters/httpapi/handler.go",
}

const (
	wiringFile = "internal/bootstrap/modules.go"
	wireMarker = "\t// modulegen:wire"
	ownersFile = ".github/CODEOWNERS"
)

var (
	// A module name is at once a Go package name, a URL segment, a database
	// schema and (with a suffix) a database role, hence the narrow alphabet.
	validName = regexp.MustCompile(`^[a-z][a-z0-9]{1,30}$`)

	// reserved names would clash with identifiers in the wiring code, with
	// platform schemas or with Go itself.
	reserved = []string{
		"app", "bootstrap", "err", "internal", "kernel", "main", "module",
		"modules", "outbox", "p", "platform", "public", "test",
	}
)

type data struct {
	Name        string
	Description string
	ModulePath  string
}

func main() {
	name := flag.String("name", "", "module name: lower-case letters and digits, e.g. billing")
	description := flag.String("description", "", "one sentence saying what the module is responsible for")
	flag.Parse()

	if err := run(*name, *description); err != nil {
		fmt.Fprintln(os.Stderr, "newmodule:", err)
		os.Exit(1)
	}
}

func run(name, description string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("invalid name %q: use 2-31 lower-case letters and digits, starting with a letter (make module name=billing)", name)
	}
	if slices.Contains(reserved, name) {
		return fmt.Errorf("%q is reserved, pick another name", name)
	}
	if description == "" {
		description = "TODO: say in one sentence what the " + name + " module is responsible for."
	}
	description = strings.ReplaceAll(description, `"`, `'`)

	root, err := archtest.RepoRoot()
	if err != nil {
		return err
	}
	modulePath, err := archtest.ModulePath(root)
	if err != nil {
		return err
	}
	d := data{Name: name, Description: description, ModulePath: modulePath}

	moduleDir := filepath.Join(root, "internal", "modules", name)
	if _, err := os.Stat(moduleDir); err == nil {
		return fmt.Errorf("module %q already exists", name)
	}

	// Render everything before touching the disk, so a broken template
	// leaves nothing half-written.
	rendered, err := renderAll(d)
	if err != nil {
		return err
	}
	wiring, err := wire(filepath.Join(root, wiringFile), d)
	if err != nil {
		return err
	}

	for path, content := range rendered {
		full := filepath.Join(moduleDir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(full, content, 0o644); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(root, wiringFile), wiring, 0o644); err != nil {
		return err
	}
	if err := addOwner(filepath.Join(root, ownersFile), name); err != nil {
		return err
	}

	modules, err := archtest.LoadModules(root)
	if err != nil {
		return err
	}
	mapPath := filepath.Join(root, filepath.FromSlash(archtest.ModuleMapFile))
	if err := os.WriteFile(mapPath, []byte(archtest.RenderModuleMap(modules)), 0o644); err != nil {
		return err
	}

	fmt.Printf(`Created module %[1]q in internal/modules/%[1]s and wired it into %[2]s.

Next:
  1. Describe it in internal/modules/%[1]s/module.json, then run: make docs
  2. Check its owner in %[3]s
  3. Start the server (make run) and try: curl localhost:8080/api/v1/%[1]s/status
  4. Replace the Status use case with your own. ARCHITECTURE.md has the recipes.
`, name, wiringFile, ownersFile)
	return nil
}

// renderAll renders every template, keyed by path inside the module. Go
// files are gofmt-ed, which also proves the templates are valid Go.
func renderAll(d data) (map[string][]byte, error) {
	out := make(map[string][]byte, len(files))
	for tmplName, pathTmpl := range files {
		path, err := render(pathTmpl, pathTmpl, d)
		if err != nil {
			return nil, err
		}
		raw, err := templates.ReadFile("templates/" + tmplName)
		if err != nil {
			return nil, err
		}
		content, err := render(tmplName, string(raw), d)
		if err != nil {
			return nil, err
		}
		if strings.HasSuffix(string(path), ".go") {
			if content, err = format.Source(content); err != nil {
				return nil, fmt.Errorf("%s: %w", tmplName, err)
			}
		}
		out[string(path)] = content
	}
	return out, nil
}

func render(name, text string, d data) ([]byte, error) {
	tmpl, err := template.New(name).Option("missingkey=error").Parse(text)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, d); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// wire returns the composition root's module file with the new module
// imported and constructed.
func wire(path string, d data) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	src := string(raw)

	// Import: next to the other imports of this repository. gofmt sorts the
	// group afterwards.
	firstLocal := "\t\"" + d.ModulePath + "/internal/"
	i := strings.Index(src, firstLocal)
	if i < 0 || !strings.Contains(src, wireMarker) {
		return nil, fmt.Errorf("%s no longer has the shape the generator expects (a local import and the %q marker); wire the module by hand", wiringFile, strings.TrimSpace(wireMarker))
	}
	src = src[:i] + "\t\"" + d.ModulePath + "/internal/modules/" + d.Name + "\"\n" + src[i:]

	block, err := render("wire", `	{{.Name}}DB, err := p.moduleDB("{{.Name}}")
	if err != nil {
		return nil, err
	}
	modules = append(modules, {{.Name}}.New({{.Name}}.Deps{
		DB: {{.Name}}DB,
	}))

`, d)
	if err != nil {
		return nil, err
	}
	src = strings.Replace(src, wireMarker, string(block)+wireMarker, 1)

	formatted, err := format.Source([]byte(src))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", wiringFile, err)
	}
	return formatted, nil
}

// addOwner gives the module an owner: whoever owns everything by default.
func addOwner(path, name string) error {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	owner := "@TODO-owner"
	for _, line := range strings.Split(string(raw), "\n") {
		if fields := strings.Fields(line); len(fields) >= 2 && fields[0] == "*" {
			owner = strings.Join(fields[1:], " ")
		}
	}
	content := strings.TrimRight(string(raw), "\n") + "\n" +
		fmt.Sprintf("%-35s %s\n", "/internal/modules/"+name+"/", owner)
	return os.WriteFile(path, []byte(content), 0o644)
}
