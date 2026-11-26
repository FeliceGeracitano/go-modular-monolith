#!/usr/bin/env bash
# Remove the example modules (customers, orders, notifications), leaving an
# empty application ready for your own:
#
#   scripts/remove-examples.sh
#   make module name=<your-first-module>
set -euo pipefail

cd "$(dirname "$0")/.."

examples=(customers orders notifications)
module_path="$(go list -m)"

for name in "${examples[@]}"; do
  rm -rf "internal/modules/$name"
  # Drop the module's line from CODEOWNERS.
  if [[ -f .github/CODEOWNERS ]]; then
    NAME="$name" perl -ni -e 'print unless m{^/internal/modules/\Q$ENV{NAME}\E/\s}' .github/CODEOWNERS
  fi
done
rm -f internal/bootstrap/examples_test.go

# The composition root, with no modules. `make module` fills it in.
cat > internal/bootstrap/modules.go <<GO
package bootstrap

import (
	"$module_path/internal/platform/module"
)

// modules constructs every business module. This function is the system's
// dependency graph: a module that needs another one receives that module's
// public API here, explicitly, and nowhere else.
//
// \`make module name=<name>\` adds new modules at the marker below.
func (p *platform) modules() ([]module.Module, error) {
	var modules []module.Module

	// modulegen:wire
	return modules, nil
}
GO

gofmt -w internal/bootstrap/modules.go
go run ./tools/modulemap
go mod tidy
go build ./...

echo
echo "Example modules removed. Create your first module with:"
echo "  make module name=<name>"
