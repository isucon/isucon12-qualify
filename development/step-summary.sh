#!/usr/bin/env bash
set -u

PR_NUMBER=$(echo ${GITHUB_REF_NAME} | sed -e 's/\/merge//')
if [ "$(echo $PR_NUMBER | sed -e 's/[0-9]//g')" -ne "" ]; then
    exit 0
fi
PR_LINK=${GITHUB_SERVER_URL}/${GITHUB_REPOSITORY}/pull/${PR_NUMBER}
echo '### [PR \#${PR_NUMBER}](${PR_LINK})' >> $GITHUB_STEP_SUMMARY
