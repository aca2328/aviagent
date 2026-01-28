// Dark Mode Toggle Functionality
function initializeDarkModeToggle() {
    const darkModeToggle = document.getElementById('dark-mode-toggle');
    const htmlElement = document.documentElement;
    
    if (!darkModeToggle) {
        console.warn('Dark mode toggle button not found');
        return;
    }
    
    // Check for saved preference or use system preference
    const savedPreference = localStorage.getItem('darkModePreference');
    const prefersDarkMode = window.matchMedia('(prefers-color-scheme: dark)').matches;
    
    // Determine initial state
    let isDarkMode;
    if (savedPreference === 'dark') {
        isDarkMode = true;
    } else if (savedPreference === 'light') {
        isDarkMode = false;
    } else {
        // Use system preference if no saved preference
        isDarkMode = prefersDarkMode;
    }
    
    // Apply initial state
    updateDarkMode(isDarkMode);
    
    // Add click event listener
    darkModeToggle.addEventListener('click', function() {
        isDarkMode = !isDarkMode;
        updateDarkMode(isDarkMode);
        saveDarkModePreference(isDarkMode);
    });
    
    // Listen for system preference changes
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', function(e) {
        // Only update if we're using system preference (no saved preference)
        if (!localStorage.getItem('darkModePreference')) {
            updateDarkMode(e.matches);
        }
    });
}

function updateDarkMode(isDarkMode) {
    const darkModeToggle = document.getElementById('dark-mode-toggle');
    const htmlElement = document.documentElement;
    
    if (isDarkMode) {
        htmlElement.setAttribute('data-color-scheme', 'dark');
        darkModeToggle.classList.add('dark-mode-active');
        darkModeToggle.setAttribute('title', 'Switch to Light Mode');
        darkModeToggle.innerHTML = '<i class="fas fa-sun"></i>';
    } else {
        htmlElement.setAttribute('data-color-scheme', 'light');
        darkModeToggle.classList.remove('dark-mode-active');
        darkModeToggle.setAttribute('title', 'Switch to Dark Mode');
        darkModeToggle.innerHTML = '<i class="fas fa-moon"></i>';
    }
}

function saveDarkModePreference(isDarkMode) {
    const preference = isDarkMode ? 'dark' : 'light';
    localStorage.setItem('darkModePreference', preference);
}

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

// SSE Logs Functionality
let sseConnection = null;
let isSseConnected = false;
let logPauseState = false;

function initializeSSELogs() {
    const logsDisplay = document.getElementById('logs-display');
    const pauseLogsButton = document.getElementById('pause-logs');
    const clearLogsButton = document.getElementById('clear-logs');
    
    if (!logsDisplay) return;
    
    // Initialize SSE connection
    connectSSE();
    
    // Pause/Resume logs button
    if (pauseLogsButton) {
        pauseLogsButton.addEventListener('click', function() {
            logPauseState = !logPauseState;
            const icon = pauseLogsButton.querySelector('i');
            
            if (logPauseState) {
                icon.classList.remove('fa-pause');
                icon.classList.add('fa-play');
                pauseLogsButton.setAttribute('title', 'Resume logs');
            } else {
                icon.classList.remove('fa-play');
                icon.classList.add('fa-pause');
                pauseLogsButton.setAttribute('title', 'Pause logs');
            }
        });
    }
    
    // Clear logs button
    if (clearLogsButton) {
        clearLogsButton.addEventListener('click', function() {
            if (confirm('Are you sure you want to clear all logs?')) {
                logsDisplay.innerHTML = '';
                addSystemLog('Logs cleared by user');
            }
        });
    }
    
    // Log filtering
    setupLogFiltering();
}

function connectSSE() {
    if (sseConnection) {
        sseConnection.close();
    }
    
    sseConnection = new EventSource('/events');
    isSseConnected = true;
    
    sseConnection.onopen = function() {
        console.log('SSE connection established');
        addSystemLog('Connected to real-time operation logs');
    };
    
    sseConnection.onmessage = function(event) {
        if (logPauseState) return;
        
        try {
            const logEntry = JSON.parse(event.data);
            processLogEntry(logEntry);
        } catch (error) {
            console.error('Error parsing log entry:', error);
        }
    };
    
    sseConnection.onerror = function(error) {
        console.error('SSE connection error:', error);
        isSseConnected = false;
        addSystemLog('SSE connection error: ' + error.message, true);
        
        // Attempt to reconnect after delay
        setTimeout(connectSSE, 5000);
    };
}

function processLogEntry(logEntry) {
    const logsDisplay = document.getElementById('logs-display');
    if (!logsDisplay) return;
    
    // Check if log should be displayed based on filters
    if (shouldDisplayLog(logEntry.type)) {
        const logElement = createLogElement(logEntry);
        logsDisplay.appendChild(logElement);
        
        // Auto-scroll to bottom
        setTimeout(() => {
            logsDisplay.scrollTop = logsDisplay.scrollHeight;
        }, 50);
    }
}

function shouldDisplayLog(logType) {
    // Check filter checkboxes
    const showMistral = document.getElementById('show-mistral')?.checked || true;
    const showAvi = document.getElementById('show-avi')?.checked || true;
    const showSystem = document.getElementById('show-system')?.checked || true;
    
    if (logType === 'mistral_request' || logType === 'mistral_response') {
        return showMistral;
    } else if (logType === 'avi_request' || logType === 'avi_response') {
        return showAvi;
    } else {
        return showSystem;
    }
}

function setupLogFiltering() {
    const filterCheckboxes = document.querySelectorAll('#logs-header input[type="checkbox"]');
    
    filterCheckboxes.forEach(checkbox => {
        checkbox.addEventListener('change', function() {
            // When filters change, we could re-filter existing logs
            // For now, just let new logs be filtered
        });
    });
}

function createLogElement(logEntry) {
    const logElement = document.createElement('div');
    
    // Determine log type and class
    let logTypeClass = 'system-log';
    let logTypeText = 'SYSTEM';
    
    switch(logEntry.type) {
        case 'mistral_request':
            logTypeClass = 'mistral-request';
            logTypeText = 'MISTRAL REQUEST';
            break;
        case 'mistral_response':
            logTypeClass = 'mistral-response';
            logTypeText = 'MISTRAL RESPONSE';
            break;
        case 'avi_request':
            logTypeClass = 'avi-request';
            logTypeText = 'AVI REQUEST';
            break;
        case 'avi_response':
            logTypeClass = 'avi-response';
            logTypeText = 'AVI RESPONSE';
            break;
        case 'error':
            logTypeClass = 'error-log';
            logTypeText = 'ERROR';
            break;
        default:
            logTypeClass = 'system-log';
            logTypeText = 'SYSTEM';
    }
    
    logElement.className = 'log-entry ' + logTypeClass;
    
    // Create log header
    const logHeader = document.createElement('div');
    logHeader.className = 'log-header';
    
    const logTypeBadge = document.createElement('span');
    logTypeBadge.className = 'log-type-badge badge bg-secondary';
    logTypeBadge.textContent = logTypeText;
    
    const logTimestamp = document.createElement('span');
    logTimestamp.className = 'log-timestamp small text-muted';
    logTimestamp.textContent = logEntry.timestamp || new Date().toISOString();
    
    logHeader.appendChild(logTypeBadge);
    logHeader.appendChild(logTimestamp);
    
    // Create log content
    const logContent = document.createElement('div');
    logContent.className = 'log-content';
    
    // Format content based on type
    if (logEntry.message) {
        logContent.textContent = logEntry.message;
    }
    
    // Add payload if available
    if (logEntry.payload) {
        const payloadElement = document.createElement('pre');
        try {
            const formattedPayload = JSON.stringify(logEntry.payload, null, 2);
            payloadElement.textContent = formattedPayload;
        } catch (e) {
            payloadElement.textContent = logEntry.payload;
        }
        logContent.appendChild(payloadElement);
    }
    
    // Add context if available
    if (logEntry.context) {
        const contextElement = document.createElement('div');
        contextElement.className = 'log-context mt-2';
        contextElement.style.fontSize = 'var(--font-size-xs)';
        contextElement.style.color = 'var(--color-text-secondary)';
        contextElement.textContent = 'Context: ' + JSON.stringify(logEntry.context);
        logContent.appendChild(contextElement);
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
    
    // Auto-scroll to bottom
    setTimeout(() => {
        logsDisplay.scrollTop = logsDisplay.scrollHeight;
    }, 50);
}

// Column Resizing Functionality
function initializeColumnResizing() {
    const resizeHandle = document.getElementById('column-resize-handle');
    if (!resizeHandle) return;

    const container = resizeHandle.parentElement;
    const chatColumn = container.querySelector('.chat-column');
    const logsColumn = container.querySelector('.logs-column');

    let isResizing = false;
    let startX = 0;
    let startWidths = { chat: 0, logs: 0 };

    // Mouse down event - start resizing
    resizeHandle.addEventListener('mousedown', function(e) {
        isResizing = true;
        startX = e.clientX;

        // Store initial widths
        startWidths.chat = chatColumn.getBoundingClientRect().width;
        startWidths.logs = logsColumn.getBoundingClientRect().width;

        // Prevent text selection during resize
        document.body.style.userSelect = 'none';
        document.body.style.cursor = 'col-resize';
        container.classList.add('resizing');

        e.preventDefault();
    });

    // Mouse move event - resize columns
    document.addEventListener('mousemove', function(e) {
        if (!isResizing) return;

        // Calculate new widths
        const deltaX = e.clientX - startX;
        const newChatWidth = startWidths.chat + deltaX;
        const newLogsWidth = startWidths.logs - deltaX;

        // Apply minimum and maximum constraints
        const minWidth = 200;
        const maxWidth = container.clientWidth - minWidth - 8; // 8px for resize handle

        const constrainedChatWidth = Math.max(minWidth, Math.min(newChatWidth, maxWidth));
        const constrainedLogsWidth = Math.max(minWidth, Math.min(newLogsWidth, maxWidth));

        // Apply widths
        chatColumn.style.flex = `0 0 ${constrainedChatWidth}px`;
        logsColumn.style.flex = `0 0 ${constrainedLogsWidth}px`;

        // Update resize handle position
        resizeHandle.style.left = `${constrainedChatWidth}px`;
    });

    // Mouse up event - stop resizing
    document.addEventListener('mouseup', function() {
        if (!isResizing) return;

        isResizing = false;
        document.body.style.userSelect = '';
        document.body.style.cursor = '';
        container.classList.remove('resizing');

        // Save preference to localStorage
        const chatWidth = chatColumn.getBoundingClientRect().width;
        const logsWidth = logsColumn.getBoundingClientRect().width;
        localStorage.setItem('columnWidths', JSON.stringify({
            chat: chatWidth,
            logs: logsWidth
        }));
    });

    // Load saved preferences
    loadColumnPreferences();
}

function loadColumnPreferences() {
    const savedWidths = localStorage.getItem('columnWidths');
    if (!savedWidths) return;

    try {
        const widths = JSON.parse(savedWidths);
        const chatColumn = document.querySelector('.chat-column');
        const logsColumn = document.querySelector('.logs-column');

        if (chatColumn && logsColumn) {
            // Apply saved widths
            chatColumn.style.flex = `0 0 ${widths.chat}px`;
            logsColumn.style.flex = `0 0 ${widths.logs}px`;

            // Position resize handle
            const resizeHandle = document.getElementById('column-resize-handle');
            if (resizeHandle) {
                resizeHandle.style.left = `${widths.chat}px`;
            }
        }
    } catch (error) {
        console.warn('Failed to load column preferences:', error);
    }
}

// Window Resize Handling
function initializeWindowResizeHandling() {
    let resizeTimeout;

    window.addEventListener('resize', function() {
        // Debounce resize events
        clearTimeout(resizeTimeout);
        resizeTimeout = setTimeout(function() {
            // Ensure columns maintain proper proportions on resize
            const container = document.querySelector('.resizable-container');
            if (!container) return;

            const chatColumn = container.querySelector('.chat-column');
            const logsColumn = container.querySelector('.logs-column');

            if (chatColumn && logsColumn) {
                // If no custom widths set, maintain 50-50 ratio
                if (!chatColumn.style.flex && !logsColumn.style.flex) {
                    chatColumn.style.flex = '1';
                    logsColumn.style.flex = '1';
                }
            }
        }, 100);
    });
}

// Initialize when DOM is loaded
document.addEventListener('DOMContentLoaded', function() {
    // Initialize dark mode toggle
    initializeDarkModeToggle();
    
    // Display version information
    displayVersionInfo();
    
    // Initialize SSE logs
    initializeSSELogs();
    
    // Initialize column resizing
    initializeColumnResizing();
    
    // Initialize window resize handling
    initializeWindowResizeHandling();
    
    // Get DOM elements once
    const messageInput = document.getElementById('message-input');
    const chatForm = document.getElementById('chat-form');
    const clearChatButton = document.getElementById('clear-chat');
    const exportChatButton = document.getElementById('export-chat');
    
    // Auto-focus on message input
    if (messageInput) {
        messageInput.focus();
    }
    
    // Check connection status
    checkConnectionStatus();
    setInterval(checkConnectionStatus, 30000); // Check every 30 seconds
    
    // Clear chat functionality
    if (clearChatButton) {
        clearChatButton.addEventListener('click', function() {
            if (confirm('Are you sure you want to clear the chat?')) {
                const chatMessages = document.getElementById('chat-messages');
                if (chatMessages) {
                    // Keep the welcome message
                    const welcomeMessage = chatMessages.querySelector('.welcome-message');
                    chatMessages.innerHTML = '';
                    if (welcomeMessage) {
                        chatMessages.appendChild(welcomeMessage);
                    }
                }
            }
        });
    }
    
    // Handle form submission
    if (chatForm) {
        chatForm.addEventListener('htmx:afterRequest', function(event) {
            // Clear the input after successful submission
            if (event.detail.successful) {
                if (messageInput) {
                    messageInput.value = '';
                }
                // Scroll to bottom
                const chatMessages = document.getElementById('chat-messages');
                if (chatMessages) {
                    setTimeout(function() {
                        chatMessages.scrollTop = chatMessages.scrollHeight;
                    }, 100); // Small delay to ensure content is rendered
                }
            }
            
            // Re-focus on input
            if (messageInput) {
                messageInput.focus();
            }
        });
        
        // Add visual feedback during request processing
        chatForm.addEventListener('htmx:beforeRequest', function(event) {
            const loadingIndicator = document.getElementById('loading-indicator');
            if (loadingIndicator) {
                loadingIndicator.style.display = 'block';
            }
            
            // Disable submit button during processing
            const sendButton = document.getElementById('send-button');
            if (sendButton) {
                sendButton.disabled = true;
                sendButton.innerHTML = '<i class="fas fa-spinner fa-spin"></i>';
            }
        });
        
        // Restore UI after request completes
        chatForm.addEventListener('htmx:afterRequest', function(event) {
            const loadingIndicator = document.getElementById('loading-indicator');
            if (loadingIndicator) {
                loadingIndicator.style.display = 'none';
            }
            
            // Re-enable submit button
            const sendButton = document.getElementById('send-button');
            if (sendButton) {
                sendButton.disabled = false;
                sendButton.innerHTML = '<i class="fas fa-paper-plane"></i>';
            }
        });
    }
    
    // Handle Enter key in input
    if (messageInput) {
        messageInput.addEventListener('keypress', function(e) {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                if (chatForm) {
                    chatForm.dispatchEvent(new Event('submit'));
                }
            }
        });
    }
    
    // Export chat functionality
    if (exportChatButton) {
        exportChatButton.addEventListener('click', function() {
            const chatMessages = document.getElementById('chat-messages');
            if (!chatMessages) return;
            
            const messages = chatMessages.querySelectorAll('.message:not(.welcome-message .message)');
            
            let exportText = 'VMware Avi LLM Agent - Chat Export\n';
            exportText += '=====================================\n\n';
            
            messages.forEach(function(message) {
                const header = message.querySelector('.message-header strong').textContent;
                const timestamp = message.querySelector('.timestamp').textContent;
                const content = message.querySelector('.message-content').textContent.trim();
                
                exportText += `${header} (${timestamp}):\n${content}\n\n`;
            });
            
            // Create and download file
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
    fetch('/api/health')
        .then(response => response.json())
        .then(data => {
            const indicator = document.getElementById('connection-indicator');
            if (!indicator) return;
            
            const statusDot = indicator.querySelector('.status-dot');
            const statusText = indicator.querySelector('small');
            
            if (data.avi_status === 'healthy' && data.llm_status === 'healthy') {
                statusDot.className = 'status-dot status-healthy me-2';
                statusText.textContent = 'Connected';
                statusText.className = 'text-success';
            } else {
                statusDot.className = 'status-dot status-error me-2';
                statusText.textContent = 'Connection Issues';
                statusText.className = 'text-danger';
            }
        })
        .catch(error => {
            const indicator = document.getElementById('connection-indicator');
            if (!indicator) return;
            
            const statusDot = indicator.querySelector('.status-dot');
            const statusText = indicator.querySelector('small');
            
            statusDot.className = 'status-dot status-error me-2';
            statusText.textContent = 'Connection Failed';
            statusText.className = 'text-danger';
        });
}

// Utility function to format JSON for display
function formatJsonForDisplay(jsonObj) {
    try {
        return JSON.stringify(jsonObj, null, 2);
    } catch (e) {
        return String(jsonObj);
    }
}