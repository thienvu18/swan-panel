# AGENTS.md — Development Guidelines for AI Agents

Welcome to SwanPanel! This document establishes strict engineering conventions, architectural invariants, and security mandates that all AI agents (and human contributors) must adhere to when modifying or extending this codebase.

---

## 1. MISSION & CORE ARCHITECTURE

SwanPanel is a modern, enterprise-grade, zero-trust web control plane for **strongSwan** Remote Access VPNs.
* **Official Product Branding:** Always refer to the product as **SwanPanel** (PascalCase) in all user-facing documentation, UI headings, branding, titles, and logs. Technical identifiers (e.g. binary name `swan-panel`, repository path, Go module) remain kebab-cased or module-pathed.
* **100% Native Client Support:** We deliver native configuration profiles (Apple `.mobileconfig`, Windows PowerShell `Add-VpnConnection`, Linux NetworkManager, Android IKEv2) without requiring third-party client applications.
* **Single-Binary Delivery:** The entire application (Go backend + SQLite database + embedded React SPA + internal PKI + DNS server) compiles into one self-contained executable.
* **Zero-SSH Operations:** The Admin manages daemon configuration, reloads connection parameters dynamically in RAM, and streams filtered logs via the UNIX socket `/var/run/charon.vici` without ever opening an SSH terminal.

---

## 2. STRICT SECURITY MANDATES & BEST PRACTICES

As this control plane manages critical corporate network access, security is paramount:

### 2.1. Zero SQL Injection
* **RULE:** NEVER use raw SQL string concatenation or string formatting (e.g. `fmt.Sprintf("WHERE name = '%s'", val)`).
* **MANDATE:** Always use GORM's parameterized queries:
  ```go
  // CORRECT
  db.Where("email = ?", email).First(&user)
  db.Model(&Device{}).Where("resource_group_id = ? AND is_revoked = ?", rgID, false)
  
  // FORBIDDEN
  db.Raw("SELECT * FROM users WHERE email = '" + email + "'") // VIOLATION!
  ```

### 2.2. Zero Cross-Site Scripting (XSS) & Clickjacking
* **RULE:** Never use `dangerouslySetInnerHTML` in React or inject unsanitized HTML strings.
* **MANDATE:** Rely on React's automatic JSX escaping. Ensure the `middleware.SecurityHeaders()` is always registered on the root Gin router (HSTS, CSP, `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy`).

### 2.3. Timing Attack Mitigation
* **RULE:** When comparing cryptographic tokens, API keys, or password hashes, never use standard equality (`==`).
* **MANDATE:** Always use constant-time comparison:
  ```go
  if subtle.ConstantTimeCompare([]byte(tokenA), []byte(tokenB)) != 1 {
      // Reject
  }
  ```

### 2.4. Brute-Force & Rate Limiting
* **MANDATE:** All public endpoints (especially `/api/auth/login`) must be protected by the token-bucket rate limiter (`middleware.RateLimit()`).

---

## 3. DEPENDENCY & CODE STYLE RULES

### 3.1. Never Reinvent the Wheel
* Use cohesive, battle-tested, high-star libraries:
  * HTTP & Routing: `github.com/gin-gonic/gin` (~78k stars)
  * Database & ORM: `gorm.io/gorm` (~37k stars) with pure Go SQLite `github.com/glebarez/sqlite`
  * DNS Engine: `github.com/miekg/dns` (~8.3k stars)
  * strongSwan IPC: `github.com/strongswan/govici` (Official strongSwan project)
  * OIDC / SSO: `github.com/coreos/go-oidc/v3` + `golang.org/x/oauth2`
  * Frontend: React 19, Vite, Tailwind CSS v4, Lucide React, `react-i18next`, `@tanstack/react-query`
* **RULE:** Do NOT install fragmented micro-libraries for simple tasks (e.g. left-pad, date formatting, simple math). Keep the dependency footprint clean and cohesive.

### 3.2. Pure Go & Cross-Platform Compilation
* Avoid CGO wherever possible so the binary can be compiled across macOS, Linux, and Windows without requiring cross-compilation C toolchains.

### 3.3. Modern Toolchain & Latest Dependencies
* Always target modern, cutting-edge stable dependencies: Go 1.26+ (Go 1.27 active), Tailwind CSS v4/v5, React 19, Vite 6.
* Never regress to deprecated build patterns, legacy Go modules, or obsolete CSS utility configurations.

### 3.4. Strict Branch Logging & Configurable Log Level
* **RULE:** Every code branch (success paths, error returns, validations, warnings, fallbacks, background loops) MUST emit structured logs via Go standard library `log/slog` with proper severity levels:
  * `slog.Debug`: Fine-grained tracing, parameter payloads, cache hits, keepalive pulses.
  * `slog.Info`: Operational milestones (daemon reload, DB connected, user authenticated, service startup).
  * `slog.Warn`: Non-fatal rejections (rate-limit hit, invalid credentials, token expired, fallback triggers).
  * `slog.Error`: Critical failures (DB query errors, encryption failures, socket crashes).
* **MANDATE:** The application MUST support configurable log level via the `LOG_LEVEL` environment variable (`DEBUG`, `INFO`, `WARN`, `ERROR`, default `INFO`).

---

## 4. UI/UX & DESIGN SYSTEM COMPLIANCE

All frontend work must strictly follow the Google Labs `design.md` specification defined in [DESIGN.md](DESIGN.md) (and visually prototyped in `wireframe.html`):
1. **Zero AI Unicode Emojis:** NEVER use AI unicode icons or emojis in documentation, markdown files, commit messages, console output, or UI text. Always use clean plain-text typography or vector SVG icons (`lucide-react`).
2. **Design Tokens:** Always refer to the YAML tokens defined in `DESIGN.md` for colors (dark/light), typography, spacing, border radiuses, and components.
3. **Design Minimalism:** Clean slate/zinc surfaces, high-contrast borders, no flashy or rainbow gradients.
4. **Table Design:** Single responsibility per table. Action columns must use clean icon-only buttons with hover tooltips (`data-tooltip`).
5. **No Developer Jargon in User-Facing UI:**
   * Never display internal package names (`miekg/dns`, `govici`), config file names (`swanctl.conf`, `charon.conf`), RFC numbers (`RFC 4555`), socket paths, or variable names in badges.
   * Only provide human-friendly labels, clear explanatory text, and helpful placeholders/hints.
6. **Contextual Action Buttons:** The top-right header area holds the primary contextual action button for the current active page (e.g. `+ Add Device` on Resources, `+ Add Rule` on Access Rules, `Save Changes` on Server Settings).
7. **Architecture Details:** For backend workflows, database models, and sequence diagrams, see [ARCHITECTURE.md](ARCHITECTURE.md).

---

## 5. INTERNATIONALIZATION (I18N)

* **Mandate:** Never hardcode raw user-facing strings in React components.
* Always use `t('key')` from `useTranslation()`.
* Every new UI string must be added to BOTH:
  * `ui/src/locales/en.json` (English - default)
  * `ui/src/locales/vi.json` (Vietnamese)

---

## 6. REAL-TIME ENGINE (SERVER-SENT EVENTS)

* **Mandate:** Do not create client-side polling loops (`setInterval`) to refresh data.
* Use the backend broker `internal/api/realtime/broker.go` to broadcast events (`session_update`, `gateway_status`, `log_event`) over `/api/realtime/stream`.
* The frontend consumes this stream via native `EventSource` or TanStack Query invalidation.

---

## 7. VERIFICATION & RUNNING COMMANDS

* **Build Backend:**
  ```bash
  go build ./cmd/server
  ```
* **Run Tests:**
  ```bash
  go test -v ./...
  ```
* **Build Frontend:**
  ```bash
  cd ui && npm run build
  ```
