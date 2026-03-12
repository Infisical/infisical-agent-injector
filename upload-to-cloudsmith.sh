#!/usr/bin/env sh
set -e

cd helm
helm dependency update
helm package .

# Push to traditional Helm repository
for i in *.tgz; do
    [ -f "$i" ] || break
    cloudsmith push helm --republish infisical/helm-charts "$i"
done

# Push to OCI repository
echo "$CLOUDSMITH_API_KEY" | helm registry login helm.oci.cloudsmith.io \
    --username "$CLOUDSMITH_USERNAME" \
    --password-stdin

for i in *.tgz; do
    [ -f "$i" ] || break
    helm push "$i" oci://helm.oci.cloudsmith.io/infisical/helm-charts
done
