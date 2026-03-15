import { DatabaseManager } from './db.js';
import { IntelligentThemeManager } from './theme.js';
import { IntelligentVoiceRouter } from './voice.js';
import { UniversalClipboard } from './clipboard.js';

document.addEventListener('DOMContentLoaded', () => {
    // Initialize Subsystems
    const themeMgr = new IntelligentThemeManager();
    const voiceRouter = new IntelligentVoiceRouter();
    UniversalClipboard.initDelegation();

    const chatForm = document.getElementById('chatForm');
    const promptInput = document.getElementById('promptInput');
    const sendBtn = document.getElementById('sendBtn');
    const micBtn = document.getElementById('micBtn');
    const chatContainer = document.getElementById('chatContainer');
    
    // Voice Input Handler
    if (micBtn) {
        micBtn.addEventListener('click', () => {
            voiceRouter.startListening();
        });
    }

    // Custom Event Listener from Voice Router
    window.addEventListener('VOICE_COMMAND', (e) => {
        if (e.detail.action === 'execute') {
            promptInput.value = e.detail.raw;
            chatForm.dispatchEvent(new Event('submit'));
        } else if (e.detail.action === 'clear') {
            chatContainer.innerHTML = '';
        }
    });

    // Telemetry Elements
    const statusProvider = document.getElementById('statusProvider');
    const statusTokensIn = document.getElementById('statusTokensIn');
    const statusTokensOut = document.getElementById('statusTokensOut');

    // Auto-resize textarea
    promptInput.addEventListener('input', function() {
        this.style.height = 'auto';
        this.style.height = (this.scrollHeight) + 'px';
        if (this.value.trim() === '') {
            this.style.height = '48px';
        }
    });

    // Handle Enter key (Shift+Enter for new line)
    promptInput.addEventListener('keydown', function(e) {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            if (this.value.trim()) {
                chatForm.dispatchEvent(new Event('submit'));
            }
        }
    });

    // Poll Telemetry every 5 seconds
    async function updateTelemetry() {
        try {
            const res = await fetch('/api/state');
            const data = await res.json();
            statusProvider.innerText = data.provider || 'Unknown';
            statusTokensIn.innerText = data.tokens_in || 0;
            statusTokensOut.innerText = data.tokens_out || 0;
        } catch (e) {
            console.error("Telemetry error", e);
        }
    }
    setInterval(updateTelemetry, 5000);
    updateTelemetry();

    // Render Markdown securely
    function renderMD(text) {
        return DOMPurify.sanitize(marked.parse(text));
    }

    function appendMessage(role, contentHTML) {
        const div = document.createElement('div');
        div.className = `message ${role}`;
        
        const avatar = document.createElement('div');
        avatar.className = 'avatar';
        avatar.innerHTML = role === 'user' ? '<i class="fa-solid fa-user"></i>' : '<i class="fa-solid fa-robot"></i>';
        
        const contentDiv = document.createElement('div');
        contentDiv.className = 'content';
        contentDiv.style.position = 'relative'; // for copy button positioning
        contentDiv.innerHTML = contentHTML;

        if (role === 'system') {
            const macroCopy = document.createElement('button');
            macroCopy.className = 'copy-btn macro-copy btn-icon';
            macroCopy.innerHTML = '<i class="fa-regular fa-copy"></i>';
            macroCopy.setAttribute('aria-label', 'Copy Response');
            macroCopy.style.position = 'absolute';
            macroCopy.style.top = '0.5rem';
            macroCopy.style.right = '0.5rem';
            macroCopy.style.background = 'transparent';
            macroCopy.style.border = 'none';
            macroCopy.style.color = 'var(--text-secondary)';
            macroCopy.style.cursor = 'pointer';
            contentDiv.appendChild(macroCopy);
        }
        
        div.appendChild(avatar);
        div.appendChild(contentDiv);
        chatContainer.appendChild(div);
        
        chatContainer.scrollTop = chatContainer.scrollHeight;
        return contentDiv;
    }

    function appendToolBadge(targetNode, toolName) {
        const badge = document.createElement('div');
        badge.className = `tool-badge`;
        badge.id = `tool-${Date.now()}`;
        badge.innerHTML = `<i class="fa-solid fa-gear fa-spin"></i> Executing: ${toolName}`;
        targetNode.appendChild(badge);
        chatContainer.scrollTop = chatContainer.scrollHeight;
        return badge;
    }

    function markToolDone(badgeNode, toolName) {
        if (badgeNode) {
            badgeNode.className = 'tool-badge done';
            badgeNode.innerHTML = `<i class="fa-solid fa-check"></i> Completed: ${toolName}`;
        }
    }

    chatForm.addEventListener('submit', async (e) => {
        e.preventDefault();
        
        const prompt = promptInput.value.trim();
        if (!prompt) return;

        // Reset input
        promptInput.value = '';
        promptInput.style.height = '48px';
        sendBtn.disabled = true;
        promptInput.disabled = true;

        // Append User Message
        appendMessage('user', renderMD(prompt));

        // Create container for System Response
        const systemDiv = appendMessage('system', '<div class="typing-indicator"><span></span><span></span><span></span></div>');
        
        let fullResponse = "";
        let currentToolBadge = null;

        try {
            // 1. Send Post Request to initiate stream
            const res = await fetch('/api/chat', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ prompt: prompt })
            });
            
            const data = await res.json();
            const sessionId = data.session_id;

            // 2. Connect to SSE Stream
            const eventSource = new EventSource(`/api/stream?session_id=${sessionId}`);
            
            // Remove typing indicator once stream opens
            systemDiv.innerHTML = ''; 
            
            eventSource.onmessage = function(event) {
                const payload = JSON.parse(event.data);
                
                if (payload.type === 'token') {
                    fullResponse += payload.data;
                    systemDiv.innerHTML = renderMD(fullResponse);
                    // Re-append the macro copy button
                    const macroCopy = document.createElement('button');
                    macroCopy.className = 'copy-btn macro-copy btn-icon';
                    macroCopy.innerHTML = '<i class="fa-regular fa-copy"></i>';
                    macroCopy.setAttribute('aria-label', 'Copy Response');
                    macroCopy.style.position = 'absolute';
                    macroCopy.style.top = '0.5rem';
                    macroCopy.style.right = '0.5rem';
                    macroCopy.style.background = 'transparent';
                    macroCopy.style.border = 'none';
                    macroCopy.style.color = 'var(--text-secondary)';
                    macroCopy.style.cursor = 'pointer';
                    systemDiv.appendChild(macroCopy);
                    
                    chatContainer.scrollTop = chatContainer.scrollHeight;
                } 
                else if (payload.type === 'tool_start') {
                    currentToolBadge = appendToolBadge(systemDiv, payload.data);
                } 
                else if (payload.type === 'tool_done') {
                    markToolDone(currentToolBadge, payload.data);
                }
                else if (payload.type === 'auth_required') {
                    const cred = prompt(`🔐 Setup Required: ${payload.data}\n\nEnter credential:`);
                    if (cred) {
                        saveCredential('google', cred);
                    }
                }
                else if (payload.type === 'error') {
                    systemDiv.innerHTML += `<br><br><span style="color:var(--error);"><i class="fa-solid fa-triangle-exclamation"></i> Error: ${payload.data}</span>`;
                    eventSource.close();
                    finalizeTurn();
                }
                else if (payload.type === 'done') {
                    eventSource.close();
                    finalizeTurn();
                }
            };

            eventSource.onerror = function() {
                eventSource.close();
                finalizeTurn();
            };

        } catch (err) {
            if (!navigator.onLine) {
                // Offline fallback - Queue task to IndexedDB
                await DatabaseManager.queueTask(prompt);
                systemDiv.innerHTML = `<span style="color:var(--warning);"><i class="fa-solid fa-wifi"></i> Offline. Task queued locally and will sync when reconnected.</span>`;
                if ('serviceWorker' in navigator && 'SyncManager' in window) {
                    navigator.serviceWorker.ready.then(reg => {
                        reg.sync.register('gorkbot-sync');
                    });
                }
            } else {
                systemDiv.innerHTML = `<span style="color:var(--error);">Failed to connect to engine: ${err.message}</span>`;
            }
            finalizeTurn();
        }
    });

    function finalizeTurn() {
        sendBtn.disabled = false;
        promptInput.disabled = false;
        promptInput.focus();
        UniversalClipboard.wrapCodeBlocks();
        themeMgr.observeNewCodeBlocks();
        updateTelemetry();
    }

    // Handle Client-Side Sync Requests from SW
    navigator.serviceWorker?.addEventListener('message', async (event) => {
        if (event.data.type === 'TRIGGER_SYNC') {
            await processOfflineQueue();
        }
    });

    async function saveCredential(provider, val) {
        try {
            await fetch('/api/credentials/set', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({provider, value: val})
            });
            alert("Credential saved. You can now retry your request.");
        } catch (e) {
            console.error("Failed to save credential", e);
        }
    }

    async function processOfflineQueue() {
        const pending = await DatabaseManager.getPendingTasks();
        if (pending.length === 0) return;

        console.log(`Syncing ${pending.length} offline tasks...`);
        const payload = pending.map(t => ({ id: t.id, payload: t.payload, timestamp: t.timestamp }));

        try {
            const res = await fetch('/api/offline-sync', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload)
            });
            const result = await res.json();
            
            if (result.status === 'synchronized') {
                for (const t of pending) {
                    await DatabaseManager.removeTask(t.id);
                }
                console.log("Offline sync complete.");
            }
        } catch (e) {
            console.error("Sync failed, implementing backoff.", e);
            for (const t of pending) {
                await DatabaseManager.updateTaskStatus(t.id, 'queued', t.retryCount + 1);
            }
        }
    }

    // Modal Logic
    const settingsBtn = document.getElementById('settingsBtn');
    const settingsModal = document.getElementById('settingsModal');
    const closeSettings = document.getElementById('closeSettings');
    const primarySelect = document.getElementById('primaryModelSelect');
    const secondarySelect = document.getElementById('secondaryModelSelect');
    const saveSettingsBtn = document.getElementById('saveSettingsBtn');
    const primaryBadge = document.getElementById('primaryBadge');
    const secondaryBadge = document.getElementById('secondaryBadge');

    // New Modals
    const toolsModal = document.getElementById('toolsModal');
    const agentsModal = document.getElementById('agentsModal');
    const memoryModal = document.getElementById('memoryModal');

    document.getElementById('navTools').onclick = async (e) => {
        e.preventDefault();
        toolsModal.style.display = "block";
        const res = await fetch('/api/tools');
        const data = await res.json();
        const list = document.getElementById('toolsList');
        list.innerHTML = data.tools.map(t => `<div style="margin-bottom:1rem; padding:1rem; background:rgba(255,255,255,0.05); border-radius:8px; border:1px solid var(--border);">
            <div style="color:var(--accent); font-weight:bold; font-family:var(--font-mono); margin-bottom:0.5rem;">${t.name} <span class="badge">${t.category}</span></div>
            <div style="font-size:0.9rem; color:var(--text-secondary);">${t.description}</div>
        </div>`).join('');
    };

    document.getElementById('navAgents').onclick = async (e) => {
        e.preventDefault();
        agentsModal.style.display = "block";
        const res = await fetch('/api/agents');
        const data = await res.json();
        document.getElementById('agentsList').innerHTML = data.agents.map(a => `<div style="margin-bottom:1rem; padding:1rem; background:rgba(255,255,255,0.05); border-radius:8px; border:1px solid var(--border);">
            <div style="color:var(--success); font-weight:bold;"><i class="fa-solid fa-robot"></i> ${a.id}</div>
            <div style="font-size:0.9rem; color:var(--text-secondary); margin-top:0.5rem;">Type: ${a.type} | Status: ${a.status}</div>
        </div>`).join('');
    };

    document.getElementById('navMemory').onclick = async (e) => {
        e.preventDefault();
        memoryModal.style.display = "block";
        const res = await fetch('/api/memory');
        const data = await res.json();
        document.getElementById('memoryStats').innerHTML = `<div style="margin-bottom:1rem; padding:1rem; background:rgba(255,255,255,0.05); border-radius:8px; border:1px solid var(--border);">
            <div style="color:var(--accent); font-weight:bold;"><i class="fa-solid fa-brain"></i> Context Window</div>
            <div style="font-size:0.9rem; color:var(--text-secondary); margin-top:0.5rem;">Active Messages: ${data.messages_count || 0}</div>
            <div style="font-size:0.9rem; color:var(--text-secondary); margin-top:0.5rem;">Cross-Session Goals: ${data.goals_count || 0}</div>
        </div>`;
    };

    document.getElementById('closeTools').onclick = () => toolsModal.style.display = "none";
    document.getElementById('closeAgents').onclick = () => agentsModal.style.display = "none";
    document.getElementById('closeMemory').onclick = () => memoryModal.style.display = "none";

    settingsBtn.onclick = async () => {
        settingsModal.style.display = "block";
        await loadModels();
    };

    closeSettings.onclick = () => {
        settingsModal.style.display = "none";
    };

    window.onclick = (event) => {
        if (event.target == settingsModal) settingsModal.style.display = "none";
        if (event.target == toolsModal) toolsModal.style.display = "none";
        if (event.target == agentsModal) agentsModal.style.display = "none";
        if (event.target == memoryModal) memoryModal.style.display = "none";
    };

    let availableModels = [];

    async function loadModels() {
        try {
            const res = await fetch('/api/models');
            const data = await res.json();
            availableModels = data.models || [];
            
            primarySelect.innerHTML = '';
            secondarySelect.innerHTML = '<option value="auto" data-provider="">Auto (Adaptive Engine)</option>';
            
            availableModels.forEach(m => {
                const opt1 = document.createElement('option');
                opt1.value = m.id;
                opt1.textContent = `${m.name} (${m.provider})`;
                opt1.dataset.provider = m.provider;
                
                if (m.id === data.primary) opt1.selected = true;
                primarySelect.appendChild(opt1);

                const opt2 = document.createElement('option');
                opt2.value = m.id;
                opt2.textContent = `${m.name} (${m.provider})`;
                opt2.dataset.provider = m.provider;
                
                if (m.id === data.secondary) opt2.selected = true;
                secondarySelect.appendChild(opt2);
            });

            updateBadges(data.primary, data.secondary);
        } catch (e) {
            console.error("Failed to load models", e);
        }
    }

    function updateBadges(primaryId, secondaryId) {
        if(primaryId) {
            const pm = availableModels.find(m => m.id === primaryId);
            if(pm) primaryBadge.innerText = pm.name;
        }
        if(secondaryId === "auto") {
            secondaryBadge.innerText = "Secondary: Auto";
        } else if(secondaryId) {
            const sm = availableModels.find(m => m.id === secondaryId);
            if(sm) secondaryBadge.innerText = `Secondary: ${sm.name}`;
        }
    }

    saveSettingsBtn.onclick = async () => {
        const pOpt = primarySelect.options[primarySelect.selectedIndex];
        const sOpt = secondarySelect.options[secondarySelect.selectedIndex];

        try {
            await fetch('/api/models/set', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({
                    role: 'primary',
                    provider: pOpt.dataset.provider,
                    model_id: pOpt.value
                })
            });

            await fetch('/api/models/set', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({
                    role: 'secondary',
                    provider: sOpt.dataset.provider || "",
                    model_id: sOpt.value
                })
            });

            settingsModal.style.display = "none";
            updateBadges(pOpt.value, sOpt.value);
            updateTelemetry();
        } catch (e) {
            console.error("Failed to save settings", e);
            alert("Failed to save model configuration");
        }
    };

    // Initial load
    loadModels();
});
