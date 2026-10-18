#!/usr/bin/env bash
# Write random admin-key and origin-verify SecureStrings. Refuses to overwrite.
set -euo pipefail
# shellcheck source=lib.sh
# shellcheck source-path=SCRIPTDIR
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

for name in admin-key origin-verify; do
  if aws ssm get-parameter --region "$REGION" --name "$PARAM_PREFIX/$name" >/dev/null 2>&1; then
    die "$PARAM_PREFIX/$name already exists; run 99-purge.sh first to replace it"
  fi
done

for name in admin-key origin-verify; do
  aws ssm put-parameter --region "$REGION" --name "$PARAM_PREFIX/$name" \
    --type SecureString --value "$(openssl rand -hex 32)" >/dev/null
  echo "wrote $PARAM_PREFIX/$name"
done
