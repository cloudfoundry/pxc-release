#!/usr/bin/env bash

set -e

AWS_ACCESS_KEY_ID=$(lpass show --notes 'dm-ci-secrets' | yq '.dedicated-mysql-service-account.access-key-id')
AWS_SECRET_ACCESS_KEY=$(lpass show --notes 'dm-ci-secrets' | yq '.dedicated-mysql-service-account.secret-access-key')
AWS_ASSUME_ROLE_ARN=$(lpass show --notes 'dm-ci-secrets' | yq '.dedicated-mysql-service-account.cf-core-services.role_arn')

cat > ./config/private.yml <<EOF
---
blobstore:
  provider: s3
  options:
    access_key_id: ${AWS_ACCESS_KEY_ID}
    secret_access_key: ${AWS_SECRET_ACCESS_KEY}
    assume_role_arn: ${AWS_ASSUME_ROLE_ARN}
EOF
