package langfuse

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"aviagent/internal/config"
)

// LangfuseClient interface for Langfuse integration
type LangfuseClient interface {
	TraceMistralInteraction(ctx context.Context, userID, sessionID string) (string, error)
	LogPrompt(ctx context.Context, traceID, prompt string, model string) error
	LogResponse(ctx context.Context, traceID, responseContent string, usage *Usage, duration time.Duration, finishReason string) error
	LogToolCall(ctx context.Context, traceID, toolName string, toolIndex int, arguments string) error
	LogError(ctx context.Context, traceID, errorMessage string, errorType string) error
	Close() error
}

// Usage represents token usage statistics
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// Client represents the Langfuse observability client
type Client struct {
	config         *config.LangfuseConfig
	logger         *zap.Logger
	httpClient     *http.Client
	baseURL        string
	publicKey      string
	secretKey      string
}

// NewClient creates a new Langfuse client
func NewClient(config *config.LangfuseConfig, logger *zap.Logger) (LangfuseClient, error) {
	if !config.Enabled {
		logger.Info("Langfuse is disabled")
		return &NoopClient{logger: logger}, nil
	}

	// Remove trailing slash from host
	host := strings.TrimRight(config.Host, "/")
	baseURL := fmt.Sprintf("%s/api/public", host)

	return &Client{
		config:      config,
		logger:      logger,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		baseURL:     baseURL,
		publicKey:   config.PublicKey,
		secretKey:   config.SecretKey,
	}, nil
}

// TraceMistralInteraction starts a new trace for Mistral interactions
func (c *Client) TraceMistralInteraction(ctx context.Context, userID, sessionID string) (string, error) {
	traceData := map[string]interface{}{
		"name":       "mistral-interaction",
		"userId":     userID,
		"sessionId":  sessionID,
		"metadata": map[string]interface{}{
			"source": "aviagent",
			"type":   "mistral-interaction",
		},
	}

	traceID, err := c.postToLangfuse(ctx, "/trace", traceData)
	if err != nil {
		c.logger.Warn("Failed to create Langfuse trace", zap.Error(err))
		return "", err
	}

	return traceID, nil
}

// LogPrompt logs the user prompt to Langfuse
func (c *Client) LogPrompt(ctx context.Context, traceID, prompt string, model string) error {
	if traceID == "" {
		return nil
	}

	generationData := map[string]interface{}{
		"traceId": traceID,
		"name":    "prompt",
		"input":   prompt,
		"model":   model,
		"modelParameters": map[string]interface{}{
			"temperature": 0.7,
			"max_tokens":   2048,
		},
	}

	_, err := c.postToLangfuse(ctx, "/generation", generationData)
	return err
}

// LogResponse logs the Mistral response to Langfuse
func (c *Client) LogResponse(ctx context.Context, traceID, responseContent string, usage *Usage, duration time.Duration, finishReason string) error {
	if traceID == "" {
		return nil
	}

	generationData := map[string]interface{}{
		"traceId":  traceID,
		"name":     "response",
		"output":   responseContent,
		"model":    "mistral-medium",
		"usage":    usage,
		"metadata": map[string]interface{}{
			"duration_ms":    duration.Milliseconds(),
			"finish_reason":  finishReason,
			"response_type": "text",
		},
	}

	_, err := c.postToLangfuse(ctx, "/generation", generationData)
	return err
}

// LogToolCall logs tool call information to Langfuse
func (c *Client) LogToolCall(ctx context.Context, traceID, toolName string, toolIndex int, arguments string) error {
	if traceID == "" {
		return nil
	}

	spanData := map[string]interface{}{
		"traceId": traceID,
		"name":    "tool_call",
		"input":   arguments,
		"output":  "",
		"metadata": map[string]interface{}{
			"tool_name":   toolName,
			"tool_index":  toolIndex,
			"tool_type":   "function",
			"source":      "aviagent",
		},
	}

	_, err := c.postToLangfuse(ctx, "/span", spanData)
	return err
}

// LogError logs error information to Langfuse
func (c *Client) LogError(ctx context.Context, traceID, errorMessage string, errorType string) error {
	if traceID == "" {
		return nil
	}

	spanData := map[string]interface{}{
		"traceId": traceID,
		"name":    "error",
		"input":   errorMessage,
		"metadata": map[string]interface{}{
			"error_type":    errorType,
			"error_message": errorMessage,
			"source":        "aviagent",
		},
	}

	_, err := c.postToLangfuse(ctx, "/span", spanData)
	return err
}

// postToLangfuse sends data to Langfuse API
func (c *Client) postToLangfuse(ctx context.Context, endpoint string, data map[string]interface{}) (string, error) {
	url := c.baseURL + endpoint

	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Langfuse data: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return "", fmt.Errorf("failed to create Langfuse request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Langfuse-Public-Key", c.publicKey)
	req.Header.Set("X-Langfuse-Secret-Key", c.secretKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send data to Langfuse: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Langfuse API returned status %d: %s", resp.StatusCode, string(body))
	}

	// For trace creation, return the trace ID
	if endpoint == "/trace" {
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return "", fmt.Errorf("failed to decode Langfuse response: %w", err)
		}
		if traceID, ok := result["id"].(string); ok {
			return traceID, nil
		}
		return "", fmt.Errorf("no trace ID returned from Langfuse")
	}

	return "", nil
}

// Close shuts down the Langfuse client
func (c *Client) Close() error {
	// No resources to clean up for HTTP client
	return nil
}

// NoopClient is a no-op implementation for when Langfuse is disabled
type NoopClient struct {
	logger *zap.Logger
}

func (c *NoopClient) TraceMistralInteraction(ctx context.Context, userID, sessionID string) (string, error) {
	c.logger.Debug("Langfuse disabled - skipping trace creation")
	return "", nil
}

func (c *NoopClient) LogPrompt(ctx context.Context, traceID, prompt string, model string) error {
	return nil
}

func (c *NoopClient) LogResponse(ctx context.Context, traceID, responseContent string, usage *Usage, duration time.Duration, finishReason string) error {
	return nil
}

func (c *NoopClient) LogToolCall(ctx context.Context, traceID, toolName string, toolIndex int, arguments string) error {
	return nil
}

func (c *NoopClient) LogError(ctx context.Context, traceID, errorMessage string, errorType string) error {
	return nil
}

func (c *NoopClient) Close() error {
	return nil
}