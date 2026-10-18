#!/usr/bin/env bash
# Write random admin-key (SecureString) and origin-verify (String). Refuses to overwrite.
# origin-verify is a plain String: CloudFront origin headers cannot resolve a
# SecureString, and the value sits in clear text in the distribution config anyway.
# Rotate: put a new value, rerun 03-deploy.sh, and the functions pick it up on cold start.
set -euo pipefail
# shellcheck source=lib.sh
# shellcheck source-path=SCRIPTDIR
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

for name in admin-key origin-verify; do
  if aws ssm get-parameter --region "$REGION" --name "$PARAM_PREFIX/$name" >/dev/null 2>&1; then
    die "$PARAM_PREFIX/$name already exists; run 99-purge.sh first to replace it"
  fi
done

for spec in admin-key:SecureString origin-verify:String; do
  name="${spec%%:*}"
  aws ssm put-parameter --region "$REGION" --name "$PARAM_PREFIX/$name" \
    --type "${spec#*:}" --value "$(openssl rand -hex 32)" >/dev/null
  echo "wrote $PARAM_PREFIX/$name (${spec#*:})"
done
