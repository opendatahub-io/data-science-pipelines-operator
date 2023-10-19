#!/usr/bin/env bash
set -ex
set -o pipefail

mkdir -p ./pr

cat <<EOF >> /tmp/body-file-raw.txt
${PR_BODY}
EOF

sed -n '/^```yaml/,/^```/ p' < /tmp/body-file-raw.txt | sed '/^```/ d' > ./pr/config.yaml
echo Parsed config from PR body:
yq ./pr/config.yaml

# Also store pr details
echo ${PR_NUMBER} >> ./pr/pr_number
echo ${PR_STATE} >> ./pr/pr_state
echo ${PR_HEAD_SHA} >> ./pr/head_sha
