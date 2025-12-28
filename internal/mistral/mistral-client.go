package mistral

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"aviagent/internal/config"
	"aviagent/internal/llm"

	"go.uber.org/zap"
)

// Client represents the Mistral AI API client
type Client struct {
	config     *config.MistralConfig
	httpClient *http.Client
	logger     *zap.Logger
	apiKey     string
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
	Model     string        `json:"model"`
	Messages  []ChatMessage `json:"messages"`
	Tools     []Tool        `json:"tools,omitempty"`
	Stream    bool          `json:"stream,omitempty"`
	Temperature float64     `json:"temperature,omitempty"`
	MaxTokens int           `json:"max_tokens,omitempty"`
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
func NewClient(cfg *config.MistralConfig, apiKey string, logger *zap.Logger) (*Client, error) {
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
		config:     cfg,
		httpClient: httpClient,
		logger:     logger,
		apiKey:     apiKey,
	}, nil
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
		if seeker, ok := bodyReader.(io.Seekable); ok {
			// Try to read the body content for logging
			if _, err := seeker.Seek(0, io.SeekStart); err == nil {
				bodyContent, readErr := io.ReadAll(seeker)
				if readErr == nil {
					c.logger.Info("HTTP Request Body Content",
						zap.String("body_content", string(bodyContent)))
					// Reset the reader position
					seeker.Seek(0, io.SeekStart)
				}
			}
		} else {
			c.logger.Info("Request body is not seekable, cannot log content")
		}
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
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
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

	// Create request body and log it separately to ensure consistency
	requestBody := bytes.NewBuffer(jsonData)
	c.logger.Info("Request body prepared for HTTP call",
		zap.Int("body_length", requestBody.Len()))

	resp, err := c.makeRequest(ctx, "POST", "/v1/chat/completions", requestBody)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &chatResp, nil
}

// processNaturalLanguageQueryInternal processes a natural language query and returns tool calls (internal implementation)
func (c *Client) processNaturalLanguageQueryInternal(ctx context.Context, query, model string, tools []Tool, conversationHistory []ChatMessage) (*LLMResponse, error) {
	c.logger.Info("=== MESSAGE CONSTRUCTION START ===")
	
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

	// Create chat request
	chatReq := ChatRequest{
		Model:       model,
		Messages:    messages,
		Tools:       tools,
		Stream:      false,
		Temperature: c.config.Temperature,
		MaxTokens:   c.config.MaxTokens,
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
	response := &LLMResponse{
		Message: choice.Message.Content,
		Model:   chatResp.Model,
		Usage:   chatResp.Usage,
	}

	// Extract tool calls if present
	if len(choice.ToolCalls) > 0 {
		response.ToolCalls = choice.ToolCalls
	}

	return response, nil
}

// buildSystemPrompt creates the system prompt for the LLM
func (c *Client) buildSystemPrompt() string {
	return `You are an intelligent assistant for VMware Avi Load Balancer management using Mistral AI. Your role is to help users interact with the Avi Load Balancer API using natural language queries.

When users ask questions about Avi Load Balancer, you should:

1. Understand their intent and map it to appropriate API operations
2. Call the relevant API functions with the correct parameters
3. Present the results in a user-friendly format
4. Provide context and explanations for the data returned

You have access to the following types of operations:
- Virtual Service management (list, create, update, delete, scale)
- Pool management (list, create, update, scale out/in)
- Health Monitor management (list, create, update)
- Service Engine management (list, status, metrics)
- Analytics and monitoring data retrieval

When you need to perform an API operation, respond with a JSON object containing:
{
  "tool": "function_name",
  "parameters": {
    "param1": "value1",
    "param2": "value2"
  }
}

Always provide clear, helpful responses and ask for clarification if the user's request is ambiguous.

Examples:
- "List all virtual services" → {"tool": "list_virtual_services", "parameters": {}}
- "Show me pools with health issues" → {"tool": "list_pools", "parameters": {"health_status": "down"}}
- "Create a new pool with servers 10.1.1.10 and 10.1.1.11" → {"tool": "create_pool", "parameters": {"name": "new_pool", "servers": [{"ip": {"addr": "10.1.1.10", "type": "V4"}}, {"ip": {"addr": "10.1.1.11", "type": "V4"}}]}}
`
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

	// Log successful response
	c.logger.Info("ProcessNaturalLanguageQuery completed successfully",
		zap.String("response_message", mistralResp.Message),
		zap.Int("tool_calls_count", len(mistralResp.ToolCalls)))

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