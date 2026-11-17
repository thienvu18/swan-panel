#!/bin/sh
# Enforce complete statement coverage for every enumerated critical pathway.
# The input combines the full suite and the isolated first-boot CA-failure test.
# This is separate from total backend coverage and runtime Docker verification.
set -eu

cover_file="${1:-coverage.out}"
if [ ! -s "$cover_file" ]; then
  echo "Missing Go coverage profile: $cover_file" >&2
  exit 1
fi
summary="$(go tool cover -func="$cover_file")"
minimum=100
failed=0

for target in \
  'internal/api/handlers/auth.go:ChangePassword' \
  'internal/api/handlers/ca_settings.go:GetCAIdentity' \
  'internal/api/handlers/resources.go:ListDevices' \
  'internal/api/handlers/resources.go:DeleteDevice' \
  'internal/api/handlers/access.go:ListAccessRules' \
  'internal/api/handlers/access.go:DeleteAccessRule' \
  'internal/api/handlers/system.go:GetApplyStatus' \
  'internal/api/handlers/system.go:RetryApply' \
  'internal/api/handlers/resources.go:applyRevocation' \
  'internal/api/handlers/settings.go:UpdateStrongswanSettings' \
  'internal/api/handlers/ca_settings.go:RotateCAIdentity' \
  'internal/api/handlers/users.go:ToggleAdminRole' \
  'internal/api/handlers/users.go:DeleteUser' \
  'internal/api/handlers/users.go:DeleteUserGroup' \
  'internal/policy/gateway.go:ReconcileGateway' \
  'internal/policy/reconcile.go:Reconcile' \
  'internal/policy/reconcile.go:reconcileRuntime' \
  'internal/vici/client.go:GetDaemonStats' \
  'internal/pki/device.go:PublishCRL' \
  'internal/pki/pki.go:syncServerMaterial'
do
  file="${target%:*}"
  name="${target##*:}"
  percent="$(printf '%s\n' "$summary" | awk -v file="/$file:" -v name="$name" '
    index($1, file)>0 && $2==name {gsub("%", "", $3); print $3; exit}
  ')"
  if [ -z "$percent" ]; then
    printf 'FAIL %-45s not found in coverage profile\n' "$target" >&2
    failed=1
  elif ! awk -v value="$percent" -v min="$minimum" 'BEGIN {exit (value+0 < min)}'; then
    printf 'FAIL %-45s %s%% < %s%%\n' "$target" "$percent" "$minimum" >&2
    failed=1
  else
    printf 'PASS %-45s %s%%\n' "$target" "$percent"
  fi
done

exit "$failed"
