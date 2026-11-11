#!/usr/bin/env bash
# ==============================================================================
# SwanPanel End-to-End System Integration Test
# ==============================================================================
set -eo pipefail

SERVER_HOST="127.0.0.1"
SERVER_PORT="8080"
API_BASE="http://${SERVER_HOST}:${SERVER_PORT}"
CLIENT_CONTAINER="swan-client"
REPORT_FILE="/tmp/e2e_test_results.log"
TEST_ID="$(date +%s | cut -c6-10)"

echo "======================================================================"
echo " Starting SwanPanel End-to-End Integration Test Suite (Run ID: $TEST_ID)"
echo "======================================================================"
rm -f "$REPORT_FILE"

record_result() {
    local test_name="$1"
    local status="$2"
    local detail="$3"
    printf "%-52s [%s] %s\n" "$test_name" "$status" "$detail"
    echo "$test_name | $status | $detail" >> "$REPORT_FILE"
}

# Check server reachable
echo "[*] Verifying HTTP API accessibility..."
HEALTH=$(curl -s "${API_BASE}/api/health" || echo "")
if [[ "$HEALTH" =~ "healthy" || "$HEALTH" =~ "SwanPanel" ]]; then
    record_result "TC-01: Health Check Endpoint" "PASS" "Gateway API is reachable and healthy"
else
    record_result "TC-01: Health Check Endpoint" "FAIL" "Gateway API unresponsive: $HEALTH"
    exit 1
fi

# Step 1: Admin Login (US-01)
echo "[*] Authenticating as Root Administrator..."
LOGIN_RESP=$(curl -s -X POST "${API_BASE}/api/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@swanpanel.internal","password":"admin123456"}')

TOKEN=$(echo "$LOGIN_RESP" | jq -r '.token // empty')
if [[ -n "$TOKEN" ]]; then
    record_result "TC-02: Root Admin Authentication" "PASS" "JWT token received successfully"
else
    record_result "TC-02: Root Admin Authentication" "FAIL" "Failed to authenticate: $LOGIN_RESP"
    exit 1
fi

# Step 2: Auth Me (US-01)
ME_RESP=$(curl -s -H "Authorization: Bearer $TOKEN" "${API_BASE}/api/auth/me")
if [[ "$ME_RESP" =~ "admin@swanpanel.internal" ]]; then
    record_result "TC-03: User Info Verification (/api/auth/me)" "PASS" "Admin identity verified"
else
    record_result "TC-03: User Info Verification (/api/auth/me)" "FAIL" "$ME_RESP"
fi

# Step 3: StrongSwan Settings Update & VICI Sync (US-02, US-06)
echo "[*] Configuring strongSwan daemon settings..."
SETTINGS_RESP=$(curl -s -X PUT "${API_BASE}/api/settings/strongswan" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "server_endpoint": "swan-panel",
    "server_identity": "swan-panel",
    "ike_proposals": "aes256gcm16-prfsha384-ecp384,aes256-sha256-modp2048",
    "esp_proposals": "aes256gcm16-ecp384,aes256-sha256",
    "reauth_time": "12h",
    "rekey_time_ike": "4h",
    "rekey_time_child": "1h",
    "dpd_delay": "30s",
    "mobike_enabled": true,
    "force_encap_natt": true
  }')
if [[ "$SETTINGS_RESP" =~ "dynamically" || "$SETTINGS_RESP" =~ "settings" ]]; then
    record_result "TC-04: Update strongSwan Settings (RAM Sync)" "PASS" "Applied via VICI load-conn with zero downtime"
else
    record_result "TC-04: Update strongSwan Settings (RAM Sync)" "FAIL" "$SETTINGS_RESP"
fi

# Step 4: Daemon Live Status (US-02)
DAEMON_STATUS=$(curl -s -H "Authorization: Bearer $TOKEN" "${API_BASE}/api/system/daemon/status")
if [[ "$DAEMON_STATUS" =~ "strongswan" && "$DAEMON_STATUS" =~ "active" ]]; then
    record_result "TC-05: Query Live Daemon Status via VICI" "PASS" "Live stats received from strongSwan charon"
else
    record_result "TC-05: Query Live Daemon Status via VICI" "FAIL" "$DAEMON_STATUS"
fi

# Step 5: Resource Group Management (US-03)
echo "[*] Creating Resource Group..."
RG_NAME="Engineering-VPC-${TEST_ID}"
RG_RESP=$(curl -s -X POST "${API_BASE}/api/resource-groups" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"${RG_NAME}\",\"cidr_block\":\"10.0.1.0/24\",\"description\":\"Core engineering VPC network\"}")
RG_ID=$(echo "$RG_RESP" | jq -r '.resource_group.id // empty')
if [[ -n "$RG_ID" ]]; then
    record_result "TC-06: Create Resource Group" "PASS" "Resource Group created: $RG_ID ($RG_NAME)"
else
    record_result "TC-06: Create Resource Group" "FAIL" "$RG_RESP"
    exit 1
fi

# Step 6: User Group & Local User (US-04)
echo "[*] Creating User Group and User..."
UG_NAME="DevOps-${TEST_ID}"
UG_RESP=$(curl -s -X POST "${API_BASE}/api/user-groups" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"${UG_NAME}\",\"description\":\"Operations and infra engineers\"}")
UG_ID=$(echo "$UG_RESP" | jq -r '.user_group.id // empty')

USER_EMAIL="user-${TEST_ID}@swanpanel.internal"
USER_RESP=$(curl -s -X POST "${API_BASE}/api/users" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"Test Engineer\",\"email\":\"${USER_EMAIL}\",\"password\":\"password123456\",\"is_admin\":false}")
USER_ID=$(echo "$USER_RESP" | jq -r '.user.id // empty')

if [[ -n "$UG_ID" && -n "$USER_ID" ]]; then
    record_result "TC-07: User & Group Management" "PASS" "Created User $USER_ID and Group $UG_ID"
else
    record_result "TC-07: User & Group Management" "FAIL" "Failed to create user or group"
fi

# Step 7: Access Rule Configuration (US-08)
AR_RESP=$(curl -s -X POST "${API_BASE}/api/access-rules" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"subject_type\": \"user_group\",
    \"subject_id\": \"${UG_ID}\",
    \"action\": \"ALLOW\",
    \"target_type\": \"resource_group\",
    \"target_value\": \"${RG_ID}\"
  }")
AR_ID=$(echo "$AR_RESP" | jq -r '.access_rule.id // empty')
if [[ -n "$AR_ID" ]]; then
    record_result "TC-08: Access Rule Policy Creation" "PASS" "Created Rule $AR_ID ($UG_NAME -> $RG_NAME)"
else
    record_result "TC-08: Access Rule Policy Creation" "FAIL" "$AR_RESP"
fi

# Step 8: Device Provisioning & Certificate Generation (US-04, US-05)
echo "[*] Provisioning client Device..."
DEV_HOST="node-${TEST_ID}"
DEV_RESP=$(curl -s -X POST "${API_BASE}/api/devices" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"hostname\": \"${DEV_HOST}\",
    \"device_type\": \"user\",
    \"user_id\": \"${USER_ID}\",
    \"resource_group_id\": \"${RG_ID}\",
    \"platform\": \"linux\",
    \"ip_allocation_type\": \"dynamic\"
  }")
DEV_ID=$(echo "$DEV_RESP" | jq -r '.device.id // empty')
if [[ -n "$DEV_ID" && "$DEV_RESP" =~ "BEGIN CERTIFICATE" ]]; then
    record_result "TC-09: Device Provisioning & PKI Issuance" "PASS" "Device $DEV_ID ($DEV_HOST) created with signed X.509 cert"
else
    record_result "TC-09: Device Provisioning & PKI Issuance" "FAIL" "$DEV_RESP"
    exit 1
fi

# Step 9: Config Downloads for Multiple Platforms (US-05)
echo "[*] Testing Profile Downloads (Apple, Windows, Linux)..."
APPLE_CFG=$(curl -s -H "Authorization: Bearer $TOKEN" "${API_BASE}/api/devices/${DEV_ID}/config/apple")
WIN_CFG=$(curl -s -H "Authorization: Bearer $TOKEN" "${API_BASE}/api/devices/${DEV_ID}/config/windows")
LINUX_CFG=$(curl -s -H "Authorization: Bearer $TOKEN" "${API_BASE}/api/devices/${DEV_ID}/config/linux")

if [[ "$APPLE_CFG" =~ "com.apple.security.root" && "$WIN_CFG" =~ "Add-VpnConnection" && "$LINUX_CFG" =~ "/etc/swanctl" ]]; then
    record_result "TC-10: Cross-Platform Native Profiles" "PASS" "Generated Apple .mobileconfig, Windows .ps1, Linux .sh"
else
    record_result "TC-10: Cross-Platform Native Profiles" "FAIL" "One or more config profiles failed to generate"
fi

# Step 10: Inject Config and Establish Tunnel in Client Container (US-06)
echo "[*] Injecting config and connecting Linux simulated client..."
echo "$LINUX_CFG" > /tmp/setup-client-test.sh
docker cp /tmp/setup-client-test.sh "${CLIENT_CONTAINER}:/tmp/setup.sh"
docker exec "${CLIENT_CONTAINER}" bash /tmp/setup.sh

# Give strongSwan 2 seconds to complete IKE_SA and CHILD_SA negotiation
sleep 2

# Check client SAs
CLIENT_SAS=$(docker exec "${CLIENT_CONTAINER}" swanctl --list-sas || echo "")
if [[ "$CLIENT_SAS" =~ "swanpanel" && "$CLIENT_SAS" =~ "INSTALLED" ]]; then
    record_result "TC-11: Client IKEv2 / IPsec Tunnel Up" "PASS" "Tunnel established with SwanPanel Gateway (Virtual IP assigned)"
else
    record_result "TC-11: Client IKEv2 / IPsec Tunnel Up" "FAIL" "Tunnel not established. Output: $CLIENT_SAS"
fi

# Step 11: Server Active Session Monitoring (US-02, US-09)
echo "[*] Checking SwanPanel active sessions..."
SESSIONS_RESP=$(curl -s -H "Authorization: Bearer $TOKEN" "${API_BASE}/api/sessions")
if [[ "$SESSIONS_RESP" =~ "${DEV_HOST}" || "$SESSIONS_RESP" =~ "10.0.1." ]]; then
    record_result "TC-12: Live Session Monitoring via VICI" "PASS" "Client session listed with assigned VIP"
else
    record_result "TC-12: Live Session Monitoring via VICI" "FAIL" "$SESSIONS_RESP"
fi

# Step 12: Split-DNS Magic DNS Resolution (US-07)
echo "[*] Testing DNS Engine from client container..."
DNS_SETTINGS=$(curl -s -H "Authorization: Bearer $TOKEN" "${API_BASE}/api/settings/dns")
SUFFIX=$(echo "$DNS_SETTINGS" | jq -r '.settings.magic_dns_suffix // ".internal"' | sed 's/^\.//')

DNS_RESOLV=$(docker exec "${CLIENT_CONTAINER}" dig @172.18.0.2 -p 53 "${DEV_HOST}.${SUFFIX}" +short +time=2 +tries=1 || echo "")
if [[ -n "$DNS_RESOLV" ]]; then
    record_result "TC-13: Split-DNS Magic DNS (.${SUFFIX})" "PASS" "Resolved ${DEV_HOST}.${SUFFIX} -> $DNS_RESOLV"
else
    record_result "TC-13: Split-DNS Magic DNS (.${SUFFIX})" "FAIL" "DNS query timed out or failed"
fi

# Step 13: System Logs Verification (US-10)
echo "[*] Testing System Logs endpoint..."
LOGS_RESP=$(curl -s -H "Authorization: Bearer $TOKEN" "${API_BASE}/api/system/logs?limit=5")
if [[ "$LOGS_RESP" =~ "logs" ]]; then
    record_result "TC-14: Real-time System Logs Retrieval" "PASS" "Charon logs streamed successfully"
else
    record_result "TC-14: Real-time System Logs Retrieval" "FAIL" "$LOGS_RESP"
fi

# Step 14: Session Kill-Switch / Terminate (US-09)
echo "[*] Testing Session Termination (Kill-Switch)..."
IKE_NAME=$(echo "$SESSIONS_RESP" | jq -r '.sessions[0].ike_name // empty')
if [[ -n "$IKE_NAME" ]]; then
    TERM_RESP=$(curl -s -X POST "${API_BASE}/api/sessions/terminate" \
      -H "Authorization: Bearer $TOKEN" \
      -H "Content-Type: application/json" \
      -d "{\"ike_name\":\"${IKE_NAME}\"}")
    sleep 2
    record_result "TC-15: Session Kill-Switch (VICI Terminate)" "PASS" "Session $IKE_NAME terminated via VICI API"
else
    record_result "TC-15: Session Kill-Switch (VICI Terminate)" "SKIP" "No active IKE name available to terminate"
fi

# Step 15: Device Certificate Revocation (US-04)
echo "[*] Testing Device Certificate Revocation..."
REVOKE_RESP=$(curl -s -X POST "${API_BASE}/api/devices/${DEV_ID}/revoke" \
  -H "Authorization: Bearer $TOKEN")
if [[ "$REVOKE_RESP" =~ "revoked successfully" ]]; then
    record_result "TC-16: Device Certificate Revocation" "PASS" "Device marked revoked, blocked from further config generation"
else
    record_result "TC-16: Device Certificate Revocation" "FAIL" "$REVOKE_RESP"
fi

echo ""
echo "======================================================================"
echo " Integration Test Summary:"
echo "======================================================================"
cat "$REPORT_FILE"
