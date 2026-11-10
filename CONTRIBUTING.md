# Contributing to SwanPanel

Thank you for your interest in contributing to SwanPanel! We welcome contributions from developers, network administrators, technical writers, and translators.

---

## Code of Conduct

SwanPanel has adopted the [Contributor Covenant](CODE_OF_CONDUCT.md). By participating in this project, you agree to abide by its terms.

---

## Getting Started

### Prerequisites
- **Go 1.26+** (compiled with `CGO_ENABLED=0` for pure Go static builds)
- **Node.js 20+** and **npm**
- **strongSwan 5.9+** with `charon.vici` enabled (optional for local mock testing)

### Local Development Setup

1. **Fork and Clone the Repository:**
   ```bash
   git clone https://github.com/thienvu18/swan-panel.git
   cd swan-panel
   ```

2. **Build and Run Backend:**
   ```bash
   # Run all backend unit and integration tests
   go test -v ./...

   # Start local backend server (starts mock VICI when socket is absent)
   go run ./cmd/server
   ```

3. **Run Frontend Development Server:**
   ```bash
   cd ui
   npm install
   npm run dev
   ```

---

## Engineering Guidelines

### 1. Strict Security Mandates
- **Zero SQL Injection:** Always use GORM's parameterized queries (`db.Where("email = ?", email)`). Dynamic raw SQL string concatenation is strictly forbidden.
- **Timing Attack Mitigation:** Use `crypto/subtle.ConstantTimeCompare` for all credential, token, and hash comparisons.
- **Structured Logging:** Use Go's standard `log/slog` library with appropriate log levels (`Debug`, `Info`, `Warn`, `Error`).
- **No Developer Jargon in UI:** Avoid exposing daemon socket paths or internal library names (`miekg/dns`, `govici`) directly in user-facing views.

### 2. Internationalization (i18n)
All user-facing text in the React frontend must be localized using `react-i18next`:
- English: `ui/src/locales/en.json` (baseline)
- Vietnamese: `ui/src/locales/vi.json`
- Pull requests modifying UI strings must update both translation files.

### 3. Clean Typography & Zero AI Emojis
- Do not use AI unicode emojis in documentation, commit messages, or UI components. Use clean plain-text typography or vector SVG icons (`lucide-react`).

---

## Pull Request Workflow

1. Create a descriptive feature branch from `main`:
   ```bash
   git checkout -b feat/your-feature-name
   ```
2. Commit your changes using conventional commit messages:
   - `feat: add custom DNS override filtering`
   - `fix: correct SA session termination timeout`
   - `docs: update native client setup guide`
3. Run verification before opening a pull request:
   ```bash
   go test -v ./...
   cd ui && npm run build
   ```
4. Push your branch and open a Pull Request against `main`.
