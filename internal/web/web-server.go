package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"aviagent/internal/avi"
	"aviagent/internal/config"
	"aviagent/internal/langfuse"
	"aviagent/internal/llm"
	"aviagent/internal/mcpavi"
	"aviagent/internal/python"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// LLMClient interface defines the methods required for LLM clients
type LLMClient interface {
	GetAvailableModels() []string
	ValidateModel(ctx context.Context, modelName string) (bool, error)
	ProcessNaturalLanguageQuery(ctx context.Context, query, model string, tools interface{}, conversationHistory interface{}) (*llm.LLMResponse, error)
}



// AviClientInterface defines the interface for Avi clients
type AviClientInterface interface {
	ListVirtualServices(ctx context.Context, params map[string]string) (interface{}, error)
	GetVirtualService(ctx context.Context, uuid string, params map[string]string) (interface{}, error)
	CreateVirtualService(ctx context.Context, data map[string]interface{}) (interface{}, error)
	UpdateVirtualService(ctx context.Context, uuid string, data map[string]interface{}) (interface{}, error)
	DeleteVirtualService(ctx context.Context, uuid string) error
	ListPools(ctx context.Context, params map[string]string) (interface{}, error)
	GetPool(ctx context.Context, uuid string, params map[string]string) (interface{}, error)
	CreatePool(ctx context.Context, data map[string]interface{}) (interface{}, error)
	ScaleOutPool(ctx context.Context, uuid string, params map[string]interface{}) error
	ScaleInPool(ctx context.Context, uuid string, params map[string]interface{}) error
	ListHealthMonitors(ctx context.Context, params map[string]string) (interface{}, error)
	GetHealthMonitor(ctx context.Context, uuid string, params map[string]string) (interface{}, error)
	ListServiceEngines(ctx context.Context, params map[string]string) (interface{}, error)
	GetServiceEngine(ctx context.Context, uuid string, params map[string]string) (interface{}, error)
	GetAnalytics(ctx context.Context, resourceType, uuid string, params map[string]string) (interface{}, error)
	ExecuteGenericOperation(ctx context.Context, method, endpoint string, body interface{}, params map[string]string) (interface{}, error)
	Close() error
}

// Server represents the web server
type Server struct {
	config        *config.Config
	logger        *zap.Logger
	aviClient     AviClientInterface
	llmClient      LLMClient
	router        *gin.Engine
	appName       string
	version       string
	buildDate     string
	// Lazy Avi client initialization
	aviClientInit sync.Once
	aviClientErr  error
	aviClientMu   sync.Mutex
	// Lazy Avi MCP client initialization (comprehensive tool-calling path;
	// falls back to the static internal/llm tool set when unavailable). Not a
	// sync.Once: a failed connect (e.g. bundle not yet built, subprocess died)
	// must be retryable on the next message, not sticky for the process lifetime.
	mcpAviClient   *mcpavi.Client
	mcpAviClientMu sync.Mutex
	ShutdownContext context.Context
	// Operation logging for real-time visibility
	operationLogClients map[string]chan map[string]interface{}
	operationLogBuffer  []map[string]interface{}
	operationLogMu      sync.Mutex
	// Simple log buffer for API endpoint (no locking needed for reads)
	simpleLogBuffer []map[string]interface{}
	enhancedLogBuffer *LogBuffer
	sessionStore      SessionStore
}

// EnhancedLogEntry represents a structured log entry with filtering support
type EnhancedLogEntry struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Type      string                 `json:"type"`      // mistral_request, avi_request, user_request, etc.
	Level     string                 `json:"level"`     // info, warn, error, debug
	Message   string                 `json:"message"`
	Context   map[string]interface{} `json:"context,omitempty"`   // headers, payload, etc.
	Metadata map[string]string       `json:"metadata,omitempty"`  // additional tags
	TurnID    string                 `json:"turn_id,omitempty"`  // correlates every step of one chat turn
}

// turnIDContextKey carries a per-request turn ID through context.Context so
// the logging helpers called deep inside processChatMessage (which don't
// otherwise know which chat turn they belong to) can stamp it onto their
// entries without changing their signatures at all ~37 call sites.
type turnIDContextKey struct{}

func contextWithTurn(ctx context.Context, turnID string) context.Context {
	return context.WithValue(ctx, turnIDContextKey{}, turnID)
}

func turnIDFromContext(ctx context.Context) string {
	turnID, _ := ctx.Value(turnIDContextKey{}).(string)
	return turnID
}

// Session write mode. A session's mode gates whether executeToolCall may
// dispatch a write to the Avi controller; see isWriteTool/describeWrite.
const (
	sessionModeReadOnly  = "read-only"
	sessionModeReadWrite = "read-write"
)

// isReadOnlyMode treats anything other than the exact read-write string as
// read-only -- an empty mode (a pre-Phase-5 session, or the JSON /api/chat
// path, which carries no session context at all) is the safe default, not
// an unguarded one.
func isReadOnlyMode(mode string) bool {
	return mode != sessionModeReadWrite
}

type sessionModeContextKey struct{}

func contextWithMode(ctx context.Context, mode string) context.Context {
	return context.WithValue(ctx, sessionModeContextKey{}, mode)
}

func modeFromContext(ctx context.Context) string {
	mode, _ := ctx.Value(sessionModeContextKey{}).(string)
	return mode
}

// activeSessionCookie names the cookie that tracks which session a browser is
// currently talking to. Its max-age matches sessionRetention so a cookie
// never outlives the session file it points at.
const activeSessionCookie = "active_session_id"

func setActiveSessionCookie(c *gin.Context, sessionID string) {
	c.SetCookie(activeSessionCookie, sessionID, int(sessionRetention.Seconds()), "/", "", false, true)
}

func clearActiveSessionCookie(c *gin.Context) {
	c.SetCookie(activeSessionCookie, "", -1, "/", "", false, true)
}

// getOrCreateActiveSession returns the session a chat turn should be
// persisted to: the browser's active_session_id cookie if it still points at
// a live session, or a freshly created one (refreshing the cookie either
// way, so a session's expiry keeps sliding while it's actually in use).
func (s *Server) getOrCreateActiveSession(c *gin.Context, model string) *ChatSession {
	if id, err := c.Cookie(activeSessionCookie); err == nil && id != "" {
		if session, err := s.sessionStore.Get(id); err == nil {
			setActiveSessionCookie(c, id)
			return session
		}
	}

	session, err := s.sessionStore.Create(model)
	if err != nil {
		s.logger.Error("Failed to create chat session", zap.Error(err))
		return nil
	}
	setActiveSessionCookie(c, session.ID)
	return session
}

// LogBuffer represents an enhanced log buffer with filtering capabilities
type LogBuffer struct {
	entries      []EnhancedLogEntry
	maxSize      int
	mu           sync.RWMutex
	sseClients   []chan EnhancedLogEntry
	sseClientsMu sync.Mutex
}

// LogBuffer methods
func NewLogBuffer(maxSize int) *LogBuffer {
	return &LogBuffer{
		entries:    make([]EnhancedLogEntry, 0, maxSize),
		maxSize:    maxSize,
		sseClients: make([]chan EnhancedLogEntry, 0),
	}
}

func (b *LogBuffer) AddEntry(entry EnhancedLogEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Add entry
	b.entries = append(b.entries, entry)

	// Enforce max size
	if len(b.entries) > b.maxSize {
		b.entries = b.entries[len(b.entries)-b.maxSize:]
	}
}

func (b *LogBuffer) GetEntries() []EnhancedLogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Return a copy to avoid race conditions
	entries := make([]EnhancedLogEntry, len(b.entries))
	copy(entries, b.entries)
	return entries
}

func (b *LogBuffer) GetFilteredEntries(logType, level, search, turn string) []EnhancedLogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var filtered []EnhancedLogEntry

	for _, entry := range b.entries {
		// Type filter
		if logType != "" && logType != "all" && !strings.HasPrefix(entry.Type, logType) {
			continue
		}

		// Level filter
		if level != "" && level != "all" && entry.Level != level {
			continue
		}

		// Turn filter
		if turn != "" && entry.TurnID != turn {
			continue
		}

		// Search filter
		if search != "" {
			searchLower := strings.ToLower(search)
			messageMatch := strings.Contains(strings.ToLower(entry.Message), searchLower)
			contextMatch := false
			
			if entry.Context != nil {
				contextJSON, _ := json.Marshal(entry.Context)
				contextMatch = strings.Contains(strings.ToLower(string(contextJSON)), searchLower)
			}
			
			if !messageMatch && !contextMatch {
				continue
			}
		}

		filtered = append(filtered, entry)
	}

	return filtered
}

// LogToFile appends log entries to a file
func (b *LogBuffer) LogToFile(filename string) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if len(b.entries) == 0 {
		return nil
	}

	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	for _, entry := range b.entries {
		data, err := json.Marshal(entry)
		if err != nil {
			// Skip entries that can't be marshaled
			continue
		}
		if _, err := file.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("failed to write log entry: %w", err)
		}
	}

	return nil
}

// AddSSEClient adds an SSE client channel to receive real-time log updates
func (b *LogBuffer) AddSSEClient(clientChan chan EnhancedLogEntry) {
	b.sseClientsMu.Lock()
	defer b.sseClientsMu.Unlock()
	
	b.sseClients = append(b.sseClients, clientChan)
}

// RemoveSSEClient removes an SSE client channel
func (b *LogBuffer) RemoveSSEClient(clientChan chan EnhancedLogEntry) {
	b.sseClientsMu.Lock()
	defer b.sseClientsMu.Unlock()
	
	// Find and remove the client channel
	for i, ch := range b.sseClients {
		if ch == clientChan {
			// Remove the channel by slicing it out
			b.sseClients = append(b.sseClients[:i], b.sseClients[i+1:]...)
			close(ch) // Close the channel to signal to the client
			break
		}
	}
}

// BroadcastToSSEClients sends a log entry to all connected SSE clients
func (b *LogBuffer) BroadcastToSSEClients(entry EnhancedLogEntry) {
	b.sseClientsMu.Lock()
	defer b.sseClientsMu.Unlock()
	
	// Send to all connected clients (non-blocking)
	for _, clientChan := range b.sseClients {
		select {
		case clientChan <- entry:
			// Successfully sent
		default:
			// Client channel full or blocked, skip this client
		}
	}
}

// RotateLogs performs log rotation by moving current file to archive
func RotateLogs(currentFile, archiveDir string) error {
	// Create archive directory if it doesn't exist
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return fmt.Errorf("failed to create archive directory: %w", err)
	}

	// Check if current log file exists
	if _, err := os.Stat(currentFile); os.IsNotExist(err) {
		return nil // No file to rotate
	}

	// Generate archive filename with timestamp
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	archiveFile := filepath.Join(archiveDir, fmt.Sprintf("logs_%s.jsonl", timestamp))

	// Move current file to archive
	if err := os.Rename(currentFile, archiveFile); err != nil {
		return fmt.Errorf("failed to rotate log file: %w", err)
	}

	return nil
}

// ChatMessage represents a chat message for the web interface
type ChatMessage struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`      // "user" or "assistant"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	Model     string    `json:"model,omitempty"`
	ToolCalls []string  `json:"tool_calls,omitempty"`
}

// ChatSession represents a chat session
type ChatSession struct {
	ID       string        `json:"id"`
	Model    string        `json:"model"`
	Mode     string        `json:"mode,omitempty"` // sessionModeReadOnly (default) or sessionModeReadWrite
	Messages []ChatMessage `json:"messages"`
	Created  time.Time     `json:"created"`
}

// getAviClient provides lazy initialization of Avi client
func (s *Server) getAviClient() (AviClientInterface, error) {
	s.aviClientInit.Do(func() {
		s.aviClientMu.Lock()
		defer s.aviClientMu.Unlock()

		if s.aviClient != nil {
			return // Already initialized
		}

		s.broadcastOperationLog("info", "Initializing Avi client", map[string]interface{}{
			"host": s.config.Avi.Host,
			"username": s.config.Avi.Username,
		})

		client, err := avi.NewClient(&s.config.Avi, s.logger)
		if err != nil {
			s.broadcastOperationLog("error", "Avi client initialization failed", map[string]interface{}{
				"error": err.Error(),
			})
			s.aviClientErr = fmt.Errorf("avi client initialization failed: %w", err)
			return
		}

		s.aviClient = client
		s.broadcastOperationLog("success", "Avi client initialized successfully", nil)
	})

	s.aviClientMu.Lock()
	defer s.aviClientMu.Unlock()

	return s.aviClient, s.aviClientErr
}

// getMcpAviClient provides lazy initialization of the Avi MCP client, and
// returns the existing one once connected. Unlike getAviClient, a failed
// connection attempt is NOT cached: MCP setup can fail for reasons that
// resolve without a process restart (bundle not yet built, subprocess briefly
// down), so every call while unconnected retries rather than staying broken
// for the process lifetime.
func (s *Server) getMcpAviClient(ctx context.Context) (*mcpavi.Client, error) {
	if !s.config.MCP.Enabled {
		return nil, fmt.Errorf("avi mcp client disabled by config")
	}

	s.mcpAviClientMu.Lock()
	defer s.mcpAviClientMu.Unlock()

	if s.mcpAviClient != nil {
		return s.mcpAviClient, nil
	}

	client, err := mcpavi.NewClient(&s.config.Avi, &s.config.MCP, s.logger)
	if err != nil {
		return nil, fmt.Errorf("avi mcp client setup failed: %w", err)
	}

	if err := client.Connect(ctx); err != nil {
		return nil, fmt.Errorf("avi mcp client connection failed: %w", err)
	}

	s.mcpAviClient = client
	s.broadcastOperationLog("success", "Avi MCP client connected", map[string]interface{}{
		"tool_count": len(client.Tools()),
	})
	return s.mcpAviClient, nil
}

// NewServer creates a new web server
func NewServer(cfg *config.Config, logger *zap.Logger, appName, version, buildDate string) (*Server, error) {
	logger.Info("NewServer called", 
		zap.String("app_name", appName),
		zap.String("version", version),
		zap.String("build_date", buildDate))

	// Verify Avi controller configuration
	logger.Info("Avi Controller Configuration",
		zap.String("host", cfg.Avi.Host),
		zap.String("username", cfg.Avi.Username),
		zap.String("tenant", cfg.Avi.Tenant),
		zap.Bool("insecure", cfg.Avi.Insecure))

	sessionStore, err := NewJSONLSessionStore(cfg.Sessions.Dir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize session store: %w", err)
	}

	// Create server with version info immediately
	server := &Server{
		config:        cfg,
		logger:        logger,
		appName:       appName,
		version:       version,
		buildDate:     buildDate,
		ShutdownContext: context.Background(), // Will be updated when server starts
		operationLogClients: make(map[string]chan map[string]interface{}),
		simpleLogBuffer:     make([]map[string]interface{}, 0),
		enhancedLogBuffer:   NewLogBuffer(10000), // Store up to 10,000 enhanced log entries
		sessionStore:        sessionStore,
	}

	// Initialize Langfuse client
	if _, err := langfuse.NewClient(&cfg.Langfuse, logger); err != nil {
		logger.Error("Failed to initialize Langfuse client", zap.Error(err))
		// Continue without Langfuse if it fails
	}

	// Initialize LLM client (fast operation)
	var llmClient LLMClient

	if cfg.Provider == "ollama" {
		// Initialize Ollama client
		ollamaClient, err := llm.NewClient(&cfg.LLM, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Ollama client: %w", err)
		}
		llmClient = ollamaClient
		logger.Info("Initialized Ollama LLM client", zap.String("provider", "ollama"))
	} else if cfg.Provider == "python" {
		// Initialize Python Mistral client with bridge
		logger.Info("Initializing Python Mistral client bridge")
		
		// Create Python bridge
		pythonBridge := python.NewPythonBridge(&cfg.Mistral, python.WithLogger(logger))
		
		// Initialize the bridge
		if err := pythonBridge.Initialize(); err != nil {
			logger.Error("Failed to initialize Python Mistral client bridge", zap.Error(err))
			return nil, fmt.Errorf("failed to initialize Python Mistral client bridge: %w", err)
		}
		
		// Use Python bridge as LLM client
		llmClient = pythonBridge
		logger.Info("Python Mistral client bridge initialized successfully", 
			zap.String("provider", "python"),
			zap.Any("bridge_status", pythonBridge.GetStatus()))
	} else {
		return nil, fmt.Errorf("unsupported LLM provider: %s", cfg.Provider)
	}

	// Set LLM client (Avi client will be initialized lazily)
	server.llmClient = llmClient

	// Initialize router (doesn't depend on Avi client)
	server.setupRouter()

	// Start Avi client initialization in background
	go server.initializeAviClientAsync()

	return server, nil
}

// toJSON converts interface{} to JSON string
func toJSON(v interface{}) string {
	bytes, err := json.Marshal(v)
	if err != nil {
		return `{"error": "failed to marshal JSON"}`
	}
	return string(bytes)
}

// renderChatMessage converts a chat message's lightweight markdown (headers, bold,
// bullets, fenced code blocks) into HTML. Fenced code block content is emitted
// verbatim as a single escaped block rather than re-parsed line-by-line, so
// multi-line content like pretty-printed JSON renders as one readable block
// instead of hundreds of individually-wrapped paragraph elements.
func renderChatMessage(message string) template.HTML {
	var b strings.Builder
	inCodeBlock := false
	blockIsAPIResult := false
	blockIsWriteRefusal := false
	pendingToolName := ""
	pendingIsWriteRefusal := false

	for _, line := range strings.Split(message, "\n") {
		if inCodeBlock {
			if line == "```" {
				b.WriteString("</code></pre>")
				if blockIsAPIResult || blockIsWriteRefusal {
					b.WriteString("</div>")
					blockIsAPIResult = false
					blockIsWriteRefusal = false
				}
				inCodeBlock = false
			} else {
				b.WriteString(html.EscapeString(line))
				b.WriteString("\n")
			}
			continue
		}

		switch {
		case strings.HasPrefix(line, "```"):
			if strings.HasSuffix(line, "```") && len(line) >= 6 {
				b.WriteString(`<code class="bg-light px-2 py-1 rounded">`)
				b.WriteString(html.EscapeString(line[3 : len(line)-3]))
				b.WriteString(`</code>`)
			} else {
				jsonID := ""
				if pendingToolName != "" && line == "```json" {
					if pendingIsWriteRefusal {
						blockIsWriteRefusal = true
						jsonID = "write-refusal-" + uuid.New().String()
						b.WriteString(`<div class="write-refusal-card" data-tool="`)
						b.WriteString(html.EscapeString(pendingToolName))
						b.WriteString(`">`)
						b.WriteString(`<div class="write-refusal-toolbar d-flex gap-2 mb-2">`)
						b.WriteString(`<button type="button" class="btn btn-sm btn-outline-warning write-unlock-btn">`)
						b.WriteString(`<i class="fas fa-lock-open"></i> Unlock and apply</button>`)
						b.WriteString(`<button type="button" class="btn btn-sm btn-outline-secondary" data-bs-toggle="collapse" data-bs-target="#`)
						b.WriteString(jsonID)
						b.WriteString(`" aria-expanded="false" aria-controls="`)
						b.WriteString(jsonID)
						b.WriteString(`"><i class="fas fa-code"></i> Show/Hide details</button>`)
						b.WriteString(`</div>`)
					} else {
						blockIsAPIResult = true
						jsonID = "api-result-" + uuid.New().String()
						b.WriteString(`<div class="api-result-block" data-tool="`)
						b.WriteString(html.EscapeString(pendingToolName))
						b.WriteString(`">`)
						b.WriteString(`<div class="api-result-toolbar d-flex gap-2 mb-2">`)
						b.WriteString(`<button type="button" class="btn btn-sm btn-outline-secondary diagram-download-btn" title="Download this result as a standalone node-link diagram page">`)
						b.WriteString(`<i class="fas fa-diagram-project"></i> Download Diagram</button>`)
						b.WriteString(`<button type="button" class="btn btn-sm btn-outline-secondary" data-bs-toggle="collapse" data-bs-target="#`)
						b.WriteString(jsonID)
						b.WriteString(`" aria-expanded="false" aria-controls="`)
						b.WriteString(jsonID)
						b.WriteString(`"><i class="fas fa-code"></i> Show/Hide JSON</button>`)
						b.WriteString(`</div>`)
					}
				}
				b.WriteString(`<pre class="bg-light p-3 rounded`)
				if jsonID != "" {
					b.WriteString(` collapse" id="`)
					b.WriteString(jsonID)
				}
				b.WriteString(`"><code>`)
				inCodeBlock = true
			}
			pendingToolName = ""
			pendingIsWriteRefusal = false
		case strings.HasPrefix(line, "WRITE BLOCKED"):
			b.WriteString(`<h6 class="mt-3 mb-2 text-warning"><i class="fas fa-lock"></i> `)
			b.WriteString(html.EscapeString(line))
			b.WriteString(`</h6>`)
			if open := strings.Index(line, "("); open != -1 {
				if closeIdx := strings.Index(line[open:], ")"); closeIdx != -1 {
					pendingToolName = line[open+1 : open+closeIdx]
					pendingIsWriteRefusal = true
				}
			}
		case strings.HasPrefix(line, "API Result"):
			b.WriteString(`<h6 class="mt-3 mb-2"><i class="fas fa-code"></i> `)
			b.WriteString(html.EscapeString(line))
			b.WriteString(`</h6>`)
			// heading looks like "API Result (tool_name):" — used to name the
			// downloadable diagram; falls back to empty if not present.
			if open := strings.Index(line, "("); open != -1 {
				if closeIdx := strings.Index(line[open:], ")"); closeIdx != -1 {
					pendingToolName = line[open+1 : open+closeIdx]
				}
			}
		case strings.HasPrefix(line, "##"):
			b.WriteString(`<h5 class="mt-3 mb-2">`)
			b.WriteString(html.EscapeString(strings.TrimPrefix(line, "##")))
			b.WriteString(`</h5>`)
		case strings.HasPrefix(line, "#"):
			b.WriteString(`<h4 class="mt-3 mb-2">`)
			b.WriteString(html.EscapeString(strings.TrimPrefix(line, "#")))
			b.WriteString(`</h4>`)
		case strings.HasPrefix(line, "**") && strings.HasSuffix(line, "**") && len(line) >= 4:
			b.WriteString(`<strong>`)
			b.WriteString(html.EscapeString(line[2 : len(line)-2]))
			b.WriteString(`</strong>`)
		case strings.HasPrefix(line, "-"):
			b.WriteString(`<li>`)
			b.WriteString(html.EscapeString(strings.TrimPrefix(strings.TrimPrefix(line, "-"), " ")))
			b.WriteString(`</li>`)
		case line != "":
			b.WriteString(`<p>`)
			b.WriteString(html.EscapeString(line))
			b.WriteString(`</p>`)
		}
	}

	if inCodeBlock {
		b.WriteString("</code></pre>")
	}

	return template.HTML(b.String())
}

// broadcastOperationLog sends operation logs to all connected SSE clients
func (s *Server) broadcastOperationLog(logType, message string, context map[string]interface{}) {
	s.operationLogMu.Lock()
	defer s.operationLogMu.Unlock()

	logEntry := map[string]interface{}{
		"type":    logType,
		"message": message,
	}

	// Add context if provided
	if context != nil {
		for key, value := range context {
			logEntry[key] = value
		}
	}

	// Create enhanced log entry for new buffer
	enhancedEntry := EnhancedLogEntry{
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		Type:      logType,
		Level:     "info", // Default level
		Message:   message,
		Context:   context,
		Metadata:  make(map[string]string),
	}

	// Detect and set appropriate log level from context
	if err, ok := context["error"]; ok && err != nil {
		enhancedEntry.Level = "error"
	}

	// Lift turn_id (stamped into context by processChatMessage's call sites)
	// onto the dedicated field so matchesFilters/GetFilteredEntries can index
	// on it directly instead of every caller unpacking the context map.
	if turnID, ok := context["turn_id"].(string); ok {
		enhancedEntry.TurnID = turnID
	}

	// Add to enhanced log buffer
	s.enhancedLogBuffer.AddEntry(enhancedEntry)
	
	// Broadcast to SSE clients
	s.enhancedLogBuffer.BroadcastToSSEClients(enhancedEntry)

	// Add to buffer for simple log streaming (convert to map for backward compatibility)
	s.operationLogBuffer = append(s.operationLogBuffer, logEntry)
	// Keep buffer size reasonable (max 1000 entries)
	if len(s.operationLogBuffer) > 1000 {
		s.operationLogBuffer = s.operationLogBuffer[1:]
	}
	// Get clients snapshot while still holding the lock
	clients := make(map[string]chan map[string]interface{})
	for clientID, clientChan := range s.operationLogClients {
		clients[clientID] = clientChan
	}
	
	// Add to simple buffer (no locking needed for append)
	s.simpleLogBuffer = append(s.simpleLogBuffer, logEntry)
	// Keep buffer size reasonable (max 1000 entries)
	if len(s.simpleLogBuffer) > 1000 {
		s.simpleLogBuffer = s.simpleLogBuffer[1:]
	}
	
	// Debug: log that we added to simple buffer
	s.logger.Info("Added to simple log buffer", 
		zap.String("log_type", logType),
		zap.String("message", message),
		zap.Int("new_buffer_size", len(s.simpleLogBuffer)))
	
	// Send to all connected clients (outside of lock to avoid deadlocks)
	for clientID, clientChan := range clients {
		select {
		case clientChan <- logEntry:
			// Successfully sent
		default:
			// Client channel full or blocked, skip this client
			s.logger.Warn("Operation log channel full, dropping log entry", 
				zap.String("client_id", clientID))
		}
	}
}

// logAPICall logs API call details including headers and payload
func (s *Server) logAPICall(apiType, method, endpoint string, headers map[string]string, payload interface{}, context map[string]interface{}) {
	// Determine the correct log type based on apiType
	var logType string
	if apiType == "mistral" {
		logType = "mistral_request"
	} else if apiType == "avi" {
		logType = "avi_request"
	} else {
		// For user requests and other types, use the apiType directly
		logType = apiType + "_request"
	}

	// Create log entry with API call details
	logEntry := map[string]interface{}{
		"type":    logType,
		"message": fmt.Sprintf("%s %s", method, endpoint),
		"method":  method,
		"endpoint": endpoint,
	}

	// Add headers if provided
	if headers != nil && len(headers) > 0 {
		logEntry["headers"] = headers
	}

	// Add payload if provided
	if payload != nil {
		// Try to convert payload to JSON for logging
		payloadJSON, err := json.Marshal(payload)
		if err == nil {
			logEntry["payload"] = string(payloadJSON)
		} else {
			logEntry["payload"] = fmt.Sprintf("Failed to marshal payload: %v", err)
		}
	}

	// Add additional context if provided
	if context != nil {
		for key, value := range context {
			logEntry[key] = value
		}
	}

	// Broadcast the log entry with the correct type
	s.broadcastOperationLog(logType, logEntry["message"].(string), logEntry)
}

// logAPIResponse logs API response details including headers and payload
func (s *Server) logAPIResponse(apiType, method, endpoint string, statusCode int, responseHeaders map[string]string, responsePayload interface{}, context map[string]interface{}) {
	// Determine the correct log type based on apiType
	var logType string
	if apiType == "mistral" {
		logType = "mistral_response"
	} else if apiType == "avi" {
		logType = "avi_response"
	} else {
		// For user responses and other types, use the apiType directly
		logType = apiType + "_response"
	}

	// Create log entry with API response details
	logEntry := map[string]interface{}{
		"type":    logType,
		"message": fmt.Sprintf("%s %s - %d", method, endpoint, statusCode),
		"method":  method,
		"endpoint": endpoint,
		"status_code": statusCode,
	}

	// Add response headers if provided
	if responseHeaders != nil && len(responseHeaders) > 0 {
		logEntry["response_headers"] = responseHeaders
	}

	// Add response payload if provided
	if responsePayload != nil {
		// Try to convert response payload to JSON for logging
		payloadJSON, err := json.Marshal(responsePayload)
		if err == nil {
			logEntry["response_payload"] = string(payloadJSON)
		} else {
			logEntry["response_payload"] = fmt.Sprintf("Failed to marshal response payload: %v", err)
		}
	}

	// Add additional context if provided
	if context != nil {
		for key, value := range context {
			logEntry[key] = value
		}
	}

	// Broadcast the log entry with the correct type
	s.broadcastOperationLog(logType, logEntry["message"].(string), logEntry)
}

// initializeAviClientAsync initializes Avi client in background
func (s *Server) initializeAviClientAsync() {
	// Add small delay to allow server to start first
	time.Sleep(1 * time.Second)

	if _, err := s.getAviClient(); err != nil {
		s.logger.Error("Background Avi client initialization failed", zap.Error(err))
		// Schedule retry
		go s.scheduleAviClientRetry()
	}
}

// isHealthCheckProbe checks if request is from a health check probe
func (s *Server) isHealthCheckProbe(c *gin.Context) bool {
	userAgent := c.Request.Header.Get("User-Agent")
	return strings.Contains(userAgent, "kube-probe") ||
	       strings.Contains(userAgent, "GoogleHC") ||
	       strings.Contains(userAgent, "healthcheck") ||
	       strings.Contains(userAgent, "load-balancer-health-check") ||
	       strings.Contains(userAgent, "ELB-HealthChecker") ||
	       strings.Contains(userAgent, "AWS-HealthChecker")
}

// scheduleAviClientRetry implements retry logic for Avi client initialization
func (s *Server) scheduleAviClientRetry() {
	retryInterval := 30 * time.Second
	maxRetries := 5
	retryCount := 0

	for retryCount < maxRetries {
		retryCount++
		s.logger.Info("Retrying Avi client initialization",
			zap.Int("attempt", retryCount),
			zap.Int("max_attempts", maxRetries))

		select {
		case <-time.After(retryInterval):
			if _, err := s.getAviClient(); err == nil {
				s.logger.Info("Avi client initialization successful on retry",
					zap.Int("attempt", retryCount))
				return
			}
		case <-s.ShutdownContext.Done():
			s.logger.Info("Server shutting down, canceling Avi client retries")
			return
		}
	}

	s.logger.Error("Max Avi client initialization retries reached",
		zap.Int("attempts", maxRetries))
}

// setupRouter sets up the Gin router with all routes and middleware
func (s *Server) setupRouter() {
	// Set Gin mode based on log level
	if s.config.Log.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	
	// Debug: Log the actual Gin mode that was set
	s.logger.Info("Gin mode set", zap.String("mode", gin.Mode()))

	s.router = gin.New()

	// Add middleware
	// Only use Gin logger in debug mode, otherwise use our own logging
	if gin.Mode() == gin.DebugMode {
		s.router.Use(gin.Logger())
	}
	s.router.Use(gin.Recovery())
	s.router.Use(s.corsMiddleware())

	// Set up template functions
	s.router.SetFuncMap(template.FuncMap{
		"now": time.Now,
		"renderChatMessage": renderChatMessage,
	})

	// Load HTML templates
	// In Docker: working directory is /web, so templates are at templates/*
	// In local dev: working directory is project root, so templates are at web/templates/*
	templatePath := "templates/*"
	if _, err := os.Stat("web/templates"); err == nil {
		templatePath = "web/templates/*"
	}
	s.router.LoadHTMLGlob(templatePath)

	// Serve static files
	staticPath := "static"
	if _, err := os.Stat("web/static"); err == nil {
		staticPath = "web/static"
	}
	s.router.Static("/static", staticPath)

	// Routes
	s.setupRoutes()
}

// setupRoutes sets up all the routes
func (s *Server) setupRoutes() {
	// Main page
	s.router.GET("/", s.handleIndex)

	// API routes
	api := s.router.Group("/api")
	{
		// Chat endpoints
		api.POST("/chat", s.handleChat)
		api.GET("/chat/history", s.handleChatHistory)
		api.DELETE("/chat/history", s.handleClearHistory)

		// Model management
		api.GET("/models", s.handleGetModels)
		api.POST("/models/validate", s.handleValidateModel)

		// Health check
		api.GET("/health", s.handleHealth)

		// Server-Sent Events for real-time operation logs
		api.GET("/events", s.handleOperationEvents)

		// Simple logs endpoint for streaming
		api.GET("/logs", s.handleGetLogs)

		// Enhanced logs endpoint with filtering support
		api.GET("/logs/enhanced", s.handleEnhancedLogs)

		// Avi API proxy (for direct API access)
		api.Any("/avi/*path", s.handleAviProxy)

		// Session management (delete is called via plain fetch, not htmx —
		// see initializeSessionRail in app.js)
		api.DELETE("/sessions/:id", s.handleDeleteSession)
		api.POST("/sessions/:id/mode", s.handleSetSessionMode)

		// Explicit unlock-and-apply for a write executeToolCall refused
		api.POST("/tools/apply", s.handleApplyTool)
	}

	// HTMX specific routes
	htmx := s.router.Group("/htmx")
	{
		htmx.POST("/chat", s.handleHTMXChat)
		htmx.GET("/models", s.handleHTMXModels)
		htmx.GET("/history", s.handleHTMXHistory)

		// Session rail: list/create/load. "sessions" GET is an alias for
		// "history" (same handler) so the route names match the design plan
		// while keeping one implementation.
		htmx.GET("/sessions", s.handleHTMXHistory)
		htmx.POST("/sessions", s.handleCreateSession)
		htmx.GET("/sessions/:id", s.handleGetSessionMessages)
	}
}

// Router returns the Gin router
func (s *Server) Router() *gin.Engine {
	return s.router
}

// handleIndex serves the main chat interface
func (s *Server) handleIndex(c *gin.Context) {
	models := s.llmClient.GetAvailableModels()
	if len(models) == 0 {
		s.logger.Warn("No models available, using default model")
		// Use the appropriate default model based on the provider
		if s.config.Provider == "python" {
			models = []string{s.config.Mistral.DefaultModel}
		} else {
			models = []string{s.config.LLM.DefaultModel}
		}
	}

	// Use the appropriate default model based on the provider
	defaultModel := s.config.LLM.DefaultModel
	if s.config.Provider == "python" {
		defaultModel = s.config.Mistral.DefaultModel
	}

	// Tool count for the empty-state lede. Peek at an already-connected MCP
	// client rather than calling getMcpAviClient, which would lazily spawn
	// the subprocess just to render the homepage.
	s.mcpAviClientMu.Lock()
	mcpClient := s.mcpAviClient
	s.mcpAviClientMu.Unlock()
	toolCount := len(llm.GetAviToolDefinitions())
	if mcpClient != nil {
		if n := len(mcpClient.Tools()); n > 0 {
			toolCount = n
		}
	}

	tenant := s.config.Avi.Tenant
	if tenant == "" {
		tenant = "admin"
	}

	sessions, err := s.sessionStore.List()
	if err != nil {
		s.logger.Error("Failed to list sessions", zap.Error(err))
	}
	activeSessionID, _ := c.Cookie(activeSessionCookie)
	activeSessionMode := sessionModeReadOnly
	if activeSessionID != "" {
		if activeSession, err := s.sessionStore.Get(activeSessionID); err == nil && activeSession.Mode == sessionModeReadWrite {
			activeSessionMode = sessionModeReadWrite
		}
	}

	c.HTML(http.StatusOK, "index.html", gin.H{
		"title":             "VMware Avi LLM Agent",
		"models":            models,
		"defaultModel":      defaultModel,
		"version":           s.version,
		"buildDate":         s.buildDate,
		"tenant":            tenant,
		"toolCount":         toolCount,
		"sessions":          sessions,
		"activeSessionID":   activeSessionID,
		"activeSessionMode": activeSessionMode,
	})
}

// handleChat handles chat API requests
func (s *Server) handleChat(c *gin.Context) {
	var request struct {
		Message string `json:"message" binding:"required"`
		Model   string `json:"model"`
		Session string `json:"session"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Broadcast operation start to SSE clients
	s.broadcastOperationLog("info", "Starting chat processing", map[string]interface{}{
		"operation": "chat_request",
		"model": request.Model,
		"message_length": len(request.Message),
	})

	// Set default model if not specified
	if request.Model == "" {
		// Use the appropriate default model based on the provider
		if s.config.Provider == "python" {
			request.Model = s.config.Mistral.DefaultModel
		} else {
			request.Model = s.config.LLM.DefaultModel
		}
		s.broadcastOperationLog("info", "Using default model", map[string]interface{}{
			"default_model": request.Model,
			"provider":    s.config.Provider,
		})
	}

	// Broadcast model validation start
	s.broadcastOperationLog("info", "Validating model", map[string]interface{}{
		"model": request.Model,
	})

	// Validate model with longer timeout for Mistral API calls
	turnID := uuid.New().String()
	ctx, cancel := context.WithTimeout(contextWithTurn(c.Request.Context(), turnID), 60*time.Second)
	defer cancel()

	validModel, err := s.llmClient.ValidateModel(ctx, request.Model)
	if err != nil {
		s.logger.Error("Failed to validate model", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to validate model"})
		return
	}

	if !validModel {
		s.broadcastOperationLog("error", "Invalid model", map[string]interface{}{
			"model": request.Model,
			"error": "Model not available",
		})
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Model '%s' is not available", request.Model)})
		return
	}

	// Broadcast processing start
	s.broadcastOperationLog("info", "Processing chat message", map[string]interface{}{
		"model": request.Model,
		"message": request.Message,
	})

	// Process the chat message
	startTime := time.Now()
	s.broadcastOperationLog("info", "Starting chat message processing", map[string]interface{}{
		"message": request.Message,
		"model": request.Model,
	})
	
	response, objectCount, err := s.processChatMessage(ctx, request.Message, request.Model, nil)
	elapsedTime := time.Since(startTime)
	
	if err != nil {
		s.broadcastOperationLog("error", "Failed to process chat message", map[string]interface{}{
			"error": err.Error(),
		})
		s.broadcastOperationLog("error", "Chat processing failed", map[string]interface{}{
			"error": err.Error(),
			"duration_ms": elapsedTime.Milliseconds(),
		})
		
		// Enhanced error classification and specific error messages
		errorResponse := gin.H{
			"error": "Failed to process message",
			"type": "error",
			"timestamp": time.Now().Format(time.RFC3339),
			"context": gin.H{
				"operation": "processChatMessage",
				"model": request.Model,
				"input_length": len(request.Message),
				"duration_ms": elapsedTime.Milliseconds(),
			},
			"suggestions": []string{
				"Check your network connection",
				"Verify the selected model is available",
				"Try a simpler query",
				"If the problem persists, contact support",
			},
		}
		
		// Classify the error for better debugging
		errorMsg := err.Error()
		if strings.Contains(errorMsg, "context deadline exceeded") || strings.Contains(errorMsg, "timeout") {
			errorResponse["error"] = "Request timed out while processing"
			errorResponse["type"] = "timeout_error"
			s.broadcastOperationLog("error", "Timeout error in chat processing", map[string]interface{}{
				"error": err.Error(),
			})
		} else if strings.Contains(errorMsg, "invalid Mistral response") {
			errorResponse["error"] = "Invalid response from AI provider"
			errorResponse["type"] = "ai_response_error"
			s.broadcastOperationLog("error", "AI response validation failed", map[string]interface{}{
				"error": err.Error(),
			})
		} else if strings.Contains(errorMsg, "tool call") {
			errorResponse["error"] = "Failed to execute tool calls"
			errorResponse["type"] = "tool_execution_error"
			s.broadcastOperationLog("error", "Tool execution failed", map[string]interface{}{
				"error": err.Error(),
			})
		} else if strings.Contains(errorMsg, "Avi API") || strings.Contains(errorMsg, "avi controller") {
			errorResponse["error"] = "Avi controller communication failed"
			errorResponse["type"] = "avi_api_error"
			s.broadcastOperationLog("error", "Avi controller communication failed", map[string]interface{}{
				"error": err.Error(),
			})
		} else {
			s.broadcastOperationLog("error", "General chat processing error", map[string]interface{}{
				"error": err.Error(),
			})
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse)
		return
	}

	// Broadcast successful completion
	s.broadcastOperationLog("success", "Chat processing completed", map[string]interface{}{
		"duration_ms": elapsedTime.Milliseconds(),
		"model": request.Model,
	})

	// Enhanced response with metadata and performance info
	enhancedResponse := gin.H{
		"message": response.Message,
		"type": "success",
		"timestamp": time.Now().Format(time.RFC3339),
		"turn_id": turnID,
		"performance": gin.H{
			"processing_time_ms": elapsedTime.Milliseconds(),
			"model": request.Model,
			"objects_returned": objectCount,
		},
	}

	// Add tool call information if present
	if response.ToolCalls != nil && len(response.ToolCalls) > 0 {
		enhancedResponse["tool_calls"] = len(response.ToolCalls)
		enhancedResponse["help"] = "This response includes automated actions. Use 'show details' to see what operations were performed."
	}

	c.JSON(http.StatusOK, enhancedResponse)
}

// handleHTMXChat handles HTMX chat requests
func (s *Server) handleHTMXChat(c *gin.Context) {
	message := c.PostForm("message")
	model := c.PostForm("model")
	timestamp := time.Now().Format("15:04:05")

	turnID := uuid.New().String()

	// Log user request with headers and payload
	userHeaders := map[string]string{
		"User-Agent": c.Request.UserAgent(),
		"Content-Type": c.Request.Header.Get("Content-Type"),
		"Accept": c.Request.Header.Get("Accept"),
		"Referer": c.Request.Referer(),
	}

	userPayload := map[string]string{
		"message": message,
		"model": model,
	}

	s.logAPICall("user", "POST", "/chat", userHeaders, userPayload, map[string]interface{}{
		"timestamp": timestamp,
		"turn_id":   turnID,
	})

	if message == "" {
		s.broadcastOperationLog("warning", "Empty message received in HTMX chat request", nil)
		c.HTML(http.StatusBadRequest, "chat.html", gin.H{
			"error": "Message cannot be empty",
			"timestamp": timestamp,
		})
		return
	}

	if model == "" {
		// Use the appropriate default model based on the provider
		if s.config.Provider == "python" {
			model = s.config.Mistral.DefaultModel
		} else {
			model = s.config.LLM.DefaultModel
		}
		s.broadcastOperationLog("info", "Using default model", map[string]interface{}{
			"model": model,
			"provider": s.config.Provider,
		})
	}

	s.broadcastOperationLog("info", "Processing HTMX chat message", map[string]interface{}{
		"message": message,
		"model": model,
		"timestamp": timestamp,
	})

	session := s.getOrCreateActiveSession(c, model)
	sessionID := ""
	sessionMode := ""
	if session != nil {
		sessionID = session.ID
		sessionMode = session.Mode
	}

	// Process the chat message
	ctx := contextWithMode(contextWithTurn(c.Request.Context(), turnID), sessionMode)
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	turnStart := time.Now()
	response, objectCount, err := s.processChatMessage(ctx, message, model, nil)
	turnDuration := time.Since(turnStart)
	if err != nil {
		s.logger.Error("Failed to process chat message", zap.Error(err))
		c.HTML(http.StatusInternalServerError, "chat.html", gin.H{
			"error": "Failed to process message: " + err.Error(),
		})
		return
	}

	// Enhance response handling for tool calls
	assistantMessage := response.Message
	hasToolCalls := len(response.ToolCalls) > 0
	
	// If there are tool calls but no assistant message, provide a meaningful status message
	if hasToolCalls && assistantMessage == "" {
		assistantMessage = "Processing your request using API tools..."
		s.logger.Info("Generated status message for tool calls with empty content",
			zap.String("message", message),
			zap.Int("tool_calls", len(response.ToolCalls)))
	}
	
	// Check if the response contains tool execution errors
	hasToolErrors := strings.Contains(response.Message, "❌")

	if sessionID != "" {
		toolNames := make([]string, 0, len(response.ToolCalls))
		for _, tc := range response.ToolCalls {
			toolNames = append(toolNames, tc.Function.Name)
		}
		now := time.Now()
		if err := s.sessionStore.AppendMessage(sessionID, ChatMessage{
			ID: uuid.New().String(), Role: "user", Content: message, Timestamp: now,
		}); err != nil {
			s.logger.Error("Failed to persist user message", zap.Error(err))
		}
		if err := s.sessionStore.AppendMessage(sessionID, ChatMessage{
			ID: uuid.New().String(), Role: "assistant", Content: assistantMessage,
			Timestamp: now, Model: response.Model, ToolCalls: toolNames,
		}); err != nil {
			s.logger.Error("Failed to persist assistant message", zap.Error(err))
		}
	}

	// Render the response as HTML
	s.broadcastOperationLog("info", "HTMX response ready", map[string]interface{}{
		"message": message,
		"assistant_message": assistantMessage,
		"tool_calls": len(response.ToolCalls),
		"response_length": len(assistantMessage),
		"duration_ms": turnDuration.Milliseconds(),
		"turn_id":     turnID,
	})

	if session != nil {
		c.Header("X-Session-Id", session.ID)
		c.Header("X-Session-Mode", session.Mode)
	}

	c.HTML(http.StatusOK, "chat.html", gin.H{
		"userMessage":      message,
		"assistantMessage": assistantMessage,
		"model":           response.Model,
		"toolCalls":       response.ToolCalls,
		"hasToolCalls":    hasToolCalls,
		"hasToolErrors":   hasToolErrors,
		"timestamp":       time.Now().Format("15:04:05"),
		"showDebug":       true,
		"turnID":          turnID,
		"toolCallCount":   len(response.ToolCalls),
		"durationMs":      turnDuration.Milliseconds(),
		"objectCount":     objectCount,
	})

	s.broadcastOperationLog("success", "HTMX chat completed successfully", map[string]interface{}{
		"message": message,
		"response_length": len(assistantMessage),
		"tool_calls": len(response.ToolCalls),
		"duration_ms": turnDuration.Milliseconds(),
		"turn_id":     turnID,
	})
}

// countResultObjects estimates how many Avi objects a tool result represents,
// for the per-turn trace summary chip: an Avi list envelope's `results` array,
// a bare JSON array, or else a single object.
func countResultObjects(result interface{}) int {
	switch v := result.(type) {
	case map[string]interface{}:
		if results, ok := v["results"].([]interface{}); ok {
			return len(results)
		}
		return 1
	case []interface{}:
		return len(v)
	default:
		return 1
	}
}

// processChatMessage processes a chat message and returns a response along
// with the number of Avi objects its tool calls returned (for the trace
// summary chip). turn_id is read off ctx (see contextWithTurn) and stamped
// onto every log entry emitted here so the inspector can correlate them.
func (s *Server) processChatMessage(ctx context.Context, message, model string, history []llm.ChatMessage) (*llm.LLMResponse, int, error) {
	turnID := turnIDFromContext(ctx)

	// Convert history to the appropriate type based on provider
	var convertedHistory interface{}
	if s.config.Provider == "ollama" {
		convertedHistory = history
	}

	// Get tool definitions: prefer the comprehensive Avi MCP tool set, falling
	// back to the static Go tool set if MCP is disabled or unreachable.
	var tools interface{}
	if s.config.Provider == "ollama" || s.config.Provider == "python" {
		if mcpClient, err := s.getMcpAviClient(ctx); err == nil {
			tools = mcpClient.Tools()
		} else {
			s.broadcastOperationLog("warning", "Avi MCP client unavailable, using built-in tool set", map[string]interface{}{
				"error":   err.Error(),
				"turn_id": turnID,
			})
			tools = llm.GetAviToolDefinitions()
		}
	}

	// Process the message with the appropriate LLM client
	s.logAPICall("mistral", "POST", "/chat/completions", nil, map[string]interface{}{
		"model":   model,
		"message": message,
	}, map[string]interface{}{
		"step":    "plan",
		"turn_id": turnID,
	})
	llmStart := time.Now()
	var err error
	llmResponse, err := s.llmClient.ProcessNaturalLanguageQuery(ctx, message, model, tools, convertedHistory)
	llmDuration := time.Since(llmStart)
	if err != nil {
		s.logAPIResponse("mistral", "POST", "/chat/completions", 0, nil, nil, map[string]interface{}{
			"step":        "plan",
			"model":       model,
			"duration_ms": llmDuration.Milliseconds(),
			"error":       err.Error(),
			"turn_id":     turnID,
		})
		if s.config.Provider == "ollama" {
			return nil, 0, fmt.Errorf("Ollama LLM processing failed: %w", err)
		}
		return nil, 0, fmt.Errorf("LLM processing failed: %w", err)
	}
	s.logAPIResponse("mistral", "POST", "/chat/completions", 200, nil, nil, map[string]interface{}{
		"step":                "plan",
		"model":               model,
		"duration_ms":         llmDuration.Milliseconds(),
		"tool_calls_selected": len(llmResponse.ToolCalls),
		"turn_id":             turnID,
	})

	objectCount := 0

	// If there are tool calls, execute them
	if len(llmResponse.ToolCalls) > 0 {
		s.broadcastOperationLog("info", "Executing tool calls", map[string]interface{}{
			"tool_call_count": len(llmResponse.ToolCalls),
			"turn_id":         turnID,
		})

		var toolResults []string
		var toolErrors []string

		for i, toolCall := range llmResponse.ToolCalls {
			s.broadcastOperationLog("info", "Executing tool call", map[string]interface{}{
				"tool_call_index": i,
				"tool_name": toolCall.Function.Name,
				"arguments": toolCall.Function.Arguments,
				"turn_id":   turnID,
			})

			toolStart := time.Now()
			result, err := s.executeToolCall(ctx, toolCall)
			toolDuration := time.Since(toolStart)
			if refusal, ok := err.(*writeRefusedError); ok {
				cardJSON, marshalErr := json.Marshal(map[string]interface{}{
					"tool":   refusal.Tool,
					"method": refusal.Method,
					"path":   refusal.Path,
					"body":   refusal.Body,
					"args":   refusal.Args,
				})
				if marshalErr != nil {
					cardJSON = []byte("{}")
				}
				llmResponse.Message += fmt.Sprintf("\n\nWRITE BLOCKED (%s):\n```json\n%s\n```", refusal.Tool, cardJSON)
				s.broadcastOperationLog("warning", "Write blocked by read-only mode", map[string]interface{}{
					"tool": refusal.Tool, "method": refusal.Method, "path": refusal.Path,
					"duration_ms": toolDuration.Milliseconds(), "turn_id": turnID,
				})
				continue
			}
			if err != nil {
				errorMsg := fmt.Sprintf("Tool call failed: %s - Error: %v", toolCall.Function.Name, err)
				toolErrors = append(toolErrors, errorMsg)
				s.broadcastOperationLog("error", "Tool call failed", map[string]interface{}{
					"tool": toolCall.Function.Name,
					"error": err.Error(),
					"duration_ms": toolDuration.Milliseconds(),
					"turn_id":     turnID,
				})
				// Continue with other tool calls even if one fails
				continue
			}

			// Add the result to the response message
			if result != nil {
				objectCount += countResultObjects(result)
				resultStr := fmt.Sprintf("Tool call result: %s - Success", toolCall.Function.Name)
				toolResults = append(toolResults, resultStr)
				resultJSON, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					resultJSON = []byte(fmt.Sprintf("%v", result))
				}
				llmResponse.Message += fmt.Sprintf("\n\nAPI Result (%s):\n```json\n%s\n```", toolCall.Function.Name, resultJSON)
				s.broadcastOperationLog("success", "Tool call succeeded", map[string]interface{}{
					"tool": toolCall.Function.Name,
					"result": result,
					"duration_ms": toolDuration.Milliseconds(),
					"turn_id":     turnID,
				})
			} else {
				s.broadcastOperationLog("warning", "Tool call returned empty result", map[string]interface{}{
					"tool": toolCall.Function.Name,
					"duration_ms": toolDuration.Milliseconds(),
					"turn_id":     turnID,
				})
			}
		}

		// Add summary to response
		if len(toolResults) > 0 || len(toolErrors) > 0 {
			llmResponse.Message += "\n\n=== Tool Execution Summary ==="
			for _, result := range toolResults {
				llmResponse.Message += fmt.Sprintf("\n✅ %s", result)
			}
			for _, error := range toolErrors {
				llmResponse.Message += fmt.Sprintf("\n❌ %s", error)
			}
		}
	}

	return llmResponse, objectCount, nil
}

// executeToolCall is the guarded entry point every LLM-initiated tool call
// goes through: a write attempted on a read-only session (the default, see
// sessionModeReadOnly) is refused before dispatch, returning a
// *writeRefusedError the caller renders as a confirmation card instead of a
// result. /api/tools/apply (an explicit, user-initiated unlock) calls
// dispatchToolCall directly, bypassing this guard on purpose.
func (s *Server) executeToolCall(ctx context.Context, toolCall llm.ToolCall) (interface{}, error) {
	if isReadOnlyMode(modeFromContext(ctx)) && isWriteTool(toolCall) {
		method, path, body := describeWrite(toolCall)
		return nil, &writeRefusedError{
			Tool: toolCall.Function.Name, Method: method, Path: path, Body: body, Args: toolCall.Args,
		}
	}
	return s.dispatchToolCall(ctx, toolCall)
}

// isWriteTool classifies a tool call as a write needing read-write mode.
// Defaults to true (write) whenever it can't prove otherwise -- for an
// unrecognized tool name, and for execute_generic_operation/avi_action whose
// method is an argument rather than part of the name -- because a false
// refusal costs one Unlock click, but a false allow bypasses the whole gate.
func isWriteTool(toolCall llm.ToolCall) bool {
	switch toolCall.Function.Name {
	case "avi_list", "avi_get", "avi_get_analytics", "avi_list_object_types",
		"list_virtual_services", "get_virtual_service", "list_pools", "get_pool",
		"list_health_monitors", "get_health_monitor", "list_service_engines",
		"get_service_engine", "get_analytics":
		return false
	case "avi_create", "avi_update", "avi_patch", "avi_delete", "avi_action",
		"create_virtual_service", "update_virtual_service", "delete_virtual_service",
		"create_pool", "scale_out_pool", "scale_in_pool":
		return true
	case "execute_generic_operation":
		method, ok := toolCall.Args["method"].(string)
		return !ok || !strings.EqualFold(method, "GET")
	default:
		return true
	}
}

// describeWrite reconstructs the Avi Controller request a write tool call
// would have made, for display on the refusal confirmation card. Mirrors the
// REST conventions both dispatchToolCall's switch and the Avi MCP server's
// generic CRUD tools already follow (see CLAUDE.md's Avi MCP server section).
func describeWrite(toolCall llm.ToolCall) (method, path string, body interface{}) {
	args := toolCall.Args
	objectPath := "/api/" + strings.ToLower(fmt.Sprint(args["object_type"]))
	uuid, _ := args["uuid"].(string)

	switch toolCall.Function.Name {
	case "avi_create":
		return "POST", objectPath, args["body"]
	case "avi_update":
		return "PUT", objectPath + "/" + uuid, args["body"]
	case "avi_patch":
		return "PATCH", objectPath + "/" + uuid, args["body"]
	case "avi_delete":
		return "DELETE", objectPath + "/" + uuid, nil
	case "avi_action":
		action, _ := args["action"].(string)
		method, _ := args["method"].(string)
		if method == "" {
			method = "POST"
		}
		if uuid != "" {
			return method, objectPath + "/" + uuid + "/" + action, args["body"]
		}
		return method, objectPath + "/" + action, args["body"]
	case "create_virtual_service":
		return "POST", "/api/virtualservice", args
	case "update_virtual_service":
		return "PUT", "/api/virtualservice/" + uuid, args
	case "delete_virtual_service":
		return "DELETE", "/api/virtualservice/" + uuid, nil
	case "create_pool":
		return "POST", "/api/pool", args
	case "scale_out_pool":
		return "POST", "/api/pool/" + uuid + "/scaleout", args
	case "scale_in_pool":
		return "POST", "/api/pool/" + uuid + "/scalein", args
	case "execute_generic_operation":
		m, _ := args["method"].(string)
		if m == "" {
			m = "POST"
		}
		p, _ := args["endpoint"].(string)
		return m, p, args["body"]
	default:
		return "POST", objectPath, args
	}
}

// writeRefusedError is returned by executeToolCall in place of dispatching a
// write on a read-only session. Args carries the tool call's original
// arguments (not just the human-readable Method/Path/Body) so the
// confirmation card's "Unlock and apply" button can replay it byte-for-byte
// through /api/tools/apply.
type writeRefusedError struct {
	Tool   string
	Method string
	Path   string
	Body   interface{}
	Args   map[string]interface{}
}

func (e *writeRefusedError) Error() string {
	return fmt.Sprintf("refused to run %s (%s %s) in read-only mode", e.Tool, e.Method, e.Path)
}

// dispatchToolCall executes a tool call against the Avi API, unconditionally
// -- callers needing the read-only gate use executeToolCall instead. Tool
// calls whose name matches a tool advertised by the Avi MCP client are
// routed there; everything else falls back to the built-in Go dispatch below
// (the static tool set from internal/llm/tools.go, used when MCP is
// unavailable).
func (s *Server) dispatchToolCall(ctx context.Context, toolCall llm.ToolCall) (interface{}, error) {
	mcpClient, mcpErr := s.getMcpAviClient(ctx)
	if mcpErr == nil && mcpClient.HasTool(toolCall.Function.Name) {
		return mcpClient.CallTool(ctx, toolCall.Function.Name, toolCall.Args)
	}
	if strings.HasPrefix(toolCall.Function.Name, "avi_") {
		// This name only ever comes from the MCP tool list (the static
		// fallback tools use bare names like list_virtual_services), so if we
		// get here the MCP client is down -- say so instead of falling into
		// "unknown tool" below, which the model can't self-correct from.
		return nil, fmt.Errorf("avi mcp tool %s is unavailable: %w", toolCall.Function.Name, mcpErr)
	}

	// Get Avi client (lazy initialization)
	aviClient, err := s.getAviClient()
	if err != nil {
		return nil, fmt.Errorf("Avi client not available: %w", err)
	}

	switch toolCall.Function.Name {
	case "list_virtual_services":
		params := make(map[string]string)
		if toolCall.Args != nil {
			for key, value := range toolCall.Args {
				if str, ok := value.(string); ok {
					params[key] = str
				}
			}
		}
		return aviClient.ListVirtualServices(ctx, params)

	case "get_virtual_service":
		uuid, ok := toolCall.Args["uuid"].(string)
		if !ok {
			return nil, fmt.Errorf("uuid parameter required")
		}
		params := make(map[string]string)
		if fields, ok := toolCall.Args["fields"].(string); ok {
			params["fields"] = fields
		}
		return aviClient.GetVirtualService(ctx, uuid, params)

	case "create_virtual_service":
		return aviClient.CreateVirtualService(ctx, toolCall.Args)

	case "update_virtual_service":
		uuid, ok := toolCall.Args["uuid"].(string)
		if !ok {
			return nil, fmt.Errorf("uuid parameter required")
		}
		delete(toolCall.Args, "uuid") // Remove UUID from the data
		return aviClient.UpdateVirtualService(ctx, uuid, toolCall.Args)

	case "delete_virtual_service":
		uuid, ok := toolCall.Args["uuid"].(string)
		if !ok {
			return nil, fmt.Errorf("uuid parameter required")
		}
		return nil, aviClient.DeleteVirtualService(ctx, uuid)

	case "list_pools":
		params := make(map[string]string)
		if toolCall.Args != nil {
			for key, value := range toolCall.Args {
				if str, ok := value.(string); ok {
					params[key] = str
				}
			}
		}
		return aviClient.ListPools(ctx, params)

	case "get_pool":
		uuid, ok := toolCall.Args["uuid"].(string)
		if !ok {
			return nil, fmt.Errorf("uuid parameter required")
		}
		params := make(map[string]string)
		if fields, ok := toolCall.Args["fields"].(string); ok {
			params["fields"] = fields
		}
		return aviClient.GetPool(ctx, uuid, params)

	case "create_pool":
		return aviClient.CreatePool(ctx, toolCall.Args)

	case "scale_out_pool":
		uuid, ok := toolCall.Args["uuid"].(string)
		if !ok {
			return nil, fmt.Errorf("uuid parameter required")
		}
		delete(toolCall.Args, "uuid") // Remove UUID from the parameters
		return nil, aviClient.ScaleOutPool(ctx, uuid, toolCall.Args)

	case "scale_in_pool":
		uuid, ok := toolCall.Args["uuid"].(string)
		if !ok {
			return nil, fmt.Errorf("uuid parameter required")
		}
		delete(toolCall.Args, "uuid") // Remove UUID from the parameters
		return nil, aviClient.ScaleInPool(ctx, uuid, toolCall.Args)

	case "list_health_monitors":
		params := make(map[string]string)
		if toolCall.Args != nil {
			for key, value := range toolCall.Args {
				if str, ok := value.(string); ok {
					params[key] = str
				}
			}
		}
		return aviClient.ListHealthMonitors(ctx, params)

	case "get_health_monitor":
		uuid, ok := toolCall.Args["uuid"].(string)
		if !ok {
			return nil, fmt.Errorf("uuid parameter required")
		}
		params := make(map[string]string)
		if fields, ok := toolCall.Args["fields"].(string); ok {
			params["fields"] = fields
		}
		return aviClient.GetHealthMonitor(ctx, uuid, params)

	case "list_service_engines":
		params := make(map[string]string)
		if toolCall.Args != nil {
			for key, value := range toolCall.Args {
				if str, ok := value.(string); ok {
					params[key] = str
				}
			}
		}
		return aviClient.ListServiceEngines(ctx, params)

	case "get_service_engine":
		uuid, ok := toolCall.Args["uuid"].(string)
		if !ok {
			return nil, fmt.Errorf("uuid parameter required")
		}
		params := make(map[string]string)
		if fields, ok := toolCall.Args["fields"].(string); ok {
			params["fields"] = fields
		}
		return aviClient.GetServiceEngine(ctx, uuid, params)

	case "get_analytics":
		resourceType, ok := toolCall.Args["resource_type"].(string)
		if !ok {
			return nil, fmt.Errorf("resource_type parameter required")
		}
		uuid, ok := toolCall.Args["uuid"].(string)
		if !ok {
			return nil, fmt.Errorf("uuid parameter required")
		}
		params := make(map[string]string)
		if metric, ok := toolCall.Args["metric"].(string); ok {
			params["metric"] = metric
		}
		if timeRange, ok := toolCall.Args["time_range"].(string); ok {
			params["time_range"] = timeRange
		}
		return aviClient.GetAnalytics(ctx, resourceType, uuid, params)

	case "execute_generic_operation":
		method, ok := toolCall.Args["method"].(string)
		if !ok {
			return nil, fmt.Errorf("method parameter required")
		}
		endpoint, ok := toolCall.Args["endpoint"].(string)
		if !ok {
			return nil, fmt.Errorf("endpoint parameter required")
		}

		var body interface{}
		if b, exists := toolCall.Args["body"]; exists {
			body = b
		}

		params := make(map[string]string)
		if p, exists := toolCall.Args["parameters"].(map[string]interface{}); exists {
			for key, value := range p {
				if str, ok := value.(string); ok {
					params[key] = str
				}
			}
		}

		return aviClient.ExecuteGenericOperation(ctx, method, endpoint, body, params)

	default:
		return nil, fmt.Errorf("unknown tool: %s", toolCall.Function.Name)
	}
}

// handleGetModels returns available models
func (s *Server) handleGetModels(c *gin.Context) {
	var models []string
	var defaultModel string

	if s.config.Provider == "ollama" {
		ollamaClient := s.llmClient.(*llm.Client)
		models = ollamaClient.GetAvailableModels()
		defaultModel = s.config.LLM.DefaultModel
	} else if s.config.Provider == "python" {
		models = s.config.Mistral.Models
		defaultModel = s.config.Mistral.DefaultModel
	}

	c.JSON(http.StatusOK, gin.H{
		"models": models,
		"default": defaultModel,
		"provider": s.config.Provider,
	})
}

// handleHTMXModels returns models for HTMX
func (s *Server) handleHTMXModels(c *gin.Context) {
	var models []string
	var defaultModel string

	if s.config.Provider == "ollama" {
		ollamaClient := s.llmClient.(*llm.Client)
		models = ollamaClient.GetAvailableModels()
		defaultModel = s.config.LLM.DefaultModel
	} else if s.config.Provider == "python" {
		models = s.config.Mistral.Models
		defaultModel = s.config.Mistral.DefaultModel
	}

	c.HTML(http.StatusOK, "models.html", gin.H{
		"models": models,
		"default": defaultModel,
		"provider": s.config.Provider,
	})
}

// handleValidateModel validates a model
func (s *Server) handleValidateModel(c *gin.Context) {
	var request struct {
		Model string `json:"model" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	var valid bool
	var err error

	if s.config.Provider == "ollama" {
		ollamaClient := s.llmClient.(*llm.Client)
		valid, err = ollamaClient.ValidateModel(ctx, request.Model)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"valid": valid})
}

// handleChatHistory returns the session list as JSON, for API consumers.
func (s *Server) handleChatHistory(c *gin.Context) {
	sessions, err := s.sessionStore.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sessions": sessions})
}

// handleHTMXHistory renders the session rail's list of sessions. It also
// backs the "GET /htmx/sessions" alias (see setupRoutes) and is what the
// rail's JS re-fetches after creating or deleting a session.
func (s *Server) handleHTMXHistory(c *gin.Context) {
	sessions, err := s.sessionStore.List()
	if err != nil {
		s.logger.Error("Failed to list sessions", zap.Error(err))
		sessions = nil
	}
	activeSessionID, _ := c.Cookie(activeSessionCookie)
	c.HTML(http.StatusOK, "history.html", gin.H{
		"sessions":        sessions,
		"activeSessionID": activeSessionID,
	})
}

// handleClearHistory deletes every stored session (there is no per-session
// delete here; see handleDeleteSession for that).
func (s *Server) handleClearHistory(c *gin.Context) {
	if err := s.sessionStore.DeleteAll(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	clearActiveSessionCookie(c)
	c.JSON(http.StatusOK, gin.H{"message": "History cleared"})
}

// handleCreateSession starts a new, empty session (the rail's "New session"
// button), makes it the active session via cookie, and returns the refreshed
// session list so the caller can swap it straight into the rail.
func (s *Server) handleCreateSession(c *gin.Context) {
	defaultModel := s.config.LLM.DefaultModel
	if s.config.Provider == "python" {
		defaultModel = s.config.Mistral.DefaultModel
	}

	session, err := s.sessionStore.Create(defaultModel)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	setActiveSessionCookie(c, session.ID)
	c.Header("X-Session-Id", session.ID)
	c.Header("X-Session-Mode", session.Mode)

	sessions, err := s.sessionStore.List()
	if err != nil {
		s.logger.Error("Failed to list sessions", zap.Error(err))
		sessions = nil
	}
	c.HTML(http.StatusOK, "history.html", gin.H{
		"sessions":        sessions,
		"activeSessionID": session.ID,
	})
}

// handleGetSessionMessages loads one session's full history and renders it
// for the conversation pane (the rail item's click target), making it the
// active session.
func (s *Server) handleGetSessionMessages(c *gin.Context) {
	id := c.Param("id")
	session, err := s.sessionStore.Get(id)
	if err != nil {
		c.String(http.StatusNotFound, "Session not found")
		return
	}
	setActiveSessionCookie(c, id)
	c.Header("X-Session-Id", session.ID)
	c.Header("X-Session-Mode", session.Mode)
	c.HTML(http.StatusOK, "session_messages.html", gin.H{
		"messages": session.Messages,
	})
}

// handleDeleteSession deletes one session. Called via a plain fetch from the
// rail's delete button (see initializeSessionRail in app.js), not htmx, so it
// stays under /api and returns JSON rather than a page fragment.
func (s *Server) handleDeleteSession(c *gin.Context) {
	id := c.Param("id")
	if err := s.sessionStore.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if activeID, _ := c.Cookie(activeSessionCookie); activeID == id {
		clearActiveSessionCookie(c)
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// handleSetSessionMode flips a session between read-only (the default) and
// read-write. Called both by the composer's segmented control (a proactive,
// user-initiated unlock) and by a write-refusal card's "Unlock and apply"
// button (which calls this, then /api/tools/apply, in sequence).
func (s *Server) handleSetSessionMode(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Mode string `json:"mode" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Mode != sessionModeReadOnly && req.Mode != sessionModeReadWrite {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode must be read-only or read-write"})
		return
	}
	if err := s.sessionStore.SetMode(id, req.Mode); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"mode": req.Mode})
}

// handleApplyTool re-dispatches a tool call that executeToolCall previously
// refused, after the user has explicitly unlocked the session. Re-checks
// mode server-side (never trusts the client's word that it called
// handleSetSessionMode first) and calls dispatchToolCall directly, bypassing
// the read-only gate on purpose -- this endpoint IS the unlock path.
func (s *Server) handleApplyTool(c *gin.Context) {
	var req struct {
		Tool string                 `json:"tool" binding:"required"`
		Args map[string]interface{} `json:"args"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sessionID, _ := c.Cookie(activeSessionCookie)
	session, err := s.sessionStore.Get(sessionID)
	if err != nil || isReadOnlyMode(session.Mode) {
		c.JSON(http.StatusForbidden, gin.H{"error": "session is read-only; unlock it first"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	result, err := s.dispatchToolCall(ctx, llm.ToolCall{
		Function: llm.ToolCallFunction{Name: req.Tool},
		Args:     req.Args,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": result})
}

// handleHealth returns health status
func (s *Server) handleHealth(c *gin.Context) {
	s.broadcastOperationLog("info", "Health check requested", map[string]interface{}{
		"endpoint": "/health",
		"app_name": s.appName,
		"version": s.version,
	})
	
	ctx := c.Request.Context()
	
	status := gin.H{
		"status": "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"provider": s.config.Provider,
		"version": s.version,
		"build_date": s.buildDate,
		"app_name": s.appName,
	}

	// Check Avi client status
	s.aviClientMu.Lock()
	aviClientAvailable := s.aviClient != nil
	aviClientError := s.aviClientErr
	s.aviClientMu.Unlock()

	if aviClientAvailable {
		status["avi_status"] = "initialized"

		// Quick health check if this isn't a frequent probe
		if !s.isHealthCheckProbe(c) {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
			defer cancel()

			// Use lazy Avi client initialization
			aviClient, err := s.getAviClient()
			if err != nil {
				status["avi_status"] = "unhealthy"
				status["avi_error"] = err.Error()
			} else if _, err := aviClient.ListVirtualServices(ctx, map[string]string{"limit_by": "1"}); err != nil {
				status["avi_status"] = "unhealthy"
				status["avi_error"] = err.Error()
			} else {
				status["avi_status"] = "healthy"
			}
		} else {
			status["avi_status"] = "skipped" // Skip expensive check for probes
		}
	} else {
		status["avi_status"] = "initializing"
		if aviClientError != nil {
			status["avi_error"] = aviClientError.Error()
		}
	}

	// Check LLM connection based on provider, but only if it's not a frequent health check
	if !s.isHealthCheckProbe(c) {
		
		if s.config.Provider == "ollama" {
			ollamaClient := s.llmClient.(*llm.Client)
			if _, err := ollamaClient.ListModels(ctx); err != nil {
				status["llm_status"] = "unhealthy"
				status["llm_error"] = err.Error()
			} else {
				status["llm_status"] = "healthy"
			}
		} else if s.config.Provider == "python" {
			pythonBridge := s.llmClient.(*python.PythonBridge)
			if initialized, _ := pythonBridge.GetStatus()["initialized"].(bool); initialized {
				status["llm_status"] = "healthy"
			} else {
				status["llm_status"] = "unhealthy"
			}
		}
	} else {
		// For known health check probes, skip LLM check to reduce API calls
		status["llm_status"] = "skipped"
	}

	// Update overall status based on component health
	s.logger.Info("DEBUG: Starting status update logic", 
		zap.Any("current_status", status["status"]),
		zap.Any("avi_status", status["avi_status"]),
		zap.Any("llm_status", status["llm_status"]))
	
	isHealthy := true
	if aviStatus, exists := status["avi_status"]; exists && aviStatus == "unhealthy" {
		isHealthy = false
		s.logger.Info("DEBUG: Avi status is unhealthy, setting overall status to degraded")
	}
	if llmStatus, exists := status["llm_status"]; exists && llmStatus == "unhealthy" {
		isHealthy = false
		s.logger.Info("DEBUG: LLM status is unhealthy, setting overall status to degraded")
	}
	
	s.logger.Info("DEBUG: Final status decision", zap.Bool("isHealthy", isHealthy))
	
	if isHealthy {
		status["status"] = "healthy"
		s.logger.Info("DEBUG: Setting overall status to healthy")
	} else {
		status["status"] = "degraded"
		s.logger.Info("DEBUG: Setting overall status to degraded")
	}
	
	c.JSON(http.StatusOK, status)
}

// handleOperationEvents provides Server-Sent Events for real-time operation logging
func (s *Server) handleOperationEvents(c *gin.Context) {
	// Set headers for SSE
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")

	// Create a channel to receive operation logs
	logChannel := make(chan map[string]interface{})
	
	// Register this client with the server
	clientID := fmt.Sprintf("client-%d", time.Now().UnixNano())
	s.operationLogClients[clientID] = logChannel
	
	// Remove client when connection closes
	defer func() {
		delete(s.operationLogClients, clientID)
		close(logChannel)
		s.logger.Info("SSE client disconnected", zap.String("client_id", clientID))
	}()

	// Send welcome message
	welcomeMessage := fmt.Sprintf("data: %s\n\n", toJSON(map[string]interface{}{
		"type": "system",
		"message": "Connected to real-time operation logs",
		"timestamp": time.Now().Format(time.RFC3339),
	}))
	c.Writer.Write([]byte(welcomeMessage))
	c.Writer.Flush()

	// Stream operation logs to client
	for logEntry := range logChannel {
		logEntry["timestamp"] = time.Now().Format(time.RFC3339)
		logMessage := fmt.Sprintf("data: %s\n\n", toJSON(logEntry))
		if _, err := c.Writer.Write([]byte(logMessage)); err != nil {
			s.logger.Error("Failed to send SSE event", zap.Error(err))
			return
		}
		c.Writer.Flush()
	}
}

// handleGetLogs provides simple log streaming endpoint
func (s *Server) handleGetLogs(c *gin.Context) {
	// Debug: log buffer status
	s.logger.Info("handleGetLogs called", zap.Int("simple_log_buffer_size", len(s.simpleLogBuffer)))
	
	// Get logs from the simple log buffer (no locking needed)
	var logs []map[string]interface{}
	for _, logEntry := range s.simpleLogBuffer {
		// Create a copy to avoid race conditions
		logCopy := make(map[string]interface{})
		for key, value := range logEntry {
			logCopy[key] = value
		}
		logs = append(logs, logCopy)
	}
	
	// Return logs as JSON
	c.JSON(http.StatusOK, logs)
}

// handleEnhancedLogs provides filtered log retrieval and SSE streaming
func (s *Server) handleEnhancedLogs(c *gin.Context) {
	// Get filter parameters
	logType := c.Query("type")
	level := c.Query("level")
	search := c.Query("search")
	turn := c.Query("turn")
	limit := 1000 // Default limit

	if limitParam := c.Query("limit"); limitParam != "" {
		if parsedLimit, err := strconv.Atoi(limitParam); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	// Check if this is an SSE request
	if c.GetHeader("Accept") == "text/event-stream" {
		// SSE streaming mode
		s.handleEnhancedLogsSSE(c, logType, level, search, turn)
		return
	}

	// Regular HTTP mode - return filtered logs
	entries := s.enhancedLogBuffer.GetFilteredEntries(logType, level, search, turn)
	
	// Apply limit
	if len(entries) > limit {
		entries = entries[:limit]
	}

	// Return logs as JSON
	c.JSON(http.StatusOK, entries)
}

// handleEnhancedLogsSSE provides Server-Sent Events with filtering
func (s *Server) handleEnhancedLogsSSE(c *gin.Context, logType, level, search, turn string) {
	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	// Send existing logs that match filters
	entries := s.enhancedLogBuffer.GetFilteredEntries(logType, level, search, turn)
	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			s.logger.Error("Failed to marshal log entry for SSE", zap.Error(err))
			continue
		}
		fmt.Fprintf(c.Writer, "data: %s\n\n", string(data))
		c.Writer.Flush()
	}

	// Set up a channel to receive new log entries
	logChan := make(chan EnhancedLogEntry, 100)
	
	// Register this SSE connection to receive real-time log updates
	s.enhancedLogBuffer.AddSSEClient(logChan)
	defer s.enhancedLogBuffer.RemoveSSEClient(logChan)
	
	// Stream new log entries in real-time
	for {
		select {
		case newEntry := <-logChan:
			// Apply filters to new entries
			if s.matchesFilters(newEntry, logType, level, search, turn) {
				data, err := json.Marshal(newEntry)
				if err != nil {
					s.logger.Error("Failed to marshal new log entry for SSE", zap.Error(err))
					continue
				}
				fmt.Fprintf(c.Writer, "data: %s\n\n", string(data))
				c.Writer.Flush()
			}
		case <-c.Request.Context().Done():
			// Client disconnected
			return
		}
	}
}

// matchesFilters checks if a log entry matches the given filters
func (s *Server) matchesFilters(entry EnhancedLogEntry, logType, level, search, turn string) bool {
	// Type filter
	if logType != "" && logType != "all" && !strings.HasPrefix(entry.Type, logType) {
		return false
	}

	// Level filter
	if level != "" && level != "all" && entry.Level != level {
		return false
	}

	// Turn filter
	if turn != "" && entry.TurnID != turn {
		return false
	}

	// Search filter
	if search != "" {
		searchLower := strings.ToLower(search)
		messageMatch := strings.Contains(strings.ToLower(entry.Message), searchLower)
		contextMatch := false
		
		if entry.Context != nil {
			contextJSON, _ := json.Marshal(entry.Context)
			contextMatch = strings.Contains(strings.ToLower(string(contextJSON)), searchLower)
		}
		
		if !messageMatch && !contextMatch {
			return false
		}
	}

	return true
}

// removeLogClient removes a client channel from the operation log clients
func (s *Server) removeLogClient(clientChan chan map[string]interface{}) {
	s.operationLogMu.Lock()
	defer s.operationLogMu.Unlock()
	
	for clientID, clientChanEntry := range s.operationLogClients {
		if clientChanEntry == clientChan {
			delete(s.operationLogClients, clientID)
			close(clientChan)
			break
		}
	}
}

// StartLogPersistence starts a background goroutine to periodically save logs to file
func (s *Server) StartLogPersistence(logFile, archiveDir string, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute // Default interval
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Save current logs to file
				if err := s.enhancedLogBuffer.LogToFile(logFile); err != nil {
					s.logger.Error("Failed to save logs to file", zap.Error(err))
				}

				// Rotate logs if file gets too large
				if fileInfo, err := os.Stat(logFile); err == nil && fileInfo.Size() > 10*1024*1024 {
					// File is larger than 10MB, rotate it
					if err := RotateLogs(logFile, archiveDir); err != nil {
						s.logger.Error("Failed to rotate log file", zap.Error(err))
					}
				}

			case <-s.ShutdownContext.Done():
				// Server is shutting down, save final logs
				if err := s.enhancedLogBuffer.LogToFile(logFile); err != nil {
					s.logger.Error("Failed to save final logs on shutdown", zap.Error(err))
				}
				return
			}
		}
	}()
}

// handleAviProxy provides direct access to Avi API (for advanced users)
func (s *Server) handleAviProxy(c *gin.Context) {
	path := c.Param("path")
	method := c.Request.Method

	// Log Avi proxy request with proper typing
	aviHeaders := map[string]string{
		"User-Agent": c.Request.UserAgent(),
		"Content-Type": c.Request.Header.Get("Content-Type"),
		"Accept": c.Request.Header.Get("Accept"),
	}
	
	// Capture body if present
	var aviPayload interface{}
	if c.Request.Body != nil {
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err == nil {
			// Try to parse as JSON
			var jsonData interface{}
			if jsonErr := json.Unmarshal(bodyBytes, &jsonData); jsonErr == nil {
				aviPayload = jsonData
			} else {
				aviPayload = string(bodyBytes)
			}
			// Reset body for later reading
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}
	}
	
	s.logAPICall("avi", method, "/avi/"+path, aviHeaders, aviPayload, map[string]interface{}{
		"proxy": true,
	})

	// Parse parameters
	params := make(map[string]string)
	for key, values := range c.Request.URL.Query() {
		if len(values) > 0 {
			params[key] = values[0]
		}
	}

	// Get request body for POST/PUT/PATCH
	var body interface{}
	if method == "POST" || method == "PUT" || method == "PATCH" {
		if err := c.ShouldBindJSON(&body); err != nil && err != io.EOF {
			s.broadcastOperationLog("error", "Invalid request body", map[string]interface{}{
				"error": err.Error(),
			})
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	// Execute the operation with context (using lazy Avi client initialization)
	aviClient, err := s.getAviClient()
	if err != nil {
		s.broadcastOperationLog("error", "Avi client not available", map[string]interface{}{
			"error": err.Error(),
		})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Avi client not available: " + err.Error()})
		return
	}
	
	s.broadcastOperationLog("info", "Executing Avi operation", map[string]interface{}{
		"method": method,
		"path": path,
		"params": params,
	})
	
	result, err := aviClient.ExecuteGenericOperation(c.Request.Context(), method, path, body, params)
	if err != nil {
		s.broadcastOperationLog("error", "Avi operation failed", map[string]interface{}{
			"method": method,
			"path": path,
			"error": err.Error(),
		})
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.broadcastOperationLog("success", "Avi operation completed", map[string]interface{}{
		"method": method,
		"path": path,
	})
	
	c.JSON(http.StatusOK, result)
}

// corsMiddleware adds CORS headers
func (s *Server) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// Close closes the server and performs cleanup
func (s *Server) Close() error {
	s.aviClientMu.Lock()
	defer s.aviClientMu.Unlock()
	
	if s.aviClient != nil {
		return s.aviClient.Close()
	}
	return nil
}