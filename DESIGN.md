---
name: SwanPanel
version: "1.0"
description: "Design specification and token definitions for the SwanPanel Zero-Trust VPN Control Plane, following the Google Labs design.md specification."
tokens:
  colors:
    dark:
      background: "#0b0f17"
      surface: "#111827"
      surface-header: "#0f172a"
      surface-nav: "#0d131f"
      surface-input: "#0b0f17"
      border: "#1f2937"
      border-subtle: "#1f2937"
      border-focus: "#3b82f6"
      text-primary: "#f8fafc"
      text-subtle: "#94a3b8"
      text-dim: "#64748b"
      primary: "#2563eb"
      primary-hover: "#3b82f6"
      primary-subtle: "rgba(37, 99, 235, 0.1)"
      success: "#10b981"
      success-subtle: "rgba(16, 185, 129, 0.1)"
      warning: "#f59e0b"
      warning-subtle: "rgba(245, 158, 11, 0.15)"
      danger: "#f43f5e"
      danger-subtle: "rgba(244, 63, 94, 0.1)"
    light:
      background: "#f8fafc"
      surface: "#ffffff"
      surface-header: "#f1f5f9"
      surface-nav: "#ffffff"
      surface-input: "#ffffff"
      border: "#e2e8f0"
      border-subtle: "#e2e8f0"
      border-focus: "#2563eb"
      text-primary: "#0f172a"
      text-subtle: "#64748b"
      text-dim: "#94a3b8"
      primary: "#2563eb"
      primary-hover: "#1d4ed8"
      primary-subtle: "rgba(37, 99, 235, 0.08)"
      success: "#059669"
      success-subtle: "rgba(5, 150, 105, 0.1)"
      warning: "#d97706"
      warning-subtle: "rgba(217, 119, 6, 0.1)"
      danger: "#e11d48"
      danger-subtle: "rgba(225, 29, 72, 0.08)"
  typography:
    fonts:
      sans: "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif"
      mono: "'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace"
    sizes:
      xxs: "10px"
      xs: "11px"
      sm: "12px"
      base: "13px"
      md: "14px"
      lg: "16px"
      xl: "20px"
      xxl: "24px"
    weights:
      normal: 400
      medium: 500
      semibold: 600
      bold: 700
  layout:
    sidebarWidth: "14rem"
    headerHeight: "3.5rem"
    containerMaxWidth: "72rem"
    breakpoints:
      sm: "640px"
      md: "768px"
      lg: "1024px"
      xl: "1280px"
    spacing:
      xs: "0.25rem"
      sm: "0.5rem"
      md: "0.75rem"
      base: "1rem"
      lg: "1.25rem"
      xl: "1.5rem"
  shapes:
    radii:
      sm: "6px"
      md: "8px"
      lg: "10px"
      xl: "12px"
      full: "9999px"
    borderWidth: "1px"
---

# DESIGN.md — UI/UX Design System Specification for SwanPanel

This document provides the authoritative visual and behavioral design contract for SwanPanel. All AI coding agents must align their UI implementations, styling choices, component hierarchies, and interactive behaviors with the guidelines below.

---

## Overview

### Brand Identity & Design Philosophy
SwanPanel follows an aesthetic of **Tailscale-grade architectural minimalism and high information density**:
* **Zero Decorative Gradients:** Visual hierarchy is communicated through high-contrast structural borders, intentional typography weights, and semantic background surfaces—never through flashy gradients or rainbow accents.
* **Functional Minimalism:** Every button, table column, and metric card directly serves an explicit operational user story. Extraneous chrome, decorative badges, and redundant developer jargon are eliminated.
* **Dual Theme Parity:** Both Dark Mode (`#0b0f17`) and Light Mode (`#f8fafc`) must maintain identical information density, crisp 1px borders, and high WCAG AA contrast.
* **Desktop & Mobile Responsiveness:** The UI adapts seamlessly from desktop multi-column layouts to mobile single-column drawers with collapsible slide-overs and horizontal scrolling tables.

---

## Colors

### Semantic Palette & Roles
Colors represent network state and user actions, never decoration:

| Token | Dark Value | Light Value | Semantic Application |
| :--- | :--- | :--- | :--- |
| `background` | `#0b0f17` | `#f8fafc` | Global viewport canvas |
| `surface` | `#111827` | `#ffffff` | Content cards, table bodies, modal containers |
| `surface-header` | `#0f172a` | `#f1f5f9` | Table thead, inner control groups, switch panels |
| `surface-nav` | `#0d131f` | `#ffffff` | Left sidebar, top sticky header |
| `border` | `#1f2937` | `#e2e8f0` | 1px dividers, card boundaries, input strokes |
| `primary` | `#2563eb` | `#2563eb` | Main CTA buttons (`Save Changes`, `+ Add Device`), active nav tab |
| `success` | `#10b981` | `#059669` | Live sessions, online gateway, connected tunnel state |
| `warning` | `#f59e0b` | `#d97706` | Pending approvals requiring administrator review |
| `danger` | `#f43f5e` | `#e11d48` | Destructive actions (Disconnect tunnel, delete device, drop rule) |

---

## Typography

### Font Families & Hierarchy
* **Sans (`Inter`):** All UI labels, card headers, instructional paragraphs, navigation items, tooltips, and modal titles.
* **Mono (`JetBrains Mono`):** IP addresses (`10.0.1.15`), CIDR subnets (`10.100.0.0/16`), domain suffixes (`.strongnet`), timestamps (`12:54:10`), and cipher proposal strings.

### Type Scale
* **Header Numbers:** `24px` (`text-2xl`), bold, monospace for live session counters and metric totals.
* **Section Titles:** `14px` (`text-sm`), font-weight 700, tracking tight.
* **Card Titles:** `12px` (`text-xs`), font-weight 600.
* **Body / Form Labels:** `12px` (`text-xs`), font-weight 500.
* **Help Hints / Subtitles:** `11px` (`text-xs`), font-color `text-dim` (`#64748b` dark / `#94a3b8` light), italic or standard.
* **Badges & Tooltips:** `10px` / `11px`, font-weight 500, uppercase or monospace.

---

## Layout

### Viewport & Spatial Grid
* **Sidebar:** Fixed width `w-56` (14rem / 224px). Sticky on desktop. On mobile (`<md` / <768px), it transitions into an off-canvas drawer with a backdrop overlay.
* **Header:** Sticky height `h-14` (3.5rem / 56px) with backdrop blur, holding page title, live SSE indicator pill, and contextual action buttons.
* **Content Container:** Centered with max-width `max-w-6xl` (72rem / 1152px) and responsive padding: `p-4` on mobile, `p-6` on desktop.
* **Responsive Grid Breakpoints:**
  * Metric Cards: 1 column on mobile (`grid-cols-1`), 2 columns on tablet (`sm:grid-cols-2`), 4 columns on desktop (`lg:grid-cols-4`).
  * Configuration Cards: 1 column on mobile, 2 columns on desktop (`lg:grid-cols-2`).

---

## Elevation & Depth

* **Flat Layering:** Surfaces do NOT rely on heavy drop shadows. Depth is achieved via `surface-card` backgrounds placed over `background` canvas, separated by crisp `1px` subtle borders (`#1f2937` / `#e2e8f0`).
* **Interactive Tooltips:** Subtle shadow `box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.2)` with background `#1e293b` and 1px border.
* **Modals / Drawers:** Fixed backdrop with `bg-black/60` and `backdrop-blur-xs`, modal box elevated with `shadow-2xl`.

---

## Shapes

* **Card Corners:** `rounded-xl` (`12px`) for all main cards, tables, and modal dialogs.
* **Input Fields & Buttons:** `rounded-lg` (`8px`) for consistent tactile feel.
* **Badges & Micro-tags:** `rounded-md` (`6px`) with 1px border.
* **Pills & Avatars:** `rounded-full` for status dots and user initials.

---

## Components

### 1. Buttons
* **Primary Button:** `bg-blue-600 hover:bg-blue-500 text-white font-medium px-3 py-1.5 rounded-lg shadow-xs text-xs`.
* **Secondary / Ghost Button:** `border subtle-border hover:bg-slate-500/10 text-subtle hover:text-current px-2.5 py-1.5 rounded-lg text-xs`.
* **Destructive Button:** `border border-rose-500/30 text-rose-400 bg-rose-500/10 hover:bg-rose-500/20 px-3 py-1.5 rounded-lg text-xs`.
* **Table Action Buttons:** Icon-only (`p-1.5 rounded-md hover:bg-slate-500/10 text-subtle hover:text-current`) with hover tooltips (`data-tooltip="Action Description"`). Never use text labels inside table action columns.

### 2. Contextual Action Area
* Placed at the top-right of the header (`contextual-action-area`).
* Dynamically displays the primary page action (e.g., `+ Add Device` on Resources, `+ Add Rule` on Access Rules, `Save Changes` on Server Settings).

### 3. Data Tables
* Wrapped in an `overflow-x-auto` container to ensure horizontal scrolling on mobile devices without breaking viewport width.
* Single-responsibility: Each table presents only attributes belonging to its entity domain.
* Rows feature subtle hover feedback (`hover-row`).

### 4. Toggle Switches
* Used for binary network features (MOBIKE, Force UDP NAT-T, Packet Fragmentation).
* Structure:
  ```html
  <label class="relative inline-flex items-center cursor-pointer">
    <input type="checkbox" class="sr-only peer" checked>
    <div class="w-9 h-5 bg-slate-700 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-slate-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-blue-600"></div>
  </label>
  ```

### 5. Status Indicators & Live Signals
* **Online / Active:** Green dot `bg-emerald-500` with pulse animation + badge `bg-emerald-500/10 text-emerald-500 border border-emerald-500/20`.
* **Live SSE Stream Pill:** Header pill with live pulse dot demonstrating real-time data push without manual page refresh.

---

## Do's and Don'ts

### Do:
* **DO** use exact token references for colors, spacing, and typography across components.
* **DO** maintain 100% i18n key translation: all user-facing strings must use `t('key')` and exist in both `ui/src/locales/en.json` and `ui/src/locales/vi.json`.
* **DO** keep table action columns icon-only with semantic tooltips.
* **DO** provide helpful input hints explaining the operational impact of configuration options (e.g. Zero-Trust re-authentication lifetime, roaming mechanics).
* **DO** wrap data tables in responsive containers (`overflow-x-auto`) to guarantee mobile compatibility.
* **DO** target modern toolchain releases: Tailwind CSS v4/v5, React 19, and clean vector SVG icons (`lucide-react`).

### Don't:
* **DON'T** use AI unicode emoji icons in UI headings, button labels, toasts, badges, or markdown documents. Always use clean plain-text typography or vector SVG icons (Lucide React).
* **DON'T** use flashy gradients, rainbow cards, or neon glow effects.
* **DON'T** expose raw developer internals or package names to end-users (never render `swanctl.conf`, `charon.vici`, `miekg/dns`, or raw RFC numbers in UI badges).
* **DON'T** create client-side polling loops with `setInterval`—always consume Server-Sent Events from `/api/realtime/stream`.
* **DON'T** hardcode text strings inside React JSX components.
* **DON'T** break dark/light mode parity: every component must be tested and fully readable in both themes.
