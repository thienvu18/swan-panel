const { spawn } = require('child_process');
const fs = require('fs');
const path = require('path');

const PORT = 18088;
const DEBUG_PORT = 9228;
const BASE_URL = `http://127.0.0.1:${PORT}/`;

const SUITES = [
  { lang: 'en', theme: 'dark' },
  { lang: 'en', theme: 'light' },
  { lang: 'vi', theme: 'dark' },
  { lang: 'vi', theme: 'light' },
];

async function run() {
  console.log('--- Starting SwanPanel Comprehensive Verified Screenshot Capture ---');

  try { require('child_process').execSync('pkill -9 -f "chrome-tour-suite" || true'); } catch {}
  try { require('child_process').execSync('pkill -9 -f "swan-panel-test" || true'); } catch {}
  try { require('child_process').execSync('rm -rf /tmp/chrome-tour-suite-verified'); } catch {}
  await new Promise(r => setTimeout(r, 500));

  // Start backend server
  const server = spawn('/tmp/swan-panel-test', [], {
    env: {
      ...process.env,
      PORT: String(PORT),
      DB_PATH: 'scratch/audit_app.db',
      ENABLE_DNS: 'false',
      APP_ENV: 'test',
    }
  });

  server.stdout.on('data', () => {});
  server.stderr.on('data', d => {
    const s = d.toString();
    if (!s.includes('DEBUG')) console.error('Server:', s.trim());
  });

  // Get Admin token with retries
  let token = null;
  let user = null;
  for (let attempt = 0; attempt < 15; attempt++) {
    try {
      const res = await fetch(`${BASE_URL}api/auth/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: 'admin@enterprise.corp', password: 'Corporate-Admin-Password-2026!' })
      });
      if (res.ok) {
        const data = await res.json();
        if (data.token) {
          token = data.token;
          user = data.user;
          break;
        }
      }
    } catch {}
    await new Promise(r => setTimeout(r, 500));
  }

  if (!token) {
    throw new Error('Admin login failed after retries');
  }
  console.log('Obtained Admin token for:', user.email);

  // Start Chrome
  const chrome = spawn('/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', [
    '--headless=new',
    `--remote-debugging-port=${DEBUG_PORT}`,
    '--user-data-dir=/tmp/chrome-tour-suite-verified',
    '--window-size=1440,900',
    '--disable-gpu',
    '--no-first-run',
    '--hide-scrollbars'
  ]);

  let target = null;
  for (let attempt = 0; attempt < 20; attempt++) {
    try {
      const res = await fetch(`http://127.0.0.1:${DEBUG_PORT}/json/new?${BASE_URL}`, { method: 'PUT' });
      if (res.ok) {
        target = await res.json();
        break;
      }
    } catch {}
    await new Promise(r => setTimeout(r, 500));
  }
  if (!target) {
    throw new Error('Chrome remote debugging connection failed after retries');
  }
  const ws = new WebSocket(target.webSocketDebuggerUrl);

  let msgId = 1;
  const pending = new Map();
  let mockSetupRequired = false;
  let mockCanRotateCA = false;

  ws.onmessage = async (event) => {
    const data = JSON.parse(event.data);
    if (data.id && pending.has(data.id)) {
      pending.get(data.id)(data);
      pending.delete(data.id);
    }

    if (data.method === 'Fetch.requestPaused') {
      const { requestId, request } = data.params;
      const url = request.url;

      if (url.includes('/api/auth/setup/status')) {
        const body = Buffer.from(JSON.stringify({ setup_required: mockSetupRequired })).toString('base64');
        ws.send(JSON.stringify({
          id: msgId++,
          method: 'Fetch.fulfillRequest',
          params: { requestId, responseCode: 200, responseHeaders: [{ name: 'Content-Type', value: 'application/json' }], body }
        }));
        return;
      }

      if (url.includes('/api/sessions') && request.method === 'GET') {
        const body = Buffer.from(JSON.stringify({
          sessions: [
            {
              ike_name: "dev_ff131e1a-c988-4997-85ba-5372b42a02c7",
              peer_identity: "alice-macbook-pro",
              virtual_ip: "10.150.0.10",
              remote_wan: "198.51.100.42",
              uptime: "8100s",
              established_seconds: 8100,
              cipher_suite: "AES_GCM_16_256 / CURVE25519"
            }
          ]
        })).toString('base64');
        ws.send(JSON.stringify({
          id: msgId++,
          method: 'Fetch.fulfillRequest',
          params: { requestId, responseCode: 200, responseHeaders: [{ name: 'Content-Type', value: 'application/json' }], body }
        }));
        return;
      }

      if (url.includes('/api/dashboard/stats')) {
        const body = Buffer.from(JSON.stringify({
          stats: {
            active_sessions: 1,
            total_devices: 6,
            total_users: 5,
            pending_users: 1,
            gateway_status: "online"
          }
        })).toString('base64');
        ws.send(JSON.stringify({
          id: msgId++,
          method: 'Fetch.fulfillRequest',
          params: { requestId, responseCode: 200, responseHeaders: [{ name: 'Content-Type', value: 'application/json' }], body }
        }));
        return;
      }

      if (url.includes('/api/system/daemon/status')) {
        const body = Buffer.from(JSON.stringify({
          service: "strongSwan",
          status: "active",
          pid: 1284,
          memory: "24.6 MB",
          uptime: "432000s",
          control_supported: true
        })).toString('base64');
        ws.send(JSON.stringify({
          id: msgId++,
          method: 'Fetch.fulfillRequest',
          params: { requestId, responseCode: 200, responseHeaders: [{ name: 'Content-Type', value: 'application/json' }], body }
        }));
        return;
      }

      if (url.includes('/api/oidc/providers') && request.method === 'GET') {
        const body = Buffer.from(JSON.stringify({
          providers: [
            {
              id: "oidc_okta_sso",
              name: "Enterprise Okta SSO",
              provider_type: "okta",
              issuer_url: "https://identity.enterprise.corp/oauth2/v1",
              client_id: "0oa7821bc78a87b",
              is_active: true
            },
            {
              id: "oidc_google_workspace",
              name: "Google Workspace IDP",
              provider_type: "google",
              issuer_url: "https://accounts.google.com",
              client_id: "892187319827-corp.apps.googleusercontent.com",
              is_active: true
            }
          ],
          redirect_uri: "https://vpn.enterprise.corp/api/auth/oidc/callback",
          redirect_uri_valid: true,
          external_origin_configured: true,
          production: true
        })).toString('base64');
        ws.send(JSON.stringify({
          id: msgId++,
          method: 'Fetch.fulfillRequest',
          params: { requestId, responseCode: 200, responseHeaders: [{ name: 'Content-Type', value: 'application/json' }], body }
        }));
        return;
      }

      if (url.includes('/api/settings/ca') && request.method === 'GET' && mockCanRotateCA) {
        const body = Buffer.from(JSON.stringify({
          ca: {
            common_name: "SwanPanel Internal Root CA",
            organization: "Enterprise Global Corp",
            country: "US",
            province: "California",
            locality: "San Francisco",
            serial: "4A:2F:89:1B:0C:6E",
            not_after: "2036-11-19T00:00:00Z"
          },
          can_rotate: true
        })).toString('base64');
        ws.send(JSON.stringify({
          id: msgId++,
          method: 'Fetch.fulfillRequest',
          params: { requestId, responseCode: 200, responseHeaders: [{ name: 'Content-Type', value: 'application/json' }], body }
        }));
        return;
      }

      // Default: continue
      ws.send(JSON.stringify({
        id: msgId++,
        method: 'Fetch.continueRequest',
        params: { requestId }
      }));
    }
  };

  const send = (method, params = {}) => new Promise((resolve, reject) => {
    const id = msgId++;
    const timer = setTimeout(() => {
      if (pending.has(id)) {
        pending.delete(id);
        reject(new Error(`CDP command "${method}" timed out after 15s`));
      }
    }, 15000);
    pending.set(id, (res) => {
      clearTimeout(timer);
      resolve(res);
    });
    ws.send(JSON.stringify({ id, method, params }));
  });

  if (ws.readyState !== 1) {
    await new Promise((resolve, reject) => {
      ws.onopen = resolve;
      ws.onerror = reject;
      setTimeout(() => reject(new Error('WebSocket connection timeout')), 5000);
    });
  }
  await send('Page.enable');
  await send('Runtime.enable');
  await send('Page.addScriptToEvaluateOnNewDocument', {
    source: `
      window.__setReactVal = function(el, val) {
        if (!el) return;
        const proto = el instanceof HTMLTextAreaElement ? window.HTMLTextAreaElement.prototype :
                      el instanceof HTMLSelectElement ? window.HTMLSelectElement.prototype :
                      window.HTMLInputElement.prototype;
        const setter = Object.getOwnPropertyDescriptor(proto, 'value').set;
        setter.call(el, val);
        el.dispatchEvent(new Event('input', { bubbles: true }));
        el.dispatchEvent(new Event('change', { bubbles: true }));
      };
    `
  });
  await send('Fetch.enable', {
    patterns: [
      { urlPattern: '*api/auth/setup/status*' },
      { urlPattern: '*api/sessions*' },
      { urlPattern: '*api/dashboard/stats*' },
      { urlPattern: '*api/system/daemon/status*' },
      { urlPattern: '*api/oidc/providers*' },
      { urlPattern: '*api/settings/ca*' }
    ]
  });

  const setViewport = async (width, height, scale = 2, mobile = false) => {
    await send('Emulation.setDeviceMetricsOverride', {
      width,
      height,
      deviceScaleFactor: scale,
      mobile
    });
    await new Promise(r => setTimeout(r, 300));
  };

  const reloadPage = async (waitMs = 1200) => {
    await send('Page.reload', { ignoreCache: true });
    await new Promise(r => setTimeout(r, waitMs));
  };

  const navigateTo = async (hashOrPath = '', waitMs = 1200) => {
    await send('Page.navigate', { url: `${BASE_URL}${hashOrPath}` });
    await new Promise(r => setTimeout(r, waitMs));
  };

  await setViewport(1440, 900, 2, false);

  const evaluate = async (exp) => {
    let script = exp.trim();
    if (!script.includes('return ') && !script.includes(';\n') && !script.includes('const ') && !script.includes('let ') && !script.includes('var ')) {
      script = `return (${script});`;
    }
    const res = await send('Runtime.evaluate', {
      expression: `(async () => { ${script} })()`,
      awaitPromise: true,
      returnByValue: true
    });
    if (res.exceptionDetails) {
      throw new Error(`Eval failed: ${res.exceptionDetails.text} - ${JSON.stringify(res.exceptionDetails.exception)}`);
    }
    return res.result?.result?.value;
  };

  const waitForCondition = async (desc, exp, timeout = 6000) => {
    const start = Date.now();
    while (Date.now() - start < timeout) {
      try {
        const ok = await evaluate(exp);
        if (ok) {
          console.log(`  [OK] ${desc}`);
          return true;
        }
      } catch {}
      await new Promise(r => setTimeout(r, 150));
    }
    const html = await evaluate(`document.body.innerText`).catch(() => 'N/A');
    throw new Error(`Timed out waiting for condition "${desc}". Body text: ${html ? html.slice(0, 300) : 'empty'}`);
  };

  const waitForSelector = async (sel, timeout = 6000) => {
    return waitForCondition(`Selector "${sel}" visible`, `Boolean(document.querySelector(${JSON.stringify(sel)}))`, timeout);
  };

  const switchNavTab = async (tabName) => {
    await evaluate(`
      const btn = document.querySelector('button[data-tab="${tabName}"]');
      if (btn) btn.click();
      else window.location.hash = '#/${tabName}';
    `);
    await new Promise(r => setTimeout(r, 800));
  };

  const clickAddPrimaryButton = async () => {
    await evaluate(`
      const addBtn = Array.from(document.querySelectorAll('main button')).find(b => b.querySelector('svg.lucide-plus') || (b.classList.contains('bg-blue-600') && (b.textContent.includes('Add') || b.textContent.includes('Thêm'))));
      if (addBtn) addBtn.click();
    `);
    await new Promise(r => setTimeout(r, 800));
  };

  const closeModal = async () => {
    await evaluate(`
      const cancelBtn = Array.from(document.querySelectorAll(':is([role="dialog"], [role="alertdialog"]) button')).find(b => b.textContent.includes('Cancel') || b.textContent.includes('Hủy') || b.textContent.trim() === '×' || b.getAttribute('aria-label') === 'Close' || b.getAttribute('aria-label') === 'Đóng');
      if (cancelBtn) cancelBtn.click();
    `);
    await new Promise(r => setTimeout(r, 500));
  };

  const capture = async (relPath) => {
    await evaluate('await document.fonts.ready');
    await new Promise(r => setTimeout(r, 600));

    const shot = await send('Page.captureScreenshot', { format: 'png' });
    const fullPath = path.join(__dirname, '..', 'docs', 'assets', 'images', 'user-guide', relPath);
    fs.mkdirSync(path.dirname(fullPath), { recursive: true });
    fs.writeFileSync(fullPath, Buffer.from(shot.result.data, 'base64'));
    console.log(`[SAVED] ${relPath}`);
  };

  // --- PART 1: 4 SUITES (EN/VI x Dark/Light) ---
  const suitesToRun = process.argv.includes('--responsive-only') ? [] : SUITES;
  for (const suite of suitesToRun) {
    console.log(`\n======================================================`);
    console.log(`=== Processing Suite: ${suite.lang.toUpperCase()} / ${suite.theme.toUpperCase()} ===`);
    console.log(`======================================================`);

    await setViewport(1440, 900, 2, false);

    // 1. Setup Token Page (logged out, setup required)
    mockSetupRequired = true;
    await evaluate(`
      localStorage.clear();
      localStorage.setItem('theme', '${suite.theme}');
      localStorage.setItem('i18nextLng', '${suite.lang}');
      if ('${suite.theme}' === 'dark') document.documentElement.classList.add('dark');
      else document.documentElement.classList.remove('dark');
    `);
    await navigateTo('', 1200);
    await waitForCondition('Setup token input is rendered', `Boolean(document.querySelector('input[type="password"]'))`);
    await capture(`${suite.lang}/${suite.theme}/01b_setup_token.png`);

    // 2. Login Page (logged out, setup completed)
    mockSetupRequired = false;
    await evaluate(`
      localStorage.clear();
      localStorage.setItem('theme', '${suite.theme}');
      localStorage.setItem('i18nextLng', '${suite.lang}');
      if ('${suite.theme}' === 'dark') document.documentElement.classList.add('dark');
      else document.documentElement.classList.remove('dark');
    `);
    await navigateTo('', 1200);
    await waitForCondition('Login email input is rendered', `Boolean(document.querySelector('input[type="email"]'))`);
    await capture(`${suite.lang}/${suite.theme}/01_login_page.png`);

    // Authenticate Admin session
    await evaluate(`
      localStorage.setItem('token', ${JSON.stringify(token)});
      localStorage.setItem('user', ${JSON.stringify(JSON.stringify(user))});
      localStorage.setItem('theme', '${suite.theme}');
      localStorage.setItem('i18nextLng', '${suite.lang}');
      if ('${suite.theme}' === 'dark') document.documentElement.classList.add('dark');
      else document.documentElement.classList.remove('dark');
      window.location.hash = '#/dashboard';
    `);
    await reloadPage(1500);

    // 3. Dashboard (Admin View)
    await switchNavTab('dashboard');
    await new Promise(r => setTimeout(r, 800));
    await waitForCondition('Dashboard sessions table is rendered', `Boolean(document.querySelector('table'))`);
    await capture(`${suite.lang}/${suite.theme}/02_dashboard.png`);

    // 4. Dashboard (User View)
    await evaluate(`
      const toggleContainer = document.querySelector('div.text-xs.border.subtle-border');
      if (toggleContainer) {
        const btns = toggleContainer.querySelectorAll('button');
        if (btns.length >= 2) btns[1].click();
      }
    `);
    await new Promise(r => setTimeout(r, 800));
    await waitForCondition('Dashboard User View is active', `document.body.innerText.includes('My Personal Devices') || document.body.innerText.includes('Thiết bị của tôi')`);
    await capture(`${suite.lang}/${suite.theme}/02b_dashboard_user_view.png`);

    // Switch back to Admin View
    await evaluate(`
      const toggleContainer = document.querySelector('div.text-xs.border.subtle-border');
      if (toggleContainer) {
        const btns = toggleContainer.querySelectorAll('button');
        if (btns.length >= 1) btns[0].click();
      }
    `);
    await new Promise(r => setTimeout(r, 500));

    // 5. Resources / Subnets View
    await switchNavTab('resources');
    await new Promise(r => setTimeout(r, 800));
    // Click Subnets tab (button index 1 in subnav)
    await evaluate(`
      const subnav = document.querySelector('main div.surface-header.text-xs');
      if (subnav) {
        const btns = subnav.querySelectorAll('button');
        if (btns.length >= 2) btns[1].click();
      }
    `);
    await new Promise(r => setTimeout(r, 800));
    await waitForCondition('Subnets view is active (Engineering VPC visible)', `document.body.innerText.includes('Engineering VPC')`);
    await capture(`${suite.lang}/${suite.theme}/03_resources_subnets.png`);

    // 6. Resources / Add Subnet Modal
    await clickAddPrimaryButton();
    await waitForCondition('Add Subnet dialog is open', `Boolean(document.querySelector('[role="dialog"], [role="alertdialog"]'))`);
    await evaluate(`
      const modal = document.querySelector('[role="dialog"], [role="alertdialog"]');
      if (modal) {
        const inputs = modal.querySelectorAll('input');
        if (inputs.length >= 2) {
          window.__setReactVal(inputs[0], 'Production Services VPC');
          window.__setReactVal(inputs[1], '10.200.0.0/16');
        }
      }
    `);
    await new Promise(r => setTimeout(r, 400));
    await waitForCondition('Subnet CIDR preview is rendered', `Boolean(document.getElementById('subnet-preview'))`);
    await capture(`${suite.lang}/${suite.theme}/04_modal_add_subnet.png`);
    await closeModal();

    // 7. Resources / Devices Table
    await evaluate(`
      const subnav = document.querySelector('main div.surface-header.text-xs');
      if (subnav) {
        const btns = subnav.querySelectorAll('button');
        if (btns.length >= 1) btns[0].click();
      }
    `);
    await new Promise(r => setTimeout(r, 800));
    await waitForCondition('Devices table is active (alice-macbook-pro visible)', `document.body.innerText.includes('alice-macbook-pro')`);
    await capture(`${suite.lang}/${suite.theme}/05_resources_devices.png`);

    // 8. Resources / Add Device Modal
    await clickAddPrimaryButton();
    await waitForCondition('Add Device dialog is open', `Boolean(document.querySelector('[role="dialog"], [role="alertdialog"]'))`);
    await capture(`${suite.lang}/${suite.theme}/06_modal_add_device.png`);
    await closeModal();

    // 9. Android Native Setup Modal (19_modal_android_setup.png)
    await evaluate(`
      const rows = Array.from(document.querySelectorAll('tbody tr'));
      const androidRow = rows.find(r => r.textContent.toLowerCase().includes('android') || r.textContent.includes('pixel'));
      if (androidRow) {
        const setupBtn = androidRow.querySelector('td:last-child button');
        if (setupBtn) setupBtn.click();
      }
    `);
    await new Promise(r => setTimeout(r, 800));
    await waitForCondition('Android setup dialog is open', `Boolean(document.querySelector('[role="dialog"], [role="alertdialog"]')) && document.querySelector('[role="dialog"], [role="alertdialog"]').innerText.includes('Android')`);
    await capture(`${suite.lang}/${suite.theme}/19_modal_android_setup.png`);

    // Close android modal
    await evaluate(`
      const closeBtn = document.querySelector(':is([role="dialog"], [role="alertdialog"]) button[aria-label="Close"]') || Array.from(document.querySelectorAll(':is([role="dialog"], [role="alertdialog"]) button')).find(b => b.textContent.trim() === 'Close' || b.textContent.trim() === 'Đóng');
      if (closeBtn) closeBtn.click();
    `);
    await new Promise(r => setTimeout(r, 500));

    // 10. Confirmation Dialog (20_confirm_dialog.png)
    await evaluate(`
      const rows = Array.from(document.querySelectorAll('tbody tr'));
      const activeRow = rows.find(r => !r.textContent.toLowerCase().includes('revoked') && !r.textContent.toLowerCase().includes('thu hồi'));
      if (activeRow) {
        const revokeBtn = activeRow.querySelector('button.text-status-danger') || activeRow.querySelector('button:last-child');
        if (revokeBtn) revokeBtn.click();
      }
    `);
    await new Promise(r => setTimeout(r, 800));
    await waitForCondition('Confirmation dialog is open', `Boolean(document.querySelector('[role="dialog"], [role="alertdialog"]'))`);
    await capture(`${suite.lang}/${suite.theme}/20_confirm_dialog.png`);

    // Cancel confirmation dialog
    await evaluate(`
      const cancelBtn = Array.from(document.querySelectorAll(':is([role="dialog"], [role="alertdialog"]) button')).find(b => b.textContent.includes('Cancel') || b.textContent.includes('Hủy'));
      if (cancelBtn) cancelBtn.click();
    `);
    await new Promise(r => setTimeout(r, 500));

    // 11. Users Directory (07_users_directory.png)
    await switchNavTab('users');
    await new Promise(r => setTimeout(r, 800));
    // Ensure Directory subview
    await evaluate(`
      const subnav = document.querySelector('main div.surface-header.text-xs');
      if (subnav) {
        const btns = subnav.querySelectorAll('button');
        if (btns.length >= 1) btns[0].click();
      }
    `);
    await new Promise(r => setTimeout(r, 800));
    await waitForCondition('Users directory is active', `document.body.innerText.includes('alice.chen') || document.body.innerText.includes('admin@enterprise.corp')`);
    await capture(`${suite.lang}/${suite.theme}/07_users_directory.png`);

    // 11b. Focus on local auth row action (07_users_directory_local_auth_focus.png)
    await evaluate(`
      const localBadge = Array.from(document.querySelectorAll('tbody span')).find(s => s.textContent.toLowerCase().includes('local') || s.textContent.toLowerCase().includes('nội bộ'));
      if (localBadge) {
        const row = localBadge.closest('tr');
        const btn = row.querySelector('button.text-status-danger') || row.querySelector('td:last-child button');
        if (btn) {
          btn.focus();
          btn.classList.add('ring-2', 'ring-blue-500', 'ring-offset-2');
        }
      }
    `);
    await new Promise(r => setTimeout(r, 400));
    await capture(`${suite.lang}/${suite.theme}/07_users_directory_local_auth_focus.png`);

    // 12. Modal Add User (08_modal_add_user.png)
    await clickAddPrimaryButton();
    await waitForCondition('Add User dialog is open', `Boolean(document.querySelector('[role="dialog"], [role="alertdialog"]'))`);
    await capture(`${suite.lang}/${suite.theme}/08_modal_add_user.png`);
    await closeModal();

    // 13. Users / User Groups (09_user_groups.png)
    await evaluate(`
      const subnav = document.querySelector('main div.surface-header.text-xs');
      if (subnav) {
        const btns = subnav.querySelectorAll('button');
        if (btns.length >= 2) btns[1].click();
      }
    `);
    await new Promise(r => setTimeout(r, 800));
    await waitForCondition('User Groups table is active', `document.body.innerText.includes('Engineering') || document.body.innerText.includes('Kỹ sư') || document.body.innerText.includes('Core Engineering')`);
    await capture(`${suite.lang}/${suite.theme}/09_user_groups.png`);

    // 14. Modal Add User Group (10_modal_add_group.png)
    await clickAddPrimaryButton();
    await waitForCondition('Add User Group dialog is open', `Boolean(document.querySelector('[role="dialog"], [role="alertdialog"]'))`);
    await capture(`${suite.lang}/${suite.theme}/10_modal_add_group.png`);
    await closeModal();

    // 15. Access Rules (11_access_rules.png)
    await switchNavTab('access');
    await new Promise(r => setTimeout(r, 1000));
    await waitForCondition('Access Rules table is active', `document.body.innerText.includes('ALLOW') && document.body.innerText.includes('DENY')`);
    await capture(`${suite.lang}/${suite.theme}/11_access_rules.png`);

    // 15b. Filter DENY (11_access_rules_deny_filter.png)
    await evaluate(`
      const denyChip = Array.from(document.querySelectorAll('button')).find(b => b.textContent.trim().startsWith('DENY'));
      if (denyChip) denyChip.click();
    `);
    await new Promise(r => setTimeout(r, 600));
    await capture(`${suite.lang}/${suite.theme}/11_access_rules_deny_filter.png`);

    // Reset filter
    await evaluate(`
      const allChip = Array.from(document.querySelectorAll('button')).find(b => b.textContent.toLowerCase().includes('all') || b.textContent.toLowerCase().includes('tất cả'));
      if (allChip) allChip.click();
    `);
    await new Promise(r => setTimeout(r, 500));

    // 15c. Effective Access: ALLOW (11_access_rules_effective_allow.png)
    await evaluate(`
      const panel = document.querySelector('section[aria-labelledby="effective-policy-title"]');
      if (panel) {
        const select = panel.querySelector('select');
        window.__setReactVal(select, 'dev_ff131e1a-c988-4997-85ba-5372b42a02c7');
        const input = panel.querySelector('input');
        window.__setReactVal(input, '10.99.2.10');
        const form = panel.querySelector('form');
        if (form) form.dispatchEvent(new Event('submit', { bubbles: true }));
      }
    `);
    await new Promise(r => setTimeout(r, 800));
    await waitForCondition('Effective Access ALLOW verdict rendered', `
      const panel = document.querySelector('section[aria-labelledby="effective-policy-title"]');
      return panel && (panel.innerText.includes('ALLOW') || panel.innerText.includes('Allowed') || panel.innerText.includes('cho phép'));
    `);
    await capture(`${suite.lang}/${suite.theme}/11_access_rules_effective_allow.png`);

    // 15d. Effective Access: EXPLICIT DENY (11_access_rules_effective_explicit_deny.png)
    await evaluate(`
      const panel = document.querySelector('section[aria-labelledby="effective-policy-title"]');
      if (panel) {
        const input = panel.querySelector('input');
        window.__setReactVal(input, '10.99.1.10');
        const form = panel.querySelector('form');
        if (form) form.dispatchEvent(new Event('submit', { bubbles: true }));
      }
    `);
    await new Promise(r => setTimeout(r, 800));
    await waitForCondition('Effective Access EXPLICIT DENY verdict rendered', `
      const panel = document.querySelector('section[aria-labelledby="effective-policy-title"]');
      return panel && (panel.innerText.includes('DENY') || panel.innerText.includes('Denied') || panel.innerText.includes('chặn') || Boolean(panel.querySelector('.text-status-danger')));
    `);
    await capture(`${suite.lang}/${suite.theme}/11_access_rules_effective_explicit_deny.png`);

    // 15e. Effective Access: DEFAULT DENY (11_access_rules_effective_default_deny.png)
    await evaluate(`
      const panel = document.querySelector('section[aria-labelledby="effective-policy-title"]');
      if (panel) {
        const input = panel.querySelector('input');
        window.__setReactVal(input, '172.16.0.1');
        const form = panel.querySelector('form');
        if (form) form.dispatchEvent(new Event('submit', { bubbles: true }));
      }
    `);
    await new Promise(r => setTimeout(r, 800));
    await waitForCondition('Effective Access DEFAULT DENY verdict rendered', `
      const panel = document.querySelector('section[aria-labelledby="effective-policy-title"]');
      return panel && (panel.innerText.includes('DENY') || panel.innerText.includes('Denied') || panel.innerText.includes('chặn') || Boolean(panel.querySelector('.text-status-danger')));
    `);
    await capture(`${suite.lang}/${suite.theme}/11_access_rules_effective_default_deny.png`);

    // 16. Modal Add Rule (12_modal_add_rule.png)
    await clickAddPrimaryButton();
    await waitForCondition('Add Rule dialog is open', `Boolean(document.querySelector('[role="dialog"], [role="alertdialog"]'))`);
    await capture(`${suite.lang}/${suite.theme}/12_modal_add_rule.png`);

    // 16b. Add Rule DENY Action (12_modal_add_rule_deny_action.png)
    await evaluate(`
      const denyBtn = Array.from(document.querySelectorAll(':is([role="dialog"], [role="alertdialog"]) button')).find(b => b.textContent.trim() === 'DENY');
      if (denyBtn) denyBtn.click();
    `);
    await new Promise(r => setTimeout(r, 400));
    await waitForCondition('DENY action is selected in modal', `
      const denyBtn = Array.from(document.querySelectorAll(':is([role="dialog"], [role="alertdialog"]) button')).find(b => b.textContent.trim() === 'DENY');
      return denyBtn && denyBtn.getAttribute('aria-pressed') === 'true';
    `);
    await capture(`${suite.lang}/${suite.theme}/12_modal_add_rule_deny_action.png`);

    // 16c. Add Rule Device Select Open (12_modal_add_rule_device_select_open.png)
    await evaluate(`
      const devTypeBtn = Array.from(document.querySelectorAll(':is([role="dialog"], [role="alertdialog"]) button')).find(b => b.textContent.includes('Device') || b.textContent.includes('Thiết bị'));
      if (devTypeBtn) devTypeBtn.click();
      const sel = document.getElementById('access-rule-subject');
      if (sel) sel.focus();
    `);
    await new Promise(r => setTimeout(r, 400));
    await capture(`${suite.lang}/${suite.theme}/12_modal_add_rule_device_select_open.png`);

    // 16d. Add Rule Device Selected (12_modal_add_rule_device_selected.png)
    await evaluate(`
      const sel = document.getElementById('access-rule-subject');
      if (sel && sel.options.length > 1) {
        sel.selectedIndex = 1;
        window.__setReactVal(sel, sel.options[1].value);
      }
    `);
    await new Promise(r => setTimeout(r, 400));
    await waitForCondition('Device is selected in Add Rule modal', `document.getElementById('access-rule-subject').value !== ''`);
    await capture(`${suite.lang}/${suite.theme}/12_modal_add_rule_device_selected.png`);

    // 16e. Add Rule Subject Select Open (12_modal_add_rule_subject_select_open.png)
    await evaluate(`
      const userTypeBtn = Array.from(document.querySelectorAll(':is([role="dialog"], [role="alertdialog"]) button')).find(b => b.textContent.includes('User') || b.textContent.includes('Người dùng'));
      if (userTypeBtn) userTypeBtn.click();
      const sel = document.getElementById('access-rule-subject');
      if (sel) {
        window.__setReactVal(sel, '');
        sel.focus();
      }
    `);
    await new Promise(r => setTimeout(r, 400));
    await capture(`${suite.lang}/${suite.theme}/12_modal_add_rule_subject_select_open.png`);

    // 16f. Add Rule User Selected (12_modal_add_rule_user_selected.png)
    await evaluate(`
      const sel = document.getElementById('access-rule-subject');
      if (sel && sel.options.length > 1) {
        sel.selectedIndex = 1;
        window.__setReactVal(sel, sel.options[1].value);
      }
    `);
    await new Promise(r => setTimeout(r, 400));
    await waitForCondition('User is selected in Add Rule modal', `document.getElementById('access-rule-subject').value !== ''`);
    await capture(`${suite.lang}/${suite.theme}/12_modal_add_rule_user_selected.png`);

    // Close Add Rule modal
    await closeModal();

    // 17. DNS Engine (13_dns_engine.png)
    await switchNavTab('dns');
    await new Promise(r => setTimeout(r, 1000));
    await waitForCondition('DNS Engine table is rendered', `document.body.innerText.includes('A') || document.body.innerText.includes('CNAME')`);
    await capture(`${suite.lang}/${suite.theme}/13_dns_engine.png`);

    // 18. Modal Add DNS Override (14_modal_add_dns_override.png)
    await clickAddPrimaryButton();
    await waitForCondition('Add DNS Override dialog is open', `Boolean(document.querySelector('[role="dialog"], [role="alertdialog"]'))`);
    await capture(`${suite.lang}/${suite.theme}/14_modal_add_dns_override.png`);

    // 18b. Disabled Save state (14_modal_add_dns_override_disabled_save.png)
    await waitForCondition('Save button is disabled on empty DNS form', `
      const saveBtn = Array.from(document.querySelectorAll(':is([role="dialog"], [role="alertdialog"]) button')).find(b => b.textContent.includes('Save') || b.textContent.includes('Lưu'));
      return saveBtn && saveBtn.disabled;
    `);
    await capture(`${suite.lang}/${suite.theme}/14_modal_add_dns_override_disabled_save.png`);

    // 18c. Record Type dropdown focused (14_modal_add_dns_override_record_type_open.png)
    await evaluate(`
      const typeSel = document.querySelector(':is([role="dialog"], [role="alertdialog"]) select');
      if (typeSel) typeSel.focus();
    `);
    await new Promise(r => setTimeout(r, 400));
    await capture(`${suite.lang}/${suite.theme}/14_modal_add_dns_override_record_type_open.png`);

    // 18d. CNAME record filled (14_modal_add_dns_override_cname.png)
    await evaluate(`
      const modal = document.querySelector('[role="dialog"], [role="alertdialog"]');
      if (modal) {
        const typeSel = modal.querySelector('select');
        window.__setReactVal(typeSel, 'CNAME');
        const inputs = modal.querySelectorAll('input');
        if (inputs.length >= 2) {
          window.__setReactVal(inputs[0], 'api.internal.corp');
          window.__setReactVal(inputs[1], 'gateway.enterprise.corp');
        }
      }
    `);
    await new Promise(r => setTimeout(r, 500));
    await waitForCondition('CNAME inputs populated', `
      const typeSel = document.querySelector(':is([role="dialog"], [role="alertdialog"]) select');
      return typeSel && typeSel.value === 'CNAME';
    `);
    await capture(`${suite.lang}/${suite.theme}/14_modal_add_dns_override_cname.png`);

    // 18e. Validation Error on invalid A record (14_modal_add_dns_override_error.png)
    await evaluate(`
      const modal = document.querySelector('[role="dialog"], [role="alertdialog"]');
      if (modal) {
        const typeSel = modal.querySelector('select');
        window.__setReactVal(typeSel, 'A');
        const inputs = modal.querySelectorAll('input');
        if (inputs.length >= 2) {
          window.__setReactVal(inputs[0], 'db.internal.corp');
          window.__setReactVal(inputs[1], '999.999.999.999');
          inputs[1].dispatchEvent(new Event('blur', { bubbles: true }));
        }
      }
    `);
    await new Promise(r => setTimeout(r, 500));
    await capture(`${suite.lang}/${suite.theme}/14_modal_add_dns_override_error.png`);

    // Close DNS modal
    await closeModal();

    // 19. OIDC Providers (15_oidc_providers.png)
    await switchNavTab('oidc');
    await new Promise(r => setTimeout(r, 1000));
    await waitForCondition('OIDC table is rendered', `document.body.innerText.includes('Enterprise Okta SSO')`);
    await capture(`${suite.lang}/${suite.theme}/15_oidc_providers.png`);

    // 20. Modal Add OIDC Provider (16_modal_add_oidc.png)
    await clickAddPrimaryButton();
    await waitForCondition('Add OIDC dialog is open', `Boolean(document.querySelector('[role="dialog"], [role="alertdialog"]'))`);
    await capture(`${suite.lang}/${suite.theme}/16_modal_add_oidc.png`);

    // Close OIDC modal
    await closeModal();

    // 21. Server Settings (Basic) (17_server_settings.png)
    await switchNavTab('server');
    await new Promise(r => setTimeout(r, 1000));
    await evaluate(`
      const tabs = document.querySelectorAll('main nav button');
      if (tabs.length >= 3) tabs[0].click();
    `);
    await new Promise(r => setTimeout(r, 800));
    await waitForCondition('Server Settings basic form rendered', `Boolean(document.querySelector('input'))`);
    await capture(`${suite.lang}/${suite.theme}/17_server_settings.png`);

    // 21b. Server Settings Unsaved banner (17_server_settings_unsaved.png)
    await evaluate(`
      const fqdnInput = Array.from(document.querySelectorAll('input')).find(i => i.value.includes('vpn') || i.value.includes('192') || i.value.includes('corp') || i.value.includes('localhost'));
      if (fqdnInput) {
        window.__setReactVal(fqdnInput, 'vpn-dirty-audit.enterprise.corp');
      }
    `);
    await new Promise(r => setTimeout(r, 600));
    await waitForCondition('Unsaved changes warning banner visible', `
      const alert = document.querySelector('p[role="status"]') || Array.from(document.querySelectorAll('p, div')).find(el => el.textContent.includes('unsaved') || el.textContent.includes('chưa lưu'));
      return Boolean(alert);
    `);
    await capture(`${suite.lang}/${suite.theme}/17_server_settings_unsaved.png`);

    // 22. Server Settings (CA Tab) (17b_server_settings_ca.png)
    mockCanRotateCA = true;
    await evaluate(`
      const tabs = document.querySelectorAll('main nav button');
      if (tabs.length >= 3) tabs[2].click();
    `);
    await new Promise(r => setTimeout(r, 800));
    await waitForCondition('CA tab rendered', `document.body.innerText.includes('SwanPanel Internal Root CA')`);
    await capture(`${suite.lang}/${suite.theme}/17b_server_settings_ca.png`);

    // 22b. CA Rotation Confirm Dialog (21_ca_rotate_confirm.png)
    await evaluate(`
      const details = document.querySelector('details');
      if (details) {
        details.open = true;
        const inputs = details.querySelectorAll('input');
        const confirmInput = inputs[inputs.length - 1];
        if (confirmInput) {
          window.__setReactVal(confirmInput, 'ROTATE CA');
        }
        const rotateBtn = details.querySelector('button');
        if (rotateBtn) rotateBtn.click();
      }
    `);
    await new Promise(r => setTimeout(r, 800));
    await waitForCondition('CA Rotate Confirmation Dialog opened', `Boolean(document.querySelector('[role="dialog"], [role="alertdialog"]'))`);
    await capture(`${suite.lang}/${suite.theme}/21_ca_rotate_confirm.png`);

    // Cancel CA Rotate dialog
    await evaluate(`
      const cancelBtn = Array.from(document.querySelectorAll(':is([role="dialog"], [role="alertdialog"]) button')).find(b => b.textContent.includes('Cancel') || b.textContent.includes('Hủy'));
      if (cancelBtn) cancelBtn.click();
    `);
    await new Promise(r => setTimeout(r, 500));
    mockCanRotateCA = false;

    // 23. Server Settings (Advanced JSON) (17c_server_settings_advanced.png)
    await evaluate(`
      const tabs = document.querySelectorAll('main nav button');
      if (tabs.length >= 3) tabs[1].click();
    `);
    await new Promise(r => setTimeout(r, 800));
    await waitForCondition('Advanced JSON editor rendered', `Boolean(document.querySelector('textarea') || document.querySelector('pre'))`);
    await capture(`${suite.lang}/${suite.theme}/17c_server_settings_advanced.png`);

    // 24. System Logs (18_system_logs.png)
    await switchNavTab('logs');
    await new Promise(r => setTimeout(r, 1000));
    await waitForCondition('System logs terminal rendered', `document.body.innerText.includes('SYSTEM LOGS') || document.body.innerText.includes('NHẬT KÝ HỆ THỐNG') || Boolean(document.querySelector('div.font-mono'))`);
    await capture(`${suite.lang}/${suite.theme}/18_system_logs.png`);
  }

  // --- PART 2: RESPONSIVE SCREENSHOTS (Group C - 26 files) ---
  console.log(`\n======================================================`);
  console.log(`=== Processing Group C: Responsive Screenshots (26) ===`);
  console.log(`======================================================`);

  // Ensure logged in admin, dark theme, English baseline for responsive tests
  await evaluate(`
    localStorage.setItem('token', ${JSON.stringify(token)});
    localStorage.setItem('user', ${JSON.stringify(JSON.stringify(user))});
    localStorage.setItem('theme', 'dark');
    localStorage.setItem('i18nextLng', 'en');
    if (document.documentElement) document.documentElement.classList.add('dark');
    window.location.hash = '#/dashboard';
  `);
  await reloadPage(1500);
  await waitForCondition('Dashboard sessions table is rendered', `Boolean(document.querySelector('table'))`);

  // 1. access_rules_320px.png & access_rules_768px.png
  await switchNavTab('access');
  await setViewport(320, 640, 2, true);
  await new Promise(r => setTimeout(r, 800));
  await capture('responsive/access_rules_320px.png');

  await setViewport(768, 1024, 2, false);
  await new Promise(r => setTimeout(r, 800));
  await capture('responsive/access_rules_768px.png');

  // 2. android_modal_320px.png & android_modal_768px.png
  await switchNavTab('resources');
  await setViewport(1440, 900, 2, false);
  await new Promise(r => setTimeout(r, 800));
  // Open Android modal
  await evaluate(`
    const rows = Array.from(document.querySelectorAll('tbody tr'));
    const androidRow = rows.find(r => r.textContent.toLowerCase().includes('android') || r.textContent.includes('pixel'));
    if (androidRow) {
      const setupBtn = androidRow.querySelector('td:last-child button');
      if (setupBtn) setupBtn.click();
    }
  `);
  await new Promise(r => setTimeout(r, 800));
  await waitForCondition('Android modal open for responsive', `Boolean(document.querySelector('[role="dialog"], [role="alertdialog"]'))`);

  await setViewport(320, 640, 2, true);
  await new Promise(r => setTimeout(r, 800));
  await capture('responsive/android_modal_320px.png');

  await setViewport(768, 1024, 2, false);
  await new Promise(r => setTimeout(r, 800));
  await capture('responsive/android_modal_768px.png');

  // Close modal
  await evaluate(`
    const closeBtn = document.querySelector(':is([role="dialog"], [role="alertdialog"]) button[aria-label="Close"]') || Array.from(document.querySelectorAll(':is([role="dialog"], [role="alertdialog"]) button')).find(b => b.textContent.trim() === 'Close');
    if (closeBtn) closeBtn.click();
  `);
  await new Promise(r => setTimeout(r, 500));

  // 3. server_settings_320px.png & server_settings_768px.png
  await switchNavTab('server');
  await setViewport(320, 640, 2, true);
  await new Promise(r => setTimeout(r, 800));
  await capture('responsive/server_settings_320px.png');

  await setViewport(768, 1024, 2, false);
  await new Promise(r => setTimeout(r, 800));
  await capture('responsive/server_settings_768px.png');

  // 4. layout_320px_dashboard.png, layout_768px_dashboard.png, layout_zoom200_dashboard.png
  await switchNavTab('dashboard');
  await setViewport(320, 640, 2, true);
  await new Promise(r => setTimeout(r, 800));
  await capture('responsive/layout_320px_dashboard.png');

  await setViewport(768, 1024, 2, false);
  await new Promise(r => setTimeout(r, 800));
  await capture('responsive/layout_768px_dashboard.png');

  // Zoom 200% on dashboard: viewport 720x450 at scale 2
  await setViewport(720, 450, 2, false);
  await new Promise(r => setTimeout(r, 800));
  await capture('responsive/layout_zoom200_dashboard.png');

  // 5. Dashboard Table Overflow & Keyboard Focus Navigation at 320px:
  // dashboard_320px_table_scroll_start.png, dashboard_320px_table_scroll_end.png
  // dashboard_320px_action_keyboard_focus.png, dashboard_320px_table_keyboard_step1_region.png
  // dashboard_320px_table_keyboard_step2_scrolled.png, dashboard_320px_keyboard_step3_dialog_opened.png
  // dashboard_320px_keyboard_step4_dialog_dismissed.png
  await setViewport(320, 640, 2, true);
  await switchNavTab('dashboard');
  await new Promise(r => setTimeout(r, 800));

  // Scroll start
  await evaluate(`
    const scroller = document.querySelector('div[role="region"]') || document.querySelector('.overflow-x-auto');
    if (scroller) scroller.scrollLeft = 0;
  `);
  await new Promise(r => setTimeout(r, 400));
  await capture('responsive/dashboard_320px_table_scroll_start.png');

  // Step 1: focus table region
  await evaluate(`
    const region = document.querySelector('div[role="region"]') || document.querySelector('.overflow-x-auto');
    if (region) region.focus();
  `);
  await new Promise(r => setTimeout(r, 400));
  await capture('responsive/dashboard_320px_table_keyboard_step1_region.png');

  // Scroll end
  await evaluate(`
    const scroller = document.querySelector('div[role="region"]') || document.querySelector('.overflow-x-auto');
    if (scroller) scroller.scrollLeft = scroller.scrollWidth;
  `);
  await new Promise(r => setTimeout(r, 400));
  await capture('responsive/dashboard_320px_table_scroll_end.png');
  await capture('responsive/dashboard_320px_table_keyboard_step2_scrolled.png');

  // Focus action button (Disconnect / Revoke)
  await evaluate(`
    const btn = document.querySelector('tbody button.text-status-danger') || document.querySelector('tbody td:last-child button');
    if (btn) {
      btn.focus();
      btn.classList.add('ring-2', 'ring-rose-500', 'ring-offset-2');
    }
  `);
  await new Promise(r => setTimeout(r, 400));
  await capture('responsive/dashboard_320px_action_keyboard_focus.png');

  // Open disconnect confirmation dialog
  await evaluate(`
    const btn = document.querySelector('tbody button.text-status-danger') || document.querySelector('tbody td:last-child button');
    if (btn) btn.click();
  `);
  await new Promise(r => setTimeout(r, 600));
  await waitForCondition('Disconnect dialog open at 320px', `Boolean(document.querySelector(':is([role="dialog"], [role="alertdialog"])'))`);
  await capture('responsive/dashboard_320px_keyboard_step3_dialog_opened.png');

  // Dismiss dialog
  await evaluate(`
    const cancelBtn = Array.from(document.querySelectorAll(':is([role="dialog"], [role="alertdialog"]) button')).find(b => b.textContent.includes('Cancel') || b.textContent.includes('Hủy'));
    if (cancelBtn) cancelBtn.click();
  `);
  await new Promise(r => setTimeout(r, 500));
  await waitForCondition('Disconnect dialog dismissed at 320px', `!document.querySelector(':is([role="dialog"], [role="alertdialog"])')`);
  await capture('responsive/dashboard_320px_keyboard_step4_dialog_dismissed.png');

  // 6. Dashboard Table Overflow & Keyboard Focus Navigation at 768px:
  // dashboard_768px_table_scroll_start.png, dashboard_768px_table_scroll_end.png
  // dashboard_768px_action_keyboard_focus.png, dashboard_768px_table_keyboard_step1_region.png
  // dashboard_768px_table_keyboard_step2_scrolled.png, dashboard_768px_keyboard_step3_dialog_opened.png
  // dashboard_768px_keyboard_step4_dialog_dismissed.png
  await setViewport(768, 1024, 2, false);
  await switchNavTab('dashboard');
  await new Promise(r => setTimeout(r, 800));

  // Scroll start
  await evaluate(`
    const scroller = document.querySelector('div[role="region"]') || document.querySelector('.overflow-x-auto');
    if (scroller) scroller.scrollLeft = 0;
  `);
  await new Promise(r => setTimeout(r, 400));
  await capture('responsive/dashboard_768px_table_scroll_start.png');

  // Step 1: focus table region
  await evaluate(`
    const region = document.querySelector('div[role="region"]') || document.querySelector('.overflow-x-auto');
    if (region) region.focus();
  `);
  await new Promise(r => setTimeout(r, 400));
  await capture('responsive/dashboard_768px_table_keyboard_step1_region.png');

  // Scroll end
  await evaluate(`
    const scroller = document.querySelector('div[role="region"]') || document.querySelector('.overflow-x-auto');
    if (scroller) scroller.scrollLeft = scroller.scrollWidth;
  `);
  await new Promise(r => setTimeout(r, 400));
  await capture('responsive/dashboard_768px_table_scroll_end.png');
  await capture('responsive/dashboard_768px_table_keyboard_step2_scrolled.png');

  // Focus action button
  await evaluate(`
    const btn = document.querySelector('tbody button.text-status-danger') || document.querySelector('tbody td:last-child button');
    if (btn) {
      btn.focus();
      btn.classList.add('ring-2', 'ring-rose-500', 'ring-offset-2');
    }
  `);
  await new Promise(r => setTimeout(r, 400));
  await capture('responsive/dashboard_768px_action_keyboard_focus.png');

  // Open dialog
  await evaluate(`
    const btn = document.querySelector('tbody button.text-status-danger') || document.querySelector('tbody td:last-child button');
    if (btn) btn.click();
  `);
  await new Promise(r => setTimeout(r, 600));
  await waitForCondition('Disconnect dialog open at 768px', `Boolean(document.querySelector(':is([role="dialog"], [role="alertdialog"])'))`);
  await capture('responsive/dashboard_768px_keyboard_step3_dialog_opened.png');

  // Dismiss dialog
  await evaluate(`
    const cancelBtn = Array.from(document.querySelectorAll(':is([role="dialog"], [role="alertdialog"]) button')).find(b => b.textContent.includes('Cancel') || b.textContent.includes('Hủy'));
    if (cancelBtn) cancelBtn.click();
  `);
  await new Promise(r => setTimeout(r, 500));
  await waitForCondition('Disconnect dialog dismissed at 768px', `!document.querySelector(':is([role="dialog"], [role="alertdialog"])')`);
  await capture('responsive/dashboard_768px_keyboard_step4_dialog_dismissed.png');

  // 7. Mobile Sidebar Open (320px) in EN & VI
  await setViewport(320, 640, 2, true);
  // EN:
  await evaluate(`
    localStorage.setItem('i18nextLng', 'en');
  `);
  await reloadPage(1200);
  await evaluate(`
    const menuBtn = document.querySelector('header button');
    if (menuBtn) menuBtn.click();
  `);
  await new Promise(r => setTimeout(r, 600));
  await waitForCondition('Mobile sidebar open EN', `Boolean(document.querySelector('aside.translate-x-0'))`);
  await capture('responsive/sidebar_320px_mobile_open_en.png');
  await evaluate(`
    const closeBtn = document.querySelector('aside button[aria-label="Close"]') || Array.from(document.querySelectorAll('aside button')).find(b => b.textContent.trim() === '×') || document.querySelector('.fixed.inset-0');
    if (closeBtn) closeBtn.click();
  `);
  await new Promise(r => setTimeout(r, 400));

  // VI:
  await evaluate(`
    const menuBtn = document.querySelector('header button');
    if (menuBtn) menuBtn.click();
  `);
  await new Promise(r => setTimeout(r, 600));
  await waitForCondition('Mobile sidebar open for VI toggle', `Boolean(document.querySelector('aside.translate-x-0'))`);
  // Click language toggle button to switch to VI
  await evaluate(`
    const langBtn = Array.from(document.querySelectorAll('aside button')).find(b => b.textContent.includes('EN') || b.textContent.includes('VI'));
    if (langBtn && langBtn.textContent.includes('EN')) {
      langBtn.click();
    }
  `);
  await new Promise(r => setTimeout(r, 600));
  await waitForCondition('Mobile sidebar open VI', `Boolean(document.querySelector('aside.translate-x-0')) && (document.querySelector('aside').innerText.includes('Tài nguyên') || document.querySelector('aside').innerText.includes('Bảng điều khiển'))`);
  await capture('responsive/sidebar_320px_mobile_open_vi.png');
  await evaluate(`
    const closeBtn = document.querySelector('aside button[aria-label="Đóng"]') || document.querySelector('aside button[aria-label="Close"]') || Array.from(document.querySelectorAll('aside button')).find(b => b.textContent.trim() === '×') || document.querySelector('.fixed.inset-0');
    if (closeBtn) closeBtn.click();
  `);
  await new Promise(r => setTimeout(r, 400));

  // 8. Desktop 200% Zoom in VI: sidebar_zoom200_vi.png (Must show VI, sidebar open)
  await setViewport(720, 450, 2, false);
  await new Promise(r => setTimeout(r, 600));
  await evaluate(`
    const menuBtn = document.querySelector('header button');
    if (menuBtn) menuBtn.click();
  `);
  await new Promise(r => setTimeout(r, 600));
  await waitForCondition('Sidebar open at Zoom 200% VI', `Boolean(document.querySelector('aside.translate-x-0')) && (document.querySelector('aside').innerText.includes('Tài nguyên') || document.querySelector('aside').innerText.includes('Bảng điều khiển'))`);
  await capture('responsive/sidebar_zoom200_vi.png');
  // Switch back to EN inside sidebar before closing
  await evaluate(`
    const langBtn = Array.from(document.querySelectorAll('aside button')).find(b => b.textContent.includes('EN') || b.textContent.includes('VI'));
    if (langBtn && langBtn.textContent.includes('VI')) {
      langBtn.click();
    }
  `);
  await new Promise(r => setTimeout(r, 600));
  await evaluate(`
    const closeBtn = document.querySelector('aside button[aria-label="Đóng"]') || document.querySelector('aside button[aria-label="Close"]') || Array.from(document.querySelectorAll('aside button')).find(b => b.textContent.trim() === '×') || document.querySelector('.fixed.inset-0');
    if (closeBtn) closeBtn.click();
  `);
  await new Promise(r => setTimeout(r, 400));

  // 9. Additional Responsive Coverage (10 images): Users, Resources, OIDC, Server Settings
  await setViewport(1440, 900, 2, false);

  // Users Page: 320px, 768px, Zoom 200%
  await switchNavTab('users');
  await setViewport(320, 640, 2, true);
  await new Promise(r => setTimeout(r, 600));
  await waitForCondition('Users page rendered at 320px', `document.body.innerText.includes('alice.chen') || document.body.innerText.includes('admin@enterprise.corp')`);
  await capture('responsive/users_320px.png');

  await setViewport(768, 1024, 2, false);
  await new Promise(r => setTimeout(r, 600));
  await capture('responsive/users_768px.png');

  await setViewport(720, 450, 2, false);
  await new Promise(r => setTimeout(r, 600));
  await capture('responsive/users_zoom200.png');

  // Resources Page: 320px, 768px, Zoom 200%
  await switchNavTab('resources');
  await setViewport(320, 640, 2, true);
  await new Promise(r => setTimeout(r, 600));
  await waitForCondition('Resources page rendered at 320px', `document.body.innerText.includes('alice-macbook-pro') || document.body.innerText.includes('Devices')`);
  await capture('responsive/resources_320px.png');

  await setViewport(768, 1024, 2, false);
  await new Promise(r => setTimeout(r, 600));
  await capture('responsive/resources_768px.png');

  await setViewport(720, 450, 2, false);
  await new Promise(r => setTimeout(r, 600));
  await capture('responsive/resources_zoom200.png');

  // OIDC Page: 320px, 768px, Zoom 200%
  await switchNavTab('oidc');
  await setViewport(320, 640, 2, true);
  await new Promise(r => setTimeout(r, 600));
  await waitForCondition('OIDC page rendered at 320px', `document.body.innerText.includes('Enterprise Okta SSO') || document.body.innerText.includes('OIDC')`);
  await capture('responsive/oidc_320px.png');

  await setViewport(768, 1024, 2, false);
  await new Promise(r => setTimeout(r, 600));
  await capture('responsive/oidc_768px.png');

  await setViewport(720, 450, 2, false);
  await new Promise(r => setTimeout(r, 600));
  await capture('responsive/oidc_zoom200.png');

  // Server Settings Page: Zoom 200%
  await switchNavTab('server');
  await setViewport(720, 450, 2, false);
  await new Promise(r => setTimeout(r, 600));
  await waitForCondition('Server Settings rendered at Zoom 200%', `Boolean(document.querySelector('input'))`);
  await capture('responsive/server_settings_zoom200.png');

  ws.close();
  chrome.kill();
  server.kill();
  console.log('\n--- ALL VERIFIED SCREENSHOTS CAPTURED SUCCESSFULLY! ---');
}

run().catch((err) => {
  console.error('Capture failed:', err);
  process.exit(1);
});
