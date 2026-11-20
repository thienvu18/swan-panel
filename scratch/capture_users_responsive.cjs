const { spawn } = require('child_process');
const fs = require('fs');
const path = require('path');

const PORT = 18088;
const DEBUG_PORT = 9228;
const BASE_URL = `http://127.0.0.1:${PORT}/`;

async function run() {
  console.log('--- Capturing and Verifying 3 Responsive Users Screenshots ---');

  try { require('child_process').execSync('pkill -9 -f "chrome-tour-suite" || true'); } catch {}
  try { require('child_process').execSync('pkill -9 -f "swan-panel-test" || true'); } catch {}
  try { require('child_process').execSync('rm -rf /tmp/chrome-tour-suite-users'); } catch {}
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

  // Get Admin token
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

  if (!token) throw new Error('Admin login failed');
  console.log('Authenticated admin:', user.email);

  // Start Chrome
  const chrome = spawn('/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', [
    '--headless=new',
    `--remote-debugging-port=${DEBUG_PORT}`,
    '--user-data-dir=/tmp/chrome-tour-suite-users',
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
  if (!target) throw new Error('Chrome remote debugging connection failed');

  const ws = new WebSocket(target.webSocketDebuggerUrl);
  let msgId = 1;
  const pending = new Map();

  ws.onmessage = (event) => {
    const data = JSON.parse(event.data);
    if (data.id && pending.has(data.id)) {
      pending.get(data.id)(data);
      pending.delete(data.id);
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
      setTimeout(() => reject(new Error('WebSocket timeout')), 5000);
    });
  }

  await send('Page.enable');
  await send('Runtime.enable');
  await send('DOMStorage.enable');

  const setViewport = async (width, height, scale = 2, mobile = false) => {
    await send('Emulation.setDeviceMetricsOverride', {
      width,
      height,
      deviceScaleFactor: scale,
      mobile
    });
    await new Promise(r => setTimeout(r, 300));
  };

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
      throw new Error(`Eval failed: ${res.exceptionDetails.text}`);
    }
    return res.result?.result?.value;
  };

  // Pass session values as CDP data, never as JavaScript source.
  const setSession = async () => {
    const storageId = { securityOrigin: new URL(BASE_URL).origin, isLocalStorage: true };
    for (const [key, value] of [['token', token], ['user', JSON.stringify(user)]]) {
      const response = await send('DOMStorage.setDOMStorageItem', { storageId, key, value });
      if (response.error) throw new Error(`Failed to set session ${key}: ${response.error.message}`);
    }
  };

  const capture = async (relPath) => {
    await evaluate('await document.fonts.ready');
    await new Promise(r => setTimeout(r, 500));
    const shot = await send('Page.captureScreenshot', { format: 'png' });
    const fullPath = path.join(__dirname, '..', 'docs', 'assets', 'images', 'user-guide', relPath);
    fs.mkdirSync(path.dirname(fullPath), { recursive: true });
    fs.writeFileSync(fullPath, Buffer.from(shot.result.data, 'base64'));
    console.log(`[SAVED] ${relPath} (${fs.statSync(fullPath).size} bytes)`);
  };

  // Set logged in session
  await setSession();
  await evaluate(`
    localStorage.setItem('theme', 'dark');
    localStorage.setItem('i18nextLng', 'en');
    if (document.documentElement) document.documentElement.classList.add('dark');
    window.location.hash = '#/users';
  `);
  await send('Page.reload', { ignoreCache: true });
  await new Promise(r => setTimeout(r, 1200));

  // --- 1. users_320px.png ---
  console.log('\n--- Checking 1: users_320px.png (320px) ---');
  await setViewport(320, 640, 2, true);
  await new Promise(r => setTimeout(r, 800));

  const check320 = await evaluate(`
    const subnav = document.querySelector('main div.surface-header.text-xs');
    if (!subnav) return { error: 'Subnav not found' };
    const btns = Array.from(subnav.querySelectorAll('button'));
    const r1 = btns[0]?.getBoundingClientRect();
    const r2 = btns[1]?.getBoundingClientRect();
    const subnavRect = subnav.getBoundingClientRect();
    const bodyWidth = document.body.clientWidth;
    const scrollWidth = document.documentElement.scrollWidth;

    return {
      tabCount: btns.length,
      tab1Text: btns[0]?.innerText.trim(),
      tab2Text: btns[1]?.innerText.trim(),
      tab1Rect: { top: r1?.top, bottom: r1?.bottom, width: r1?.width, height: r1?.height },
      tab2Rect: { top: r2?.top, bottom: r2?.bottom, width: r2?.width, height: r2?.height },
      isVerticalStacked: r1 && r2 ? (r2.top >= r1.bottom - 1) : false,
      subnavWidth: subnavRect.width,
      bodyWidth,
      scrollWidth,
      hasDocumentOverflow: scrollWidth > bodyWidth
    };
  `);
  console.log('Check 320px results:', JSON.stringify(check320, null, 2));
  await capture('responsive/users_320px.png');

  // --- 2. users_768px.png ---
  console.log('\n--- Checking 2: users_768px.png (768px) ---');
  await setViewport(768, 1024, 2, false);
  await new Promise(r => setTimeout(r, 800));

  const check768 = await evaluate(`
    const badges = Array.from(document.querySelectorAll('span')).filter(s => s.textContent.includes('Local Auth'));
    const scroller = document.querySelector('.overflow-x-auto');
    const badge = badges[0];
    const badgeRect = badge?.getBoundingClientRect();
    const computed = badge ? window.getComputedStyle(badge) : null;

    return {
      localAuthCount: badges.length,
      badgeText: badge?.innerText.trim(),
      badgeWidth: badgeRect?.width,
      badgeHeight: badgeRect?.height,
      whiteSpace: computed?.whiteSpace,
      tableScrollerExists: Boolean(scroller),
      scrollerScrollWidth: scroller?.scrollWidth,
      scrollerClientWidth: scroller?.clientWidth,
      tableCanScrollHorizontally: scroller ? (scroller.scrollWidth > scroller.clientWidth) : false
    };
  `);
  console.log('Check 768px results:', JSON.stringify(check768, null, 2));
  await capture('responsive/users_768px.png');

  // --- 3. users_zoom200.png ---
  console.log('\n--- Checking 3: users_zoom200.png (Zoom 200% / 720x450) ---');
  await setViewport(720, 450, 2, false);
  await new Promise(r => setTimeout(r, 800));

  const checkZoom = await evaluate(`
    const badges = Array.from(document.querySelectorAll('span')).filter(s => s.textContent.includes('Local Auth'));
    const badge = badges[0];
    const badgeRect = badge?.getBoundingClientRect();
    const computed = badge ? window.getComputedStyle(badge) : null;

    const subnav = document.querySelector('main div.surface-header.text-xs');
    const subnavRect = subnav?.getBoundingClientRect();

    const searchInput = document.querySelector('input[type="text"]');
    const searchRect = searchInput?.getBoundingClientRect();

    const addBtn = Array.from(document.querySelectorAll('main button')).find(b => b.textContent.includes('Add User'));
    const addBtnRect = addBtn?.getBoundingClientRect();

    const windowWidth = window.innerWidth;
    const bodyWidth = document.body.clientWidth;
    const scrollWidth = document.documentElement.scrollWidth;

    return {
      localAuthBadge: {
        text: badge?.innerText.trim(),
        width: badgeRect?.width,
        height: badgeRect?.height,
        whiteSpace: computed?.whiteSpace,
        isSingleLine: badgeRect ? (badgeRect.height < 28) : false
      },
      subnav: {
        width: subnavRect?.width,
        isClipped: subnavRect ? (subnavRect.right > windowWidth) : false
      },
      toolbar: {
        searchInputWidth: searchRect?.width,
        addBtnWidth: addBtnRect?.width,
        addBtnRight: addBtnRect?.right,
        isClipped: addBtnRect ? (addBtnRect.right > windowWidth) : false
      },
      hasDocumentOverflow: scrollWidth > bodyWidth
    };
  `);
  console.log('Check Zoom 200% results:', JSON.stringify(checkZoom, null, 2));
  await capture('responsive/users_zoom200.png');

  ws.close();
  chrome.kill();
  server.kill();
  console.log('\n--- VERIFICATION AND CAPTURE COMPLETED SUCCESSFULLY ---');
}

run().catch(err => {
  console.error('Failed:', err);
  process.exit(1);
});
