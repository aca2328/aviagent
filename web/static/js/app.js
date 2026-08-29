// Function to display version information
function displayVersionInfo() {
    fetch('/api/health')
        .then(response => response.json())
        .then(data => {
            if (data.version) {
                document.getElementById('app-version').textContent = data.version;
            }
            if (data.build_date) {
                document.getElementById('build-date').textContent = data.build_date;
            }
        })
        .catch(error => {
            console.warn('Failed to fetch version info:', error);
        });
}

// Node-link diagram download — generates a standalone copy of
// web/static/diagram/template.html with the clicked API result's JSON
// embedded, then downloads it via a Blob (see internal/web/web-server.go's
// renderChatMessage, which wraps each "API Result" JSON block in a
// .api-result-block with a .diagram-download-btn button).
const DIAGRAM_TEMPLATE_URL = '/static/diagram/template.html';
const DIAGRAM_DATA_SENTINEL = /\/\*__AVI_DIAGRAM_DATA_START__\*\/[\s\S]*?\/\*__AVI_DIAGRAM_DATA_END__\*\//;
const DIAGRAM_TITLE_TEXT = 'Avi API Response — Node Graph';
let diagramTemplatePromise = null;

function getDiagramTemplate() {
    if (!diagramTemplatePromise) {
        diagramTemplatePromise = fetch(DIAGRAM_TEMPLATE_URL).then(function(response) {
            if (!response.ok) {
                throw new Error('Failed to load diagram template: ' + response.status);
            }
            return response.text();
        });
    }
    return diagramTemplatePromise;
}

function initializeDiagramDownload() {
    document.body.addEventListener('click', function(event) {
        const button = event.target.closest('.diagram-download-btn');
        if (!button) return;

        const block = button.closest('.api-result-block');
        const codeEl = block ? block.querySelector('pre code') : null;
        if (!codeEl) return;

        let data;
        try {
            data = JSON.parse(codeEl.textContent);
        } catch (err) {
            console.error('Diagram download: result is not valid JSON', err);
            alert('This result could not be parsed as JSON, so a diagram cannot be generated.');
            return;
        }

        const toolName = block.dataset.tool || '';
        const originalHtml = button.innerHTML;
        button.disabled = true;
        button.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Preparing…';

        getDiagramTemplate().then(function(templateHtml) {
            const title = toolName ? toolName + ' — Node Graph' : DIAGRAM_TITLE_TEXT;
            const dataBlock = '/*__AVI_DIAGRAM_DATA_START__*/\nconst DATA = ' +
                JSON.stringify(data, null, 2) + ';\n/*__AVI_DIAGRAM_DATA_END__*/';

            const html = templateHtml
                .split(DIAGRAM_TITLE_TEXT).join(title)
                .replace(DIAGRAM_DATA_SENTINEL, dataBlock);

            const blob = new Blob([html], { type: 'text/html' });
            const url = window.URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            const stamp = new Date().toISOString().replace(/[:.]/g, '-');
            a.download = (toolName || 'avi-result') + '-diagram-' + stamp + '.html';
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            window.URL.revokeObjectURL(url);
        }).catch(function(err) {
            console.error('Failed to generate diagram', err);
            alert('Failed to generate the diagram file: ' + err.message);
        }).finally(function() {
            button.disabled = false;
            button.innerHTML = originalHtml;
        });
    });
}

// Trace (API log) streaming
let logPauseState = false;
let logJsonIdCounter = 0;
let pausedEntries = [];
// Per-turn correlation (Phase 3): set by clicking a message's trace-summary
// chip, cleared via the inspector's "Clear" banner. reconnectTraceStream is
// wired up by initializeTraceFiltering so this module-scope setter can force
// a fresh /api/logs/enhanced?turn=... stream without knowing about segments
// or search state.
let turnFilter = null;
let reconnectTraceStream = null;

function setTurnFilter(turnId) {
    turnFilter = turnId;
    const banner = document.getElementById('trace-turn-banner');
    if (banner) banner.classList.toggle('d-none', !turnId);
    if (reconnectTraceStream) reconnectTraceStream();
}

function updatePausedFooter() {
    const statusText = document.getElementById('trace-status-text');
    if (statusText) {
        statusText.textContent = `Paused · ${pausedEntries.length} event${pausedEntries.length === 1 ? '' : 's'} buffered`;
    }
}

function processLogEntry(logEntry) {
    if (!shouldDisplayLog(logEntry)) return;

    if (logPauseState) {
        pausedEntries.push(logEntry);
        updatePausedFooter();
        return;
    }

    const logsDisplay = document.getElementById('logs-display');
    if (!logsDisplay) return;

    const logElement = createLogElement(logEntry);
    logsDisplay.appendChild(logElement);

    setTimeout(() => {
        logsDisplay.scrollTop = logsDisplay.scrollHeight;
    }, 50);
}

// Suppresses health-check noise from the trace stream — /health is polled
// every 30s by checkConnectionStatus and isn't useful to show as a step.
function shouldDisplayLog(logEntry) {
    const ctx = logEntry.context || {};
    if (logEntry.message && logEntry.message.includes('Health check requested')) {
        return false;
    }
    if (ctx.endpoint === '/health') {
        return false;
    }
    return true;
}

// Classifies a raw trace/log entry into the step "kind" the design specifies
// (prompt / llm / tool / system), picking an icon chip, title, status and
// duration. Falls back to a quiet generic system row for everything else
// (there are far more log types in this app than the three named kinds).
// Server entries arrive as { type, message, context: {...} } — broadcastOperationLog's
// extra fields (duration_ms, method, tool, result, error, ...) live under context,
// not at the top level.
function classifyTraceEntry(logEntry) {
    const ctx = logEntry.context || {};
    const durationMs = typeof ctx.duration_ms === 'number' ? ctx.duration_ms : null;

    if (logEntry.type === 'user_request') {
        let excerpt = '';
        try {
            excerpt = JSON.parse(ctx.payload || '{}').message || '';
        } catch (e) { /* payload not JSON-parseable; leave excerpt empty */ }
        return {
            kind: 'prompt',
            icon: 'fa-user',
            chipClass: 'chip-prompt',
            title: 'Prompt received',
            subline: excerpt,
            duration: null,
        };
    }

    if (logEntry.type === 'mistral_response') {
        const step = ctx.step === 'compose' ? 'compose' : 'plan';
        const failed = !!ctx.error;
        let subline;
        if (failed) {
            subline = ctx.error;
        } else if (typeof ctx.tool_calls_selected === 'number') {
            subline = ctx.tool_calls_selected > 0
                ? `${ctx.tool_calls_selected} tool${ctx.tool_calls_selected === 1 ? '' : 's'} selected`
                : 'responded directly, no tools needed';
        } else {
            subline = '';
        }
        return {
            kind: 'llm',
            icon: 'fa-microchip',
            chipClass: 'chip-llm',
            title: `${ctx.model || 'model'} · ${step}`,
            subline,
            duration: durationMs,
            status: failed ? 'error' : null,
            detail: { method: ctx.method, path: ctx.endpoint, returned: failed ? null : (subline || undefined), error: failed ? ctx.error : null },
            method: ctx.method,
            endpoint: ctx.endpoint,
            payload: ctx.payload,
        };
    }

    if (logEntry.message === 'Tool call succeeded' || logEntry.message === 'Tool call failed' || logEntry.message === 'Tool call returned empty result') {
        const failed = logEntry.message === 'Tool call failed';
        const empty = logEntry.message === 'Tool call returned empty result';
        let returned;
        if (failed) {
            returned = null;
        } else if (empty) {
            returned = 'empty result';
        } else if (ctx.result && typeof ctx.result === 'object') {
            if (Array.isArray(ctx.result.results)) {
                returned = `${ctx.result.results.length} object${ctx.result.results.length === 1 ? '' : 's'}`;
            } else {
                returned = 'ok';
            }
        } else {
            returned = 'ok';
        }
        return {
            kind: 'tool',
            icon: 'fa-server',
            chipClass: 'chip-tool',
            title: ctx.tool || 'tool call',
            subline: failed ? ctx.error : '',
            duration: durationMs,
            status: failed ? 'error' : (empty ? 'warning' : 'healthy'),
            detail: { returned, error: failed ? ctx.error : null },
        };
    }

    // Generic system row — still shown (nothing is dropped), just quieter.
    return {
        kind: 'system',
        icon: logEntry.type === 'error' ? 'fa-exclamation-triangle' : (logEntry.type === 'success' ? 'fa-check-circle' : 'fa-info-circle'),
        chipClass: 'chip-system',
        title: logEntry.message || logEntry.type || 'event',
        subline: '',
        duration: durationMs,
        status: logEntry.type === 'error' ? 'error' : (logEntry.type === 'warning' ? 'warning' : null),
    };
}

function formatTraceDuration(ms) {
    if (ms === null || ms === undefined) return '';
    if (ms === 0) return '0ms';
    return ms >= 1000 ? (ms / 1000).toFixed(2) + 's' : ms + 'ms';
}

function buildCurlCommand(info) {
    if (!info.method || !info.endpoint) return null;
    const base = info.endpoint.startsWith('http') ? '' : window.location.origin;
    let cmd = `curl -X ${info.method} '${base}${info.endpoint}'`;
    if (info.payload) {
        cmd += ` \\\n  -H 'Content-Type: application/json' \\\n  -d '${info.payload}'`;
    }
    return cmd;
}

function createLogElement(logEntry) {
    const info = classifyTraceEntry(logEntry);

    const step = document.createElement('div');
    step.className = 'trace-step trace-step-' + info.kind;
    if (logEntry.turn_id) {
        step.dataset.turnId = logEntry.turn_id;
    }

    const gutter = document.createElement('div');
    gutter.className = 'trace-step-gutter';
    const chip = document.createElement('span');
    chip.className = 'trace-step-chip ' + info.chipClass;
    chip.innerHTML = `<i class="fas ${info.icon}"></i>`;
    const connector = document.createElement('span');
    connector.className = 'trace-step-connector';
    gutter.appendChild(chip);
    gutter.appendChild(connector);

    const body = document.createElement('div');
    body.className = 'trace-step-body';

    const titleRow = document.createElement('div');
    titleRow.className = 'trace-step-title-row';
    if (logEntry.turn_id) {
        titleRow.classList.add('is-turn-linked');
        titleRow.title = "Jump to this turn's message";
    }

    const title = document.createElement('span');
    title.className = 'trace-step-title';
    title.textContent = info.title;
    titleRow.appendChild(title);

    const metaSpacer = document.createElement('div');
    metaSpacer.className = 'trace-step-spacer';
    titleRow.appendChild(metaSpacer);

    if (info.status) {
        const statusBadge = document.createElement('span');
        statusBadge.className = 'trace-step-status trace-step-status-' + info.status;
        statusBadge.innerHTML = '<span class="status-dot"></span>' + info.status;
        titleRow.appendChild(statusBadge);
    }

    if (info.duration !== null && info.duration !== undefined) {
        const durationEl = document.createElement('span');
        durationEl.className = 'trace-step-duration' + (info.duration === 0 ? ' is-zero' : '');
        durationEl.textContent = formatTraceDuration(info.duration);
        titleRow.appendChild(durationEl);
    }

    body.appendChild(titleRow);

    if (info.subline) {
        const sublineEl = document.createElement('div');
        sublineEl.className = 'trace-step-subline';
        sublineEl.textContent = info.subline;
        body.appendChild(sublineEl);
    }

    // Expanded detail: a method/path/returned key-value grid for steps that
    // have one, plus a collapsed "Raw JSON" dump and, when we know the real
    // method+endpoint, a "Copy as curl" button.
    const detailRows = [];
    if (info.detail) {
        if (info.detail.method) detailRows.push(['method', info.detail.method]);
        if (info.detail.path) detailRows.push(['path', info.detail.path]);
        if (info.detail.returned) detailRows.push(['returned', info.detail.returned]);
        if (info.detail.error) detailRows.push(['error', info.detail.error]);
    }

    if (detailRows.length > 0 || info.kind === 'tool' || info.kind === 'llm') {
        const detailBox = document.createElement('div');
        detailBox.className = 'trace-step-detail';

        if (detailRows.length > 0) {
            const grid = document.createElement('div');
            grid.className = 'trace-step-kv';
            detailRows.forEach(function([key, value]) {
                const k = document.createElement('div');
                k.className = 'trace-step-kv-key';
                k.textContent = key;
                const v = document.createElement('div');
                v.className = 'trace-step-kv-value';
                v.textContent = value;
                grid.appendChild(k);
                grid.appendChild(v);
            });
            detailBox.appendChild(grid);
        }

        const toolbar = document.createElement('div');
        toolbar.className = 'trace-step-toolbar';

        const rawId = 'trace-raw-' + (++logJsonIdCounter);
        const rawToggle = document.createElement('button');
        rawToggle.type = 'button';
        rawToggle.className = 'trace-step-toolbar-btn';
        rawToggle.setAttribute('data-bs-toggle', 'collapse');
        rawToggle.setAttribute('data-bs-target', '#' + rawId);
        rawToggle.setAttribute('aria-expanded', 'false');
        rawToggle.setAttribute('aria-controls', rawId);
        rawToggle.innerHTML = '<i class="fas fa-code"></i> Raw JSON';
        toolbar.appendChild(rawToggle);

        const curlCmd = buildCurlCommand(info);
        if (curlCmd) {
            const curlBtn = document.createElement('button');
            curlBtn.type = 'button';
            curlBtn.className = 'trace-step-toolbar-btn';
            curlBtn.innerHTML = '<i class="fas fa-terminal"></i> Copy as curl';
            curlBtn.addEventListener('click', function() {
                navigator.clipboard.writeText(curlCmd).then(function() {
                    const original = curlBtn.innerHTML;
                    curlBtn.innerHTML = '<i class="fas fa-check"></i> Copied';
                    setTimeout(function() { curlBtn.innerHTML = original; }, 1500);
                }).catch(function() { /* clipboard unavailable; silently ignore */ });
            });
            toolbar.appendChild(curlBtn);
        }

        detailBox.appendChild(toolbar);

        const rawPre = document.createElement('pre');
        rawPre.className = 'log-payload-content collapse';
        rawPre.id = rawId;
        try {
            rawPre.textContent = JSON.stringify(logEntry, null, 2);
        } catch (e) {
            rawPre.textContent = String(logEntry);
        }
        detailBox.appendChild(rawPre);

        body.appendChild(detailBox);
    }

    step.appendChild(gutter);
    step.appendChild(body);

    return step;
}

function addSystemLog(message, isError = false) {
    const logsDisplay = document.getElementById('logs-display');
    if (!logsDisplay) return;
    processLogEntry({ type: isError ? 'error' : 'system', message: message, timestamp: new Date().toISOString() });
}

// Trace filter row: a 4-segment control (All / LLM / Avi / Errors) plus a
// search field, replacing the old type+level selects and legacy checkboxes.
// Segments map to the same /api/logs/enhanced query the old selects drove.
function initializeTraceFiltering() {
    const segButtons = document.querySelectorAll('#trace-type-seg .trace-seg-opt');
    const searchInput = document.getElementById('log-search');
    const clearSearchBtn = document.getElementById('clear-search');
    const clearLogsButton = document.getElementById('clear-logs');
    const pauseButton = document.getElementById('pause-logs');
    const logsDisplay = document.getElementById('logs-display');
    const statusText = document.getElementById('trace-status-text');
    const statusDot = document.getElementById('trace-status-dot');

    if (!logsDisplay || segButtons.length === 0 || !searchInput) return;

    function flushPausedEntries() {
        pausedEntries.forEach(function(entry) {
            const logElement = createLogElement(entry);
            logsDisplay.appendChild(logElement);
        });
        pausedEntries = [];
        logsDisplay.scrollTop = logsDisplay.scrollHeight;
    }

    if (pauseButton) {
        pauseButton.addEventListener('click', function() {
            logPauseState = !logPauseState;
            const icon = pauseButton.querySelector('i');
            if (logPauseState) {
                icon.classList.remove('ph-pause');
                icon.classList.add('ph-play');
                pauseButton.setAttribute('title', 'Resume stream');
                updatePausedFooter();
            } else {
                icon.classList.remove('ph-play');
                icon.classList.add('ph-pause');
                pauseButton.setAttribute('title', 'Pause stream');
                flushPausedEntries();
                if (statusDot) statusDot.classList.add('is-healthy');
                if (statusText) statusText.textContent = 'Live · following stream';
            }
        });
    }

    let currentEventSource = null;
    let activeSeg = document.querySelector('#trace-type-seg .trace-seg-opt.is-active') || segButtons[0];

    function connectTraceStream() {
        if (currentEventSource) {
            currentEventSource.close();
        }

        // Clear the current view — the new connection replays full matching
        // history, so old entries must go or they'd just pile up underneath it.
        logsDisplay.innerHTML = '';

        const logType = activeSeg.dataset.type || 'all';
        const level = activeSeg.dataset.level || 'all';
        const search = searchInput.value;

        let url = `/api/logs/enhanced?type=${logType}&level=${level}&search=${encodeURIComponent(search)}`;
        if (turnFilter) {
            url += `&turn=${encodeURIComponent(turnFilter)}`;
        }

        currentEventSource = new EventSource(url);

        currentEventSource.onopen = function() {
            if (statusDot) statusDot.classList.add('is-healthy');
            if (statusText) statusText.textContent = 'Live · following stream';
        };

        currentEventSource.onmessage = function(e) {
            const log = JSON.parse(e.data);
            processLogEntry(log);
        };

        currentEventSource.onerror = function() {
            console.error('Trace stream error, reconnecting…');
            if (statusDot) statusDot.classList.remove('is-healthy');
            if (statusText) statusText.textContent = 'Stream disconnected · retrying';
            setTimeout(connectTraceStream, 2000);
        };
    }

    segButtons.forEach(function(btn) {
        btn.addEventListener('click', function() {
            if (btn === activeSeg) return;
            activeSeg.classList.remove('is-active');
            btn.classList.add('is-active');
            activeSeg = btn;
            connectTraceStream();
        });
    });

    if (clearSearchBtn) {
        clearSearchBtn.addEventListener('click', function() {
            searchInput.value = '';
            connectTraceStream();
        });
    }

    if (clearLogsButton) {
        clearLogsButton.addEventListener('click', function() {
            logsDisplay.innerHTML = '';
            addSystemLog('Logs cleared by user');
        });
    }

    let searchTimeout;
    searchInput.addEventListener('input', function() {
        clearTimeout(searchTimeout);
        searchTimeout = setTimeout(connectTraceStream, 500);
    });

    const clearTurnFilterBtn = document.getElementById('clear-turn-filter');
    if (clearTurnFilterBtn) {
        clearTurnFilterBtn.addEventListener('click', function() {
            setTurnFilter(null);
        });
    }

    // Trace step -> message: click a step's title row to scroll to and
    // highlight the assistant message it belongs to (the other half of the
    // chip's message -> steps link below).
    logsDisplay.addEventListener('click', function(e) {
        const titleRow = e.target.closest('.trace-step-title-row');
        if (!titleRow || !titleRow.classList.contains('is-turn-linked')) return;
        const step = titleRow.closest('.trace-step');
        const turnId = step && step.dataset.turnId;
        if (!turnId) return;
        const message = document.querySelector('.assistant-message[data-turn-id="' + turnId + '"]');
        if (!message) return;
        message.scrollIntoView({ behavior: 'smooth', block: 'center' });
        message.classList.add('is-highlighted');
        setTimeout(function() { message.classList.remove('is-highlighted'); }, 1800);
    });

    reconnectTraceStream = connectTraceStream;
    connectTraceStream();
}

// Message -> trace steps: click a message's trace-summary chip to isolate
// the inspector to that turn's steps (via /api/logs/enhanced?turn=...).
// Delegated on #chat-messages since assistant messages are swapped in by
// htmx, not present at DOMContentLoaded time.
function initializeTurnChipLinking() {
    const chatMessages = document.getElementById('chat-messages');
    if (!chatMessages) return;
    chatMessages.addEventListener('click', function(e) {
        const chip = e.target.closest('.trace-summary-chip');
        if (!chip) return;
        setTurnFilter(chip.dataset.turnId);
    });
}

// Empty-state prompt buttons (screen 2a): fill the composer and submit.
function initializePromptButtons() {
    document.querySelectorAll('.prompt-btn[data-prompt]').forEach(function(btn) {
        btn.addEventListener('click', function() {
            const messageInput = document.getElementById('message-input');
            const chatForm = document.getElementById('chat-form');
            if (!messageInput || !chatForm) return;
            messageInput.value = btn.dataset.prompt;
            chatForm.requestSubmit ? chatForm.requestSubmit() : chatForm.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
        });
    });
}

// Session rail (Phase 4): "New session" creates a session server-side (via
// POST /htmx/sessions, which also sets the active_session_id cookie) and
// refreshes the rail list; clicking a session loads its history via htmx
// (hx-get on .rail-session-link, in history.html); delete is a plain fetch
// since it's just a DOM removal, not a page swap.
function replaceSessionRailList(html) {
    const railList = document.getElementById('session-rail-list');
    if (!railList) return;
    const wrapper = document.createElement('div');
    wrapper.innerHTML = html.trim();
    const newList = wrapper.firstElementChild;
    if (!newList) return;
    railList.replaceWith(newList);
    if (window.htmx) window.htmx.process(newList);
}

function initializeSessionRail() {
    const newSessionButton = document.getElementById('new-session-btn');
    const rail = document.getElementById('session-rail');
    if (!rail) return;

    if (newSessionButton) {
        newSessionButton.addEventListener('click', function() {
            resetToEmptyState();
            fetch('/htmx/sessions', { method: 'POST' })
                .then(function(r) { return r.text(); })
                .then(replaceSessionRailList)
                .catch(function() { /* rail refresh is best-effort */ });
        });
    }

    rail.addEventListener('click', function(e) {
        const link = e.target.closest('.rail-session-link');
        if (link) {
            rail.querySelectorAll('.rail-session-item.is-active').forEach(function(el) {
                el.classList.remove('is-active');
            });
            const item = link.closest('.rail-session-item');
            if (item) item.classList.add('is-active');
            return;
        }

        const deleteBtn = e.target.closest('.rail-session-delete');
        if (!deleteBtn) return;
        const sessionId = deleteBtn.dataset.sessionId;
        const item = deleteBtn.closest('.rail-session-item');
        const wasActive = !!(item && item.classList.contains('is-active'));
        fetch('/api/sessions/' + encodeURIComponent(sessionId), { method: 'DELETE' })
            .then(function() {
                if (item) item.remove();
                if (wasActive) resetToEmptyState();
            })
            .catch(function() { /* best-effort */ });
    });
}

// Cached at load time (see DOMContentLoaded below) because loading a past
// session's history replaces #chat-messages' entire innerHTML, permanently
// detaching the #empty-state node from the document. Keeping the actual node
// (not a re-parsed HTML string) means its prompt-button listeners, bound
// once by initializePromptButtons, still work after it's reattached.
let emptyStateNode = null;

function resetToEmptyState() {
    const chatMessages = document.getElementById('chat-messages');
    if (!chatMessages) return;
    chatMessages.innerHTML = '';
    if (emptyStateNode) {
        chatMessages.appendChild(emptyStateNode);
    }
}

// Initialize when DOM is loaded
document.addEventListener('DOMContentLoaded', function() {
    emptyStateNode = document.getElementById('empty-state');
    displayVersionInfo();
    initializeDiagramDownload();
    initializeTooltips();
    initializeTraceFiltering();
    initializeTurnChipLinking();
    initializePromptButtons();
    initializeSessionRail();

    const messageInput = document.getElementById('message-input');
    const chatForm = document.getElementById('chat-form');
    const clearChatButton = document.getElementById('clear-chat');
    const exportChatButton = document.getElementById('export-chat');

    if (messageInput) {
        messageInput.focus();
    }

    checkConnectionStatus();
    setInterval(checkConnectionStatus, 30000);

    if (clearChatButton) {
        clearChatButton.addEventListener('click', function() {
            if (confirm('Are you sure you want to clear the chat?')) {
                resetToEmptyState();
            }
        });
    }

    if (chatForm) {
        chatForm.addEventListener('htmx:afterRequest', function(event) {
            if (event.detail.successful) {
                if (messageInput) {
                    messageInput.value = '';
                }
                const chatMessages = document.getElementById('chat-messages');
                if (chatMessages) {
                    setTimeout(function() {
                        chatMessages.scrollTop = chatMessages.scrollHeight;
                    }, 100);
                }
            }
            if (messageInput) {
                messageInput.focus();
            }
        });
    }

    if (messageInput) {
        messageInput.addEventListener('keypress', function(e) {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                if (chatForm) {
                    chatForm.requestSubmit ? chatForm.requestSubmit() : chatForm.dispatchEvent(new Event('submit'));
                }
            }
        });
    }

    if (exportChatButton) {
        exportChatButton.addEventListener('click', function() {
            const chatMessages = document.getElementById('chat-messages');
            if (!chatMessages) return;

            const messages = chatMessages.querySelectorAll('.message');

            let exportText = 'Avi Agent - Chat Export\n';
            exportText += '=====================================\n\n';

            messages.forEach(function(message) {
                const headerStrong = message.querySelector('.message-header strong');
                const timestamp = message.querySelector('.timestamp');
                const content = message.querySelector('.message-content');
                if (!headerStrong || !content) return;

                exportText += `${headerStrong.textContent} (${timestamp ? timestamp.textContent : ''}):\n${content.textContent.trim()}\n\n`;
            });

            const blob = new Blob([exportText], { type: 'text/plain' });
            const url = window.URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = `avi-chat-${new Date().toISOString().split('T')[0]}.txt`;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            window.URL.revokeObjectURL(url);
        });
    }
});

function checkConnectionStatus() {
    const indicator = document.getElementById('connection-indicator');
    if (!indicator) return;

    const statusDot = indicator.querySelector('.status-dot');
    const statusText = indicator.querySelector('small');
    const startedAt = performance.now();

    fetch('/api/health')
        .then(response => response.json())
        .then(data => {
            const latencyMs = Math.round(performance.now() - startedAt);
            if (data.avi_status === 'healthy' && data.llm_status === 'healthy') {
                statusDot.className = 'status-dot is-healthy';
                statusText.textContent = `Controller healthy · ${latencyMs}ms`;
            } else {
                statusDot.className = 'status-dot is-critical';
                statusText.textContent = 'Controller degraded';
            }
        })
        .catch(() => {
            statusDot.className = 'status-dot is-critical';
            statusText.textContent = 'Controller unreachable';
        });
}

// Simple tooltip implementation for [title] elements
function initializeTooltips() {
    const tooltipElements = document.querySelectorAll('[title]');

    tooltipElements.forEach(element => {
        element.addEventListener('mouseenter', function() {
            const title = this.getAttribute('title');
            if (title) {
                const tooltip = document.createElement('div');
                tooltip.className = 'custom-tooltip';
                tooltip.textContent = title;
                tooltip.style.position = 'absolute';
                tooltip.style.backgroundColor = 'rgba(20, 22, 31, 0.95)';
                tooltip.style.color = '#e9e9ed';
                tooltip.style.padding = '4px 8px';
                tooltip.style.borderRadius = '4px';
                tooltip.style.fontSize = '11px';
                tooltip.style.zIndex = '1000';
                tooltip.style.whiteSpace = 'nowrap';
                tooltip.style.pointerEvents = 'none';

                document.body.appendChild(tooltip);
                this._tooltip = tooltip;

                const rect = this.getBoundingClientRect();
                tooltip.style.left = (rect.left + window.scrollX + rect.width/2 - tooltip.offsetWidth/2) + 'px';
                tooltip.style.top = (rect.top + window.scrollY - tooltip.offsetHeight - 5) + 'px';
            }
        });

        element.addEventListener('mouseleave', function() {
            if (this._tooltip) {
                document.body.removeChild(this._tooltip);
                this._tooltip = null;
            }
        });
    });
}
