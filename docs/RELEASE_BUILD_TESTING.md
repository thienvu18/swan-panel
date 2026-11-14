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

## Remaining non-static validation

- Verify each binary runs on supported Linux distributions and remains statically linked.
- Exercise a real strongSwan VICI server and nftables rule installation on an isolated Linux gateway.
- Verify client provisioning imports on Windows, Linux, macOS/iOS and Android.
- Confirm the release job publishes the two binaries and a checksums manifest only after every test/build job passes.

Static inspection cannot replace compiling, linting, or executing tests against the real deployment OS.
