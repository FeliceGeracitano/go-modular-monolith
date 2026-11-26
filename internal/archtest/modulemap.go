package archtest

import (
	"fmt"
	"strings"
)

// ModuleMapFile is where the generated module map lives, relative to the
// repository root.
const ModuleMapFile = "docs/MODULES.md"

// RenderModuleMap describes the modules and how they relate, as Markdown.
// The output is derived entirely from the code, so it cannot go stale: a
// test fails when the committed file differs from what this returns.
func RenderModuleMap(modules []Module) string {
	var b strings.Builder
	b.WriteString("# Module map\n\n")
	b.WriteString("<!-- Generated from the code by `make docs`. Do not edit by hand. -->\n\n")

	if len(modules) == 0 {
		b.WriteString("There are no modules yet. Create the first one with `make module name=<name>`.\n")
		return b.String()
	}

	b.WriteString("A solid arrow means \"calls, through its contract\". A dotted arrow is an\nintegration event, from publisher to subscriber.\n\n")
	b.WriteString("```mermaid\nflowchart LR\n")
	for _, m := range modules {
		fmt.Fprintf(&b, "    %s\n", m.Name)
	}
	for _, m := range modules {
		for _, used := range m.Uses {
			fmt.Fprintf(&b, "    %s --> %s\n", m.Name, used)
		}
	}
	for _, m := range modules {
		for _, event := range m.Subscribes {
			publisher, _, _ := strings.Cut(event, ".")
			fmt.Fprintf(&b, "    %s -. \"%s\" .-> %s\n", publisher, event, m.Name)
		}
	}
	b.WriteString("```\n\n")

	b.WriteString("| Module | Responsibility | Owners | Depends on |\n| --- | --- | --- | --- |\n")
	for _, m := range modules {
		fmt.Fprintf(&b, "| [%s](#%s) | %s | %s | %s |\n",
			m.Name, m.Name, m.Description, list(m.Owners, " "), list(code(m.Uses), ", "))
	}

	for _, m := range modules {
		fmt.Fprintf(&b, "\n## %s\n\n%s\n\n", m.Name, m.Description)
		fmt.Fprintf(&b, "- **Code:** [`internal/modules/%[1]s`](../internal/modules/%[1]s)\n", m.Name)
		fmt.Fprintf(&b, "- **Database schema:** `%s`\n", m.Name)
		fmt.Fprintf(&b, "- **Depends on:** %s\n", list(code(m.Uses), ", "))
		fmt.Fprintf(&b, "- **Publishes:** %s\n", list(code(m.Publishes), ", "))
		fmt.Fprintf(&b, "- **Subscribes to:** %s\n", list(code(m.Subscribes), ", "))
		if len(m.Routes) == 0 {
			b.WriteString("- **HTTP:** none\n")
			continue
		}
		b.WriteString("- **HTTP:**\n")
		for _, route := range m.Routes {
			fmt.Fprintf(&b, "  - `%s`\n", route)
		}
	}
	return b.String()
}

func code(values []string) []string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = "`" + v + "`"
	}
	return quoted
}

func list(values []string, sep string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, sep)
}
