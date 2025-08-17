package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

// Config holds the application configuration
type Config struct {
	Server ServerConfig `mapstructure:"server"`
	Avi    AviConfig    `mapstructure:"avi"`
	LLM    LLMConfig    `mapstructure:"llm"`
	Log    LogConfig    `mapstructure:"log"`
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
	Host     string `mapstructure:"host"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	Version  string `mapstructure:"version"`
	Tenant   string `mapstructure:"tenant"`
	Timeout  int    `mapstructure:"timeout"`
	Insecure bool   `mapstructure:"insecure"`
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

// LogConfig holds logging configuration
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// Load loads configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	// Set default values
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.read_timeout", 30)
	viper.SetDefault("server.write_timeout", 30)
	viper.SetDefault("server.idle_timeout", 60)
	
	viper.SetDefault("avi.version", "31.2.1")
	viper.SetDefault("avi.tenant", "admin")
	viper.SetDefault("avi.timeout", 30)
	viper.SetDefault("avi.insecure", true)
	
	viper.SetDefault("llm.ollama_host", "http://localhost:11434")
	viper.SetDefault("llm.default_model", "llama3.2")
	viper.SetDefault("llm.models", []string{"llama3.2", "mistral", "codellama"})
	viper.SetDefault("llm.timeout", 60)
	viper.SetDefault("llm.temperature", 0.7)
	viper.SetDefault("llm.max_tokens", 2048)
	
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "json")

	// Set environment variable bindings
	viper.SetEnvPrefix("AVI_AGENT")
	viper.AutomaticEnv()

	// Bind specific environment variables
	viper.BindEnv("avi.host", "AVI_HOST")
	viper.BindEnv("avi.username", "AVI_USERNAME")
	viper.BindEnv("avi.password", "AVI_PASSWORD")
	viper.BindEnv("llm.ollama_host", "OLLAMA_HOST")

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
	if cfg.LLM.OllamaHost == "" {
		return fmt.Errorf("llm.ollama_host is required")
	}
	if len(cfg.LLM.Models) == 0 {
		return fmt.Errorf("at least one LLM model must be configured")
	}
	return nil
}

// fileExists checks if a file exists
func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}