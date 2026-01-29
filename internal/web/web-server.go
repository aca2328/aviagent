package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	ShutdownContext context.Context
	// Operation logging for real-time visibility
	operationLogClients map[string]chan map[string]interface{}
	operationLogBuffer  []map[string]interface{}
	operationLogMu      sync.Mutex
	// Simple log buffer for API endpoint (no locking needed for reads)
	simpleLogBuffer []map[string]interface{}
	enhancedLogBuffer *LogBuffer
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

func (b *LogBuffer) GetFilteredEntries(logType, level, search string) []EnhancedLogEntry {
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
		"split": strings.Split,
		"hasPrefix": strings.HasPrefix,
		"hasSuffix": strings.HasSuffix,
		"substr": func(s string, start int, length int) string {
			if start < 0 {
				start = 0
			}
			if start >= len(s) {
				return ""
			}
			end := start + length
			if end > len(s) {
				end = len(s)
			}
			return s[start:end]
		},
		"sub": func(s string, start int, length int) string {
			if start < 0 {
				start = 0
			}
			if start >= len(s) {
				return ""
			}
			end := start + length
			if end > len(s) {
				end = len(s)
			}
			return s[start:end]
		},
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
	}

	// HTMX specific routes
	htmx := s.router.Group("/htmx")
	{
		htmx.POST("/chat", s.handleHTMXChat)
		htmx.GET("/models", s.handleHTMXModels)
		htmx.GET("/history", s.handleHTMXHistory)
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

	c.HTML(http.StatusOK, "index.html", gin.H{
		"title":        "VMware Avi LLM Agent",
		"models":       models,
		"defaultModel": defaultModel,
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
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
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
	
	response, err := s.processChatMessage(ctx, request.Message, request.Model, nil)
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
		"performance": gin.H{
			"processing_time_ms": elapsedTime.Milliseconds(),
			"model": request.Model,
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

	// Process the chat message
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	response, err := s.processChatMessage(ctx, message, model, nil)
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
	
	// Render the response as HTML
	s.broadcastOperationLog("info", "HTMX response ready", map[string]interface{}{
		"message": message,
		"assistant_message": assistantMessage,
		"tool_calls": len(response.ToolCalls),
		"response_length": len(assistantMessage),
	})
	
	c.HTML(http.StatusOK, "chat.html", gin.H{
		"userMessage":      message,
		"assistantMessage": assistantMessage,
		"model":           response.Model,
		"toolCalls":       response.ToolCalls,
		"hasToolCalls":    hasToolCalls,
		"hasToolErrors":   hasToolErrors,
		"timestamp":       time.Now().Format("15:04:05"),
		"showDebug":       true,
	})
	
	s.broadcastOperationLog("success", "HTMX chat completed successfully", map[string]interface{}{
		"message": message,
		"response_length": len(assistantMessage),
		"tool_calls": len(response.ToolCalls),
	})
}

// processChatMessage processes a chat message and returns a response
func (s *Server) processChatMessage(ctx context.Context, message, model string, history []llm.ChatMessage) (*llm.LLMResponse, error) {
	// Convert history to the appropriate type based on provider
	var convertedHistory interface{}
	if s.config.Provider == "ollama" {
		convertedHistory = history
	}

	// Get tool definitions
	var tools interface{}
	if s.config.Provider == "ollama" {
		tools = llm.GetAviToolDefinitions()
	}

	// Process the message with the appropriate LLM client
	var err error
	llmResponse, err := s.llmClient.ProcessNaturalLanguageQuery(ctx, message, model, tools, convertedHistory)
	if err != nil {
		if s.config.Provider == "ollama" {
			return nil, fmt.Errorf("Ollama LLM processing failed: %w", err)
		}
		return nil, fmt.Errorf("LLM processing failed: %w", err)
	}

	// If there are tool calls, execute them
	if len(llmResponse.ToolCalls) > 0 {
		s.broadcastOperationLog("info", "Executing tool calls", map[string]interface{}{
			"tool_call_count": len(llmResponse.ToolCalls),
		})
		
		var toolResults []string
		var toolErrors []string
		
		for i, toolCall := range llmResponse.ToolCalls {
			s.broadcastOperationLog("info", "Executing tool call", map[string]interface{}{
				"tool_call_index": i,
				"tool_name": toolCall.Function.Name,
				"arguments": toolCall.Function.Arguments,
			})
			
			result, err := s.executeToolCall(ctx, toolCall)
			if err != nil {
				errorMsg := fmt.Sprintf("Tool call failed: %s - Error: %v", toolCall.Function.Name, err)
				toolErrors = append(toolErrors, errorMsg)
				s.broadcastOperationLog("error", "Tool call failed", map[string]interface{}{
					"tool": toolCall.Function.Name,
					"error": err.Error(),
				})
				// Continue with other tool calls even if one fails
				continue
			}

			// Add the result to the response message
			if result != nil {
				resultStr := fmt.Sprintf("Tool call result: %s - Success", toolCall.Function.Name)
				toolResults = append(toolResults, resultStr)
				llmResponse.Message += fmt.Sprintf("\n\nAPI Result:\n```json\n%v\n```", result)
				s.broadcastOperationLog("success", "Tool call succeeded", map[string]interface{}{
					"tool": toolCall.Function.Name,
					"result": result,
				})
			} else {
				s.broadcastOperationLog("warning", "Tool call returned empty result", map[string]interface{}{
					"tool": toolCall.Function.Name,
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

	return llmResponse, nil
}

// executeToolCall executes a tool call against the Avi API
func (s *Server) executeToolCall(ctx context.Context, toolCall llm.ToolCall) (interface{}, error) {
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

// handleChatHistory returns chat history (placeholder implementation)
func (s *Server) handleChatHistory(c *gin.Context) {
	// For now, return empty history
	// In a real implementation, you'd store and retrieve chat sessions
	c.JSON(http.StatusOK, gin.H{"sessions": []ChatSession{}})
}

// handleHTMXHistory returns history for HTMX (placeholder)
func (s *Server) handleHTMXHistory(c *gin.Context) {
	c.HTML(http.StatusOK, "history.html", gin.H{
		"sessions": []ChatSession{},
	})
}

// handleClearHistory clears chat history (placeholder)
func (s *Server) handleClearHistory(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "History cleared"})
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
	limit := 1000 // Default limit
	
	if limitParam := c.Query("limit"); limitParam != "" {
		if parsedLimit, err := strconv.Atoi(limitParam); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	// Check if this is an SSE request
	if c.GetHeader("Accept") == "text/event-stream" {
		// SSE streaming mode
		s.handleEnhancedLogsSSE(c, logType, level, search)
		return
	}

	// Regular HTTP mode - return filtered logs
	entries := s.enhancedLogBuffer.GetFilteredEntries(logType, level, search)
	
	// Apply limit
	if len(entries) > limit {
		entries = entries[:limit]
	}

	// Return logs as JSON
	c.JSON(http.StatusOK, entries)
}

// handleEnhancedLogsSSE provides Server-Sent Events with filtering
func (s *Server) handleEnhancedLogsSSE(c *gin.Context, logType, level, search string) {
	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	// Send existing logs that match filters
	entries := s.enhancedLogBuffer.GetFilteredEntries(logType, level, search)
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
			if s.matchesFilters(newEntry, logType, level, search) {
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
func (s *Server) matchesFilters(entry EnhancedLogEntry, logType, level, search string) bool {
	// Type filter
	if logType != "" && logType != "all" && !strings.HasPrefix(entry.Type, logType) {
		return false
	}

	// Level filter
	if level != "" && level != "all" && entry.Level != level {
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