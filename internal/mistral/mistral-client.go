package mistral

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"aviagent/internal/config"
	"aviagent/internal/langfuse"
	"aviagent/internal/llm"

	"github.com/fatih/color"
	"go.uber.org/zap"
)

// Client represents the Mistral AI API client
type Client struct {
	config      *config.MistralConfig
	httpClient  *http.Client
	logger      *zap.Logger
	apiKey      string
	activeFlows map[string]*mistralFlow
	langfuseClient langfuse.LangfuseClient
	aviClientProvider func() (AviClientInterface, error)
	// Color functions for enhanced logging
	requestColor func(...interface{}) string
	responseColor func(...interface{}) string
	toolColor func(...interface{}) string
	errorColor func(...interface{}) string
	infoColor func(...interface{}) string
	flowColor func(...interface{}) string
}

// AviClientInterface defines the interface for Avi clients
type AviClientInterface interface {
	ListVirtualServices(ctx context.Context, params map[string]string) (interface{}, error)
	GetVirtualService(ctx context.Context, uuid string, params map[string]string) (interface{}, error)
	// Add other methods as needed for fallback scenarios
}

// mistralFlow tracks the request/response flow for better logging
type mistralFlow struct {
	flowID      string
	requestTime time.Time
	steps       []string
}

// ChatMessage represents a chat message for Mistral AI
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Tool represents a tool/function that can be called by the LLM
type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

// Function represents a function definition for the LLM
type Function struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

// ChatRequest represents a chat completion request for Mistral AI
type ChatRequest struct {
	Model      string        `json:"model"`
	Messages   []ChatMessage `json:"messages"`
	Tools      []Tool        `json:"tools,omitempty"`
	ToolChoice interface{}   `json:"tool_choice,omitempty"`
	Stream     bool          `json:"stream,omitempty"`
	Temperature float64     `json:"temperature,omitempty"`
	MaxTokens  int           `json:"max_tokens,omitempty"`
}

// ChatResponse represents a chat completion response from Mistral AI
type ChatResponse struct {
	ID              string      `json:"id"`
	Object          string      `json:"object"`
	Created         int64       `json:"created"`
	Model           string      `json:"model"`
	Choices         []Choice    `json:"choices"`
	Usage           Usage       `json:"usage"`
	SystemFingerprint string    `json:"system_fingerprint"`
}

// Choice represents a response choice from Mistral AI
type Choice struct {
	Index        int           `json:"index"`
	Message      ChatMessage   `json:"message"`
	FinishReason string        `json:"finish_reason"`
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
}

// ToolCall represents a tool call made by the LLM
type ToolCall struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Function ToolCallFunction       `json:"function"`
	Args     map[string]interface{} `json:"args,omitempty"`
}

// ToolCallFunction represents the function part of a tool call
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Usage represents token usage statistics
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ModelsResponse represents the response from Mistral AI models endpoint
type ModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

// Model represents an available Mistral AI model
type Model struct {
	ID         string `json:"id"`
	Object     string `json:"object"`
	Created    int64  `json:"created"`
	OwnedBy    string `json:"owned_by"`
	Permission []struct {
		ID                 string `json:"id"`
		Object             string `json:"object"`
		Created            int64  `json:"created"`
		AllowCreateEngine  bool   `json:"allow_create_engine"`
		AllowSampling      bool   `json:"allow_sampling"`
		AllowLogprobs      bool   `json:"allow_logprobs"`
		AllowSearchIndices bool   `json:"allow_search_indices"`
		AllowView          bool   `json:"allow_view"`
		AllowFineTuning    bool   `json:"allow_fine_tuning"`
		Organization       string `json:"organization"`
		Group              string `json:"group,omitempty"`
		IsBlocking         bool   `json:"is_blocking"`
	} `json:"permission,omitempty"`
	Root string `json:"root,omitempty"`
	Parent string `json:"parent,omitempty"`
}

// NewClient creates a new Mistral AI client
func NewClient(cfg *config.MistralConfig, apiKey string, logger *zap.Logger, langfuseClient langfuse.LangfuseClient, aviClientProvider func() (AviClientInterface, error)) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("mistral config cannot be nil")
	}

	if apiKey == "" {
		return nil, fmt.Errorf("mistral API key cannot be empty")
	}

	// Create HTTP client with timeout
	httpClient := &http.Client{
		Timeout: time.Duration(cfg.Timeout) * time.Second,
	}

	return &Client{
		config:          cfg,
		httpClient:      httpClient,
		logger:          logger,
		apiKey:          apiKey,
		activeFlows:     make(map[string]*mistralFlow),
		langfuseClient: langfuseClient,
		aviClientProvider: aviClientProvider,
		// Initialize color functions
		requestColor:  color.New(color.FgBlue).SprintFunc(),
		responseColor: color.New(color.FgGreen).SprintFunc(),
		toolColor:     color.New(color.FgMagenta).SprintFunc(),
		errorColor:    color.New(color.FgRed).SprintFunc(),
		infoColor:     color.New(color.FgCyan).SprintFunc(),
		flowColor:     color.New(color.FgYellow).SprintFunc(),
	}, nil
}

// logFlowStart begins tracking a new request/response flow
func (c *Client) logFlowStart(query string) string {
	flowID := "flow-" + fmt.Sprintf("%08x", rand.Int31())
	c.activeFlows[flowID] = &mistralFlow{
		flowID:      flowID,
		requestTime: time.Now(),
		steps:       []string{fmt.Sprintf("🚀 Request started: %s", query)},
	}

	// Log the flow start with color
	flowMarker := c.flowColor(fmt.Sprintf("=== MISTRAL FLOW START [%s] ===", flowID))
	c.logger.Info(flowMarker,
		zap.String("flow_id", flowID),
		zap.String("query", query),
		zap.String("timestamp", time.Now().Format(time.RFC3339)))

	return flowID
}

// logFlowStep adds a step to the current flow with color coding
func (c *Client) logFlowStep(flowID, stepType, message string, fields ...zap.Field) {
	if flow, exists := c.activeFlows[flowID]; exists {
		flow.steps = append(flow.steps, message)
	}

	// Determine color based on step type
	var colorFunc func(...interface{}) string
	switch stepType {
	case "request":
		colorFunc = c.requestColor
	case "response":
		colorFunc = c.responseColor
	case "tool":
		colorFunc = c.toolColor
	case "error":
		colorFunc = c.errorColor
	default:
		colorFunc = c.infoColor
	}

	// Format the log message with color
	flowMarker := c.flowColor(fmt.Sprintf("[%s]", flowID))
	stepMarker := colorFunc(fmt.Sprintf("%s:", strings.ToUpper(stepType)))
	formattedMessage := fmt.Sprintf("%s %s %s", flowMarker, stepMarker, message)

	// Add color-coded fields
	colorFields := append([]zap.Field{zap.String("flow_id", flowID)}, fields...)
	c.logger.Info(formattedMessage, colorFields...)
}

// logFlowEnd completes the flow tracking
func (c *Client) logFlowEnd(flowID, outcome string) {
	if flow, exists := c.activeFlows[flowID]; exists {
		duration := time.Since(flow.requestTime)
		flow.steps = append(flow.steps, fmt.Sprintf("✅ %s (duration: %v)", outcome, duration))
		
		// Log summary
		flowMarker := c.flowColor(fmt.Sprintf("=== MISTRAL FLOW END [%s] ===", flowID))
		c.logger.Info(flowMarker,
			zap.String("flow_id", flowID),
			zap.String("outcome", outcome),
			zap.Duration("duration", duration),
			zap.Int("steps", len(flow.steps)))
		
		// Clean up
		delete(c.activeFlows, flowID)
	}
}

// logFlowError records an error in the flow
func (c *Client) logFlowError(flowID, message string, err error) {
	if flow, exists := c.activeFlows[flowID]; exists {
		flow.steps = append(flow.steps, fmt.Sprintf("❌ Error: %s", message))
	}

	errorMarker := c.errorColor(fmt.Sprintf("ERROR: %s", message))
	
	c.logger.Error(errorMarker,
		zap.String("flow_id", flowID),
		zap.String("error", message),
		zap.Error(err))
}

// makeRequest performs an authenticated API request to Mistral AI
func (c *Client) makeRequest(ctx context.Context, method, endpoint string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewBuffer(jsonData)
	}

	requestURL := fmt.Sprintf("%s%s", c.config.APIBaseURL, endpoint)

	req, err := http.NewRequestWithContext(ctx, method, requestURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set required headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))

	// Log complete HTTP request details
	c.logger.Info("HTTP Request Details",
		zap.String("method", method),
		zap.String("url", requestURL),
		zap.String("content_type", req.Header.Get("Content-Type")),
		zap.String("authorization", "Bearer ***REDACTED***"))

	// Log request headers
	c.logger.Info("Request Headers",
		zap.Any("headers", req.Header))

	// If this is a POST request with a body, log the body content
	if method == "POST" && bodyReader != nil {
		// Try to read the body content for logging
		if bytesBuffer, ok := bodyReader.(*bytes.Buffer); ok {
			bodyContent := bytesBuffer.Bytes()
			c.logger.Info("HTTP Request Body Content",
				zap.String("body_content", string(bodyContent)),
				zap.Int("body_length", len(bodyContent)))
			
			// Critical validation: ensure body is not empty
			if len(bodyContent) == 0 {
				c.logger.Error("CRITICAL: HTTP request body is empty for POST request!")
			}
		} else {
			c.logger.Info("Request body is not a bytes.Buffer, cannot log content without consuming it")
		}
	} else if method == "POST" && bodyReader == nil {
		c.logger.Error("CRITICAL: POST request has nil body reader!")
	}

	c.logger.Info("Making Mistral AI API request")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("Mistral AI request failed",
			zap.String("method", method),
			zap.String("endpoint", endpoint),
			zap.Error(err))
		return nil, fmt.Errorf("Mistral AI request failed: %w", err)
	}

	// Log response details
	c.logger.Info("HTTP Response Received",
		zap.Int("status_code", resp.StatusCode),
		zap.String("status", resp.Status))

	return resp, nil
}

// ListModels retrieves available models from Mistral AI
func (c *Client) ListModels(ctx context.Context) ([]Model, error) {
	resp, err := c.makeRequest(ctx, "GET", "/v1/models", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
		c.logger.Error("Models request failed",
			zap.String("error", err.Error()),
			zap.Int("status_code", resp.StatusCode))
		return nil, err
	}

	var modelsResp ModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return modelsResp.Data, nil
}

// ChatCompletion sends a chat completion request to Mistral AI
func (c *Client) ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	// Set default model if not specified
	if req.Model == "" {
		req.Model = c.config.DefaultModel
	}

	// Set default temperature if not specified
	if req.Temperature == 0 {
		req.Temperature = c.config.Temperature
	}

	// Set default max tokens if not specified
	if req.MaxTokens == 0 {
		req.MaxTokens = c.config.MaxTokens
	}

	// Extract query for flow tracking (use last user message as query)
	query := "API request"
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			query = req.Messages[i].Content
			if len(query) > 50 {
				query = query[:47] + "..."
			}
			break
		}
	}

	// Start flow tracking
	flowID := c.logFlowStart(query)
	defer c.logFlowEnd(flowID, "Chat completion completed")

	// Log with flow tracking
	c.logFlowStep(flowID, "request", "Starting Mistral API request",
		zap.String("model", req.Model),
		zap.Int("message_count", len(req.Messages)),
		zap.Bool("has_tools", len(req.Tools) > 0))

	// Comprehensive debug logging for request analysis
	c.logger.Info("=== MISTRAL API REQUEST START ===")
	c.logger.Info("Mistral ChatCompletion request details",
		zap.String("model", req.Model),
		zap.Int("message_count", len(req.Messages)),
		zap.Bool("has_tools", len(req.Tools) > 0),
		zap.Float64("temperature", req.Temperature),
		zap.Int("max_tokens", req.MaxTokens),
		zap.String("stream_mode", fmt.Sprintf("%t", req.Stream)))

	// Log each message individually for detailed analysis
	for i, msg := range req.Messages {
		c.logger.Info("Message analysis",
			zap.Int("message_index", i),
			zap.String("role", msg.Role),
			zap.String("content_length", fmt.Sprintf("%d", len(msg.Content))),
			zap.String("content_preview", fmt.Sprintf("%.50s...", msg.Content)))
	}

	// Log tools if present
	if len(req.Tools) > 0 {
		c.logger.Info("Tools included in request", zap.Int("tool_count", len(req.Tools)))
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Log the complete JSON payload
	c.logger.Info("Complete Mistral API request payload",
		zap.String("json_length", fmt.Sprintf("%d", len(jsonData))),
		zap.String("full_json", string(jsonData)))

	// Start Langfuse trace for this Mistral interaction
	var langfuseTraceID string
	if c.langfuseClient != nil {
		// Extract user ID and session ID from context or use defaults
		userID := "anonymous"
		sessionID := fmt.Sprintf("session-%d", time.Now().Unix())
		
		var err error
		langfuseTraceID, err = c.langfuseClient.TraceMistralInteraction(ctx, userID, sessionID)
		if err != nil {
			c.logger.Warn("Failed to start Langfuse trace", zap.Error(err))
		}
		
		// Log the prompt to Langfuse
		prompt := c.formatMessagesForLangfuse(req.Messages)
		if err := c.langfuseClient.LogPrompt(ctx, langfuseTraceID, prompt, req.Model); err != nil {
			c.logger.Warn("Failed to log prompt to Langfuse", zap.Error(err))
		}
	}

	// Record start time for performance tracking
	startTime := time.Now()

	// Pass the request struct directly to makeRequest for proper marshaling
	resp, err := c.makeRequest(ctx, "POST", "/v1/chat/completions", req)
	if err != nil {
		// Log error to Langfuse if available
		if c.langfuseClient != nil && langfuseTraceID != "" {
			_ = c.langfuseClient.LogError(ctx, langfuseTraceID, err.Error(), "api_request_failed")
		}
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		// Log error to Langfuse if available
		if c.langfuseClient != nil && langfuseTraceID != "" {
			_ = c.langfuseClient.LogError(ctx, langfuseTraceID, err.Error(), "response_decode_failed")
		}
		c.logFlowError(flowID, "Failed to decode response", err)
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Log response to Langfuse
	if c.langfuseClient != nil && langfuseTraceID != "" {
		duration := time.Since(startTime)
		
		// Calculate token usage
		usage := &langfuse.Usage{
			InputTokens:     chatResp.Usage.PromptTokens,
			OutputTokens:    chatResp.Usage.CompletionTokens,
			TotalTokens:     chatResp.Usage.TotalTokens,
		}
		
		// Get response content
		responseContent := ""
		if len(chatResp.Choices) > 0 {
			responseContent = chatResp.Choices[0].Message.Content
		}
		
		// Log the response
		finishReason := "stop"
		if len(chatResp.Choices) > 0 {
			finishReason = chatResp.Choices[0].FinishReason
		}
		
		if err := c.langfuseClient.LogResponse(ctx, langfuseTraceID, responseContent, usage, duration, finishReason); err != nil {
			c.logger.Warn("Failed to log response to Langfuse", zap.Error(err))
		}
		
		// Log tool calls if present
		if len(chatResp.Choices) > 0 && len(chatResp.Choices[0].ToolCalls) > 0 {
			for i, toolCall := range chatResp.Choices[0].ToolCalls {
				if err := c.langfuseClient.LogToolCall(ctx, langfuseTraceID, toolCall.Function.Name, i, toolCall.Function.Arguments); err != nil {
					c.logger.Warn("Failed to log tool call to Langfuse", zap.Error(err), zap.String("tool_name", toolCall.Function.Name))
				}
			}
		}
	}

	// Log response details with flow tracking
	toolCallCount := 0
	if len(chatResp.Choices) > 0 && len(chatResp.Choices[0].ToolCalls) > 0 {
		toolCallCount = len(chatResp.Choices[0].ToolCalls)
	}

	c.logFlowStep(flowID, "response", "Received Mistral API response",
		zap.Int("choice_count", len(chatResp.Choices)),
		zap.Int("tool_call_count", toolCallCount),
		zap.String("model", chatResp.Model))

	// Log tool calls if present
	if toolCallCount > 0 {
		for _, toolCall := range chatResp.Choices[0].ToolCalls {
			c.logFlowStep(flowID, "tool", fmt.Sprintf("Tool call: %s", toolCall.Function.Name),
				zap.String("tool_id", toolCall.ID),
				zap.String("tool_type", toolCall.Type))
		}
	} else {
		c.logFlowStep(flowID, "info", "No tool calls in response")
	}

	return &chatResp, nil
}

// determineBestToolForQuery maps user queries to the most appropriate tool
func determineBestToolForQuery(query string) string {
	lowerQuery := strings.ToLower(query)

	// Virtual Service queries
	if strings.Contains(lowerQuery, "virtual service") ||
	   strings.Contains(lowerQuery, "load balancer service") ||
	   strings.Contains(lowerQuery, "vs ") {
		if strings.Contains(lowerQuery, "detail") ||
		   strings.Contains(lowerQuery, "specific") ||
		   strings.Contains(lowerQuery, "uuid") {
			return "get_virtual_service"
		}
		return "list_virtual_services"
	}

	// Pool queries
	if strings.Contains(lowerQuery, "pool") ||
	   strings.Contains(lowerQuery, "backend server") ||
	   strings.Contains(lowerQuery, "server pool") {
		if strings.Contains(lowerQuery, "detail") ||
		   strings.Contains(lowerQuery, "specific") ||
		   strings.Contains(lowerQuery, "uuid") {
			return "get_pool"
		}
		return "list_pools"
	}

	// Health Monitor queries
	if strings.Contains(lowerQuery, "health monitor") ||
	   strings.Contains(lowerQuery, "health check") ||
	   strings.Contains(lowerQuery, "monitor") {
		if strings.Contains(lowerQuery, "detail") ||
		   strings.Contains(lowerQuery, "specific") {
			return "get_health_monitor"
		}
		return "list_health_monitors"
	}

	// Service Engine queries
	if strings.Contains(lowerQuery, "service engine") ||
	   strings.Contains(lowerQuery, "se ") ||
	   strings.Contains(lowerQuery, "load balancer instance") {
		if strings.Contains(lowerQuery, "detail") ||
		   strings.Contains(lowerQuery, "specific") {
			return "get_service_engine"
		}
		return "list_service_engines"
	}

	// Analytics/Metrics queries
	if strings.Contains(lowerQuery, "analytic") ||
	   strings.Contains(lowerQuery, "metric") ||
	   strings.Contains(lowerQuery, "performance") ||
	   strings.Contains(lowerQuery, "statistic") {
		return "get_analytics"
	}

	// Fallback to generic operation
	return "execute_generic_operation"
}

// processNaturalLanguageQueryInternal processes a natural language query and returns tool calls (internal implementation)
func (c *Client) processNaturalLanguageQueryInternal(ctx context.Context, query, model string, tools []Tool, conversationHistory []ChatMessage) (*LLMResponse, error) {
	c.logger.Info("=== MESSAGE CONSTRUCTION START ===")
	c.logger.Info("processNaturalLanguageQueryInternal called", zap.String("query", query), zap.String("model", model))
	
	// Ensure conversation history is not nil
	if conversationHistory == nil {
		c.logger.Info("Nil conversation history detected, converting to empty slice")
		conversationHistory = []ChatMessage{}
	}

	// Build messages including conversation history
	messages := make([]ChatMessage, 0, len(conversationHistory)+2)

	// Add system message
	systemMessage := ChatMessage{
		Role:    "system",
		Content: c.buildSystemPrompt(),
	}
	messages = append(messages, systemMessage)
	c.logger.Info("Added system message", zap.Int("system_content_length", len(systemMessage.Content)))

	// Add conversation history
	c.logger.Info("Adding conversation history", zap.Int("history_message_count", len(conversationHistory)))
	for i, msg := range conversationHistory {
		c.logger.Info("History message",
			zap.Int("history_index", i),
			zap.String("role", msg.Role),
			zap.Int("content_length", len(msg.Content)))
	}
	messages = append(messages, conversationHistory...)

	// Add current user query
	userMessage := ChatMessage{
		Role:    "user",
		Content: query,
	}
	messages = append(messages, userMessage)
	c.logger.Info("Added user message", zap.String("user_query", query))

	// Validate message alternation pattern
	c.logger.Info("Message alternation validation")
	for i, msg := range messages {
		c.logger.Info("Message validation",
			zap.Int("message_index", i),
			zap.String("role", msg.Role),
			zap.Int("content_length", len(msg.Content)))
	}

	// Validate that we have at least the system and user messages
	if len(messages) < 2 {
		c.logger.Error("Invalid message construction", zap.Int("actual_message_count", len(messages)))
		return nil, fmt.Errorf("invalid message construction: expected at least system and user messages, got %d", len(messages))
	}
	
	c.logger.Info("Message construction completed successfully", zap.Int("total_messages", len(messages)))

	// Check if this query should force tool usage based on specific patterns
	forceToolUsage := false
	lowerQuery := strings.ToLower(query)
	
	// Patterns that should ALWAYS use tools
	if strings.Contains(lowerQuery, "list") && (strings.Contains(lowerQuery, "virtual service") || strings.Contains(lowerQuery, "pool") || strings.Contains(lowerQuery, "service")) {
		forceToolUsage = true
		c.logger.Info("Forcing tool usage for list query", zap.String("query", query))
	}
	
	if strings.Contains(lowerQuery, "show") && (strings.Contains(lowerQuery, "virtual service") || strings.Contains(lowerQuery, "pool") || strings.Contains(lowerQuery, "service")) {
		forceToolUsage = true
		c.logger.Info("Forcing tool usage for show query", zap.String("query", query))
	}
	
	if strings.Contains(lowerQuery, "health status") || strings.Contains(lowerQuery, "current status") || strings.Contains(lowerQuery, "status") {
		forceToolUsage = true
		c.logger.Info("Forcing tool usage for status query", zap.String("query", query))
	}

	// Create chat request
	chatReq := ChatRequest{
		Model:       model,
		Messages:    messages,
		Tools:       tools,
		Stream:      false,
		Temperature: c.config.Temperature,
		MaxTokens:   c.config.MaxTokens,
	}
	
	// Use auto tool choice for all queries - let Mistral decide when to use tools
	chatReq.ToolChoice = "auto"
	c.logger.Info("Using automatic tool selection for all queries",
		zap.String("query", query),
		zap.String("tool_choice_type", "auto"))
	
	// Add detailed logging for tool choice decisions
	c.logger.Info("Tool choice decision completed",
		zap.String("query", query),
		zap.Bool("suggest_tool_usage", forceToolUsage),
		zap.String("tool_choice_type", "auto"),
		zap.String("strategy", "auto_with_guidance"))
	
	// Note: Removed system message enhancement to reduce payload size
	// The base system message already contains sufficient tool usage guidance
	// This reduces the request size by ~43% and prevents Mistral API timeouts

	// For testing: Try with minimal tools first if we have many tools
	if len(tools) > 2 {
		c.logger.Info("Attempting minimal tool request first for reliability",
			zap.Int("original_tool_count", len(tools)),
			zap.String("query", query))
		
		// Create a minimal tool set with just the most relevant tools
		minimalTools := c.filterToolsForQuery(query, tools)
		
		if len(minimalTools) > 0 && len(minimalTools) < len(tools) {
			// Try with minimal tools first
			chatReqMinimal := chatReq
			chatReqMinimal.Tools = minimalTools
			
			c.logger.Info("Testing with minimal tool set",
				zap.Int("minimal_tool_count", len(minimalTools)))
			
			chatResp, err := c.ChatCompletion(ctx, chatReqMinimal)
			if err == nil {
				c.logger.Info("Minimal tool request succeeded",
					zap.Int("tool_count_used", len(minimalTools)))
				return c.processLLMResponse(chatResp)
			}
			
			c.logger.Warn("Minimal tool request failed, falling back to full tool set",
				zap.Error(err))
		}
	}

	// Send request to Mistral AI with full tool set
	chatResp, err := c.ChatCompletion(ctx, chatReq)
	if err != nil {
		// Enhanced error logging for debugging Go HTTP client issues
		c.logger.Error("Mistral API request failed with detailed error information",
			zap.Error(err),
			zap.String("query", query),
			zap.String("error_type", fmt.Sprintf("%T", err)),
			zap.String("model", chatReq.Model),
			zap.Int("tool_count", len(chatReq.Tools)))
		
		// Check if this is a virtual service query that we can handle directly
		if c.aviClientProvider != nil && c.canHandleFallback(query) {
			c.logger.Warn("Mistral API failed, attempting fallback to direct Avi API call",
				zap.Error(err),
				zap.String("query", query))
			
			// Get Avi client using the provider function
			aviClient, aviErr := c.aviClientProvider()
			if aviErr != nil {
				c.logger.Error("Failed to get Avi client for fallback",
					zap.Error(aviErr),
					zap.String("query", query))
				return nil, fmt.Errorf("chat completion failed: %w", err)
			}
			
			if aviClient != nil {
				// Call Avi controller API directly
				fallbackResponse, fallbackErr := c.handleFallbackQuery(ctx, aviClient, query)
				if fallbackErr != nil {
					c.logger.Error("Fallback Avi API call failed",
						zap.Error(fallbackErr),
						zap.String("query", query))
					return nil, fmt.Errorf("chat completion failed: %w", err)
				}
				
				c.logger.Info("Fallback to Avi API succeeded",
					zap.String("query", query),
					zap.String("fallback_type", fallbackResponse.fallbackType))
				
				return fallbackResponse.llmResponse, nil
			}
		}
		
		return nil, fmt.Errorf("chat completion failed: %w", err)
	}

	// Process response and extract tool calls
	return c.processLLMResponse(chatResp)
}

// fallbackResponse represents a response from the fallback mechanism
type fallbackResponse struct {
	llmResponse  *LLMResponse
	fallbackType string
}

// canHandleFallback determines if we can handle this query with direct Avi API calls
func (c *Client) canHandleFallback(query string) bool {
	lowerQuery := strings.ToLower(query)
	
	// Queries we can handle with direct Avi API calls
	if strings.Contains(lowerQuery, "list") && strings.Contains(lowerQuery, "virtual service") {
		return true
	}
	if strings.Contains(lowerQuery, "show") && strings.Contains(lowerQuery, "virtual service") {
		return true
	}
	if strings.Contains(lowerQuery, "get") && strings.Contains(lowerQuery, "virtual service") {
		return true
	}
	
	return false
}

// handleFallbackQuery handles queries by calling Avi API directly
func (c *Client) handleFallbackQuery(ctx context.Context, aviClient AviClientInterface, query string) (*fallbackResponse, error) {
	lowerQuery := strings.ToLower(query)
	
	// Handle "list all virtual services" query
	if strings.Contains(lowerQuery, "list") && strings.Contains(lowerQuery, "virtual service") {
		return c.handleListVirtualServicesFallback(ctx, aviClient)
	}
	
	// Handle "show virtual service" or "get virtual service" queries
	if (strings.Contains(lowerQuery, "show") || strings.Contains(lowerQuery, "get")) && strings.Contains(lowerQuery, "virtual service") {
		// For now, return list as we don't have UUID extraction
		return c.handleListVirtualServicesFallback(ctx, aviClient)
	}
	
	return nil, fmt.Errorf("no fallback handler available for query: %s", query)
}

// handleListVirtualServicesFallback handles "list all virtual services" queries
func (c *Client) handleListVirtualServicesFallback(ctx context.Context, aviClient AviClientInterface) (*fallbackResponse, error) {
	// Call Avi controller API directly
	result, err := aviClient.ListVirtualServices(ctx, map[string]string{"limit_by": "10"})
	if err != nil {
		return nil, fmt.Errorf("failed to list virtual services: %w", err)
	}
	
	// Create a response that mimics what Mistral would return
	// This allows the rest of the system to process it normally
	
	// Convert result to JSON for the tool call
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal virtual services result: %w", err)
	}
	
	// Create tool call that mimics what Mistral would generate
	toolCall := ToolCall{
		ID:   "fallback-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		Type: "function",
		Function: ToolCallFunction{
			Name: "list_virtual_services",
			Arguments: string(resultJSON),
		},
	}
	
	// Create LLM response
	llmResponse := &LLMResponse{
		Message:   fmt.Sprintf("Retrieved virtual services from Avi controller (fallback mode)"),
		ToolCalls: []ToolCall{toolCall},
		Model:     "avi-fallback",
		Usage: Usage{
			PromptTokens:     0,
			CompletionTokens: 0,
			TotalTokens:     0,
		},
	}
	
	return &fallbackResponse{
		llmResponse:  llmResponse,
		fallbackType: "list_virtual_services",
	}, nil
}

// LLMResponse represents a processed LLM response
// This matches the Ollama LLMResponse for compatibility
type LLMResponse struct {
	Message   string     `json:"message"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Model     string     `json:"model"`
	Usage     Usage      `json:"usage"`
}

// filterToolsForQuery filters tools to return only the most relevant ones for a given query
// This helps reduce payload size and improve reliability
func (c *Client) filterToolsForQuery(query string, tools []Tool) []Tool {
	lowerQuery := strings.ToLower(query)
	
	// If query contains "virtual service", return only virtual service tools
	if strings.Contains(lowerQuery, "virtual service") || 
	   strings.Contains(lowerQuery, "load balancer service") ||
	   strings.Contains(lowerQuery, "vs ") {
		
		var filteredTools []Tool
		for _, tool := range tools {
			if strings.Contains(tool.Function.Name, "virtual_service") {
				filteredTools = append(filteredTools, tool)
			}
		}
		
		if len(filteredTools) > 0 {
			c.logger.Info("Filtered tools for virtual service query",
				zap.Int("original_count", len(tools)),
				zap.Int("filtered_count", len(filteredTools)))
			return filteredTools
		}
	}
	
	// If query contains "pool", return only pool tools
	if strings.Contains(lowerQuery, "pool") {
		var filteredTools []Tool
		for _, tool := range tools {
			if strings.Contains(tool.Function.Name, "pool") {
				filteredTools = append(filteredTools, tool)
			}
		}
		
		if len(filteredTools) > 0 {
			c.logger.Info("Filtered tools for pool query",
				zap.Int("original_count", len(tools)),
				zap.Int("filtered_count", len(filteredTools)))
			return filteredTools
		}
	}
	
	// For other queries, return a minimal set of most common tools
	minimalToolNames := []string{"list_virtual_services", "get_virtual_service", "list_pools", "get_pool"}
	var minimalTools []Tool
	
	for _, tool := range tools {
		for _, name := range minimalToolNames {
			if tool.Function.Name == name {
				minimalTools = append(minimalTools, tool)
				break
			}
		}
	}
	
	if len(minimalTools) > 0 {
		c.logger.Info("Using minimal tool set for general query",
			zap.Int("original_count", len(tools)),
			zap.Int("minimal_count", len(minimalTools)))
		return minimalTools
	}
	
	// If no filtering applied, return original tools
	return tools
}

// validateMistralResponse validates the Mistral API response and provides detailed error information
func (c *Client) validateMistralResponse(chatResp *ChatResponse) error {
	if chatResp == nil {
		c.logger.Error("Nil response received from Mistral AI")
		return fmt.Errorf("nil response from Mistral AI")
	}
	
	if len(chatResp.Choices) == 0 {
		c.logger.Error("No choices returned from Mistral AI",
			zap.String("model", chatResp.Model),
			zap.Any("usage", chatResp.Usage))
		return fmt.Errorf("no choices returned from Mistral AI")
	}
	
	// Check each choice for validity
	for i, choice := range chatResp.Choices {
		if choice.Message.Role == "" {
			c.logger.Error("Invalid choice: missing role",
				zap.Int("choice_index", i),
				zap.String("finish_reason", choice.FinishReason))
			return fmt.Errorf("invalid choice at index %d: missing role", i)
		}
		
		// Check for tool calls - this is actually a success case, not an error
		if len(choice.ToolCalls) > 0 {
			c.logger.Info("Valid tool calls detected in response",
				zap.Int("choice_index", i),
				zap.Int("tool_call_count", len(choice.ToolCalls)))
			// Validate each tool call
			for j, toolCall := range choice.ToolCalls {
				if toolCall.Function.Name == "" {
					c.logger.Error("Invalid tool call: missing function name",
						zap.Int("choice_index", i),
						zap.Int("tool_call_index", j))
					return fmt.Errorf("invalid tool call at choice %d, index %d: missing function name", i, j)
				}
			}
			return nil // Tool calls are valid - this is a success case
		}
	}
	
	// If we get here, the response is valid but contains no tool calls
	c.logger.Info("Valid Mistral response with no tool calls")
	return nil
}

// processLLMResponse processes the raw LLM response and extracts tool calls
func (c *Client) processLLMResponse(chatResp *ChatResponse) (*LLMResponse, error) {
	// First validate the response
	if err := c.validateMistralResponse(chatResp); err != nil {
		c.logger.Error("Mistral response validation failed", zap.Error(err))
		return nil, fmt.Errorf("invalid Mistral response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices returned from Mistral AI")
	}

	choice := chatResp.Choices[0]
	
	// Debug logging for tool calls
	c.logger.Info("Mistral API response analysis",
		zap.Int("choice_count", len(chatResp.Choices)),
		zap.String("message_content", choice.Message.Content),
		zap.Int("message_content_length", len(choice.Message.Content)),
		zap.Int("tool_calls_count", len(choice.ToolCalls)))
	
	if len(choice.ToolCalls) > 0 {
		c.logger.Info("Tool calls detected in Mistral response")
		for i, toolCall := range choice.ToolCalls {
			c.logger.Info("Tool call details",
				zap.Int("tool_call_index", i),
				zap.String("function_name", toolCall.Function.Name),
				zap.String("arguments", toolCall.Function.Arguments))
		}
	}

	response := &LLMResponse{
		Message: choice.Message.Content,
		Model:   chatResp.Model,
		Usage:   chatResp.Usage,
	}

	// Extract tool calls if present
	if len(choice.ToolCalls) > 0 {
		response.ToolCalls = choice.ToolCalls
		c.logger.Info("Successfully extracted tool calls", zap.Int("count", len(response.ToolCalls)))
		
		// If there are tool calls but no message content, provide a default informative message
		if response.Message == "" {
			response.Message = "I need to use API tools to fulfill your request. Processing your request..."
			c.logger.Info("Generated default message for tool calls with empty content")
		} else {
			// If there's existing content, enhance it with tool processing info
			response.Message = choice.Message.Content + "\n\nProcessing your request using API tools..."
		}
	} else {
		c.logger.Info("No tool calls found in Mistral response")
	}

	return response, nil
}

// buildSystemPrompt creates the system prompt for the LLM
func (c *Client) buildSystemPrompt() string {
	return `You are an AI assistant for VMware Avi Load Balancer management with access to API tools.

TOOL USAGE RULES:
- Use tools for any request about current system state, real-time data, or specific configurations
- Use tools for queries containing: show, list, get, display, current, status, health, configuration
- Use tools for: pools, virtual services, health monitors, service engines, analytics
- Never answer system state questions with general knowledge - always use tools

When using tools, explain actions and provide context. Call tools sequentially as needed.`
}

// ValidateModel checks if the specified model is available
func (c *Client) ValidateModel(ctx context.Context, modelName string) (bool, error) {
	models, err := c.ListModels(ctx)
	if err != nil {
		return false, err
	}

	for _, model := range models {
		if model.ID == modelName {
			return true, nil
		}
	}

	return false, nil
}

// convertMistralToolCalls converts Mistral ToolCalls to LLM ToolCalls
func convertMistralToolCalls(mistralCalls []ToolCall) []llm.ToolCall {
	llmCalls := make([]llm.ToolCall, len(mistralCalls))
	for i, call := range mistralCalls {
		llmCalls[i] = llm.ToolCall{
			ID:       call.ID,
			Type:     call.Type,
			Function: llm.ToolCallFunction{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
		}
	}
	return llmCalls
}

// convertMistralUsage converts Mistral Usage to LLM Usage
func convertMistralUsage(mistralUsage Usage) llm.Usage {
	return llm.Usage{
		PromptTokens:     mistralUsage.PromptTokens,
		CompletionTokens: mistralUsage.CompletionTokens,
		TotalTokens:      mistralUsage.TotalTokens,
	}
}

// ProcessNaturalLanguageQuery implements the LLMClient interface method
func (c *Client) ProcessNaturalLanguageQuery(ctx context.Context, query, model string, tools interface{}, conversationHistory interface{}) (*llm.LLMResponse, error) {
	// Log method entry with parameter details
	c.logger.Info("=== PROCESS NATURAL LANGUAGE QUERY START ===")
	c.logger.Info("ProcessNaturalLanguageQuery called",
		zap.String("query", query),
		zap.String("model", model),
		zap.String("tools_type", fmt.Sprintf("%T", tools)),
		zap.String("history_type", fmt.Sprintf("%T", conversationHistory)))

	// Convert interface{} parameters to Mistral types
	mistralTools, ok1 := tools.([]Tool)
	mistralHistory, ok2 := conversationHistory.([]ChatMessage)
	
	if !ok1 || !ok2 {
		c.logger.Error("Type conversion failed",
			zap.Bool("tools_conversion_ok", ok1),
			zap.Bool("history_conversion_ok", ok2),
			zap.String("tools_actual_type", fmt.Sprintf("%T", tools)),
			zap.String("history_actual_type", fmt.Sprintf("%T", conversationHistory)))
		return nil, fmt.Errorf("invalid parameter types for Mistral client")
	}

	// Log conversation history details
	c.logger.Info("Conversation history analysis",
		zap.Int("history_length", len(mistralHistory)),
		zap.Bool("history_is_nil", mistralHistory == nil))

	// Log tools details
	c.logger.Info("Tools analysis",
		zap.Int("tools_length", len(mistralTools)))

	// Call the actual Mistral implementation
	mistralResp, err := c.processNaturalLanguageQueryInternal(ctx, query, model, mistralTools, mistralHistory)
	if err != nil {
		c.logger.Error("processNaturalLanguageQueryInternal failed", zap.Error(err))
		return nil, err
	}

	// Log successful response with full details
	c.logger.Info("ProcessNaturalLanguageQuery completed successfully",
		zap.String("response_message", mistralResp.Message),
		zap.Int("tool_calls_count", len(mistralResp.ToolCalls)),
		zap.String("model", mistralResp.Model),
		zap.Any("usage", mistralResp.Usage))

	// Log the raw Mistral response for debugging
	if c.config.Debug {
		c.logger.Debug("About to log raw Mistral response")
		responseJSON, err := json.Marshal(mistralResp)
		if err == nil {
			c.logger.Debug("Raw Mistral API response",
				zap.String("raw_response", string(responseJSON)))
		} else {
			c.logger.Debug("Failed to marshal Mistral response", zap.Error(err))
		}
	}

	// Convert Mistral response to LLMResponse format
	return &llm.LLMResponse{
		Message:   mistralResp.Message,
		ToolCalls: convertMistralToolCalls(mistralResp.ToolCalls),
		Model:     mistralResp.Model,
		Usage:     convertMistralUsage(mistralResp.Usage),
	}, nil
}

// GetAvailableModels returns the list of configured available models
func (c *Client) GetAvailableModels() []string {
	return c.config.Models
}

// formatMessagesForLangfuse formats messages for Langfuse logging
func (c *Client) formatMessagesForLangfuse(messages []ChatMessage) string {
	var sb strings.Builder
	for _, msg := range messages {
		sb.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, msg.Content))
	}
	return sb.String()
}