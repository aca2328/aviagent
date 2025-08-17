package web

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"vmware-avi-llm-agent/internal/avi"
	"vmware-avi-llm-agent/internal/config"
	"vmware-avi-llm-agent/internal/llm"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Server represents the web server
type Server struct {
	config    *config.Config
	logger    *zap.Logger
	aviClient *avi.Client
	llmClient *llm.Client
	router    *gin.Engine
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

// NewServer creates a new web server
func NewServer(cfg *config.Config, logger *zap.Logger) (*Server, error) {
	// Initialize Avi client
	aviClient, err := avi.NewClient(&cfg.Avi, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Avi client: %w", err)
	}

	// Initialize LLM client
	llmClient, err := llm.NewClient(&cfg.LLM, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize LLM client: %w", err)
	}

	server := &Server{
		config:    cfg,
		logger:    logger,
		aviClient: aviClient,
		llmClient: llmClient,
	}

	// Initialize router
	server.setupRouter()

	return server, nil
}

// setupRouter sets up the Gin router with all routes and middleware
func (s *Server) setupRouter() {
	// Set Gin mode based on log level
	if s.config.Log.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	s.router = gin.New()

	// Add middleware
	s.router.Use(gin.Logger())
	s.router.Use(gin.Recovery())
	s.router.Use(s.corsMiddleware())

	// Load HTML templates
	s.router.LoadHTMLGlob("web/templates/*")

	// Serve static files
	s.router.Static("/static", "web/static")

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
	models, err := s.llmClient.GetAvailableModels()
	if err != nil {
		s.logger.Error("Failed to get available models", zap.Error(err))
		models = []string{s.config.LLM.DefaultModel}
	}

	c.HTML(http.StatusOK, "index.html", gin.H{
		"title":        "VMware Avi LLM Agent",
		"models":       models,
		"defaultModel": s.config.LLM.DefaultModel,
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

	// Set default model if not specified
	if request.Model == "" {
		request.Model = s.config.LLM.DefaultModel
	}

	// Validate model
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	validModel, err := s.llmClient.ValidateModel(ctx, request.Model)
	if err != nil {
		s.logger.Error("Failed to validate model", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to validate model"})
		return
	}

	if !validModel {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Model '%s' is not available", request.Model)})
		return
	}

	// Process the chat message
	response, err := s.processChatMessage(ctx, request.Message, request.Model, nil)
	if err != nil {
		s.logger.Error("Failed to process chat message", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process message"})
		return
	}

	c.JSON(http.StatusOK, response)
}

// handleHTMXChat handles HTMX chat requests
func (s *Server) handleHTMXChat(c *gin.Context) {
	message := c.PostForm("message")
	model := c.PostForm("model")

	if message == "" {
		c.HTML(http.StatusBadRequest, "chat.html", gin.H{
			"error": "Message cannot be empty",
		})
		return
	}

	if model == "" {
		model = s.config.LLM.DefaultModel
	}

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

	// Render the response as HTML
	c.HTML(http.StatusOK, "chat.html", gin.H{
		"userMessage":      message,
		"assistantMessage": response.Message,
		"model":           response.Model,
		"toolCalls":       response.ToolCalls,
		"timestamp":       time.Now().Format("15:04:05"),
	})
}

// processChatMessage processes a chat message and returns a response
func (s *Server) processChatMessage(ctx context.Context, message, model string, history []llm.ChatMessage) (*llm.LLMResponse, error) {
	// Get tool definitions
	tools := llm.GetAviToolDefinitions()

	// Process the message with the LLM
	llmResponse, err := s.llmClient.ProcessNaturalLanguageQuery(ctx, message, model, tools, history)
	if err != nil {
		return nil, fmt.Errorf("LLM processing failed: %w", err)
	}

	// If there are tool calls, execute them
	if len(llmResponse.ToolCalls) > 0 {
		for _, toolCall := range llmResponse.ToolCalls {
			result, err := s.executeToolCall(ctx, toolCall)
			if err != nil {
				s.logger.Error("Tool call failed", 
					zap.String("tool", toolCall.Function.Name),
					zap.Error(err))
				// Continue with other tool calls even if one fails
				continue
			}

			// Add the result to the response message
			if result != nil {
				llmResponse.Message += fmt.Sprintf("\n\nAPI Result:\n```json\n%v\n```", result)
			}
		}
	}

	return llmResponse, nil
}

// executeToolCall executes a tool call against the Avi API
func (s *Server) executeToolCall(ctx context.Context, toolCall llm.ToolCall) (interface{}, error) {
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
		return s.aviClient.ListVirtualServices(params)

	case "get_virtual_service":
		uuid, ok := toolCall.Args["uuid"].(string)
		if !ok {
			return nil, fmt.Errorf("uuid parameter required")
		}
		params := make(map[string]string)
		if fields, ok := toolCall.Args["fields"].(string); ok {
			params["fields"] = fields
		}
		return s.aviClient.GetVirtualService(uuid, params)

	case "create_virtual_service":
		return s.aviClient.CreateVirtualService(toolCall.Args)

	case "update_virtual_service":
		uuid, ok := toolCall.Args["uuid"].(string)
		if !ok {
			return nil, fmt.Errorf("uuid parameter required")
		}
		delete(toolCall.Args, "uuid") // Remove UUID from the data
		return s.aviClient.UpdateVirtualService(uuid, toolCall.Args)

	case "delete_virtual_service":
		uuid, ok := toolCall.Args["uuid"].(string)
		if !ok {
			return nil, fmt.Errorf("uuid parameter required")
		}
		return nil, s.aviClient.DeleteVirtualService(uuid)

	case "list_pools":
		params := make(map[string]string)
		if toolCall.Args != nil {
			for key, value := range toolCall.Args {
				if str, ok := value.(string); ok {
					params[key] = str
				}
			}
		}
		return s.aviClient.ListPools(params)

	case "get_pool":
		uuid, ok := toolCall.Args["uuid"].(string)
		if !ok {
			return nil, fmt.Errorf("uuid parameter required")
		}
		params := make(map[string]string)
		if fields, ok := toolCall.Args["fields"].(string); ok {
			params["fields"] = fields
		}
		return s.aviClient.GetPool(uuid, params)

	case "create_pool":
		return s.aviClient.CreatePool(toolCall.Args)

	case "scale_out_pool":
		uuid, ok := toolCall.Args["uuid"].(string)
		if !ok {
			return nil, fmt.Errorf("uuid parameter required")
		}
		delete(toolCall.Args, "uuid") // Remove UUID from the parameters
		return nil, s.aviClient.ScaleOutPool(uuid, toolCall.Args)

	case "scale_in_pool":
		uuid, ok := toolCall.Args["uuid"].(string)
		if !ok {
			return nil, fmt.Errorf("uuid parameter required")
		}
		delete(toolCall.Args, "uuid") // Remove UUID from the parameters
		return nil, s.aviClient.ScaleInPool(uuid, toolCall.Args)

	case "list_health_monitors":
		params := make(map[string]string)
		if toolCall.Args != nil {
			for key, value := range toolCall.Args {
				if str, ok := value.(string); ok {
					params[key] = str
				}
			}
		}
		return s.aviClient.ListHealthMonitors(params)

	case "get_health_monitor":
		uuid, ok := toolCall.Args["uuid"].(string)
		if !ok {
			return nil, fmt.Errorf("uuid parameter required")
		}
		params := make(map[string]string)
		if fields, ok := toolCall.Args["fields"].(string); ok {
			params["fields"] = fields
		}
		return s.aviClient.GetHealthMonitor(uuid, params)

	case "list_service_engines":
		params := make(map[string]string)
		if toolCall.Args != nil {
			for key, value := range toolCall.Args {
				if str, ok := value.(string); ok {
					params[key] = str
				}
			}
		}
		return s.aviClient.ListServiceEngines(params)

	case "get_service_engine":
		uuid, ok := toolCall.Args["uuid"].(string)
		if !ok {
			return nil, fmt.Errorf("uuid parameter required")
		}
		params := make(map[string]string)
		if fields, ok := toolCall.Args["fields"].(string); ok {
			params["fields"] = fields
		}
		return s.aviClient.GetServiceEngine(uuid, params)

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
		return s.aviClient.GetAnalytics(resourceType, uuid, params)

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

		return s.aviClient.ExecuteGenericOperation(method, endpoint, body, params)

	default:
		return nil, fmt.Errorf("unknown tool: %s", toolCall.Function.Name)
	}
}

// handleGetModels returns available models
func (s *Server) handleGetModels(c *gin.Context) {
	models := s.llmClient.GetAvailableModels()
	c.JSON(http.StatusOK, gin.H{
		"models": models,
		"default": s.config.LLM.DefaultModel,
	})
}

// handleHTMXModels returns models for HTMX
func (s *Server) handleHTMXModels(c *gin.Context) {
	models := s.llmClient.GetAvailableModels()
	c.HTML(http.StatusOK, "models.html", gin.H{
		"models": models,
		"default": s.config.LLM.DefaultModel,
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

	valid, err := s.llmClient.ValidateModel(ctx, request.Model)
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
	status := gin.H{
		"status": "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	// Check Avi connection
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	if _, err := s.aviClient.ListVirtualServices(map[string]string{"limit_by": "1"}); err != nil {
		status["avi_status"] = "unhealthy"
		status["avi_error"] = err.Error()
	} else {
		status["avi_status"] = "healthy"
	}

	// Check LLM connection
	if _, err := s.llmClient.ListModels(ctx); err != nil {
		status["llm_status"] = "unhealthy"
		status["llm_error"] = err.Error()
	} else {
		status["llm_status"] = "healthy"
	}

	c.JSON(http.StatusOK, status)
}

// handleAviProxy provides direct access to Avi API (for advanced users)
func (s *Server) handleAviProxy(c *gin.Context) {
	path := c.Param("path")
	method := c.Request.Method

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
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	// Execute the operation
	result, err := s.aviClient.ExecuteGenericOperation(method, path, body, params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

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
	if s.aviClient != nil {
		return s.aviClient.Close()
	}
	return nil
}