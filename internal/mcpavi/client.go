// Package mcpavi connects to the avi-mcp-server (mcp-avi-server/) over stdio
// and exposes its tools to the chat pipeline in the same shape as
// internal/llm.GetAviToolDefinitions, so the LLM tool-calling loop in
// internal/web can use either source interchangeably.
package mcpavi

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"aviagent/internal/config"
	"aviagent/internal/llm"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"
)

// Client manages a connection to the Avi MCP server subprocess. Connection is
// lazy: the subprocess is not spawned, and the Avi controller is not
// authenticated against, until the first call to Connect.
type Client struct {
	mu      sync.Mutex
	session *mcp.ClientSession
	tools   []llm.Tool
	toolSet map[string]bool

	command string
	args    []string
	env     []string
	logger  *zap.Logger
}

// NewClient builds a Client for the given Avi and MCP configuration. It does
// not connect; call Connect before use.
func NewClient(aviCfg *config.AviConfig, mcpCfg *config.MCPConfig, logger *zap.Logger) (*Client, error) {
	serverPath := mcpCfg.ServerPath
	if serverPath == "" {
		var err error
		serverPath, err = resolveServerPath()
		if err != nil {
			return nil, err
		}
	}

	aviEnv := []string{
		"AVI_HOST=" + aviCfg.Host,
		"AVI_USERNAME=" + aviCfg.Username,
		"AVI_PASSWORD=" + aviCfg.Password,
		"AVI_VERSION=" + aviCfg.Version,
		"AVI_TENANT=" + aviCfg.Tenant,
		"AVI_AUTH_METHOD=" + aviCfg.AuthMethod,
		fmt.Sprintf("AVI_TIMEOUT=%d", aviCfg.Timeout),
		fmt.Sprintf("AVI_INSECURE=%t", aviCfg.Insecure),
	}

	return &Client{
		command: mcpCfg.Command,
		args:    []string{serverPath},
		env:     append(inheritedEnvWithoutAviVars(), aviEnv...),
		logger:  logger,
	}, nil
}

// resolveServerPath locates the compiled MCP server bundle, mirroring the
// template/static path resolution in internal/web/web-server.go: relative to
// the repo root in local dev, or a fixed location in the Docker image.
func resolveServerPath() (string, error) {
	candidates := []string{
		"mcp-avi-server/build/index.js", // local dev, cwd = repo root
		"/opt/avi-mcp-server/build/index.js", // docker image
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("could not locate mcp-avi-server build (tried %s); run 'npm run build' in mcp-avi-server/ or set mcp.server_path", strings.Join(candidates, ", "))
}

func inheritedEnvWithoutAviVars() []string {
	env := os.Environ()
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, "AVI_") {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered
}

// Connect spawns the MCP server subprocess (if not already connected) and
// discovers its tools. Safe to call multiple times.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session != nil {
		return nil
	}

	cmd := exec.Command(c.command, c.args...)
	cmd.Env = c.env

	client := mcp.NewClient(&mcp.Implementation{Name: "aviagent", Version: "1.0.0"}, nil)
	transport := &mcp.CommandTransport{Command: cmd}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to avi mcp server: %w", err)
	}

	listResult, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		session.Close()
		return fmt.Errorf("failed to list tools from avi mcp server: %w", err)
	}

	tools := make([]llm.Tool, 0, len(listResult.Tools))
	toolSet := make(map[string]bool, len(listResult.Tools))
	for _, t := range listResult.Tools {
		tools = append(tools, llm.Tool{
			Type: "function",
			Function: llm.Function{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  stripSchemaMeta(t.InputSchema),
			},
		})
		toolSet[t.Name] = true
	}

	c.session = session
	c.tools = tools
	c.toolSet = toolSet
	c.logger.Info("Connected to avi mcp server", zap.Int("tool_count", len(tools)))
	return nil
}

// Tools returns the tool definitions discovered from the MCP server, in the
// same []llm.Tool shape internal/llm.GetAviToolDefinitions returns. Call
// Connect first.
func (c *Client) Tools() []llm.Tool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tools
}

// HasTool reports whether the connected MCP server exposes a tool with this name.
func (c *Client) HasTool(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.toolSet[name]
}

// CallTool invokes a tool by name and returns its parsed JSON result (or the
// raw text if the result isn't JSON).
func (c *Client) CallTool(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
	c.mu.Lock()
	session := c.session
	c.mu.Unlock()
	if session == nil {
		return nil, fmt.Errorf("avi mcp client not connected")
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return nil, fmt.Errorf("mcp tool call failed: %w", err)
	}

	text := extractText(result)
	if result.IsError {
		return nil, fmt.Errorf("avi mcp tool %s returned an error: %s", name, text)
	}
	if text == "" {
		return nil, nil
	}

	var parsed interface{}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return text, nil
	}
	return parsed, nil
}

// Close terminates the MCP server subprocess.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session == nil {
		return nil
	}
	err := c.session.Close()
	c.session = nil
	return err
}

func extractText(result *mcp.CallToolResult) string {
	for _, content := range result.Content {
		if tc, ok := content.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

// stripSchemaMeta removes JSON Schema meta-keys (added by the TypeScript
// SDK's zod-to-json-schema conversion) that some LLM providers' strict tool
// schema validators reject, e.g. Mistral. Only strips top-level keys --
// nested "additionalProperties" inside a property (e.g. a record/map field)
// is meaningful schema, not meta noise.
func stripSchemaMeta(schema any) any {
	m, ok := schema.(map[string]interface{})
	if !ok {
		return schema
	}
	cleaned := make(map[string]interface{}, len(m))
	for k, v := range m {
		if k == "$schema" || k == "additionalProperties" {
			continue
		}
		cleaned[k] = v
	}
	return cleaned
}
