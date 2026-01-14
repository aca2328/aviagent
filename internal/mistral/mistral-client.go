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
	// Color functions for enhanced logging
	requestColor func(...interface{}) string
	responseColor func(...interface{}) string
	toolColor func(...interface{}) string
	errorColor func(...interface{}) string
	infoColor func(...interface{}) string
	flowColor func(...interface{}) string
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
func NewClient(cfg *config.MistralConfig, apiKey string, logger *zap.Logger, langfuseClient langfuse.LangfuseClient) (*Client, error) {
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
	
	// If we're suggesting tool usage, enhance the system message with better guidance
	if forceToolUsage && len(messages) > 0 {
		originalSystemMessage := messages[0].Content
		toolName := determineBestToolForQuery(query)
		
		// Create enhanced system message with specific tool guidance
		enhancedSystemMessage := originalSystemMessage + fmt.Sprintf(
			"\n\n=== TOOL USAGE GUIDANCE ===\n"+
			"For the query '%s', you should use the '%s' tool.\n"+
			"This query requires accessing real-time Avi controller data.\n"+
			"DO NOT answer with general knowledge - use the tool instead.",
			query, toolName)
		
		messages[0].Content = enhancedSystemMessage
		chatReq.Messages = messages
		c.logger.Info("Enhanced system message with tool usage guidance",
			zap.String("tool", toolName))
	}

	// Send request to Mistral AI
	chatResp, err := c.ChatCompletion(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("chat completion failed: %w", err)
	}

	// Process response and extract tool calls
	return c.processLLMResponse(chatResp)
}

// LLMResponse represents a processed LLM response
// This matches the Ollama LLMResponse for compatibility
type LLMResponse struct {
	Message   string     `json:"message"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Model     string     `json:"model"`
	Usage     Usage      `json:"usage"`
}

// processLLMResponse processes the raw LLM response and extracts tool calls
func (c *Client) processLLMResponse(chatResp *ChatResponse) (*LLMResponse, error) {
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
	return `You are an AI assistant specialized in VMware Avi Load Balancer management. You have access to tools that allow you to interact with the Avi Load Balancer API to perform management tasks and retrieve real-time data.

IMPORTANT RULES FOR TOOL USAGE:
1. ANY request for current system state, real-time data, or specific configurations MUST use the appropriate tool
2. ANY request that mentions "show", "list", "get", "display", "current", "status", "health", "configuration" MUST use tools
3. ANY request about pools, virtual services, health monitors, service engines, or analytics MUST use tools
4. Do NOT answer questions about the current system state with general knowledge - ALWAYS use tools
5. If a user asks for data that requires API access, you MUST call the appropriate tool function

EXAMPLES THAT REQUIRE TOOLS:
- "Show me all pools" -> Use list_pools tool
- "List virtual services" -> Use list_virtual_services tool  
- "Show me all pools with their health status" -> Use list_pools tool with health_status parameter
- "Get details about virtual service XYZ" -> Use get_virtual_service tool
- "What pools are configured?" -> Use list_pools tool
- "Show me service engine status" -> Use list_service_engines tool

EXAMPLES THAT DON'T REQUIRE TOOLS:
- "What is a virtual service?" (general knowledge)
- "How do I configure a pool?" (general guidance)
- "What are the benefits of load balancing?" (conceptual question)

When using tools, always explain what you're doing and provide context for the results. If you need to use multiple tools, call them sequentially. Always be helpful, clear, and provide detailed context for your responses.`
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