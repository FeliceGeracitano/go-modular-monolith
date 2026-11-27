#!/usr/bin/env bash
# Rename the Go module after creating a new service from this blueprint.
#
#   scripts/rename-module.sh github.com/acme/billing-service
set -euo pipefail

if [[ $# -ne 1 || -z "$1" ]]; then
  echo "usage: $0 <new-module-path>" >&2
  exit 2
fi

cd "$(dirname "$0")/.."

new="$1"
old="$(go list -m)"

if [[ "$old" == "$new" ]]; then
  echo "module is already $new"
  exit 0
fi

# Every tracked or untracked (but not ignored) text file that mentions the old path.
git ls-files -z --cached --others --exclude-standard |
  xargs -0 grep -lIZ --fixed-strings -- "$old" |
  OLD="$old" NEW="$new" xargs -0 -r perl -pi -e 's/\Q$ENV{OLD}\E/$ENV{NEW}/g'

gofmt -w .
go build ./...

echo "renamed $old -> $new"
echo "next: review the diff, then run 'make check'"
