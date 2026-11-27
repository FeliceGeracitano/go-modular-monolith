package arch

import (
	"fmt"
	"strings"
)

var allowedThirdParty = []string{
	"github.com/google/uuid",
}

var forbiddenNames = []string{"common", "util", "utils", "helper", "helpers", "shared", "misc", "base", "lib"}

func checkName(path string) string {
	for _, element := range strings.Split(path, "/") {
		for _, forbidden := range forbiddenNames {
			if element == forbidden {
				return fmt.Sprintf("%q is a junk-drawer name: name the package after the single thing it provides", element)
			}
		}
	}
	return ""
}

type kind int

const (
	kindUnrestricted kind = iota // repo root, app/, docs/, test/
	kindPkg                      // pkg/...
	kindContract                 // modules/<m>/<m>api
	kindModule                   // modules/<m>/...
)

func (k kind) String() string {
	return [...]string{"root", "pkg", "module contract", "module"}[k]
}

type pkg struct {
	path   string
	kind   kind
	module string
}

func classify(path string) pkg {
	p := pkg{path: path}
	switch {
	case hasPrefix(path, "pkg"):
		p.kind = kindPkg
	case hasPrefix(path, "modules"):
		rest := strings.TrimPrefix(strings.TrimPrefix(path, "modules"), "/")
		name, inner, _ := strings.Cut(rest, "/")
		p.module = name
		if hasPrefix(inner, name+"api") {
			p.kind = kindContract
		} else {
			p.kind = kindModule
		}
	}
	return p
}

func hasPrefix(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func checkInternal(from, to pkg) string {
	sameModule := from.module != "" && from.module == to.module

	switch from.kind {
	case kindUnrestricted:
		return ""
	case kindPkg:
		if to.kind != kindPkg {
			return "pkg must not depend on business modules or app"
		}
	case kindContract:
		if to.kind != kindPkg {
			return "module contract is a leaf: it may only import pkg and stdlib"
		}
	case kindModule:
		switch {
		case to.kind == kindPkg, sameModule:
		case to.kind == kindContract:
		case to.module != "":
			return fmt.Sprintf("module %q may only use module %q through its %sapi contract", from.module, to.module, to.module)
		default:
			return "modules must not depend on app or root packages"
		}
	}
	return ""
}

func checkThirdParty(from pkg, importPath string) string {
	if from.kind != kindContract {
		return ""
	}
	for _, allowed := range allowedThirdParty {
		if hasPrefix(importPath, allowed) {
			return ""
		}
	}
	return fmt.Sprintf("%s code must stay free of third-party dependencies", from.kind)
}
