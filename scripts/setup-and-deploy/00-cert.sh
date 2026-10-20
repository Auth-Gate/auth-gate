#!/usr/bin/env bash
# Optional: request an ACM certificate in us-east-1 (CloudFront needs it there)
# with DNS validation, then print the validation CNAME and the ARN.
#   00-cert.sh DOMAIN
set -euo pipefail
# shellcheck source=lib.sh
# shellcheck source-path=SCRIPTDIR
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

domain="${1:-}"
[ -n "$domain" ] || die "usage: 00-cert.sh DOMAIN"

arn="$(aws acm request-certificate --region us-east-1 \
  --domain-name "$domain" --validation-method DNS \
  --query CertificateArn --output text)"

# The validation record appears a few seconds after the request.
rec=""
for _ in 1 2 3 4 5 6 7 8 9 10; do
  rec="$(aws acm describe-certificate --region us-east-1 --certificate-arn "$arn" \
    --query 'Certificate.DomainValidationOptions[0].ResourceRecord.[Name,Value]' --output text)"
  [ "$rec" != "None" ] && [ -n "$rec" ] && break
  sleep 3
done
{ [ "$rec" != "None" ] && [ -n "$rec" ]; } || die "validation record for $arn not available yet; check the ACM console"

echo "Add this CNAME to the DNS of $domain (name, value):"
echo "  $rec"
echo "Certificate ARN (pass as CERTIFICATE_ARN to 03-deploy.sh):"
echo "  $arn"
