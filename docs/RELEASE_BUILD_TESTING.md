# SwanPanel — Native SQLite and Release Build Verification

SwanPanel gateway binaries target **Linux AMD64 and ARM64**. SQLite is integrated through the official `gorm.io/driver/sqlite` dialector using CGO. GitHub Actions builds native Linux binaries with musl static linking.

## Developer checks (macOS with Xcode Command Line Tools)

Run after editing Go files. These commands validate syntax, formatting, dependency hygiene, application behavior, and linters.

```sh
cd /Users/thienvu/Documents/Projects/Mines/swan-panel

go mod tidy
gofmt -w embed.go embed_test.go $(find cmd internal -name '*.go')

sh -n scripts/build-linux-static.sh
sh -n scripts/test-release-config.sh
sh -n scripts/check-descriptive-test-names.sh
sh scripts/check-dependencies.sh
sh scripts/check-descriptive-test-names.sh
sh scripts/test-release-config.sh
go mod tidy -diff

CGO_ENABLED=1 go vet ./...
CGO_ENABLED=1 golangci-lint run ./...
CGO_ENABLED=1 go test -race -count=1 ./...
CGO_ENABLED=1 go build ./cmd/server

cd ui
npm ci
npm test
npm run build
```

Use `golangci-lint run ./...` without `--fix` when checking whether a change passes. Use `--fix` only when you intentionally want the tool to modify source files; inspect the diff afterwards.

## Automated regressions

| Behavior | Automated test or static check |
|---|---|
| Missing/stub frontend HTML or JavaScript blocks production startup | `embed_test.go:TestFrontendAssetsRequireBuiltJavaScriptAndHTML` and `cmd/server/main.go` |
| CI/release verifies the actual embedded UI after the Vite build | `embed_test.go:TestEmbeddedFrontendContainsProductionBuild` with `SWANPANEL_REQUIRE_BUILT_UI=1` |
| Native SQLite must enable WAL, foreign keys and busy timeout | `internal/db/sqlite_driver_test.go:TestSQLiteNativeDriverPragmasTransactionsAndPersistence` |
| Database unique constraint, rollback and persistence after reopening | Same test |
| SQLite startup refuses unsupported in-memory WAL configuration | `TestSQLiteRejectsInMemoryDatabaseWithoutWAL` |
| SQLite transactions serialize concurrent writes on the configured connection pool | `TestSQLiteNativeDriverConcurrentWritesAreSerialized` |
| Private identity key is written atomically with `0600` permission | `internal/pki/atomic_private_file_test.go:TestAtomicPrivateFilePreservesPrivatePermissions` |
| Temporary private-key file is removed when atomic rename fails | `TestAtomicPrivateFileCleansUpFailedRename` |
| No missing VICI daemon is reported as a successful operation | `internal/vici/client_test.go:TestVICIClientFailClosedWithoutSocket` |
| New VICI streaming API and command API have no deprecated invocations | `golangci-lint` staticcheck and Go compilation |
| OIDC tests close database connection and refresh fake provider state safely | `internal/api/handlers/oidc_flow_test.go` + errcheck |
| Regression test names and files describe behavior, not project phases | `scripts/check-descriptive-test-names.sh` |
| No retired SQLite dependency in `go.mod`/`go.sum` | `scripts/test-release-config.sh` |
| Native Linux AMD64/ARM64 release, static ELF and checksum manifest | `scripts/test-release-config.sh`; actual release runner builds |
| Installer shows setup-token instructions rather than default credentials | `scripts/test-release-config.sh` |

Existing authentication, policy, certificate revocation and platform profile tests are indexed in `docs/SECURITY_REGRESSION_TEST_PLAN.md`.

## Native Linux release build (Linux runner with musl-tools)

```sh
sudo apt-get update
sudo apt-get install -y --no-install-recommends build-essential musl-tools binutils file
cd /path/to/swan-panel
(cd ui && npm ci && npm test && npm run build)
CGO_ENABLED=1 go test -race -count=1 ./...
VERSION=dev sh scripts/build-linux-static.sh
file dist/swan-panel-linux-$(go env GOARCH)
readelf -l dist/swan-panel-linux-$(go env GOARCH)
dist/swan-panel-linux-$(go env GOARCH) --version
```

The script refuses mismatched host/C compiler architecture. GitHub Actions uses native `ubuntu-24.04` (AMD64) and `ubuntu-24.04-arm` (ARM64) runners. Production Docker builds use musl/CGO in a separate builder stage, then copy only the binary to the runtime image.

## Manual VPN / UI acceptance test handoff

For testers on disposable Linux VMs and real client devices, follow
[VPN_GATEWAY_ACCEPTANCE_TEST_PLAN.md](VPN_GATEWAY_ACCEPTANCE_TEST_PLAN.md).
The guide contains prerequisites, numbered commands, expected results,
evidence requirements, and PASS/FAIL/BLOCKED status tables for actual
IKEv2/XFRM, packet-level nftables, certificate revocation, daemon
restarts, native systemd, UI confirmations/focus, and platform profiles.

Release tags now run the reusable Docker IKEv2 integration workflow
before any Linux binary is built; unavailable kernel XFRM or a failed
VPN test blocks publication rather than being treated as a passing skip.

## Remaining non-static validation

- Verify each binary runs on supported Linux distributions and remains statically linked.
- Exercise a real strongSwan VICI server and nftables rule installation on an isolated Linux gateway.
- Verify client provisioning imports on Windows, Linux, macOS/iOS and Android.
- Confirm the release job publishes the two binaries and a checksums manifest only after every test/build job passes.

Static inspection cannot replace compiling, linting, or executing tests against the real deployment OS.

## Go integration coverage threshold (minimum 80%)

Run the full backend suite with cross-package instrumentation:

    APP_ENV=test CGO_ENABLED=1 go test -count=1 -coverpkg=./... -coverprofile=/tmp/swanpanel-cover.out ./...
    go tool cover -func=/tmp/swanpanel-cover.out | tail -1

The total is statement coverage across all Go packages, including the
production backend. Use -coverpkg=./... because the HTTP integration tests
are in internal/api while the handlers under test are in
internal/api/handlers. A plain go test -cover ./... reports
package-local coverage and will give materially different numbers; it is not
the metric enforced by CI. Do not exclude production files/packages or
artificially mock away security validation to satisfy the threshold.

CI's Enforce 80 percent Go integration coverage job fails if the total is
below 80%. Also run, with the intended Go version and CGO enabled:

    APP_ENV=test CGO_ENABLED=1 go test -count=1 -race -shuffle=on ./...
    golangci-lint run ./...
    go vet ./...

Tests added for this gate cover failed VICI commands, live UNIX VICI protocol
framing, SA ownership, revocation, malformed/corrupt CA and device credentials,
CA rotation/profile-signing renewal, zero-trust IP restrictions, JSON
validation, middleware rate-limit isolation, SSO provider restrictions,
administration transactions, SSE disconnects and DNS client routing.

The Go unit/integration coverage gate does not prove Linux kernel IPsec
connectivity, installed nftables policy, or active VPN tunnels. Run the
separate Docker strongSwan integration suite for those checks when a Docker
daemon with sufficient kernel privileges is available.

## Critical operation coverage gate (100% per function)

The cross-package backend total remains at least 80%. The listed twenty
critical operation functions must each reach **100% statement coverage**.
First-boot CA initialization errors require a separate test process because
the CA manager is a `sync.Once` singleton; CI merges that coverage with the
full regression suite before checking individual functions:

    SWANPANEL_TEST_ROOT_CA_UNAVAILABLE=1 APP_ENV=test CGO_ENABLED=1 go test -count=1 -coverpkg=./... -coverprofile=/tmp/swanpanel-ca-failure.out -run '^TestUnavailableRootCAFailsClosedAcrossControlPlaneOperations$' ./internal/api
    awk 'NR==FNR {print;next} FNR>1 {print}' /tmp/swanpanel-cover.out /tmp/swanpanel-ca-failure.out > /tmp/swanpanel-critical.out
    sh scripts/check-p1-critical-coverage.sh /tmp/swanpanel-critical.out

This validates statement execution, not exhaustive branch/path combinations.
The critical functions include authentication, reconciliation, credential
publication, CA rotation, administrator/user transactions, access rules,
device lifecycle and revocation, settings, apply state and live VICI status.
A good total coverage score alone cannot pass the per-operation gate.

Go tests may simulate the VICI UNIX protocol and nft executable for safe,
deterministic failure-path coverage. They do not prove real kernel XFRM or
nftables behavior: the separate Docker strongSwan integration suite remains
a release prerequisite.

Custom strongSwan installations may specify `SWANPANEL_SWANCTL_DIR`
(default `/etc/swanctl`) for deployed CA/server certificates and CRL;
`SWANPANEL_SWANCTL_CRL_DIR` overrides only the CRL destination.
`SWANPANEL_CHARON_PID_FILE` permits nonstandard daemon PID paths.

The native systemd unit retains `ProtectSystem=full`, with a narrow
`ReadWritePaths=/etc/swanctl` exception so the control plane can persist
managed certificates, CRLs and connections. Custom swanctl locations outside
that directory require an explicit systemd `ReadWritePaths` override.

Frontend confirmation and select tests currently inspect source invariants;
they are not interactive browser tests. Before a production-ready claim,
verify those dialogs and keyboard interactions in a browser, plus the Docker
IKEv2/revocation/restart and native systemd deployment checks.
