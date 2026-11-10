// SwanPanel Documentation & Landing Application

(function () {
  'use strict';

  // 1. Theme Management (Modern Web Guidance compliant)
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
    
    // SVG icons (zero unicode emojis)
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
  systemPrefersDark.addEventListener('change', function (e) {
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

  // 2. Interactive Terminal Copy Button
  const copyButtons = document.querySelectorAll('.copy-btn');
  copyButtons.forEach(function (btn) {
    btn.addEventListener('click', function () {
      const targetId = btn.getAttribute('data-target');
      const targetEl = targetId ? document.getElementById(targetId) : btn.parentElement.querySelector('.terminal-cmd');
      if (!targetEl) return;

      const text = targetEl.textContent.trim();
      navigator.clipboard.writeText(text).then(function () {
        const originalHtml = btn.innerHTML;
        btn.innerHTML = `
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#10b981" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="20 6 9 17 4 12"></polyline>
          </svg>
          <span style="color: #10b981;">Copied!</span>
        `;
        setTimeout(function () {
          btn.innerHTML = originalHtml;
        }, 2000);
      }).catch(function (err) {
        console.error('Failed to copy to clipboard', err);
      });
    });
  });

  // 3. Documentation Scroll Spy & Sidebar Active Highlighting
  const docLinks = document.querySelectorAll('.doc-nav-link');
  if (docLinks.length > 0) {
    const sections = Array.from(docLinks).map(function (link) {
      const href = link.getAttribute('href');
      return href && href.startsWith('#') ? document.querySelector(href) : null;
    }).filter(Boolean);

    window.addEventListener('scroll', function () {
      let currentSection = '';
      const scrollPos = window.scrollY + 120;

      sections.forEach(function (section) {
        if (section.offsetTop <= scrollPos) {
          currentSection = '#' + section.getAttribute('id');
        }
      });

      docLinks.forEach(function (link) {
        if (link.getAttribute('href') === currentSection) {
          link.classList.add('active');
        } else {
          link.classList.remove('active');
        }
      });
    }, { passive: true });
  }

})();
