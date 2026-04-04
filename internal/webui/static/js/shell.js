// Phase 2: App Shell Interactivity
// Handles sidebar navigation, inspector tabs, composer input, and workspace switching

(function() {
  'use strict';

  // DOM Elements
  const sidebar = document.getElementById('sidebar');
  const sidebarToggle = document.getElementById('sidebar-toggle');
  const workspaceItems = document.querySelectorAll('.workspace-item');
  const canvas = document.getElementById('canvas');
  const composer = document.getElementById('composer');
  const composerInput = document.getElementById('composer-input');
  const composerSend = document.getElementById('composer-send');
  const composerStop = document.getElementById('composer-stop');
  const inspector = document.getElementById('inspector');
  const inspectorTabs = document.querySelectorAll('.inspector-tab');
  const inspectorClose = document.getElementById('inspector-close');

  let currentWorkspace = 'chat';
  let isRunning = false;
  let workspaceCatalog = [];

  // Phase 3: Canvas and streaming support
  let currentRunId = null;
  let currentCard = null;

  // Get the current canvas element (workspace-container)
  function getCanvasElement() {
    let el = document.querySelector('.workspace-container');
    if (!el) {
      // Fallback: create one if it doesn't exist
      el = document.createElement('div');
      el.className = 'workspace-container';
      canvas.appendChild(el);
    }
    return el;
  }

  // ────────────────────────────────────────────────────────────
  // Sidebar Toggle
  // ────────────────────────────────────────────────────────────

  sidebarToggle?.addEventListener('click', () => {
    sidebar.classList.toggle('expanded');
    localStorage.setItem('sidebarExpanded', sidebar.classList.contains('expanded'));
  });

  // Restore sidebar state from localStorage
  if (localStorage.getItem('sidebarExpanded') === 'true') {
    sidebar.classList.add('expanded');
  }

  // ────────────────────────────────────────────────────────────
  // Workspace Navigation
  // ────────────────────────────────────────────────────────────

  workspaceItems.forEach(item => {
    item.addEventListener('click', (e) => {
      e.preventDefault();
      if (item.classList.contains('disabled')) {
        return;
      }
      const workspace = item.getAttribute('data-workspace');
      switchWorkspace(workspace);
    });
  });

  function switchWorkspace(workspace) {
    // Update active indicator
    workspaceItems.forEach(item => {
      item.classList.toggle('active', item.getAttribute('data-workspace') === workspace);
    });

    currentWorkspace = workspace;

    // Load workspace content
    let content = '';
    switch(workspace) {
      case 'chat':
        content = `<div class="workspace-container" style="display:flex; flex-direction:column; gap:12px; padding:16px;"></div>`;
        break;
      case 'tasks':
        content = `<div class="workspace-container"><div class="workspace-empty">Tasks workspace - coming soon</div></div>`;
        break;
      case 'tools':
        content = `<div class="workspace-container"><div class="workspace-empty">Tools workspace - coming soon</div></div>`;
        break;
      case 'agents':
        content = `<div class="workspace-container"><div class="workspace-empty">Agents workspace - coming soon</div></div>`;
        break;
      case 'memory':
        content = `<div class="workspace-container"><div class="workspace-empty">Memory workspace - coming soon</div></div>`;
        break;
      case 'analytics':
        content = `<div class="workspace-container"><div class="workspace-empty">Analytics workspace - coming soon</div></div>`;
        break;
      case 'settings':
        content = `<div class="workspace-container"><div class="workspace-empty">Settings workspace - coming soon</div></div>`;
        break;
      default:
        content = `<div class="workspace-container"><div class="workspace-empty">Loading ${workspace}...</div></div>`;
    }

    canvas.innerHTML = content;
  }

  // ────────────────────────────────────────────────────────────
  // Composer Input Handling
  // ────────────────────────────────────────────────────────────

  composerInput?.addEventListener('keydown', (e) => {
    // Ctrl+Enter or Cmd+Enter to send
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
      e.preventDefault();
      sendMessage();
    }
    // Shift+Enter for newline (default behavior)
    // Tab for focus management
  });

  composerInput?.addEventListener('input', () => {
    // Auto-expand textarea as needed
    const composer = document.querySelector('.composer');
    const maxHeight = parseInt(getComputedStyle(composer).maxHeight || '300px');
    composerInput.style.height = 'auto';
    composerInput.style.height = Math.min(composerInput.scrollHeight, maxHeight) + 'px';
  });

  composerSend?.addEventListener('click', sendMessage);

  function sendMessage() {
    const text = composerInput.value.trim();
    if (!text || isRunning) return;

    isRunning = true;
    toggleComposerState();
    appendUserMessage(text);
    composerInput.value = '';
    composerInput.style.height = 'auto';

    fetch('/api/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        prompt: text,
        session_id: getSessionId(),
      }),
    })
      .then(r => r.json())
      .then(data => {
        currentRunId = data.run_id || 'unknown';
        openSSEStream(data.session_id, currentRunId);
      })
      .catch(err => {
        appendErrorCard(err.message);
        finishRun();
      });
  }

  function toggleComposerState() {
    composerSend.style.display = isRunning ? 'none' : 'block';
    composerStop.style.display = isRunning ? 'block' : 'none';
  }

  function appendUserMessage(text) {
    const msg = document.createElement('div');
    msg.className = 'user-message';
    msg.innerHTML = `<div class="message-bubble">${escapeHtml(text)}</div>`;
    getCanvasElement().appendChild(msg);
    scrollCanvasToBottom();
  }

  function createResponseCard(runId) {
    const card = document.createElement('div');
    card.className = 'response-card';
    card.dataset.runId = runId;
    card.innerHTML = `
      <div class="card-header">
        <span class="card-status">Processing...</span>
        <div class="card-tools"></div>
      </div>
      <div class="card-content"></div>
      <div class="card-footer" style="display:none;">
        <button class="card-action">Copy</button>
        <button class="card-action">Share</button>
      </div>
    `;
    getCanvasElement().appendChild(card);
    scrollCanvasToBottom();
    return card;
  }

  function appendToken(card, token) {
    const content = card.querySelector('.card-content');
    content.textContent += token;
    scrollCanvasToBottom();
  }

  function showToolBadge(card, toolName) {
    const toolsDiv = card.querySelector('.card-tools');
    const badge = document.createElement('span');
    badge.className = 'tool-badge running';
    badge.dataset.tool = toolName;
    badge.textContent = toolName;
    toolsDiv.appendChild(badge);
  }

  function hideToolBadge(card, toolName) {
    const badge = card.querySelector(`.tool-badge[data-tool="${toolName}"]`);
    if (badge) {
      badge.classList.remove('running');
      badge.classList.add('complete');
    }
  }

  function finalizeCard(card) {
    const header = card.querySelector('.card-header');
    const status = header.querySelector('.card-status');
    status.textContent = 'Complete';
    const footer = card.querySelector('.card-footer');
    footer.style.display = 'flex';
  }

  function appendErrorCard(msg) {
    const card = document.createElement('div');
    card.className = 'error-card';
    card.innerHTML = `<div class="error-message">⚠️ ${escapeHtml(msg)}</div>`;
    getCanvasElement().appendChild(card);
    scrollCanvasToBottom();
  }

  function finishRun() {
    isRunning = false;
    toggleComposerState();
  }

  function openSSEStream(sessionId, runId) {
    currentCard = createResponseCard(runId);
    const es = new EventSource(`/api/stream?session_id=${sessionId}`);

    es.addEventListener('token', e => {
      appendToken(currentCard, e.data);
    });

    es.addEventListener('tool_start', e => {
      showToolBadge(currentCard, e.data);
    });

    es.addEventListener('tool_done', e => {
      hideToolBadge(currentCard, e.data);
    });

    es.addEventListener('done', () => {
      es.close();
      finishRun();
      finalizeCard(currentCard);
    });

    es.addEventListener('error', () => {
      es.close();
      appendErrorCard('Stream error');
      finishRun();
    });
  }

  function scrollCanvasToBottom() {
    const el = getCanvasElement();
    el.scrollTop = el.scrollHeight;
  }

  function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
  }

  function getSessionId() {
    let sessionId = sessionStorage.getItem('sessionId');
    if (!sessionId) {
      sessionId = `sess_${Date.now()}`;
      sessionStorage.setItem('sessionId', sessionId);
    }
    return sessionId;
  }

  // ────────────────────────────────────────────────────────────
  // Inspector Tab Switching
  // ────────────────────────────────────────────────────────────

  inspectorTabs.forEach(tab => {
    tab.addEventListener('click', () => {
      const tabName = tab.getAttribute('data-tab');
      switchInspectorTab(tabName);
    });
  });

  function switchInspectorTab(tabName) {
    // Update active tab indicator
    inspectorTabs.forEach(tab => {
      tab.classList.toggle('active', tab.getAttribute('data-tab') === tabName);
    });

    // Update visible panel
    const panels = document.querySelectorAll('.inspector-panel');
    panels.forEach(panel => {
      panel.classList.remove('active');
    });
    document.getElementById(`tab-${tabName}`)?.classList.add('active');
  }

  // Inspector close button (mobile)
  inspectorClose?.addEventListener('click', () => {
    inspector.classList.remove('visible');
  });

  // ────────────────────────────────────────────────────────────
  // Command Palette Trigger (Ctrl+K)
  // ────────────────────────────────────────────────────────────

  document.addEventListener('keydown', (e) => {
    if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
      e.preventDefault();
      openCommandPalette();
    }
  });

  function openCommandPalette() {
    const known = workspaceCatalog.length > 0
      ? workspaceCatalog.filter(ws => ws.enabled !== false).map(ws => ws.id)
      : ['chat', 'tasks', 'tools', 'agents', 'memory', 'analytics', 'settings'];
    const input = window.prompt(`Command palette\n- Type / for command mode\n- Or workspace id: ${known.join(', ')}`, '');
    if (!input) {
      return;
    }
    const value = input.trim().toLowerCase();
    if (value.startsWith('/')) {
      if (composerInput) {
        composerInput.focus();
        composerInput.value = value;
        composerInput.setSelectionRange(composerInput.value.length, composerInput.value.length);
      }
      return;
    }
    if (known.includes(value)) {
      switchWorkspace(value);
      return;
    }
    if (composerInput) {
      composerInput.focus();
      composerInput.value = value;
    }
  }

  // ────────────────────────────────────────────────────────────
  // Phase 3: WebSocket Connection
  // ────────────────────────────────────────────────────────────

  function connectWS() {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(`${proto}//${location.host}/api/ws`);

    ws.onopen = () => {
      console.log('WebSocket connected');
    };

    ws.onmessage = e => {
      try {
        const msg = JSON.parse(e.data);
        handleWSMessage(msg);
      } catch (err) {
        console.error('Failed to parse WebSocket message:', err);
      }
    };

    ws.onerror = () => {
      console.error('WebSocket error');
    };

    ws.onclose = () => {
      console.log('WebSocket closed, reconnecting in 3s...');
      setTimeout(connectWS, 3000);
    };
  }

  function handleWSMessage(msg) {
    // Complement SSE with richer events
    if (msg.type === 'run_status' && currentCard) {
      const status = msg.payload.status;
      const header = currentCard.querySelector('.card-header');
      if (header) {
        const statusEl = header.querySelector('.card-status');
        if (statusEl) {
          statusEl.textContent = status === 'complete' ? 'Complete' : status;
        }
      }
    }
  }

  connectWS();

  // ────────────────────────────────────────────────────────────
  // Load Provider Information
  // ────────────────────────────────────────────────────────────

  (async function loadProviders() {
    try {
      const response = await fetch('/api/providers');
      if (response.ok) {
        const data = await response.json();

        // Update topbar with provider info
        const providerEl = document.querySelector('.topbar-provider');
        if (providerEl) {
          let providerText = '';
          if (data.primary && data.primary.name) {
            providerText = data.primary.name;
            if (data.consultant && data.consultant.name) {
              providerText += ` + ${data.consultant.name}`;
            }
          }
          if (providerText) {
            providerEl.textContent = providerText;
            providerEl.title = `Primary: ${data.primary.name}${data.consultant && data.consultant.name ? ` | Consultant: ${data.consultant.name}` : ''}`;
          }
        }
      }
    } catch (err) {
      console.error('Failed to load providers:', err);
    }
  })();

  // ────────────────────────────────────────────────────────────
  // Load Theme Tokens
  // ────────────────────────────────────────────────────────────

  (async function loadTheme() {
    try {
      const response = await fetch('/api/theme/tokens.css');
      if (response.ok) {
        const css = await response.text();
        const style = document.createElement('style');
        style.textContent = css;
        document.head.appendChild(style);
      }
    } catch (err) {
      console.error('Failed to load theme tokens:', err);
    }
  })();

  // ────────────────────────────────────────────────────────────
  // Load Workspaces
  // ────────────────────────────────────────────────────────────

  (async function loadWorkspaces() {
    try {
      const response = await fetch('/api/workspaces');
      if (response.ok) {
      const payload = await response.json();
        const items = Array.isArray(payload) ? payload : (payload.workspaces || []);
        if (Array.isArray(items) && items.length > 0) {
          workspaceCatalog = items;
          workspaceItems.forEach((item) => {
            const id = item.getAttribute('data-workspace');
            const ws = items.find(entry => entry && entry.id === id);
            if (!ws) {
              return;
            }
            const label = item.querySelector('.workspace-label');
            if (label && ws.name) {
              label.textContent = ws.name;
            }
            const icon = item.querySelector('.workspace-icon');
            if (icon && ws.icon) {
              icon.textContent = ws.icon;
            }
            if (ws.enabled === false) {
              item.classList.add('disabled');
              item.setAttribute('aria-disabled', 'true');
            } else {
              item.classList.remove('disabled');
              item.removeAttribute('aria-disabled');
            }
          });
        }
      }
    } catch (err) {
      console.error('Failed to load workspaces:', err);
    }
  })();

  // ────────────────────────────────────────────────────────────
  // Responsive Behavior
  // ────────────────────────────────────────────────────────────

  function updateResponsiveLayout() {
    const width = window.innerWidth;
    if (width < 1200) {
      // Hide inspector by default on tablet
      inspector.classList.remove('visible');
    } else {
      // Show inspector on desktop
      inspector.classList.add('visible');
    }
  }

  window.addEventListener('resize', updateResponsiveLayout);
  updateResponsiveLayout();

  // ────────────────────────────────────────────────────────────
  // Keyboard Shortcuts
  // ────────────────────────────────────────────────────────────

  const shortcuts = {
    'ctrl+1': () => switchWorkspace('chat'),
    'ctrl+2': () => switchWorkspace('tasks'),
    'ctrl+3': () => switchWorkspace('tools'),
    'ctrl+4': () => switchWorkspace('agents'),
    'ctrl+5': () => switchWorkspace('memory'),
    'ctrl+6': () => switchWorkspace('analytics'),
    'ctrl+7': () => switchWorkspace('settings'),
    'escape': () => inspectorClose?.click(),
  };

  document.addEventListener('keydown', (e) => {
    const key = [];
    if (e.ctrlKey) key.push('ctrl');
    if (e.metaKey) key.push('meta');
    if (e.shiftKey) key.push('shift');
    key.push(e.key.toLowerCase());

    const shortcut = key.join('+');
    if (shortcuts[shortcut]) {
      e.preventDefault();
      shortcuts[shortcut]();
    }
  });

  console.log('App shell loaded. Shortcuts: Ctrl+1-7 (workspaces), Ctrl+K (palette), Esc (close)');
})();
