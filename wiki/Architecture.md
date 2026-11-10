# Architecture & Internals

SwanPanel integrates five primary subsystem modules into a single self-contained Go binary:

```text
+------------------------------------------------------------------------+
|                          SwanPanel Single Binary                       |
|                                                                        |
|  +--------------------+  +--------------------+  +------------------+  |
|  |   React 19 SPA     |  |   Gin REST API     |  |  Internal PKI    |  |
|  |   (embed.FS)       |  |   & SSE Broker     |  |  (X.509 & PKCS12)|  |
|  +--------------------+  +--------------------+  +------------------+  |
|            |                       |                      |            |
|  +--------------------+  +--------------------+  +------------------+  |
|  |   SQLite Database  |  |   VICI IPC Driver  |  |  Split-DNS Server|  |
|  |   (Pure Go GORM)   |  |   (govici UNIX)    |  |  (miekg/dns :53) |  |
|  +--------------------+  +--------------------+  +------------------+  |
+------------------------------------|-----------------------------------+
                                     | UNIX Socket
                                     v
                       +---------------------------+
                       |   strongSwan charon       |
                       |   IPsec Linux Kernel SA   |
                       +---------------------------+
```

---

## 1. Zero-SSH VICI Control Plane
- SwanPanel communicates directly with strongSwan's `charon` daemon via UNIX domain socket `/var/run/charon.vici` using the binary `govici` protocol.
- Connection configurations (`load-conn`), termination commands (`terminate`), and security association queries (`list-sas`) execute in RAM without writing to static files or restarting the daemon. Active tunnels experience zero downtime.

## 2. Embedded PKI (Public Key Infrastructure)
- SwanPanel acts as an autonomous internal X.509 Certificate Authority.
- On initialization, it generates a 4096-bit RSA Root CA certificate (`SwanPanel Internal Root CA`).
- When a new device is registered, SwanPanel signs a client certificate, packs the client key, certificate, and Root CA chain into a PKCS#12 (`.p12`) archive, and embeds it into native operating system profiles.

## 3. High-Performance Split-DNS & MagicDNS
- Powered by `github.com/miekg/dns`, listening on UDP/TCP port 53.
- Resolves internal device hostnames dynamically (e.g. `laptop-alice.strongnet -> 10.60.0.10`).
- Supports user-defined Custom Overrides (`api.internal -> 10.60.0.50`).
- Non-internal queries forward recursively to upstream public or corporate resolvers (Cloudflare, Google, internal bind9).

## 4. Dynamic Policy Compiler
- Evaluates relationships between User Groups, Users, Devices, and Resource Groups.
- Computes granular CIDR routing policies and exclusion rules (`DROP` filters) for fine-grained zero-trust segmentation.
