#!/usr/bin/env bash
# Delete stack auth-gate-$STAGE and both SSM parameters after a typed confirmation.
set -euo pipefail
# shellcheck source=lib.sh
# shellcheck source-path=SCRIPTDIR
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

read -r -p "Type '$STACK' to delete the stack and its SSM parameters: " answer
[ "$answer" = "$STACK" ] || die "confirmation did not match; nothing deleted"

aws cloudformation delete-stack --region "$REGION" --stack-name "$STACK"
aws cloudformation wait stack-delete-complete --region "$REGION" --stack-name "$STACK"
echo "deleted stack $STACK"

for name in admin-key origin-verify; do
  if err="$(aws ssm delete-parameter --region "$REGION" --name "$PARAM_PREFIX/$name" 2>&1)"; then
    echo "deleted $PARAM_PREFIX/$name"
  elif [[ "$err" == *ParameterNotFound* ]]; then
    echo "skip $PARAM_PREFIX/$name (absent)"
  else
    die "delete $PARAM_PREFIX/$name failed: $err"
  fi
done
