package python

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"aviagent/internal/config"
	"aviagent/internal/llm"

	"go.uber.org/zap"
)

// PythonBridge represents a bridge to the Python Mistral client
type PythonBridge struct {
	config          *config.MistralConfig
	pythonPath      string
	bridgeScript    string
	logger          *zap.Logger
	process         *exec.Cmd
	processMutex    sync.Mutex
	initialized     bool
	initMutex       sync.Mutex
}

// PythonBridgeConfig holds configuration for the Python bridge
type PythonBridgeConfig struct {
	PythonPath   string
	BridgeScript string
	Logger       *zap.Logger
}

// NewPythonBridge creates a new Python bridge instance
type PythonBridgeOption func(*PythonBridge)

func WithPythonPath(path string) PythonBridgeOption {
	return func(b *PythonBridge) {
		b.pythonPath = path
	}
}

func WithBridgeScript(script string) PythonBridgeOption {
	return func(b *PythonBridge) {
		b.bridgeScript = script
	}
}

func WithLogger(logger *zap.Logger) PythonBridgeOption {
	return func(b *PythonBridge) {
		b.logger = logger
	}
}

func NewPythonBridge(cfg *config.MistralConfig, opts ...PythonBridgeOption) *PythonBridge {
	bridge := &PythonBridge{
		config:       cfg,
		pythonPath:   "python3", // Default to python3
		bridgeScript: "/app/python_mistral/bridge.py", // Absolute path for Docker container
		initialized:  false,
	}

	for _, opt := range opts {
		opt(bridge)
	}

	return bridge
}



// Initialize starts the Python bridge process
func (b *PythonBridge) Initialize() error {
	b.initMutex.Lock()
	defer b.initMutex.Unlock()

	if b.initialized {
		b.logger.Info("Python bridge already initialized")
		return nil
	}

	b.logger.Info("Initializing Python bridge",
		zap.String("python_path", b.pythonPath),
		zap.String("bridge_script", b.bridgeScript))

	// Check if Python is available
	if err := b.checkPythonAvailable(); err != nil {
		return fmt.Errorf("python not available: %w", err)
	}

	// Check if bridge script exists
	if err := b.checkBridgeScriptExists(); err != nil {
		return fmt.Errorf("bridge script not found: %w", err)
	}

	// Initialize the Python client
	configJSON, err := json.Marshal(b.config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	b.logger.Debug("Python bridge config JSON", zap.String("config", string(configJSON)))

	// Call Python bridge initialization
	initResponse, err := b.callPythonBridge("initialize", string(configJSON))
	if err != nil {
		return fmt.Errorf("failed to initialize Python bridge: %w", err)
	}

	b.logger.Debug("Python bridge raw response", zap.String("response", initResponse))

	var initResult map[string]interface{}
	if err := json.Unmarshal([]byte(initResponse), &initResult); err != nil {
		b.logger.Error("Failed to parse Python bridge response",
			zap.String("raw_response", initResponse),
			zap.Error(err))
		return fmt.Errorf("failed to parse initialization response: %w (raw: %s)", err, initResponse)
	}

	if initResult["status"] != "success" {
		return fmt.Errorf("Python bridge initialization failed: %s", initResult["message"])
	}

	b.initialized = true
	b.logger.Info("Python bridge initialized successfully",
		zap.String("model", initResult["model"].(string)),
		zap.Int("timeout", int(initResult["timeout"].(float64))))

	return nil
}

// ProcessQuery processes a natural language query using the Python Mistral client
func (b *PythonBridge) ProcessQuery(
	ctx context.Context,
	query string,
	conversationHistory []llm.ChatMessage,
	tools []llm.Tool,
) (*llm.LLMResponse, error) {
	if !b.initialized {
		if err := b.Initialize(); err != nil {
			b.logger.Error("Failed to initialize Python bridge", zap.Error(err))
			return nil, fmt.Errorf("python bridge not initialized: %w", err)
		}
	}

	// Convert conversation history to JSON-compatible format
	conversationJSON := make([]map[string]string, len(conversationHistory))
	for i, msg := range conversationHistory {
		conversationJSON[i] = map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	// Convert tools to JSON-compatible format
	toolsJSON := make([]map[string]interface{}, len(tools))
	for i, tool := range tools {
		toolsJSON[i] = map[string]interface{}{
			"type":     tool.Type,
			"function": tool.Function,
		}
	}

	// Create query JSON
	queryJSON := map[string]interface{}{
		"query":                query,
		"conversation_history": conversationJSON,
		"tools":                toolsJSON,
	}

	queryJSONStr, err := json.Marshal(queryJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}

	// Call Python bridge
	response, err := b.callPythonBridge("query", string(queryJSONStr))
	if err != nil {
		return nil, fmt.Errorf("failed to process query: %w", err)
	}

	// Parse response
	var responseMap map[string]interface{}
	if err := json.Unmarshal([]byte(response), &responseMap); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if responseMap["status"] != "success" {
		return nil, fmt.Errorf("query processing failed: %s", responseMap["message"])
	}

	// Extract LLM response
	responseData := responseMap["response"].(map[string]interface{})

	// Convert tool calls
	var toolCalls []llm.ToolCall
	if toolCallsData, ok := responseData["tool_calls"].([]interface{}); ok && len(toolCallsData) > 0 {
		toolCalls = make([]llm.ToolCall, len(toolCallsData))
		for i, tcData := range toolCallsData {
			if tcMap, ok := tcData.(map[string]interface{}); ok {
				toolCall := llm.ToolCall{
					ID:   tcMap["id"].(string),
					Type: tcMap["type"].(string),
				}
				
				if funcData, ok := tcMap["function"].(map[string]interface{}); ok {
					// Convert parameters to JSON string
					paramsJSON, _ := json.Marshal(funcData["parameters"])
					toolCall.Function = llm.ToolCallFunction{
						Name:      funcData["name"].(string),
						Arguments: string(paramsJSON),
					}
				}
				
				if argsData, ok := tcMap["args"]; ok {
					toolCall.Args = argsData.(map[string]interface{})
				}
				
				toolCalls[i] = toolCall
			}
		}
	}

	// Convert usage
	usageData := responseData["usage"].(map[string]interface{})
	usage := llm.Usage{
		PromptTokens:     int(usageData["prompt_tokens"].(float64)),
		CompletionTokens: int(usageData["completion_tokens"].(float64)),
		TotalTokens:     int(usageData["total_tokens"].(float64)),
	}

	return &llm.LLMResponse{
		Message:   responseData["message"].(string),
		ToolCalls: toolCalls,
		Model:     responseData["model"].(string),
		Usage:     usage,
	}, nil
}

// ChatCompletion sends a chat completion request using the Python Mistral client
func (b *PythonBridge) ChatCompletion(
	ctx context.Context,
	messages []llm.ChatMessage,
	tools []llm.Tool,
	model string,
	temperature float64,
	maxTokens int,
) (*llm.LLMResponse, error) {
	if !b.initialized {
		if err := b.Initialize(); err != nil {
			return nil, fmt.Errorf("python bridge not initialized: %w", err)
		}
	}

	// Convert messages to JSON-compatible format
	messagesJSON := make([]map[string]string, len(messages))
	for i, msg := range messages {
		messagesJSON[i] = map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	// Convert tools to JSON-compatible format
	var toolsJSON []map[string]interface{}
	if tools != nil {
		toolsJSON = make([]map[string]interface{}, len(tools))
		for i, tool := range tools {
			toolsJSON[i] = map[string]interface{}{
				"type":     tool.Type,
				"function": tool.Function,
			}
		}
	}

	// Create request JSON
	requestJSON := map[string]interface{}{
		"messages":    messagesJSON,
		"tools":       toolsJSON,
		"model":       model,
		"temperature": temperature,
		"max_tokens":  maxTokens,
	}

	requestJSONStr, err := json.Marshal(requestJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Call Python bridge
	response, err := b.callPythonBridge("chat", string(requestJSONStr))
	if err != nil {
		return nil, fmt.Errorf("failed to send chat completion: %w", err)
	}

	// Parse response (same as ProcessQuery)
	var responseMap map[string]interface{}
	if err := json.Unmarshal([]byte(response), &responseMap); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if responseMap["status"] != "success" {
		return nil, fmt.Errorf("chat completion failed: %s", responseMap["message"])
	}

	// Extract LLM response
	responseData := responseMap["response"].(map[string]interface{})

	// Convert tool calls
	var toolCalls []llm.ToolCall
	if toolCallsData, ok := responseData["tool_calls"].([]interface{}); ok && len(toolCallsData) > 0 {
		toolCalls = make([]llm.ToolCall, len(toolCallsData))
		for i, tcData := range toolCallsData {
			if tcMap, ok := tcData.(map[string]interface{}); ok {
				toolCall := llm.ToolCall{
					ID:   tcMap["id"].(string),
					Type: tcMap["type"].(string),
				}
				
				if funcData, ok := tcMap["function"].(map[string]interface{}); ok {
					// Convert parameters to JSON string
					paramsJSON, _ := json.Marshal(funcData["parameters"])
					toolCall.Function = llm.ToolCallFunction{
						Name:      funcData["name"].(string),
						Arguments: string(paramsJSON),
					}
				}
				
				if argsData, ok := tcMap["args"]; ok {
					// Convert args to map[string]interface{}
					if argsMap, ok := argsData.(map[string]interface{}); ok {
						toolCall.Args = argsMap
					}
				}
				
				toolCalls[i] = toolCall
			}
		}
	}

	// Convert usage
	usageData := responseData["usage"].(map[string]interface{})
	usage := llm.Usage{
		PromptTokens:     int(usageData["prompt_tokens"].(float64)),
		CompletionTokens: int(usageData["completion_tokens"].(float64)),
		TotalTokens:     int(usageData["total_tokens"].(float64)),
	}

	return &llm.LLMResponse{
		Message:   responseData["message"].(string),
		ToolCalls: toolCalls,
		Model:     responseData["model"].(string),
		Usage:     usage,
	}, nil
}

// callPythonBridge calls the Python bridge with a command and JSON data
func (b *PythonBridge) callPythonBridge(command string, jsonData string) (string, error) {
	b.processMutex.Lock()
	defer b.processMutex.Unlock()

	// Create Python command - use module import syntax to handle relative imports
	// Change working directory to /app where python_mistral package is located
	cmd := exec.Command(b.pythonPath, "-m", "python_mistral.bridge", command, jsonData)
	cmd.Dir = "/app" // Set working directory to /app

	// Set up pipes for communication
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// Set timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start process
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start Python process: %w", err)
	}

	// Wait for completion with timeout
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			stderr := stderrBuf.String()
			if stderr == "" {
				stderr = "unknown error"
			}
			return "", fmt.Errorf("python process failed: %w (stderr: %s)", err, stderr)
		}
		return stdoutBuf.String(), nil
	case <-ctx.Done():
		// Timeout - kill the process
		if err := cmd.Process.Kill(); err != nil {
			b.logger.Warn("Failed to kill timed out Python process", zap.Error(err))
		}
		return "", fmt.Errorf("python process timed out after 30 seconds")
	}
}

// checkPythonAvailable checks if Python is available
func (b *PythonBridge) checkPythonAvailable() error {
	cmd := exec.Command(b.pythonPath, "--version")
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("python not found or not executable: %w (stderr: %s)", err, stderrBuf.String())
	}

	b.logger.Info("Python found", zap.String("version", stdoutBuf.String()))
	return nil
}

// checkBridgeScriptExists checks if the bridge script exists
func (b *PythonBridge) checkBridgeScriptExists() error {
	// Check if the bridge module exists (using the module path)
	modulePath := "/app/python_mistral/bridge.py"
	if _, err := os.Stat(modulePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("bridge script not found at %s: %w", modulePath, err)
		}
		return fmt.Errorf("error checking bridge script: %w", err)
	}

	b.logger.Info("Bridge script found", zap.String("path", modulePath))
	return nil
}

// GetAvailableModels returns available models from Python bridge
func (b *PythonBridge) GetAvailableModels() []string {
	if !b.initialized {
		return b.config.Models
	}
	return b.config.Models
}

// ValidateModel validates if a model is available
func (b *PythonBridge) ValidateModel(ctx context.Context, modelName string) (bool, error) {
	for _, model := range b.GetAvailableModels() {
		if model == modelName {
			return true, nil
		}
	}
	return false, fmt.Errorf("model %s not found in available models", modelName)
}

// ProcessNaturalLanguageQuery processes natural language query (implements LLMClient interface)
func (b *PythonBridge) ProcessNaturalLanguageQuery(
	ctx context.Context,
	query string,
	model string,
	tools interface{},
	conversationHistory interface{},
) (*llm.LLMResponse, error) {
	
	// Convert tools to proper format
	var toolsList []llm.Tool
	if tools != nil {
		if toolsSlice, ok := tools.([]llm.Tool); ok {
			toolsList = toolsSlice
		} else {
			return nil, fmt.Errorf("invalid tools format")
		}
	}
	
	// Convert conversation history to proper format
	var historyList []llm.ChatMessage
	if conversationHistory != nil {
		if historySlice, ok := conversationHistory.([]llm.ChatMessage); ok {
			historyList = historySlice
		} else {
			return nil, fmt.Errorf("invalid conversation history format")
		}
	}
	
	// Use the existing ProcessQuery method
	return b.ProcessQuery(ctx, query, historyList, toolsList)
}

// GetStatus gets the current status of the Python bridge
func (b *PythonBridge) GetStatus() map[string]interface{} {
	status := map[string]interface{}{
		"initialized": b.initialized,
		"python_path":  b.pythonPath,
		"bridge_script": b.bridgeScript,
	}

	if b.initialized {
		status["config"] = map[string]interface{}{
			"model":       b.config.DefaultModel,
			"timeout":     b.config.Timeout,
			"temperature": b.config.Temperature,
			"max_tokens":  b.config.MaxTokens,
		}
	}

	return status
}

// Close shuts down the Python bridge
func (b *PythonBridge) Close() error {
	b.initMutex.Lock()
	defer b.initMutex.Unlock()

	if b.process != nil {
		if err := b.process.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("failed to kill Python process: %w", err)
		}
	}

	b.initialized = false
	b.logger.Info("Python bridge closed")
	return nil
}