package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"aviagent/internal/config"

	"go.uber.org/zap"
)

// Client represents the Ollama LLM client
type Client struct {
	config     *config.LLMConfig
	httpClient *http.Client
	logger     *zap.Logger
}

// ChatMessage represents a chat message
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

// ChatRequest represents a chat completion request
type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Tools       []Tool        `json:"tools,omitempty"`
	Stream      bool          `json:"stream"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

// ChatResponse represents a chat completion response
type ChatResponse struct {
	Model              string      `json:"model"`
	CreatedAt          string      `json:"created_at"`
	Message            ChatMessage `json:"message"`
	Done               bool        `json:"done"`
	TotalDuration      int64       `json:"total_duration"`
	LoadDuration       int64       `json:"load_duration"`
	PromptEvalCount    int         `json:"prompt_eval_count"`
	PromptEvalDuration int64       `json:"prompt_eval_duration"`
	EvalCount          int         `json:"eval_count"`
	EvalDuration       int64       `json:"eval_duration"`
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

// ModelsResponse represents the response from /api/tags
type ModelsResponse struct {
	Models []Model `json:"models"`
}

// Model represents an available model
type Model struct {
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	Digest     string    `json:"digest"`
	ModifiedAt time.Time `json:"modified_at"`
	Details    struct {
		Format            string   `json:"format"`
		Family            string   `json:"family"`
		Families          []string `json:"families"`
		ParameterSize     string   `json:"parameter_size"`
		QuantizationLevel string   `json:"quantization_level"`
	} `json:"details"`
}

// NewClient creates a new LLM client
func NewClient(cfg *config.LLMConfig, logger *zap.Logger) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("llm config cannot be nil")
	}

	httpClient := &http.Client{
		Timeout: time.Duration(cfg.Timeout) * time.Second,
	}

	return &Client{
		config:     cfg,
		httpClient: httpClient,
		logger:     logger,
	}, nil
}

// ListModels retrieves available models from Ollama
func (c *Client) ListModels(ctx context.Context) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.config.OllamaHost+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
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

	return modelsResp.Models, nil
}

// ChatCompletion sends a chat completion request to Ollama
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

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.config.OllamaHost+"/api/chat", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
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
	// Build messages including conversation history
	messages := make([]ChatMessage, 0, len(conversationHistory)+2)
	
	// Add system message
	systemMessage := ChatMessage{
		Role:    "system",
		Content: c.buildSystemPrompt(),
	}
	messages = append(messages, systemMessage)

	// Add conversation history
	messages = append(messages, conversationHistory...)

	// Add current user query
	userMessage := ChatMessage{
		Role:    "user",
		Content: query,
	}
	messages = append(messages, userMessage)

	// Create chat request
	chatReq := ChatRequest{
		Model:       model,
		Messages:    messages,
		Tools:       tools,
		Stream:      false,
		Temperature: c.config.Temperature,
		MaxTokens:   c.config.MaxTokens,
	}

	// Send request to Ollama
	chatResp, err := c.ChatCompletion(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("chat completion failed: %w", err)
	}

	// Process response and extract tool calls
	return c.processLLMResponse(chatResp)
}

// LLMResponse represents a processed LLM response
type LLMResponse struct {
	Message   string     `json:"message"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Model     string     `json:"model"`
	Usage     Usage      `json:"usage"`
}

// Usage represents token usage statistics
type Usage struct {
	PromptTokens     int   `json:"prompt_tokens"`
	CompletionTokens int   `json:"completion_tokens"`
	TotalTokens      int   `json:"total_tokens"`
	Duration         int64 `json:"duration_ms"`
}

// processLLMResponse processes the raw LLM response and extracts tool calls
func (c *Client) processLLMResponse(chatResp *ChatResponse) (*LLMResponse, error) {
	response := &LLMResponse{
		Message: chatResp.Message.Content,
		Model:   chatResp.Model,
		Usage: Usage{
			PromptTokens:     chatResp.PromptEvalCount,
			CompletionTokens: chatResp.EvalCount,
			TotalTokens:      chatResp.PromptEvalCount + chatResp.EvalCount,
			Duration:         chatResp.TotalDuration / 1000000, // Convert to milliseconds
		},
	}

	// Try to extract tool calls from the response
	toolCalls, err := c.extractToolCalls(chatResp.Message.Content)
	if err != nil {
		c.logger.Warn("Failed to extract tool calls", zap.Error(err))
	} else if len(toolCalls) > 0 {
		response.ToolCalls = toolCalls
	}

	return response, nil
}

// extractToolCalls attempts to extract tool calls from the LLM response content
func (c *Client) extractToolCalls(content string) ([]ToolCall, error) {
	var toolCalls []ToolCall

	// Try to parse JSON tool calls from the content
	// This is a simplified approach - in production, you might want more sophisticated parsing
	if json.Valid([]byte(content)) {
		var jsonCall map[string]interface{}
		if err := json.Unmarshal([]byte(content), &jsonCall); err == nil {
			if toolName, ok := jsonCall["tool"].(string); ok {
				toolCall := ToolCall{
					ID:   fmt.Sprintf("call_%d", time.Now().UnixNano()),
					Type: "function",
					Function: ToolCallFunction{
						Name:      toolName,
						Arguments: content,
					},
				}

				if params, ok := jsonCall["parameters"].(map[string]interface{}); ok {
					toolCall.Args = params
				}

				toolCalls = append(toolCalls, toolCall)
			}
		}
	}

	return toolCalls, nil
}

// buildSystemPrompt creates the system prompt for the LLM
func (c *Client) buildSystemPrompt() string {
	return `You are an intelligent assistant for VMware Avi Load Balancer management. Your role is to help users interact with the Avi Load Balancer API using natural language queries.

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
		if model.Name == modelName {
			return true, nil
		}
	}

	return false, nil
}

// ProcessNaturalLanguageQueryInterface implements the LLMClient interface method
func (c *Client) ProcessNaturalLanguageQuery(ctx context.Context, query, model string, tools interface{}, conversationHistory interface{}) (*LLMResponse, error) {
	// Convert interface{} parameters to Ollama types
	ollamaTools, ok1 := tools.([]Tool)
	ollamaHistory, ok2 := conversationHistory.([]ChatMessage)
	
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("invalid parameter types for Ollama client")
	}
	
	// Call the actual Ollama implementation
	return c.processNaturalLanguageQueryInternal(ctx, query, model, ollamaTools, ollamaHistory)
}

// GetAvailableModels returns the list of configured available models
func (c *Client) GetAvailableModels() []string {
	return c.config.Models
}