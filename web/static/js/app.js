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

// Simple Log Streaming
let logPauseState = false;

function initializeSSELogs() {
    const logsDisplay = document.getElementById('logs-display');
    const pauseLogsButton = document.getElementById('pause-logs');
    const clearLogsButton = document.getElementById('clear-logs');
    
    if (!logsDisplay) return;
    
    // Start simple log streaming
    startLogStreaming();
    
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

// Simple log streaming - just fetch and display logs periodically
function startLogStreaming() {
    // Fetch logs every 2 seconds
    setInterval(fetchLogs, 2000);
}

function fetchLogs() {
    if (logPauseState) return;
    
    // Fetch logs from server
    fetch('/api/logs')
        .then(response => {
            if (!response.ok) {
                throw new Error('Failed to fetch logs');
            }
            return response.json();
        })
        .then(logs => {
            // Process and display logs
            logs.forEach(logEntry => {
                processLogEntry(logEntry);
            });
        })
        .catch(error => {
            console.error('Error fetching logs:', error);
        });
}

function processLogEntry(logEntry) {
    const logsDisplay = document.getElementById('logs-display');
    if (!logsDisplay) return;
    
    // Check if log should be displayed based on filters
    if (shouldDisplayLog(logEntry)) {
        const logElement = createLogElement(logEntry);
        logsDisplay.appendChild(logElement);
        
        // Auto-scroll to bottom
        setTimeout(() => {
            logsDisplay.scrollTop = logsDisplay.scrollHeight;
        }, 50);
    }
}

function shouldDisplayLog(logEntry) {
    // First, filter out health check logs
    if (logEntry.message && logEntry.message.includes("Health check requested")) {
        return false;
    }
    
    if (logEntry.endpoint === "/health") {
        return false;
    }
    
    // Check filter checkboxes
    const showMistral = document.getElementById('show-mistral')?.checked || true;
    const showAvi = document.getElementById('show-avi')?.checked || true;
    const showSystem = document.getElementById('show-system')?.checked || true;
    
    const logType = logEntry.type;
    
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
        
        const payloadTitle = document.createElement('strong');
        payloadTitle.textContent = 'Payload:';
        payloadSection.appendChild(payloadTitle);
        
        const payloadElement = document.createElement('pre');
        payloadElement.className = 'log-payload-content';
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
        
        const responsePayloadTitle = document.createElement('strong');
        responsePayloadTitle.textContent = 'Response Payload:';
        responsePayloadSection.appendChild(responsePayloadTitle);
        
        const responsePayloadElement = document.createElement('pre');
        responsePayloadElement.className = 'log-payload-content';
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
            
            const detailValue = document.createElement('span');
            detailValue.className = 'log-context-value';
            
            if (typeof value === 'object' && value !== null) {
                try {
                    detailValue.textContent = JSON.stringify(value);
                } catch (e) {
                    detailValue.textContent = String(value);
                }
            } else {
                detailValue.textContent = String(value);
            }
            
            detailItem.appendChild(detailKey);
            detailItem.appendChild(detailValue);
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

// Enhanced log filtering functionality
function initializeEnhancedLogFiltering() {
    console.log('initializeEnhancedLogFiltering called');
    
    // Try to find enhanced filtering elements
    const typeFilter = document.getElementById('log-type-filter');
    const levelFilter = document.getElementById('log-level-filter');
    const searchInput = document.getElementById('log-search');
    const clearSearchBtn = document.getElementById('clear-search');
    const clearFiltersBtn = document.getElementById('clear-filters');
    
    console.log('Filter elements found:', {
        typeFilter: !!typeFilter,
        levelFilter: !!levelFilter,
        searchInput: !!searchInput,
        clearSearchBtn: !!clearSearchBtn,
        clearFiltersBtn: !!clearFiltersBtn
    });
    
    // Check if we have the enhanced filtering UI
    const hasEnhancedFiltering = typeFilter && levelFilter && searchInput;
    
    console.log('Enhanced log filtering available:', hasEnhancedFiltering);

    if (!hasEnhancedFiltering) {
        console.log('Enhanced filtering UI not found, using legacy system');
        return false; // Enhanced filtering not available
    }
    
    // Store the current EventSource connection
    let currentEventSource = null;
    
    // Connect to enhanced logs endpoint
    function connectEnhancedLogs() {
        // Disconnect any existing connection
        if (currentEventSource) {
            currentEventSource.close();
        }
        
        const logType = typeFilter.value;
        const level = levelFilter.value;
        const search = searchInput.value;
        
        const url = `/api/logs/enhanced?type=${logType}&level=${level}&search=${encodeURIComponent(search)}`;
        console.log('Connecting to URL:', url);
        
        console.log('Connecting to enhanced logs SSE:', url);
        
        // Use EventSource for SSE
        currentEventSource = new EventSource(url);
        
        currentEventSource.onopen = function() {
            console.log('Enhanced logs SSE connection established');
        };
        
        currentEventSource.onmessage = function(e) {
            const log = JSON.parse(e.data);
            processLogEntry(log);
        };
        
        currentEventSource.onerror = function(error) {
            console.error("Enhanced logs EventSource error:", error);
            // Don't fallback to legacy - let the enhanced system handle reconnection
            setTimeout(connectEnhancedLogs, 2000); // Reconnect after delay
        };
    }
    
    // Clear search input
    clearSearchBtn.addEventListener('click', function() {
        searchInput.value = '';
        connectEnhancedLogs();
    });
    
    // Clear all filters
    clearFiltersBtn.addEventListener('click', function() {
        typeFilter.value = 'all';
        levelFilter.value = 'all';
        searchInput.value = '';
        connectEnhancedLogs();
    });
    
    // Debounced search
    let searchTimeout;
    searchInput.addEventListener('input', function() {
        clearTimeout(searchTimeout);
        searchTimeout = setTimeout(connectEnhancedLogs, 500);
    });
    
    // Filter changes
    typeFilter.addEventListener('change', connectEnhancedLogs);
    levelFilter.addEventListener('change', connectEnhancedLogs);
    
    // Initial connection
    connectEnhancedLogs();
    
    console.log('Enhanced log filtering initialized successfully');
    return true; // Enhanced filtering is active
}

// Initialize when DOM is loaded
document.addEventListener('DOMContentLoaded', function() {
    console.log('DOMContentLoaded event fired');
    
    // Small delay to ensure DOM is fully ready
    setTimeout(function() {
        // Initialize dark mode toggle
        initializeDarkModeToggle();
    
    // Display version information
    displayVersionInfo();
    
    // Initialize tooltips for API logs link
    initializeTooltips();
    
    // Initialize enhanced log filtering if available
    const enhancedFilteringActive = initializeEnhancedLogFiltering();
    
    console.log('Enhanced filtering active:', enhancedFilteringActive);
    
    // Initialize SSE logs (fallback) only if enhanced filtering is not active
    if (!enhancedFilteringActive) {
        console.log('Using legacy log system');
        initializeSSELogs();
    } else {
        console.log('Using enhanced log system - legacy system disabled');
    }
    
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

// Initialize tooltips for UI elements
function initializeTooltips() {
    const tooltipElements = document.querySelectorAll('[title]');
    
    tooltipElements.forEach(element => {
        // Simple tooltip implementation
        element.addEventListener('mouseenter', function() {
            const title = this.getAttribute('title');
            if (title) {
                const tooltip = document.createElement('div');
                tooltip.className = 'custom-tooltip';
                tooltip.textContent = title;
                tooltip.style.position = 'absolute';
                tooltip.style.backgroundColor = 'rgba(var(--color-slate-900-rgb), 0.9)';
                tooltip.style.color = 'var(--color-white)';
                tooltip.style.padding = '0.25rem 0.5rem';
                tooltip.style.borderRadius = 'var(--border-radius-sm)';
                tooltip.style.fontSize = 'var(--font-size-xs)';
                tooltip.style.zIndex = '1000';
                tooltip.style.whiteSpace = 'nowrap';
                tooltip.style.pointerEvents = 'none';
                
                document.body.appendChild(tooltip);
                this._tooltip = tooltip;
                
                // Position tooltip
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

// Utility function to format JSON for display
function formatJsonForDisplay(jsonObj) {
    try {
        return JSON.stringify(jsonObj, null, 2);
    } catch (e) {
        return String(jsonObj);
    }
}
});