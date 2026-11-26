#!/usr/bin/env bash
# Deploy one image to one environment. Called by .github/workflows/cd.yml.
#
#   scripts/deploy.sh <environment> <image>
#
# This is the one place that knows where the service runs. Replace the body
# with your platform's commands. A release should:
#
#   1. run /app/migrate to completion, with the new image;
#   2. roll out /app/api and /app/worker (or /app/server) with the new image
#      and the settings in deploy/env/<environment>.env plus the secrets;
#   3. wait until /readyz reports ready, and fail (roll back) if it does not.
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: $0 <environment> <image>" >&2
  exit 2
fi

environment="$1"
image="$2"
settings="$(dirname "$0")/../deploy/env/${environment}.env"

if [[ ! -f "$settings" ]]; then
  echo "unknown environment '$environment': $settings does not exist" >&2
  exit 2
fi

echo "Would deploy $image to $environment with settings from $settings" >&2
echo "No deployment target is configured yet: implement scripts/deploy.sh (see deploy/README.md)." >&2
exit 1
