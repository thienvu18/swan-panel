#!/bin/sh
# Require completed checks for the exact tagged commit on main.
set -eu
git merge-base --is-ancestor "$COMMIT" origin/main || {
  echo "The tagged commit is not on main." >&2
  exit 1
}
for workflow in ci.yml codeql.yml; do
  runs="$(gh api --method GET "repos/$REPOSITORY/actions/workflows/$workflow/runs" -f branch=main -f head_sha="$COMMIT" -F per_page=100)"
  run="$(printf '%s' "$runs" | jq -c '[.workflow_runs[] | select(.event == "push" or .event == "workflow_dispatch")] | sort_by(.created_at, .id) | last // empty')"
  if [ -z "$run" ] || [ "$(printf '%s' "$run" | jq -r '.status + ":" + (.conclusion // "")')" != 'completed:success' ]; then
    echo "$workflow has no successful latest main run for $COMMIT; wait for CI or rerun it, then rerun release." >&2
    exit 1
  fi
  if [ "$workflow" = ci.yml ]; then
    run_id="$(printf '%s' "$run" | jq -r .id)"
    artifacts="$(gh api --method GET "repos/$REPOSITORY/actions/runs/$run_id/artifacts" -F per_page=100)"
    artifact_id="$(printf '%s' "$artifacts" | jq -r '[.artifacts[] | select(.name == "swan-panel-ui-dist")] | sort_by(.id) | last | select(.expired == false) | .id // empty')"
    if [ -z "$artifact_id" ]; then
      echo "The frontend artifact for CI run $run_id is missing or expired; refresh CI for this commit if available, or release a newer verified main commit." >&2
      exit 1
    fi
    echo "run_id=$run_id" >> "$GITHUB_OUTPUT"
    echo "artifact_id=$artifact_id" >> "$GITHUB_OUTPUT"
  fi
done
