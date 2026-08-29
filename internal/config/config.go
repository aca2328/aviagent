package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Config holds the application configuration
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Avi       AviConfig       `mapstructure:"avi"`
	MCP       MCPConfig       `mapstructure:"mcp"`
	LLM       LLMConfig       `mapstructure:"llm"`
	Mistral   MistralConfig   `mapstructure:"mistral"`
	Langfuse  LangfuseConfig  `mapstructure:"langfuse"`
	Log       LogConfig       `mapstructure:"log"`
	Sessions  SessionsConfig  `mapstructure:"sessions"`
	Provider  string          `mapstructure:"provider"` // "ollama" or "python" (Python uses official Mistral SDK)
}

// ServerConfig holds web server configuration
type ServerConfig struct {
	Port         int `mapstructure:"port"`
	ReadTimeout  int `mapstructure:"read_timeout"`
	WriteTimeout int `mapstructure:"write_timeout"`
	IdleTimeout  int `mapstructure:"idle_timeout"`
}

// AviConfig holds VMware Avi Load Balancer configuration
type AviConfig struct {
	Host      string `mapstructure:"host"`
	Username  string `mapstructure:"username"`
	Password  string `mapstructure:"password"`
	Version   string `mapstructure:"version"`
	Tenant    string `mapstructure:"tenant"`
	Timeout   int    `mapstructure:"timeout"`
	Insecure  bool   `mapstructure:"insecure"`
	AuthMethod string `mapstructure:"auth_method"` // "session" or "basic"
}

// MCPConfig holds configuration for the Avi MCP server the chat pipeline
// uses for tool calling. When disabled or unreachable, the app falls back to
// the built-in Go tool set (internal/llm.GetAviToolDefinitions).
type MCPConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	Command    string `mapstructure:"command"`
	ServerPath string `mapstructure:"server_path"` // path to mcp-avi-server/build/index.js; empty = auto-detect
}

// LLMConfig holds Ollama LLM configuration
type LLMConfig struct {
	OllamaHost    string   `mapstructure:"ollama_host"`
	DefaultModel  string   `mapstructure:"default_model"`
	Models        []string `mapstructure:"models"`
	Timeout       int      `mapstructure:"timeout"`
	Temperature   float64  `mapstructure:"temperature"`
	MaxTokens     int      `mapstructure:"max_tokens"`
}

// LangfuseConfig holds Langfuse observability configuration
type LangfuseConfig struct {
	Enabled       bool   `mapstructure:"enabled"`
	PublicKey     string `mapstructure:"public_key"`
	SecretKey     string `mapstructure:"secret_key"`
	Host          string `mapstructure:"host"`
	Debug         bool   `mapstructure:"debug"`
	FlushInterval int    `mapstructure:"flush_interval"`
}

// MistralConfig holds Mistral AI configuration
type MistralConfig struct {
	APIBaseURL   string   `mapstructure:"api_base_url"`
	APIKey       string   `mapstructure:"api_key"`
	DefaultModel string   `mapstructure:"default_model"`
	Models       []string `mapstructure:"models"`
	Timeout      int      `mapstructure:"timeout"`
	Temperature  float64  `mapstructure:"temperature"`
	MaxTokens    int      `mapstructure:"max_tokens"`
	Debug        bool     `mapstructure:"debug"`
}

// LogConfig holds logging configuration
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// SessionsConfig holds chat session persistence configuration
type SessionsConfig struct {
	Dir string `mapstructure:"dir"` // directory of one JSONL file per session
}

// Load loads configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	// Set default values
	viper.SetDefault("server.port", 8088)
	viper.SetDefault("server.read_timeout", 30)
	viper.SetDefault("server.write_timeout", 30)
	viper.SetDefault("server.idle_timeout", 60)
	
	viper.SetDefault("avi.version", "31.2.1")
	viper.SetDefault("avi.tenant", "admin")
	viper.SetDefault("avi.timeout", 30)
	viper.SetDefault("avi.insecure", false) // Changed to false for security
	viper.SetDefault("avi.auth_method", "session") // Default to session-based auth

	viper.SetDefault("mcp.enabled", true)
	viper.SetDefault("mcp.command", "node")
	viper.SetDefault("mcp.server_path", "")

	viper.SetDefault("llm.ollama_host", "http://localhost:11434")
	viper.SetDefault("llm.default_model", "llama3.2")
	viper.SetDefault("llm.models", []string{"llama3.2", "mistral", "codellama"})
	viper.SetDefault("llm.timeout", 60)
	viper.SetDefault("llm.temperature", 0.7)
	viper.SetDefault("llm.max_tokens", 2048)

	// Mistral AI configuration defaults
	viper.SetDefault("mistral.api_base_url", "https://api.mistral.ai")
	viper.SetDefault("mistral.api_key", "")
	viper.SetDefault("mistral.default_model", "mistral-medium")
	viper.SetDefault("mistral.models", []string{"mistral-tiny", "mistral-small", "mistral-medium"})
	viper.SetDefault("mistral.timeout", 60)
	viper.SetDefault("mistral.temperature", 0.7)
	viper.SetDefault("mistral.max_tokens", 2048)
	viper.SetDefault("mistral.debug", false)

	// Default to Python for better reliability and official Mistral SDK
	viper.SetDefault("provider", "python")
	
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "json")

	viper.SetDefault("sessions.dir", "data/sessions")

	// Set environment variable bindings
	viper.SetEnvPrefix("AVI_AGENT")
	viper.AutomaticEnv()

	// Bind specific environment variables
	viper.BindEnv("avi.host", "AVI_HOST")
	viper.BindEnv("avi.username", "AVI_USERNAME")
	viper.BindEnv("avi.password", "AVI_PASSWORD")
	viper.BindEnv("avi.version", "AVI_VERSION")
	viper.BindEnv("avi.tenant", "AVI_TENANT")
	viper.BindEnv("avi.timeout", "AVI_TIMEOUT")
	viper.BindEnv("avi.insecure", "AVI_INSECURE")
	viper.BindEnv("avi.auth_method", "AVI_AUTH_METHOD")

	viper.BindEnv("mcp.enabled", "AVI_MCP_ENABLED")
	viper.BindEnv("mcp.command", "AVI_MCP_COMMAND")
	viper.BindEnv("mcp.server_path", "AVI_MCP_SERVER_PATH")

	viper.BindEnv("llm.ollama_host", "OLLAMA_HOST")
	viper.BindEnv("llm.default_model", "OLLAMA_DEFAULT_MODEL")
	viper.BindEnv("llm.models", "OLLAMA_MODELS")
	viper.BindEnv("llm.timeout", "OLLAMA_TIMEOUT")
	viper.BindEnv("llm.temperature", "OLLAMA_TEMPERATURE")
	viper.BindEnv("llm.max_tokens", "OLLAMA_MAX_TOKENS")

	viper.BindEnv("mistral.api_base_url", "MISTRAL_API_BASE_URL")
	viper.BindEnv("mistral.api_key", "MISTRAL_API_KEY")
	viper.BindEnv("mistral.default_model", "MISTRAL_DEFAULT_MODEL")
	viper.BindEnv("mistral.models", "MISTRAL_MODELS")
	viper.BindEnv("mistral.timeout", "MISTRAL_TIMEOUT")
	viper.BindEnv("mistral.temperature", "MISTRAL_TEMPERATURE")
	viper.BindEnv("mistral.max_tokens", "MISTRAL_MAX_TOKENS")

	viper.BindEnv("server.port", "SERVER_PORT")
	viper.BindEnv("server.read_timeout", "SERVER_READ_TIMEOUT")
	viper.BindEnv("server.write_timeout", "SERVER_WRITE_TIMEOUT")
	viper.BindEnv("server.idle_timeout", "SERVER_IDLE_TIMEOUT")

	viper.BindEnv("log.level", "LOG_LEVEL")
	viper.BindEnv("log.format", "LOG_FORMAT")

	viper.BindEnv("sessions.dir", "SESSIONS_DIR")

	viper.BindEnv("provider", "LLM_PROVIDER")

	// Load configuration file if it exists
	if configPath != "" && fileExists(configPath) {
		viper.SetConfigFile(configPath)
		if err := viper.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Unmarshal configuration
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Handle comma-separated environment variables for models
	if ollamaModels := viper.GetString("OLLAMA_MODELS"); ollamaModels != "" {
		cfg.LLM.Models = parseCommaSeparated(ollamaModels)
	}
	if mistralModels := viper.GetString("MISTRAL_MODELS"); mistralModels != "" {
		cfg.Mistral.Models = parseCommaSeparated(mistralModels)
	}

	// Validate required configuration
	if err := validateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// validateConfig validates required configuration values
func validateConfig(cfg *Config) error {
	if cfg.Avi.Host == "" {
		return fmt.Errorf("avi.host is required")
	}
	if cfg.Avi.Username == "" {
		return fmt.Errorf("avi.username is required")
	}
	if cfg.Avi.Password == "" {
		return fmt.Errorf("avi.password is required")
	}

	// Validate based on provider
	if cfg.Provider == "ollama" {
		if cfg.LLM.OllamaHost == "" {
			return fmt.Errorf("llm.ollama_host is required when using Ollama provider")
		}
		if len(cfg.LLM.Models) == 0 {
			return fmt.Errorf("at least one LLM model must be configured for Ollama")
		}
	} else if cfg.Provider == "python" {
		if cfg.Mistral.APIBaseURL == "" {
			return fmt.Errorf("mistral.api_base_url is required when using Python provider")
		}
		if cfg.Mistral.APIKey == "" {
			return fmt.Errorf("mistral.api_key is required when using Python provider")
		}
		if len(cfg.Mistral.Models) == 0 {
			return fmt.Errorf("at least one Mistral model must be configured for Python provider")
		}
	} else {
		return fmt.Errorf("unsupported provider: %s. Use 'ollama' or 'python'", cfg.Provider)
	}

	return nil
}

// parseCommaSeparated parses comma-separated string to slice
func parseCommaSeparated(s string) []string {
	if s == "" {
		return []string{}
	}
	var result []string
	for _, item := range strings.Split(s, ",") {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// fileExists checks if a file exists
func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}