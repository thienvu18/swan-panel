# SwanPanel — Security and provisioning regression tests

> Scope: one-time administrator setup, verified OIDC authentication, Zero-Trust policy enforcement, certificate revocation, client provisioning and S/MIME-signed Apple configuration profiles.
>
> Clean-install only. No legacy account/password migration or backwards-compatibility flows.

## Run on a developer workstation

```bash
cd /path/to/swan-panel
go mod tidy
gofmt -w $(find cmd internal -name '*.go')
CGO_ENABLED=1 go vet ./...
CGO_ENABLED=1 go test -count=1 -race ./...
CGO_ENABLED=1 go build ./cmd/server
cd ui
npm ci
npm test
npm run build
```

Go tests use native CGO SQLite on isolated SQLite files and test-mode offline data-plane. The SQLite regression suite covers WAL, foreign keys, busy timeout, rollback, unique constraints, and persistence after reopening. They **do not prove that nftables or strongSwan has applied rules**. Do not run against a production gateway. Go `go.mod`/new `pkcs7` dependency must be synchronized with `go.sum` using `go mod tidy`.

## Automated cases

| ID | Scenario | Test / assertion |
|---|---|---|
| SET-01 | First run starts with zero users and no default password | `internal/db/bootstrap_test.go:TestOneTimeBootstrap` |
| SET-02 | Token is random 256-bit value in a 0600 host-only file | Same |
| SET-03 | Wrong token, short password, empty name/email do not consume token | Same |
| SET-04 | Correct token creates exactly one ACTIVE local admin; subsequent replay rejected | Same |
| SET-05 | Token stable across restart before setup | `TestBootstrapTokenStableAcrossRestart` |
| SET-06 | Concurrent setup race creates exactly one administrator | `TestConcurrentBootstrapSingleWinner` |
| SET-07 | Missing token file fails closed rather than regenerating token | `TestBootstrapMissingTokenFileFailsClosed` |
| SET-08 | API login disabled until setup, then setup closes | `internal/api/security_regression_test.go:TestBootstrapAPIBlocksUntilAdminCreated` |
| OIDC-01 | Default Google, Microsoft Entra ID, Keycloak discovery configs | `internal/api/handlers/oidc_flow_test.go:TestOIDCConfigurationValidation` |
| OIDC-02 | Custom HTTPS authorization/token/JWKS/UserInfo endpoints; JSON claim mapping | Same and `TestOIDCAuthorizationCodeLifecycle` |
| OIDC-03 | Invalid issuer URL, insecure endpoints, partial custom endpoint sets, arbitrary admin mapping rejected | `TestOIDCConfigurationValidation` |
| OIDC-04 | Redirect contains code flow, S256 PKCE, state and nonce | `TestOIDCAuthorizationCodeLifecycle` |
| OIDC-05 | Callback without same-browser state cookie or mismatched state rejected | Same |
| OIDC-06 | RS256 ID token and JWKS exchange via TLS test provider | Same |
| OIDC-07 | Unknown SSO identity created PENDING and not made admin; approval required | Same |
| OIDC-08 | Approved identity exchanges single-use ticket for internal JWT | Same |
| OIDC-09 | Replayed callback state or ticket rejected | Same and `TestOIDCStateReplayIsRejectedEvenIfUserExists` |
| OIDC-10 | Wrong nonce, issuer, audience, expired token, bad signature rejected | `TestOIDCRejectsInvalidSignedTokens`, `TestOIDCRejectsWrongAudience` |
| OIDC-11 | Explicitly unverified email and account email collision rejected | `TestOIDCRejectsInvalidSignedTokens`, `TestOIDCRejectsEmailCollision` |
| OIDC-12 | Custom provider CRUD, disable, no secret exposure, linked issuer cannot be changed or deleted | `internal/api/security_regression_test.go:TestCustomOIDCProviderCRUDAndNoSecretLeak` |
| AUTH-01 | JWT never accepted as URL query token, including SSE | `TestNeverAcceptQueryBearerToken`, router tests |
| AUTH-02 | Role changes and suspension affect existing tokens immediately | `TestJWTPrivilegeAndStatusReadFromDB` |
| POL-01 | No implicit access when device joins a resource group | `internal/policy/zero_trust_test.go:TestDefaultDenyWithoutExplicitGrant` |
| POL-02 | Device, direct user and group ALLOW selectors | `internal/policy/compiler_test.go:TestPolicyCompiler`, `security_cases_test.go` |
| POL-03 | DENY for a specific host/subnet precedes broad ALLOW | `security_cases_test.go:TestPolicyAuthorizationCases`, firewall tests |
| POL-04 | Suspended owner, revoked device, invalid or missing target fail closed | `security_cases_test.go`, `zero_trust_test.go` |
| POL-05 | Gateway input blocks management, allows DNS only for active device; disabled DNS blocked | `security_cases_test.go` |
| REV-01 | Signed CRL has key-compromise serials, validity and verifying chain | `internal/pki/client_profile_test.go`, `revocation_cases_test.go` |
| REV-02 | Malformed/negative CRL serials rejected | `revocation_cases_test.go:TestCRLRejectsMalformedSerial` |
| REV-03 | Revoke persists ledger, prevents further downloads, deletes private-key export | `internal/api/security_regression_test.go:TestSignedAppleProfileStableIdentityAndRevocation` |
| PROF-01 | Device identity and private key remain unchanged on repeated downloads | `client_profile_test.go`, `TestSignedAppleProfileStableIdentityAndRevocation` |
| PROF-02 | Apple profile has embedded PKCS#12 and native IKEv2 payload | `client_profile_test.go`, `pki_test.go` |
| PROF-03 | Apple .mobileconfig is attached DER CMS/S/MIME, SHA-256 signer is CA-issued leaf | `internal/pki/profile_signing_test.go:TestAppleMobileconfigSMIMESignedData` |
| PROF-04 | Tampering breaks CMS verification; signer reused and its key is 0600 | `TestAppleSignedProfileDetectsModification`, `TestAppleProfileSignerPersistsAndIsPrivate` |
| PROF-05 | Empty content is not signed; untrusted CA cannot verify profile | `TestAppleProfileSigningRejectsInvalidContent`, `TestAppleProfileUntrustedSignerDoesNotVerify` |
| PROF-06 | Windows imports CA and actual PFX private key | `client_profile_test.go:TestPersistedIdentityAndNativeProfiles` |
| PROF-06a | Stored identity cannot be overwritten/tampered; bad JSON rejected | `TestTamperedStoredIdentityFailsClosed` |
| PROF-06b | PKCS#12 rejects wrong password and preserves certificate serial | `TestPKCS12WrongPasswordRejected` |
| PROF-07 | Legacy Android strongSwan `.sswan` remains a compatibility-only API export, not the primary UI path | `client_profile_test.go` |
| PROF-08 | Android native API returns a paired PKCS#12/password with CA and IKE identities; no ZIP, ADB or helper app | `internal/api/android_native_endpoint_test.go`, `internal/pki/client_profile_test.go`, `ui/tests/androidNative.test.mjs` |
| PROFILE-09 | Gateway identifiers and VICI proposals reject unsafe characters | `internal/api/handlers/gateway_validation_test.go` |
| UI-01 | SSE frames including multiline/CRLF; corrupted JSON/comment ignored | `ui/tests/transportSecurity.test.mjs` |
| UI-02 | OIDC provider IDs encoded in URL; JWT not placed in URL | Same |
| UI-03 | Profile download filename cannot inject directories/control characters | Same |

## Manual integration cases (must run on a disposable Linux VM)

Unit/static tests do **not** validate the actual OS/network data plane. An isolated Linux VM with nftables, `swanctl` and VICI is required.

1. **NET-01**: On startup, `nft list table inet swanpanel` contains the expected chains. Apply a DENY-only configuration; verify packets from an established client **cannot reach any protected target**, including gateway HTTP management. Verify legitimate DNS requests can reach the configured DNS port only if DNS is enabled.
2. **NET-02**: Add an ALLOW to a chosen CIDR. Verify only that CIDR becomes reachable. Add a more specific DENY and prove it overrides the ALLOW. Remove the DENY and verify reconciliation.
3. **NET-03**: Enroll two devices; inspect VICI connections and pools with `swanctl --list-conns` and `swanctl --list-pools`. Their certificates, IKE identities and VIPs must be distinct and each device receives only its own VIP.
4. **NET-04**: Revoke an actively connected device. Verify `swanctl --list-sas` no longer reports its SA and `swanctl --load-creds` accepts the CRL. Reattempt IKEv2 using the old certificate; handshake must fail, even after strongSwan restarts.
5. **NET-05**: Stop VICI or deny nftables permissions while saving a policy. API must return pending/degraded and never claim applied. Restore runtime and verify reconciliation.
6. **OS-01**: Import signed `.mobileconfig` on macOS and iOS; confirm the downloaded file is CMS DER, private identity installed and IKEv2 connects. Verify signature with `openssl cms -verify -inform DER -in device.mobileconfig -CAfile ca.crt -out profile.plist`. Signer trust on the device must be configured out-of-band, not inferred from embedded CA.
7. **OS-02**: Windows (PowerShell admin) imports Root CA/PFX and connects with machine certificate; only granted routes traverse VPN.
8. **OS-03**: Linux `swanctl` script imports the issued key/cert and successfully connects; route policy remains restricted.
9. **OS-04**: On Android with built-in IKEv2/IPsec RSA, download `.p12` from SwanPanel's native setup dialog, import to OS KeyChain with the paired password, and create a VPN using server/identity fields; no third-party app, ADB or ZIP. Record unsupported OEMs separately. See A01 in `VPN_GATEWAY_ACCEPTANCE_TEST_PLAN.md`.

## CI and scope

GitHub Actions runs `npm test`, frontend build, `go vet ./...`, `go test -race ./...`, Go build and Docker build. Developer-machine Go tests, even if they all pass, are **not** a substitute for NET-01 through NET-05.

Automated checks do not prove live VPN connectivity or firewall enforcement. Run the repeatable hardware and network checks in [VPN Gateway Acceptance Test Plan](VPN_GATEWAY_ACCEPTANCE_TEST_PLAN.md) on the exact build under test. Preserve the one-time setup flow; do not introduce a default administrator password.
