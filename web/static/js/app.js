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

function processLogEntry(logEntry) {
    const logsDisplay = document.getElementById('logs-display');
    if (!logsDisplay) return;

    if (!shouldDisplayLog(logEntry)) return;

    const logElement = createLogElement(logEntry);
    logsDisplay.appendChild(logElement);

    setTimeout(() => {
        logsDisplay.scrollTop = logsDisplay.scrollHeight;
    }, 50);
}

// Suppresses health-check noise from the trace stream — /health is polled
// every 30s by checkConnectionStatus and isn't useful to show as a step.
function shouldDisplayLog(logEntry) {
    if (logEntry.message && logEntry.message.includes('Health check requested')) {
        return false;
    }
    if (logEntry.endpoint === '/health') {
        return false;
    }
    return true;
}

function createLogElement(logEntry) {
    const logElement = document.createElement('div');

    // Determine log type and class
    let logTypeClass = 'system-log';
    let logTypeText = 'SYSTEM';
    let logIcon = 'fa-info-circle';

    switch(logEntry.type) {
        case 'mistral_request':
            logTypeClass = 'mistral-request';
            logTypeText = 'MISTRAL REQUEST';
            logIcon = 'fa-robot';
            break;
        case 'mistral_response':
            logTypeClass = 'mistral-response';
            logTypeText = 'MISTRAL RESPONSE';
            logIcon = 'fa-robot';
            break;
        case 'avi_request':
            logTypeClass = 'avi-request';
            logTypeText = 'AVI REQUEST';
            logIcon = 'fa-server';
            break;
        case 'avi_response':
            logTypeClass = 'avi-response';
            logTypeText = 'AVI RESPONSE';
            logIcon = 'fa-server';
            break;
        case 'error':
            logTypeClass = 'error-log';
            logTypeText = 'ERROR';
            logIcon = 'fa-exclamation-triangle';
            break;
        case 'success':
            logTypeClass = 'success-log';
            logTypeText = 'SUCCESS';
            logIcon = 'fa-check-circle';
            break;
        case 'warning':
            logTypeClass = 'warning-log';
            logTypeText = 'WARNING';
            logIcon = 'fa-exclamation-circle';
            break;
        default:
            logTypeClass = 'system-log';
            logTypeText = 'SYSTEM';
            logIcon = 'fa-info-circle';
    }

    logElement.className = 'log-entry ' + logTypeClass;

    // Create log header
    const logHeader = document.createElement('div');
    logHeader.className = 'log-header';

    const logTypeBadge = document.createElement('span');
    logTypeBadge.className = 'log-type-badge badge bg-secondary';
    logTypeBadge.innerHTML = `<i class="fas ${logIcon}"></i> ${logTypeText}`;

    const logTimestamp = document.createElement('span');
    logTimestamp.className = 'log-timestamp small text-muted';
    logTimestamp.textContent = logEntry.timestamp || new Date().toISOString();

    logHeader.appendChild(logTypeBadge);

    // Add status code for response logs
    if (logEntry.status_code) {
        const statusBadge = document.createElement('span');
        statusBadge.className = 'log-status-badge badge ms-2';

        const statusCode = logEntry.status_code;
        if (statusCode >= 200 && statusCode < 300) {
            statusBadge.classList.add('bg-success');
        } else if (statusCode >= 300 && statusCode < 400) {
            statusBadge.classList.add('bg-warning');
        } else if (statusCode >= 400 && statusCode < 500) {
            statusBadge.classList.add('bg-danger');
        } else if (statusCode >= 500) {
            statusBadge.classList.add('bg-danger');
        } else {
            statusBadge.classList.add('bg-secondary');
        }

        statusBadge.textContent = statusCode;
        logHeader.appendChild(statusBadge);
    }

    logHeader.appendChild(logTimestamp);

    // Create log content
    const logContent = document.createElement('div');
    logContent.className = 'log-content';

    // Main message
    if (logEntry.message) {
        const messageElement = document.createElement('div');
        messageElement.className = 'log-message';
        messageElement.textContent = logEntry.message;
        logContent.appendChild(messageElement);
    }

    // Add payload if available
    if (logEntry.payload) {
        const payloadSection = document.createElement('div');
        payloadSection.className = 'log-payload mt-2';

        const payloadId = 'log-json-' + (++logJsonIdCounter);

        const payloadHeader = document.createElement('div');
        payloadHeader.className = 'd-flex justify-content-between align-items-center mb-1';

        const payloadTitle = document.createElement('strong');
        payloadTitle.textContent = 'Payload:';

        const payloadToggle = document.createElement('button');
        payloadToggle.type = 'button';
        payloadToggle.className = 'btn btn-sm btn-outline-secondary log-json-toggle-btn';
        payloadToggle.setAttribute('data-bs-toggle', 'collapse');
        payloadToggle.setAttribute('data-bs-target', '#' + payloadId);
        payloadToggle.setAttribute('aria-expanded', 'false');
        payloadToggle.setAttribute('aria-controls', payloadId);
        payloadToggle.innerHTML = '<i class="fas fa-code"></i> Show/Hide JSON';

        payloadHeader.appendChild(payloadTitle);
        payloadHeader.appendChild(payloadToggle);
        payloadSection.appendChild(payloadHeader);

        const payloadElement = document.createElement('pre');
        payloadElement.className = 'log-payload-content collapse';
        payloadElement.id = payloadId;
        try {
            const formattedPayload = JSON.stringify(logEntry.payload, null, 2);
            payloadElement.textContent = formattedPayload;
        } catch (e) {
            payloadElement.textContent = logEntry.payload;
        }
        payloadSection.appendChild(payloadElement);
        logContent.appendChild(payloadSection);
    }

    // Add headers if available
    if (logEntry.headers) {
        const headersSection = document.createElement('div');
        headersSection.className = 'log-headers mt-2';

        const headersTitle = document.createElement('strong');
        headersTitle.textContent = 'Request Headers:';
        headersSection.appendChild(headersTitle);

        const headersDetails = document.createElement('div');
        headersDetails.className = 'log-headers-details';

        for (const [key, value] of Object.entries(logEntry.headers)) {
            const headerItem = document.createElement('div');
            headerItem.className = 'log-header-item';

            const headerKey = document.createElement('span');
            headerKey.className = 'log-header-key';
            headerKey.textContent = `${key}: `;

            const headerValue = document.createElement('span');
            headerValue.className = 'log-header-value';
            headerValue.textContent = String(value);

            headerItem.appendChild(headerKey);
            headerItem.appendChild(headerValue);
            headersDetails.appendChild(headerItem);
        }

        headersSection.appendChild(headersDetails);
        logContent.appendChild(headersSection);
    }

    // Add response headers if available
    if (logEntry.response_headers) {
        const responseHeadersSection = document.createElement('div');
        responseHeadersSection.className = 'log-response-headers mt-2';

        const responseHeadersTitle = document.createElement('strong');
        responseHeadersTitle.textContent = 'Response Headers:';
        responseHeadersSection.appendChild(responseHeadersTitle);

        const responseHeadersDetails = document.createElement('div');
        responseHeadersDetails.className = 'log-headers-details';

        for (const [key, value] of Object.entries(logEntry.response_headers)) {
            const headerItem = document.createElement('div');
            headerItem.className = 'log-header-item';

            const headerKey = document.createElement('span');
            headerKey.className = 'log-header-key';
            headerKey.textContent = `${key}: `;

            const headerValue = document.createElement('span');
            headerValue.className = 'log-header-value';
            headerValue.textContent = String(value);

            headerItem.appendChild(headerKey);
            headerItem.appendChild(headerValue);
            responseHeadersDetails.appendChild(headerItem);
        }

        responseHeadersSection.appendChild(responseHeadersDetails);
        logContent.appendChild(responseHeadersSection);
    }

    // Add response payload if available
    if (logEntry.response_payload) {
        const responsePayloadSection = document.createElement('div');
        responsePayloadSection.className = 'log-response-payload mt-2';

        const responsePayloadId = 'log-json-' + (++logJsonIdCounter);

        const responsePayloadHeader = document.createElement('div');
        responsePayloadHeader.className = 'd-flex justify-content-between align-items-center mb-1';

        const responsePayloadTitle = document.createElement('strong');
        responsePayloadTitle.textContent = 'Response Payload:';

        const responsePayloadToggle = document.createElement('button');
        responsePayloadToggle.type = 'button';
        responsePayloadToggle.className = 'btn btn-sm btn-outline-secondary log-json-toggle-btn';
        responsePayloadToggle.setAttribute('data-bs-toggle', 'collapse');
        responsePayloadToggle.setAttribute('data-bs-target', '#' + responsePayloadId);
        responsePayloadToggle.setAttribute('aria-expanded', 'false');
        responsePayloadToggle.setAttribute('aria-controls', responsePayloadId);
        responsePayloadToggle.innerHTML = '<i class="fas fa-code"></i> Show/Hide JSON';

        responsePayloadHeader.appendChild(responsePayloadTitle);
        responsePayloadHeader.appendChild(responsePayloadToggle);
        responsePayloadSection.appendChild(responsePayloadHeader);

        const responsePayloadElement = document.createElement('pre');
        responsePayloadElement.className = 'log-payload-content collapse';
        responsePayloadElement.id = responsePayloadId;
        try {
            const formattedPayload = JSON.stringify(logEntry.response_payload, null, 2);
            responsePayloadElement.textContent = formattedPayload;
        } catch (e) {
            responsePayloadElement.textContent = logEntry.response_payload;
        }
        responsePayloadSection.appendChild(responsePayloadElement);
        logContent.appendChild(responsePayloadSection);
    }

    // Add context if available - display as structured details
    if (logEntry.context && Object.keys(logEntry.context).length > 0) {
        const contextSection = document.createElement('div');
        contextSection.className = 'log-context mt-2';

        const contextTitle = document.createElement('strong');
        contextTitle.textContent = 'Details:';
        contextSection.appendChild(contextTitle);

        const contextDetails = document.createElement('div');
        contextDetails.className = 'log-context-details';

        // Display context as key-value pairs for better readability
        for (const [key, value] of Object.entries(logEntry.context)) {
            const detailItem = document.createElement('div');
            detailItem.className = 'log-context-item';

            const detailKey = document.createElement('span');
            detailKey.className = 'log-context-key';
            detailKey.textContent = `${key}: `;
            detailItem.appendChild(detailKey);

            let formatted;
            if (typeof value === 'object' && value !== null) {
                try {
                    formatted = JSON.stringify(value, null, 2);
                } catch (e) {
                    formatted = String(value);
                }
            } else {
                formatted = String(value);
            }

            // Large values (objects or long strings) get a collapsible block; small ones stay inline.
            if (formatted.length > 200) {
                const jsonId = 'log-json-' + (++logJsonIdCounter);

                const toggle = document.createElement('button');
                toggle.type = 'button';
                toggle.className = 'btn btn-sm btn-outline-secondary log-json-toggle-btn';
                toggle.setAttribute('data-bs-toggle', 'collapse');
                toggle.setAttribute('data-bs-target', '#' + jsonId);
                toggle.setAttribute('aria-expanded', 'false');
                toggle.setAttribute('aria-controls', jsonId);
                toggle.innerHTML = '<i class="fas fa-code"></i> Show/Hide JSON';
                detailItem.appendChild(toggle);

                const pre = document.createElement('pre');
                pre.className = 'log-payload-content collapse mt-1';
                pre.id = jsonId;
                pre.textContent = formatted;
                detailItem.appendChild(pre);
            } else {
                const detailValue = document.createElement('span');
                detailValue.className = 'log-context-value';
                detailValue.textContent = formatted;
                detailItem.appendChild(detailValue);
            }

            contextDetails.appendChild(detailItem);
        }

        contextSection.appendChild(contextDetails);
        logContent.appendChild(contextSection);
    }

    // Add any additional fields dynamically
    const additionalFields = ['model', 'duration', 'status', 'error', 'tool', 'tool_call_index', 'tool_name', 'arguments'];
    const addedFields = new Set(['type', 'message', 'timestamp', 'payload', 'context']);

    for (const field of additionalFields) {
        if (logEntry[field] !== undefined && !addedFields.has(field)) {
            const fieldElement = document.createElement('div');
            fieldElement.className = 'log-additional-field mt-1';

            const fieldKey = document.createElement('strong');
            fieldKey.textContent = `${field.charAt(0).toUpperCase() + field.slice(1)}: `;

            const fieldValue = document.createElement('span');
            if (typeof logEntry[field] === 'object') {
                try {
                    fieldValue.textContent = JSON.stringify(logEntry[field]);
                } catch (e) {
                    fieldValue.textContent = String(logEntry[field]);
                }
            } else {
                fieldValue.textContent = String(logEntry[field]);
            }

            fieldElement.appendChild(fieldKey);
            fieldElement.appendChild(fieldValue);
            logContent.appendChild(fieldElement);

            addedFields.add(field);
        }
    }

    logElement.appendChild(logHeader);
    logElement.appendChild(logContent);

    return logElement;
}

function addSystemLog(message, isError = false) {
    const logsDisplay = document.getElementById('logs-display');
    if (!logsDisplay) return;

    const logElement = document.createElement('div');
    logElement.className = 'log-entry ' + (isError ? 'error-log' : 'system-log');

    const logHeader = document.createElement('div');
    logHeader.className = 'log-header';

    const logTypeBadge = document.createElement('span');
    logTypeBadge.className = 'log-type-badge badge ' + (isError ? 'bg-danger' : 'bg-secondary');
    logTypeBadge.textContent = isError ? 'ERROR' : 'SYSTEM';

    const logTimestamp = document.createElement('span');
    logTimestamp.className = 'log-timestamp small text-muted';
    logTimestamp.textContent = new Date().toISOString();

    logHeader.appendChild(logTypeBadge);
    logHeader.appendChild(logTimestamp);

    const logContent = document.createElement('div');
    logContent.className = 'log-content';
    logContent.textContent = message;

    logElement.appendChild(logHeader);
    logElement.appendChild(logContent);

    logsDisplay.appendChild(logElement);

    setTimeout(() => {
        logsDisplay.scrollTop = logsDisplay.scrollHeight;
    }, 50);
}

// Trace filter row: a 4-segment control (All / LLM / Avi / Errors) plus a
// search field, replacing the old type+level selects and legacy checkboxes.
// Segments map to the same /api/logs/enhanced query the old selects drove.
function initializeTraceFiltering() {
    const segButtons = document.querySelectorAll('#trace-type-seg .trace-seg-opt');
    const searchInput = document.getElementById('log-search');
    const clearSearchBtn = document.getElementById('clear-search');
    const clearLogsButton = document.getElementById('clear-logs');
    const logsDisplay = document.getElementById('logs-display');
    const statusText = document.getElementById('trace-status-text');
    const statusDot = document.getElementById('trace-status-dot');

    if (!logsDisplay || segButtons.length === 0 || !searchInput) return;

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

        const url = `/api/logs/enhanced?type=${logType}&level=${level}&search=${encodeURIComponent(search)}`;

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

    connectTraceStream();
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

function resetToEmptyState() {
    const chatMessages = document.getElementById('chat-messages');
    if (!chatMessages) return;
    const emptyState = document.getElementById('empty-state');
    chatMessages.innerHTML = '';
    if (emptyState) {
        chatMessages.appendChild(emptyState);
    }
}

// Initialize when DOM is loaded
document.addEventListener('DOMContentLoaded', function() {
    displayVersionInfo();
    initializeDiagramDownload();
    initializeTooltips();
    initializeTraceFiltering();
    initializePromptButtons();

    const messageInput = document.getElementById('message-input');
    const chatForm = document.getElementById('chat-form');
    const clearChatButton = document.getElementById('clear-chat');
    const newSessionButton = document.getElementById('new-session-btn');
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

    if (newSessionButton) {
        newSessionButton.addEventListener('click', resetToEmptyState);
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
