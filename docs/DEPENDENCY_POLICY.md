# SwanPanel — Dependency policy

**Goal:** minimize the total supply-chain surface, not just the number of `require` lines. Choose the Go standard library first; prefer an *already-approved dependency* when it provides the same behavior safely. A new dependency must justify why it is safer/cheaper than maintaining the equivalent code and quantify what it adds transitively.

## Rules for introducing or replacing a library

1. **stdlib first.** Do not pull in a module for parsing URLs, encoding JSON/XML, cryptographic randomness, hashing, X.509 certificate issuance/CRL, HTTP requests, or simple transformations already handled well by Go stdlib.
2. **Consolidate when sensible.** Prefer an existing, maintained, multipurpose library over another dependency when it correctly implements the new task. Do **not** pick a huge multipurpose framework if its transitive graph or attack surface is larger than two small, specialized mature modules.
3. **Protocol/security exception.** Use audited or well-maintained ecosystem libraries for OAuth2/OIDC, JWT, PKCS#7 CMS, PKCS#12 and VICI instead of implementing cryptographic/protocol semantics in application code. Integration state, policy and application-specific validation remain internal.
4. **Evidence required before approving new modules:** active maintenance and release/security history, recognized production users, documentation and tests, acceptable license, supported Go version, vulnerability status, *net change to `go list -m all`*, and a credible standard-library/existing-dependency alternative.
5. **No wrappers without benefit.** If a small adapter can use an existing library, do not introduce a new abstraction framework or another provider-specific SDK.
6. **Pin dependencies.** Direct and transitive versions remain in `go.mod` and `go.sum`, updated deliberately. Run the complete unit/integration suite after every cryptography/auth/DB upgrade.
7. **Review barrier.** Update this document **and** `docs/approved-direct-modules.txt` when changing direct modules. CI runs `sh scripts/check-dependencies.sh` and `go mod tidy -diff` to reject unapproved/untidied changes. Do not add a new library solely for testing when stdlib mocks/fixtures are sufficient.

## Current direct modules and decisions

| Module | Why external? | Decision |
|---|---|---|
| `github.com/coreos/go-oidc/v3` | OIDC discovery, verifying issuer/audience/expiry and signed ID Tokens, JWKS refresh | **Keep.** Mature widely-used OIDC relying-party client |
| `golang.org/x/oauth2` | Authorization Code Flow, S256 PKCE, OAuth token exchange and UserInfo token source | **Keep.** Deliberately used together with go-oidc; go-oidc **already depends on it**, so direct import does not add a second independent module graph |
| `github.com/golang-jwt/jwt/v5` | SwanPanel's own signed application/session JWTs; separate from verifying external OIDC ID tokens | **Keep.** Avoid rolling custom JWT claims/alg/issuer validation; reassess only if single reusable library provides equivalent verification and test coverage |
| `github.com/smallstep/pkcs7` | Attached CMS/S/MIME SignedData for Apple `.mobileconfig`; not provided by stdlib X.509 | **Keep provisionally.** Test signatures against OpenSSL and actual Apple devices; SHA-256 only |
| `software.sslmate.com/src/go-pkcs12` | PKCS#12/PFX export to Apple/Windows/Android; no stdlib equivalent | **Keep.** Prefer Modern2023 encoding where tested compatible; legacy algorithms only when justified |
| `github.com/gin-gonic/gin` | Existing routing, middleware, validation and JSON response framework | **Keep for now; high transitive cost.** Its module graph contains multiple JSON codecs, QUIC, BSON and validation libraries; assess `net/http` only as a separate refactor with measured benefit |
| `gorm.io/gorm` | ORM, associations, schema, transactions used throughout persistence | **Keep for now.** Replacing with `database/sql` is architectural, not a dependency cleanup |
| `gorm.io/driver/sqlite` | Official GORM SQLite dialector using `github.com/mattn/go-sqlite3` (CGO) | **Keep.** Native C SQLite, shared upstream integration and better maintenance outlook than abandoned glebarez dialect. Release uses native AMD64/ARM64 musl static linking; transaction/pragma/reopen regression tests apply |
| `github.com/google/uuid` | RFC-compatible UUID generation, naming and parsing across services | **Keep.** Small focused dependency used in several packages; any replacement must avoid format/identity regressions |
| `github.com/miekg/dns` | Real DNS protocol server, records and encoding | **Keep.** Writing DNS packet parsing/serving in-house would be disproportionate |
| `github.com/strongswan/govici` | strongSwan VICI protocol and runtime commands | **Keep.** Reimplementing binary protocol is unnecessary maintenance and security risk |
| `golang.org/x/crypto` | bcrypt hashing/verifying initial and local-user passwords | **Keep.** No equivalent bcrypt implementation in Go stdlib |
| `golang.org/x/time` | `rate.Limiter` for request throttling | **Keep.** Existing actively-maintained Go ecosystem package; no stdlib equivalent |

## OIDC versus OAuth2 (why two imports are intentional)

- OAuth 2.0 governs authorization and token exchange. The `oauth2.Config` already handles S256 PKCE options and the callback code exchange.
- OpenID Connect extends OAuth2 with identity tokens and rules for verifying signed claims. `go-oidc` handles provider discovery, key selection/rotation and ID Token verification.
- These are a **complementary stack**: `go-oidc` itself declares `golang.org/x/oauth2` as a dependency. Removing the explicit direct requirement while still importing it would make `go mod tidy` restore that direct requirement.
- A unified OIDC framework (e.g. ZITADEL) also builds on OAuth2, provides unneeded provider/server components and should not be adopted merely to show one direct line in `go.mod`.
- Preserve the split unless a measured replacement demonstrably improves security, maintenance *and* transitive graph size.

## Re-evaluation priorities

1. Verify dependency graph hygiene with `go mod tidy -diff` and `go list -m all`; recheck native SQLite and static Linux binary compatibility after driver upgrades.
2. Gin's transitive footprint versus practical need for its framework features. Verify with `go mod why -m <module>` and `go list -m all` before considering replacement.
3. PKCS#12 encoding algorithms and real-client interoperability (do not switch based only on crypto strength if importing breaks on native clients).
4. Review external dependencies every release, including known vulnerabilities; priority is exploitable runtime exposure rather than count of module names.

## Suggested review commands

```sh
sh scripts/check-dependencies.sh
go mod tidy -diff
go list -m all
go mod graph
go mod why -m github.com/quic-go/quic-go
go mod why -m github.com/mattn/go-sqlite3
go vet ./...
go test -race ./...
```

Do not claim a dependency can be removed merely because `go mod why` reports that no package in the main module imports it; `go mod tidy` and Go's module graph rules determine which entries are retained. Removing `require` lines manually while still importing the library is not dependency reduction.
