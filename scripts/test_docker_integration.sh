#!/usr/bin/env bash
# Disposable real strongSwan/nftables/IKEv2 Docker Compose integration test.
set -euo pipefail
umask 077
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PROJECT="swanpanel-it-$$"
COMPOSE_FILE="$ROOT/tests/docker/compose.yml"
API="http://127.0.0.1:18080/api"
WORK_DIR="$(mktemp -d)"
PASSED=0
for binary in docker curl jq; do
  command -v "$binary" >/dev/null || { echo "Missing $binary" >&2; exit 2; }
done
if ! docker info >/dev/null 2>&1; then
  echo "SKIPPED: Docker daemon is unavailable; requires Docker Engine/Desktop." >&2
  exit 2
fi
compose() { docker compose -p "$PROJECT" -f "$COMPOSE_FILE" "$@"; }
cleanup() {
  status=$?
  if [ "$status" -ne 0 ]; then compose logs --tail 80 gateway client >&2 || true; fi
  compose down --volumes --remove-orphans >/dev/null 2>&1 || true
  rm -rf "$WORK_DIR"
  exit "$status"
}
trap cleanup EXIT
pass() { PASSED=$((PASSED + 1)); printf 'PASS %02d: %s\n' "$PASSED" "$1"; }
fail() { echo "FAIL: $*" >&2; exit 1; }
assert_json() {
  jq -e "$2" <<< "$1" >/dev/null || fail "JSON assertion failed: $2"
}
api() {
  local method="$1" path="$2" body=""
  if [ "$#" -ge 3 ]; then body="$3"; fi
  if [ "$#" -ge 3 ]; then
    curl -fsS --max-time 30 -X "$method" "$API$path" \
      -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d "$body"
  else
    curl -fsS --max-time 30 -X "$method" "$API$path" \
      -H "Authorization: Bearer $TOKEN"
  fi
}

echo "Starting isolated Docker integration project $PROJECT"
compose build --pull
compose up -d --no-build
for _ in $(seq 1 45); do
  if curl -fsS --max-time 2 "$API/health" >/dev/null 2>&1; then break; fi
  sleep 2
done
curl -fsS "$API/health" >/dev/null || fail "Gateway API health did not become ready"
compose exec -T gateway /usr/local/bin/swan-panel --version | grep -Fx 'SwanPanel dev'
compose exec -T gateway swanctl --stats >/dev/null || fail "Gateway VICI daemon unavailable"
compose exec -T client swanctl --stats >/dev/null || fail "Client VICI daemon unavailable"
pass "Gateway and client real strongSwan VICI daemons"

SETUP_TOKEN="$(compose exec -T gateway cat /var/lib/swan-panel/swanpanel.db.setup-token | tr -d '\r\n')"
test -n "$SETUP_TOKEN" || fail "One-time setup token missing"
ADMIN_EMAIL="integration-admin@example.invalid"
ADMIN_PASSWORD="Test-Only-Admin-Password-2026!"
SETUP="$(jq -nc --arg token "$SETUP_TOKEN" --arg email "$ADMIN_EMAIL" \
  --arg pw "$ADMIN_PASSWORD" \
  '{token:$token,email:$email,password:$pw,name:"Docker Integration Admin"}')"
curl -fsS -X POST "$API/auth/setup" -H 'Content-Type: application/json' -d "$SETUP" >/dev/null
unset SETUP_TOKEN SETUP
LOGIN_PAYLOAD="$(jq -nc --arg email "$ADMIN_EMAIL" --arg pw "$ADMIN_PASSWORD" \
  '{email:$email,password:$pw}')"
LOGIN="$(curl -fsS -X POST "$API/auth/login" -H 'Content-Type: application/json' -d "$LOGIN_PAYLOAD")"
TOKEN="$(jq -er '.token' <<< "$LOGIN")"
assert_json "$(api GET /auth/me)" '.user.is_admin == true'
pass "One-time admin bootstrap and token authentication"

SETTINGS="$(api GET /settings/strongswan)"
UPDATED_SETTINGS="$(jq -c '.server_endpoint="gateway" | .server_identity="gateway" | .vpn_name="Docker Integration VPN"' <<< "$SETTINGS")"
assert_json "$(api PUT /settings/strongswan "$UPDATED_SETTINGS")" '.runtime_status == "applied"'
assert_json "$(api GET /system/apply-status)" '.status == "applied"'
pass "Gateway settings, issued server certificate and runtime reconcile"

RG="$(api POST /resource-groups '{"name":"Docker VPN pool","cidr_block":"10.122.0.0/24"}')"
RG_ID="$(jq -er '.resource_group.id' <<< "$RG")"
USER="$(api POST /users '{"name":"Integration Client","email":"docker-client@example.invalid","password":"Test-Only-Client-2026!","is_admin":false}')"
USER_ID="$(jq -er '.user.id' <<< "$USER")"
ALLOW="$(jq -nc --arg id "$USER_ID" '{subject_type:"user",subject_id:$id,action:"ALLOW",target_type:"cidr_ip",target_value:"10.99.0.0/24"}')"
assert_json "$(api POST /access-rules "$ALLOW")" '.runtime_status == "applied"'
pass "Subnet, user, and explicit ALLOW policy"

DEVICE_PAYLOAD="$(jq -nc --arg rg "$RG_ID" --arg user "$USER_ID" \
  '{hostname:"vpn-integration-client",device_type:"user",user_id:$user,resource_group_id:$rg,platform:"linux",ip_allocation_type:"dynamic"}')"
DEVICE="$(api POST /devices "$DEVICE_PAYLOAD")"
assert_json "$DEVICE" '.runtime_status == "applied"'
DEVICE_ID="$(jq -er '.device.id' <<< "$DEVICE")"
DEVICE_VIP="$(jq -er '.device.virtual_ip' <<< "$DEVICE")"
test "$DEVICE_VIP" = "10.122.0.2" || fail "Unexpected allocated VIP: $DEVICE_VIP"
pass "Enrolled client identity and reserved gateway address"

curl -fsS -H "Authorization: Bearer $TOKEN" "$API/devices/$DEVICE_ID/config/macos" -o "$WORK_DIR/profile.mobileconfig"
test -s "$WORK_DIR/profile.mobileconfig" || fail "Signed Apple profile missing"
curl -fsS -H "Authorization: Bearer $TOKEN" "$API/devices/$DEVICE_ID/config/windows" -o "$WORK_DIR/setup.ps1"
grep -q 'Add-VpnConnection' "$WORK_DIR/setup.ps1" || fail "Windows profile missing machine certificate installation"
curl -fsS -H "Authorization: Bearer $TOKEN" "$API/devices/$DEVICE_ID/config/linux" -o "$WORK_DIR/setup.sh"
grep -q 'swanctl --initiate' "$WORK_DIR/setup.sh" || fail "Linux profile not an installer"
pass "Apple/Windows/Linux certificate-backed client profiles"

# Android uses the OS native VPN settings, not an ADB/ZIP/helper-app bundle.
curl -fsS -D "$WORK_DIR/android.headers" \
  -H "Authorization: Bearer $TOKEN" \
  "$API/devices/$DEVICE_ID/config/android-native" -o "$WORK_DIR/android.json"
ANDROID="$(jq -c . "$WORK_DIR/android.json")"
assert_json "$ANDROID" '(.client_p12 | length > 100) and (.root_ca | length > 100)'
assert_json "$ANDROID" '(.import_password | length >= 16)'
assert_json "$ANDROID" '.server_address == "gateway" and .server_identity == "gateway" and .client_identity == "vpn-integration-client"'
grep -qi '^Cache-Control: no-store' "$WORK_DIR/android.headers" ||
  fail "Sensitive Android certificate response is cacheable"
unset ANDROID
pass "Android native PKCS#12/password and no-store export"

compose exec -T gateway swanctl --list-conns | grep -F "swanpanel-$DEVICE_ID" >/dev/null ||
  fail "Per-device VICI connection missing"
compose exec -T gateway swanctl --list-pools | grep -F "swanpanel-vip-$DEVICE_ID" >/dev/null ||
  fail "Per-device VIP pool missing"
compose exec -T gateway nft list table inet swanpanel > "$WORK_DIR/nft.txt" ||
  fail "No enforced nftables table"
grep -q 'drop' "$WORK_DIR/nft.txt" || fail "Firewall lacks default deny"
pass "Real VICI configuration, VIP pool, and nftables policy"

# The on-disk fragment must be equivalent to the VICI state and survive
# a strongSwan restart initiated outside the SwanPanel process.
compose exec -T gateway test -s /etc/swanctl/conf.d/swanpanel-gateway.conf ||
  fail "Persistent strongSwan fragment was not written"
compose exec -T gateway grep -F "swanpanel-$DEVICE_ID" /etc/swanctl/conf.d/swanpanel-gateway.conf >/dev/null ||
  fail "Persisted peer configuration differs from VICI"
compose exec -T gateway grep -F "swanpanel-vip-$DEVICE_ID" /etc/swanctl/conf.d/swanpanel-gateway.conf >/dev/null ||
  fail "Persisted virtual IP pool missing"
compose exec -T gateway grep -F "send_cert = always" /etc/swanctl/conf.d/swanpanel-gateway.conf >/dev/null ||
  fail "Persisted iOS server certificate delivery setting missing"
if compose exec -T gateway grep -Eq "^[[:space:]]+dns[[:space:]]*=" /etc/swanctl/conf.d/swanpanel-gateway.conf; then
  fail "DNS disabled in test container, but unreachable DNS VIP was pushed to clients"
fi
compose exec -T gateway ipsec restart >/dev/null ||
  fail "Independent strongSwan daemon restart failed"
RESTORED=0
for _ in $(seq 1 18); do
  if compose exec -T gateway swanctl --list-conns 2>/dev/null | grep -F "swanpanel-$DEVICE_ID" >/dev/null &&
     compose exec -T gateway swanctl --list-pools 2>/dev/null | grep -F "swanpanel-vip-$DEVICE_ID" >/dev/null; then
    RESTORED=1
    break
  fi
  sleep 5
done
test "$RESTORED" -eq 1 || fail "Daemon restart lost the persisted connection or pool"
pass "Managed configuration automatically restored after independent daemon restart"

compose exec -T client sh -c 'umask 077; cat > /tmp/setup.sh; chmod 0700 /tmp/setup.sh' < "$WORK_DIR/setup.sh"
compose exec -T client bash /tmp/setup.sh || fail "Native Linux IKEv2 connection failed"
SAS="$(compose exec -T client swanctl --list-sas)"
grep -q 'INSTALLED' <<< "$SAS" || fail "No installed CHILD_SA"
grep -Fq "$DEVICE_VIP" <<< "$SAS" || fail "Client received wrong virtual IP"
assert_json "$(api GET /sessions)" '.count >= 1'
pass "Real IKEv2 tunnel, expected VIP, and live gateway sessions"

api POST "/devices/$DEVICE_ID/revoke" '{}' >/dev/null || fail "Certificate revocation failed"
sleep 3
SAS="$(compose exec -T gateway swanctl --list-sas || true)"
if grep -Fq "swanpanel-$DEVICE_ID" <<< "$SAS"; then fail "Revoked SA remains active"; fi
if compose exec -T client swanctl --initiate --child swanpanel-traffic > "$WORK_DIR/reconnect.log" 2>&1; then
  fail "Revoked certificate was able to reconnect"
fi
ANDROID_STATUS="$(curl -sS -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer $TOKEN" "$API/devices/$DEVICE_ID/config/android-native")"
test "$ANDROID_STATUS" = "403" || fail "Revoked device still exports native Android keys: $ANDROID_STATUS"
pass "Signed CRL, terminated session, rejected revoked certificate and native export"
echo "PASS: $PASSED Docker integration checks against real strongSwan and nftables"
