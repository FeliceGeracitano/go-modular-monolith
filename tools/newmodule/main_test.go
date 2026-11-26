package main

import (
	"strings"
	"testing"
)

// Rendering gofmt-s every Go file, so this fails on a template that is not
// valid Go: the generator can never emit a module that does not parse.
func TestTemplatesRenderToValidGo(t *testing.T) {
	out, err := renderAll(data{Name: "billing", Description: "Bills customers.", ModulePath: "example.com/svc"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(files) {
		t.Fatalf("rendered %d files, want %d", len(out), len(files))
	}
	for path, content := range out {
		if strings.Contains(string(content), "{{") {
			t.Errorf("%s still contains template syntax", path)
		}
	}
	if _, ok := out["billingapi/billingapi.go"]; !ok {
		t.Errorf("contract package missing; got %v", keys(out))
	}
}

func TestRunRejectsBadNames(t *testing.T) {
	for _, name := range []string{"", "Billing", "bill-ing", "1billing", "b", "outbox", "modules"} {
		if err := run(name, ""); err == nil {
			t.Errorf("run(%q) succeeded, want an error", name)
		}
	}
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
