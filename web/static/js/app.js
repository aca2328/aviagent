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

// Initialize when DOM is loaded
document.addEventListener('DOMContentLoaded', function() {
    // Initialize dark mode toggle
    initializeDarkModeToggle();
    
    // Auto-focus on message input
    const messageInput = document.getElementById('message-input');
    if (messageInput) {
        messageInput.focus();
    }
    
    // Handle quick actions
    document.querySelectorAll('.quick-action').forEach(function(element) {
        element.addEventListener('click', function(e) {
            e.preventDefault();
            const query = this.getAttribute('data-query');
            const messageInput = document.getElementById('message-input');
            if (messageInput) {
                messageInput.value = query;
            }
        });
    });
    
    // Check connection status
    checkConnectionStatus();
    setInterval(checkConnectionStatus, 30000); // Check every 30 seconds
    
    // Clear chat functionality
    const clearChatButton = document.getElementById('clear-chat');
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
    const chatForm = document.getElementById('chat-form');
    if (chatForm) {
        chatForm.addEventListener('htmx:afterRequest', function(event) {
            // Clear the input after successful submission
            if (event.detail.successful) {
                const messageInput = document.getElementById('message-input');
                if (messageInput) {
                    messageInput.value = '';
                }
                // Scroll to bottom
                const chatMessages = document.getElementById('chat-messages');
                if (chatMessages) {
                    chatMessages.scrollTop = chatMessages.scrollHeight;
                }
            }
            
            // Re-focus on input
            const messageInput = document.getElementById('message-input');
            if (messageInput) {
                messageInput.focus();
            }
        });
    }
    
    // Handle Enter key in input
    const messageInput = document.getElementById('message-input');
    if (messageInput) {
        messageInput.addEventListener('keypress', function(e) {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                const chatForm = document.getElementById('chat-form');
                if (chatForm) {
                    chatForm.dispatchEvent(new Event('submit'));
                }
            }
        });
    }
    
    // Export chat functionality
    const exportChatButton = document.getElementById('export-chat');
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