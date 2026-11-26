// Package archtest turns the dependency rules of ARCHITECTURE.md into
// executable checks. The rules live in this file; arch_test.go applies them
// to every import statement in the repository.
package archtest

import (
	"fmt"
	"strings"
)

// allowedThirdParty lists the external packages the pure layers (kernel,
// contracts, domain, app) may import. Everything else from outside the
// standard library belongs in an adapter or in the platform.
var allowedThirdParty = []string{
	"github.com/google/uuid",
}

// forbiddenNames are directory names that say nothing about what a package
// is for. Shared code gets a name that states its one responsibility
// (money, httpx, outbox), or it is not shared.
var forbiddenNames = []string{"common", "util", "utils", "helper", "helpers", "shared", "misc", "base", "lib"}

// checkName reports why a package path is unacceptable, or "".
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
	kindUnrestricted kind = iota // cmd/, internal/bootstrap: the composition root
	kindKernel                   // internal/kernel/...
	kindPlatform                 // internal/platform/...
	kindContract                 // internal/modules/<m>/<m>api
	kindDomain                   // internal/modules/<m>/internal/domain
	kindApp                      // internal/modules/<m>/internal/app
	kindModule                   // anything else inside a module: wiring, adapters
)

func (k kind) String() string {
	return [...]string{"composition root", "kernel", "platform", "module contract", "module domain", "module app", "module"}[k]
}

// pkg is a package path relative to the Go module root, classified.
type pkg struct {
	path   string
	kind   kind
	module string // owning business module, when inside internal/modules
}

func classify(path string) pkg {
	p := pkg{path: path}
	switch {
	case hasPrefix(path, "internal/kernel"):
		p.kind = kindKernel
	case hasPrefix(path, "internal/platform"):
		p.kind = kindPlatform
	case hasPrefix(path, "internal/modules"):
		rest := strings.TrimPrefix(strings.TrimPrefix(path, "internal/modules"), "/")
		name, inner, _ := strings.Cut(rest, "/")
		p.module = name
		switch {
		case hasPrefix(inner, name+"api"):
			p.kind = kindContract
		case hasPrefix(inner, "internal/domain"):
			p.kind = kindDomain
		case hasPrefix(inner, "internal/app"):
			p.kind = kindApp
		default:
			p.kind = kindModule
		}
	}
	return p
}

func hasPrefix(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

// checkInternal reports why package from must not import package to, both
// being packages of this repository. It returns "" when the import is fine.
func checkInternal(from, to pkg) string {
	sameModule := from.module != "" && from.module == to.module

	switch from.kind {
	case kindUnrestricted:
		return ""

	case kindKernel:
		if to.kind != kindKernel {
			return "the kernel depends on nothing but itself"
		}

	case kindPlatform:
		if to.kind != kindKernel && to.kind != kindPlatform {
			return "the platform must not know about business modules"
		}

	case kindContract:
		if to.kind != kindKernel {
			return "a module contract is a leaf: it may only import the kernel"
		}

	case kindDomain:
		if to.kind != kindKernel && (!sameModule || to.kind != kindDomain) {
			return "domain code may only import the kernel and its own domain"
		}

	case kindApp:
		ownLayer := sameModule && (to.kind == kindDomain || to.kind == kindApp || to.kind == kindContract)
		if to.kind != kindKernel && !ownLayer {
			return "application code may only import the kernel and its own module's domain, app and contract; reach anything else through a port"
		}

	case kindModule:
		switch {
		case to.kind == kindKernel, to.kind == kindPlatform, sameModule:
		case to.kind == kindContract:
		case to.module != "":
			return fmt.Sprintf("module %q may only use module %q through its %sapi contract", from.module, to.module, to.module)
		default:
			return "modules must not depend on the composition root"
		}
	}
	return ""
}

// checkThirdParty reports why package from must not import the external
// package importPath. It returns "" when the import is fine.
func checkThirdParty(from pkg, importPath string) string {
	switch from.kind {
	case kindKernel, kindContract, kindDomain, kindApp:
	default:
		return ""
	}
	for _, allowed := range allowedThirdParty {
		if hasPrefix(importPath, allowed) {
			return ""
		}
	}
	return fmt.Sprintf("%s code must stay free of third-party dependencies (see allowedThirdParty)", from.kind)
}
