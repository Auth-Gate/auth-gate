#!/usr/bin/env bash
# Deploy stack auth-gate-$STAGE and print its one BaseUrl.
# Optional env: DOMAIN_NAME, CERTIFICATE_ARN, HOSTED_ZONE_ID.
#
# Without a custom domain, BASE_URL on verify must be the distribution's own
# domain, but the distribution depends on the Function URLs, which depend on the
# functions: referencing it would be a cycle. So the stack takes it as the
# DistributionBaseUrl parameter, read from the previous deploy's output. On the
# very first deploy it is unknown, so this script deploys twice.
set -euo pipefail
# shellcheck source=lib.sh
# shellcheck source-path=SCRIPTDIR
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

domain="${DOMAIN_NAME:-}"
cert="${CERTIFICATE_ARN:-}"
zone="${HOSTED_ZONE_ID:-}"
[ -z "$domain" ] || [ -n "$cert" ] || die "DOMAIN_NAME needs CERTIFICATE_ARN (00-cert.sh)"
[ -f "$ROOT/bin/bootstrap.zip" ] || die "bin/bootstrap.zip missing; run 02-build.sh"

output() {
  aws cloudformation describe-stacks --region "$REGION" --stack-name "$STACK" \
    --query "Stacks[0].Outputs[?OutputKey=='$1'].OutputValue" --output text 2>/dev/null || true
}

deploy() {
  sam deploy --template-file "$ROOT/infra/template.yaml" \
    --stack-name "$STACK" --region "$REGION" \
    --capabilities CAPABILITY_IAM --resolve-s3 \
    --no-confirm-changeset --no-fail-on-empty-changeset \
    --parameter-overrides \
    "Stage=$STAGE" "DomainName=$domain" "CertificateArn=$cert" \
    "HostedZoneId=$zone" "DistributionBaseUrl=$1"
}

dist_url=""
if [ -z "$domain" ]; then
  d="$(output DistributionDomain)"
  [ -z "$d" ] || [ "$d" = "None" ] || dist_url="https://$d"
fi

deploy "$dist_url"

if [ -z "$domain" ]; then
  d="$(output DistributionDomain)"
  if [ "https://$d" != "$dist_url" ]; then
    echo "second pass: setting BASE_URL to https://$d"
    deploy "https://$d"
  fi
fi

echo "BaseUrl: $(output BaseUrl)"
echo "RawVerifyUrl: $(output RawVerifyUrl)"
