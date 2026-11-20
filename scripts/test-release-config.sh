#!/bin/sh
# Regression tests for the SQLite/CGO release contract.
# Read-only and runnable on macOS/Linux without Go, GCC or Docker.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

assert_contains() {
    file=$1
    needle=$2
    message=$3
    if ! grep -Fq -- "$needle" "$file"; then
        printf >&2 'FAIL %s (%s missing %s)\n' "$message" "$file" "$needle"
        exit 1
    fi
}
assert_absent() {
    file=$1
    needle=$2
    message=$3
    if grep -Fq -- "$needle" "$file"; then
        printf >&2 'FAIL %s (%s contains %s)\n' "$message" "$file" "$needle"
        exit 1
    fi
}
assert_present() {
    file=$1
    message=$2
    if [ ! -s "$file" ]; then
        printf >&2 'FAIL %s (%s missing/empty)\n' "$message" "$file"
        exit 1
    fi
}

assert_contains go.mod 'gorm.io/driver/sqlite' 'native GORM SQLite must be selected'
assert_absent go.mod 'github.com/glebarez' 'old SQLite driver must be removed'
assert_absent go.mod 'modernc.org/' 'old pure-Go SQLite dependencies must be pruned'
assert_absent go.sum 'github.com/glebarez' 'obsolete SQLite checksums must be removed'
assert_absent go.sum 'modernc.org/' 'obsolete pure-Go SQLite checksums must be removed'
if ! awk '
    $1 == "go" {
        split($2, v, ".")
        if (v[1] > 1 || (v[1] == 1 && (v[2] > 27 || (v[2] == 27 && v[3] >= 4)))) ok = 1
    }
    END { exit !ok }
' go.mod; then
    printf >&2 'FAIL: Go 1.27.4 or later required for CGO security fixes\n'
    exit 1
fi
assert_contains internal/db/db.go '"gorm.io/driver/sqlite"' 'DB must import the official driver'
assert_contains internal/db/db.go 'PRAGMA journal_mode = WAL' 'WAL must be configured'
assert_contains internal/db/db.go 'PRAGMA foreign_keys = ON' 'foreign keys must be enabled'
assert_contains internal/db/db.go 'sqlDB.SetMaxOpenConns(1)' 'SQLite pragma pool affinity required'
assert_contains internal/db/sqlite_driver_test.go 'TestSQLiteNativeDriverPragmasTransactionsAndPersistence' 'native SQLite tests must exist'
assert_contains internal/db/sqlite_driver_test.go 'TestSQLiteRejectsInMemoryDatabaseWithoutWAL' 'WAL fail-closed test must exist'

assert_contains .github/workflows/ci.yml 'CGO_ENABLED: "1"' 'CI must compile with CGO'
assert_contains .github/workflows/ci.yml 'sh scripts/build-linux-static.sh' 'CI must validate native static linking'
assert_contains .github/workflows/release.yml 'ubuntu-24.04-arm' 'native ARM64 release runner required'
assert_contains .github/workflows/release.yml 'needs: build' 'release publication must depend on both binaries'
assert_contains .github/workflows/release.yml 'needs: verify' 'release binaries must require successful main CI'
assert_contains .github/workflows/release.yml 'sh scripts/verify-release-ci.sh' 'release must validate the tagged commit CI runs'
assert_contains scripts/verify-release-ci.sh 'actions/workflows/$workflow/runs' 'release gate must query CI workflows'
assert_contains .github/workflows/ci.yml 'uses: ./.github/workflows/docker-integration.yml' 'pull-request CI must also require real IKEv2 integration'
assert_contains .github/workflows/docker-integration.yml 'workflow_call:' 'Docker IKEv2 tests must be reusable by the release gate'
assert_contains .github/workflows/release.yml 'sh scripts/build-linux-static.sh' 'shared native release build script required'
assert_contains .github/workflows/release.yml 'swan-panel-linux-amd64' 'AMD64 release required'
assert_contains .github/workflows/release.yml 'swan-panel-linux-arm64' 'ARM64 release required'
assert_contains .github/workflows/release.yml 'checksums.txt' 'release checksum manifest required'
assert_contains .github/workflows/ci.yml 'go mod tidy -diff' 'untidied Go dependencies must block CI'

assert_contains scripts/build-linux-static.sh 'CGO_ENABLED=1' 'native CGO build required'
assert_contains scripts/build-linux-static.sh 'CC=musl-gcc' 'native static musl compiler required'
assert_contains scripts/build-linux-static.sh 'sqlite_omit_load_extension' 'SQLite extension loading disabled'
assert_contains scripts/build-linux-static.sh 'readelf -l' 'ELF dynamic loader must be checked'
assert_contains scripts/build-linux-static.sh 'statically linked' 'static ELF link must be enforced'
assert_contains Dockerfile 'CGO_ENABLED=1 CC=gcc GOOS=linux' 'Docker native CGO required'
assert_contains Dockerfile "extldflags '-static'" 'Docker static link required'
assert_absent Dockerfile '    ip6tables \' 'Alpine 3.23 does not have a standalone ip6tables package'

assert_contains install.sh 'sha256sum' 'installer must verify release checksum'
assert_contains install.sh 'Setup Token File:' 'installer must describe first-run token'
assert_absent install.sh 'admin123456' 'installer must not advertise default admin password'
assert_absent install.sh 'Default Password:' 'installer must not advertise a default password'
assert_absent install.sh 'v0.0.2 download' 'installer must not fall back to outdated release'

assert_contains embed.go 'func validateFrontendAssets' 'runtime must verify embedded production frontend'
assert_contains embed.go 'SWANPANEL_DEV_UI_PLACEHOLDER' 'release must reject development HTML placeholder'
assert_contains embed.go 'assets/*.js' 'release must require frontend JavaScript bundle'
assert_contains cmd/server/main.go 'Production frontend bundle unavailable; refusing to start' 'production must fail closed when UI is missing'
assert_contains Dockerfile "find ui/dist/assets -type f -name '*.js'" 'production image must verify React JavaScript assets'
assert_contains .github/workflows/release.yml 'SWANPANEL_REQUIRE_BUILT_UI' 'ARM64 tests must verify real embedded frontend'
assert_contains .github/workflows/ci.yml 'SWANPANEL_REQUIRE_BUILT_UI' 'CI tests must verify embedded production frontend'
assert_contains deploy/docker/docker-entrypoint.sh '/etc/swanctl/x509crl' 'production Docker must prepare CRL directory'
assert_contains install.sh '/etc/swanctl/x509crl' 'native installer must prepare CRL directory'
assert_contains install.sh 'charon-systemd' 'Debian/Ubuntu installer must install VICI-enabled charon daemon'
assert_contains install.sh 'strongswan-swanctl' 'Debian/Ubuntu installer must install swanctl'
assert_contains install.sh 'strongswan-libcharon' 'Debian package containing socket-default plugin is required'
assert_contains install.sh 'libstrongswan-standard-plugins' 'standard crypto plugin package is required'
assert_contains deploy/docker/docker-compose.host-swan.yml 'ports: !override []' 'host daemon must not inherit published IKE ports'
assert_contains deploy/docker/docker-compose.host-swan.yml 'network_mode: host' 'host xfrm and nftables must use the same namespace'
assert_contains deploy/docker/docker-entrypoint.sh 'SWANPANEL_EXTERNAL_STRONGSWAN' 'external daemon startup must be protected'
assert_absent install.sh 'strongswan-charon-systemd' 'invalid distro package name must not return'
assert_contains install.sh 'command -v swanctl' 'native installer must verify swanctl availability'
assert_contains install.sh 'ExecStartPost=-${SWANCTL_BIN} --load-all --noprompt' 'native strongSwan restarts must load persisted connections'
assert_contains install.sh 'include conf.d/*.conf' 'native strongSwan must include managed connection fragment'
assert_contains deploy/systemd/swan-panel.service 'ReadWritePaths=/etc/swanctl' 'hardened systemd unit must permit managed strongSwan configuration writes'
assert_contains .dockerignore '.env' 'production Docker context must exclude local environment files'

printf 'PASS: SQLite/CGO release configuration checks\n'
