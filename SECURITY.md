# Security Policy

## Reporting Security Vulnerabilities

The SwanPanel engineering and security team takes security vulnerabilities seriously. We appreciate your efforts to responsibly disclose findings.

If you believe you have discovered a security vulnerability in SwanPanel, **please DO NOT open a public GitHub issue.** Instead, report it privately through one of the following coordinated vulnerability disclosure channels:

1. **GitHub Security Advisory (Recommended):** Submit a confidential vulnerability report directly via the [GitHub Security Advisory form](https://github.com/thienvu18/swan-panel/security/advisories/new).
2. **Direct Security Contact:** Email `thienvu1802@gmail.com` with the subject line `[SECURITY] SwanPanel Vulnerability Report`.

### Vulnerability Report Details
To assist in rapid triage and remediation, please include:
- A detailed description of the vulnerability and attack vector.
- Affected component(s) (e.g. VICI IPC driver, Split-DNS engine, X.509 PKI manager, REST API handlers, or React frontend).
- Step-by-step instructions or proof-of-concept (PoC) code to reproduce the issue.
- Potential impact and cvss assessment (if available).
- Suggested remediation or patch (if any).

### Response Timeline
- **Initial Acknowledgment:** Within 24-48 hours of receipt.
- **Triage and Impact Assessment:** Within 3-5 business days.
- **Fix and Coordinated Disclosure:** We aim to release patches for confirmed high-severity issues within 7-14 days before public disclosure.

---

## Supported Versions

Only the latest active minor release receives official security patches and updates. We strongly advise all operators to maintain their installations on the latest release tag.

| Version | Supported | Security Notes |
| :--- | :--- | :--- |
| `v0.0.x` | Supported | Current active development and stable release line |
| `< v0.0.1` | Unsupported | Pre-release and development snapshots |

---

## Architectural Security Mandates

SwanPanel is engineered specifically to manage zero-trust network infrastructure and adheres to strict security invariants:

1. **Zero SQL Injection:**
   All persistence layers strictly use GORM parameterized queries (`db.Where("col = ?", val)`). Raw SQL string formatting and concatenations are strictly forbidden.

2. **OWASP Hardened HTTP Headers:**
   The root HTTP engine enforces strict security headers on every response:
   - `Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self' data:; connect-src 'self'; img-src 'self' data: https:; frame-ancestors 'none';`
   - `Strict-Transport-Security: max-age=31536000; includeSubDomains`
   - `X-Content-Type-Options: nosniff`
   - `X-Frame-Options: DENY`
   - `Referrer-Policy: strict-origin-when-cross-origin`

3. **Timing-Safe Cryptographic Comparisons:**
   All cryptographic token, secret, and API key validations utilize constant-time comparison (`crypto/subtle.ConstantTimeCompare`) to prevent side-channel timing attacks.

4. **Brute-Force & Rate Limiting:**
   Public authentication endpoints (`/api/auth/login`) are protected by an in-memory token-bucket rate limiter (`middleware.RateLimit(5, 10)`).

5. **Local IPC Isolation:**
   Communications with strongSwan occur strictly through the UNIX domain socket `/var/run/charon.vici` via the native `govici` protocol. No unauthenticated network ports or SSH daemons are exposed for control operations.
