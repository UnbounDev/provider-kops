#!/bin/bash

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

if ! kubectl get secret aws -n crossplane-system; then

ssh-keygen -q -t ed25519 -C "email@example.com" -N "" -f "${SCRIPT_DIR}/id_ed25519" <<< y
ssh-keygen -f "${SCRIPT_DIR}/id_ed25519" -y > "${SCRIPT_DIR}/id_ed25519.pub"

kubectl create secret \
  generic aws \
  -n crossplane-system \
  "--from-file=creds=${SCRIPT_DIR}/aws-credentials.txt" \
  "--from-file=pubSshKey=${SCRIPT_DIR}/id_ed25519.pub"
fi

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: crossplane-system
---
apiVersion: kops.crossplane.io/v1alpha1
kind: ProviderConfig
metadata:
  name: aws-write
  namespace: crossplane-system
spec:
  credentials:
    source: Secret
    secretRef:
      namespace: crossplane-system
      name: aws
      key: creds
  pubSshKey:
    source: Secret
    secretRef:
      namespace: crossplane-system
      name: aws
      key: pubSshKey
EOF

