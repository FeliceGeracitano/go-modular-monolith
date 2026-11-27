#!/usr/bin/env bash
# Remove the example modules (customers, orders, notifications), leaving an
# empty application ready for your own domains.
set -euo pipefail

cd "$(dirname "$0")/.."

examples=(customers orders notifications)
module_path="$(go list -m)"

for name in "${examples[@]}"; do
  rm -rf "modules/$name"
  if [[ -f .github/CODEOWNERS ]]; then
    NAME="$name" perl -ni -e 'print unless m{^/modules/\Q$ENV{NAME}\E/\s}' .github/CODEOWNERS
  fi
done
rm -f test/e2e/examples_test.go

cat > app/modules.go <<GO
package app

import "$module_path/pkg/module"

func (d *deps) modules() []module.Module {
	return nil
}
GO

gofmt -w app/modules.go
go mod tidy
go build ./...

echo
echo "Example modules removed. Add your first module under modules/<name>/ and wire it in app/modules.go."
