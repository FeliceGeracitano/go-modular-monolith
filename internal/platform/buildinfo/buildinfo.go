// Package buildinfo reports what this binary is and which build it came from.
package buildinfo

import (
	"path"
	"runtime/debug"
)

// version is set at build time:
//
//	go build -ldflags "-X <module>/internal/platform/buildinfo.version=v1.2.3"
//
// When it is not, the VCS revision recorded by the Go toolchain is used.
var version string

// Version returns the release version or commit of this build, or "dev"
// when neither is known (for example under `go run` outside a checkout).
func Version() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	var revision, dirty string
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			if setting.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if revision == "" {
		return "dev"
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	return revision + dirty
}

// Service returns the default service name: the last element of the Go
// module path, so a project created from the template is named correctly
// without any configuration.
func Service() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Path != "" {
		return path.Base(info.Main.Path)
	}
	return "service"
}
