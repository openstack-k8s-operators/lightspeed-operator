#!/usr/bin/env bash
# Commit the dev-latest digest pin update and open (or update) the PR.
#
# Required environment:
#   DIGEST       - full sha256:... digest to pin
#   SHORT_DIGEST - short digest used in the branch/PR title
#   GH_TOKEN     - token with contents and pull-requests write access

set -euo pipefail

branch="automation/bump-dev-latest-pin"
title="bump(dev-latest): advance pin to ${SHORT_DIGEST:?}"
printf -v body "This PR advances the repo-wide \`lightspeed-stack:dev-latest\` digest pin to \`%s\`." "${DIGEST:?}"

git config user.name "openstack-k8s-operators-ci"
git config user.email "openstack-k8s-operators-ci@github.com"
git switch -c "$branch"
git add -u
git commit -m "$title" -m "Advance the repo-wide lightspeed-stack:dev-latest digest pin to ${DIGEST}."
git push --force-with-lease --set-upstream origin "$branch"

existing_pr=$(gh pr list --head "$branch" --state open --json number --jq '.[0].number // empty')
if [[ -n "$existing_pr" ]]; then
  gh pr edit "$existing_pr" --title "$title" --body "$body" --add-label "ok-to-test"
else
  gh pr create --title "$title" --body "$body" --base main --head "$branch" --label "ok-to-test"
fi
