// SwanPanel Documentation & Landing Application
// Fully compliant with Modern Web Guidance (dark-mode, navbar, scrollspy, accessibility)

(function () {
  'use strict';

  // 1. Initialize i18n
  if (window.SwanI18n) {
    window.SwanI18n.init();
    const langToggleBtn = document.getElementById('lang-toggle');
    if (langToggleBtn) {
      langToggleBtn.addEventListener('click', function () {
        window.SwanI18n.toggleLanguage();
      });
    }
  }

  // 2. Theme Management (Modern Web Guidance compliant)
  const themeToggleBtn = document.getElementById('theme-toggle');
  const storedTheme = localStorage.getItem('swanpanel-theme');
  const systemPrefersDark = window.matchMedia('(prefers-color-scheme: dark)');

  function applyTheme(theme) {
    if (theme === 'dark' || theme === 'light') {
      document.documentElement.setAttribute('data-theme', theme);
    } else {
      document.documentElement.removeAttribute('data-theme');
    }
    updateThemeIcon();
  }

  function updateThemeIcon() {
    if (!themeToggleBtn) return;
    const currentTheme = document.documentElement.getAttribute('data-theme') || 
      (systemPrefersDark.matches ? 'dark' : 'light');
    
    // Clean SVG icons (zero unicode emojis)
    if (currentTheme === 'dark') {
      themeToggleBtn.innerHTML = `
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="12" cy="12" r="5"></circle>
          <line x1="12" y1="1" x2="12" y2="3"></line>
          <line x1="12" y1="21" x2="12" y2="23"></line>
          <line x1="4.22" y1="4.22" x2="5.64" y2="5.64"></line>
          <line x1="18.36" y1="18.36" x2="19.78" y2="19.78"></line>
          <line x1="1" y1="12" x2="3" y2="12"></line>
          <line x1="21" y1="12" x2="23" y2="12"></line>
          <line x1="4.22" y1="19.78" x2="5.64" y2="18.36"></line>
          <line x1="18.36" y1="5.64" x2="19.78" y2="4.22"></line>
        </svg>
      `;
      themeToggleBtn.setAttribute('aria-label', 'Switch to light mode');
    } else {
      themeToggleBtn.innerHTML = `
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"></path>
        </svg>
      `;
      themeToggleBtn.setAttribute('aria-label', 'Switch to dark mode');
    }
  }

  if (storedTheme) {
    applyTheme(storedTheme);
  } else {
    updateThemeIcon();
  }

  // Handle system preference changes dynamically
  systemPrefersDark.addEventListener('change', function () {
    if (!localStorage.getItem('swanpanel-theme')) {
      updateThemeIcon();
    }
  });

  if (themeToggleBtn) {
    themeToggleBtn.addEventListener('click', function () {
      const current = document.documentElement.getAttribute('data-theme') || 
        (systemPrefersDark.matches ? 'dark' : 'light');
      const nextTheme = current === 'dark' ? 'light' : 'dark';
      localStorage.setItem('swanpanel-theme', nextTheme);
      applyTheme(nextTheme);
    });
  }

  // 3. Responsive Popover Site Navigation (Zero-flicker CSS + matchMedia sync)
  const siteMenu = document.getElementById('site-menu');

  if (siteMenu) {
    const desktopQuery = window.matchMedia('(min-width: 1024px)');
    
    function syncPopoverState(e) {
      if (e.matches) {
        if (siteMenu.hasAttribute('popover')) {
          try { siteMenu.hidePopover(); } catch(err) {}
          siteMenu.removeAttribute('popover');
        }
      } else {
        if (!siteMenu.hasAttribute('popover')) {
          siteMenu.setAttribute('popover', 'auto');
        }
      }
    }
    
    syncPopoverState(desktopQuery);
    if (typeof desktopQuery.addEventListener === 'function') {
      desktopQuery.addEventListener('change', syncPopoverState);
    } else if (typeof desktopQuery.addListener === 'function') {
      desktopQuery.addListener(syncPopoverState);
    }

    // Close menu when a link is clicked on mobile
    siteMenu.querySelectorAll('a').forEach(link => {
      link.addEventListener('click', () => {
        if (siteMenu.hasAttribute('popover') && typeof siteMenu.hidePopover === 'function') {
          try { siteMenu.hidePopover(); } catch(e) {}
        }
      });
    });
  }

  // 4. Interactive Terminal Copy Button
  const copyButtons = document.querySelectorAll('.copy-btn');
  copyButtons.forEach(function (btn) {
    btn.addEventListener('click', function () {
      const targetId = btn.getAttribute('data-target');
      const targetEl = targetId ? document.getElementById(targetId) : btn.parentElement.querySelector('.terminal-cmd');
      if (!targetEl) return;

      const text = targetEl.textContent.trim();
      navigator.clipboard.writeText(text).then(function () {
        const originalHtml = btn.innerHTML;
        const currentLang = window.SwanI18n ? window.SwanI18n.currentLang : 'en';
        const copiedText = currentLang === 'vi' ? 'Đã chép!' : 'Copied!';
        btn.innerHTML = `
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#10b981" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="20 6 9 17 4 12"></polyline>
          </svg>
          <span style="color: #10b981;">${copiedText}</span>
        `;
        setTimeout(function () {
          btn.innerHTML = originalHtml;
        }, 2000);
      }).catch(function (err) {
        console.error('Failed to copy to clipboard', err);
      });
    });
  });

  // 5. Scrollspy & IntersectionObserver Fallback (Modern Web Guidance scrollspy spec)
  const navLinks = document.querySelectorAll('.site-nav a[href^="#"], .doc-nav-link[href^="#"]');
  const sectionIds = Array.from(navLinks).map(link => link.getAttribute('href').substring(1)).filter(Boolean);
  const sections = sectionIds.map(id => document.getElementById(id)).filter(Boolean);

  if (sections.length > 0 && navLinks.length > 0) {
    const supportsScrollTargetGroup = CSS.supports && CSS.supports('scroll-target-group: auto');

    const updateActiveLinks = (activeId) => {
      navLinks.forEach(link => {
        const href = link.getAttribute('href');
        const isActive = href === `#${activeId}`;
        link.classList.toggle('target-current', isActive);
        link.classList.toggle('active', isActive);
        link.setAttribute('aria-current', isActive ? 'true' : 'false');
      });
    };

    if (!supportsScrollTargetGroup) {
      const observer = new IntersectionObserver((entries) => {
        entries.forEach(entry => {
          if (entry.isIntersecting) {
            updateActiveLinks(entry.target.id);
          }
        });
      }, { rootMargin: '-20% 0px -60% 0px', threshold: 0 });

      sections.forEach(sec => observer.observe(sec));
    } else {
      const syncAriaCurrent = () => {
        const current = document.querySelector('nav a:target-current, .doc-sidebar a:target-current');
        navLinks.forEach(link => {
          const isActive = link === current;
          link.setAttribute('aria-current', isActive ? 'true' : 'false');
        });
      };
      syncAriaCurrent();
      document.addEventListener('scrollend', syncAriaCurrent, { passive: true });
    }
  }

  // 6. Generic Tab Navigation
  document.querySelectorAll('.tab-nav').forEach(nav => {
    const buttons = nav.querySelectorAll('.tab-btn');
    buttons.forEach(btn => {
      btn.addEventListener('click', () => {
        const targetId = btn.getAttribute('data-tab');
        if (!targetId) return;

        // Toggle active button
        buttons.forEach(b => b.classList.remove('active'));
        btn.classList.add('active');

        // Toggle panels
        const container = nav.parentElement;
        container.querySelectorAll('.tab-panel').forEach(p => p.classList.remove('active'));
        const activePanel = container.querySelector('#' + targetId);
        if (activePanel) {
          activePanel.classList.add('active');
        }
      });
    });
  });

})();
