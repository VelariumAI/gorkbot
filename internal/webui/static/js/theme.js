export class IntelligentThemeManager {
  constructor() {
    this.detectSystemPreferences();
    this.applyHeuristics();
    this.initSyntaxHighlighting();
    
    // Listen for system changes
    if (window.matchMedia) {
      window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', e => {
        if (!localStorage.getItem('gorkbot-theme-override')) {
          this.setTheme(e.matches ? 'dark' : 'light');
        }
      });
    }
  }

  detectSystemPreferences() {
    const saved = localStorage.getItem('gorkbot-theme-override');
    if (saved) {
      this.setTheme(saved);
    } else {
      const prefersDark = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
      this.setTheme(prefersDark ? 'dark' : 'light');
    }
  }

  setTheme(theme) {
    document.documentElement.setAttribute('data-theme', theme);
    if (theme === 'dark') {
      document.documentElement.style.setProperty('--bg-dark', '#0f1115');
      document.documentElement.style.setProperty('--text-primary', '#e2e8f0');
    } else {
      document.documentElement.style.setProperty('--bg-dark', '#f8fafc');
      document.documentElement.style.setProperty('--text-primary', '#0f172a');
    }
  }

  async applyHeuristics() {
    // Auto-toggle based on time SENSE heuristic
    const hour = new Date().getHours();
    if (!localStorage.getItem('gorkbot-theme-override')) {
      if (hour >= 18 || hour <= 6) {
        this.setTheme('dark');
      }
    }
    
    // Battery heuristic: disable heavy animations on low battery
    if (navigator.getBattery) {
      try {
        const battery = await navigator.getBattery();
        if (battery.level < 0.2 && !battery.charging) {
          document.body.classList.add('low-power-mode');
          document.documentElement.style.setProperty('--animation-speed', '0s');
        }
      } catch (e) {
        console.warn("Battery API error", e);
      }
    }
  }

  initSyntaxHighlighting() {
    // Lazy load Prism.js
    const script = document.createElement('script');
    script.src = 'https://cdnjs.cloudflare.com/ajax/libs/prism/1.29.0/prism.min.js';
    script.onload = () => this.setupIntersectionObserver();
    document.head.appendChild(script);
    
    const link = document.createElement('link');
    link.rel = 'stylesheet';
    link.href = 'https://cdnjs.cloudflare.com/ajax/libs/prism/1.29.0/themes/prism-tomorrow.min.css';
    document.head.appendChild(link);
  }

  setupIntersectionObserver() {
    this.observer = new IntersectionObserver((entries) => {
      entries.forEach(entry => {
        if (entry.isIntersecting) {
          const block = entry.target;
          if (!block.classList.contains('prism-highlighted') && window.Prism) {
            window.Prism.highlightElement(block);
            block.classList.add('prism-highlighted');
            block.setAttribute('aria-label', 'Highlighted code block');
          }
        }
      });
    }, { rootMargin: '100px' });
    this.observeNewCodeBlocks();
  }

  observeNewCodeBlocks() {
    if (!this.observer) return;
    document.querySelectorAll('pre code:not(.prism-highlighted)').forEach(block => {
      this.observer.observe(block);
    });
  }
}
