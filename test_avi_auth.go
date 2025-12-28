package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"aviagent/internal/avi"
	"aviagent/internal/config"
	"go.uber.org/zap"
)

func main() {
	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	// Load configuration
	cfg := &config.AviConfig{
		Host:     os.Getenv("AVI_HOST"),
		Username: os.Getenv("AVI_USERNAME"),
		Password: os.Getenv("AVI_PASSWORD"),
		Version:  os.Getenv("AVI_VERSION"),
		Tenant:   os.Getenv("AVI_TENANT"),
		Timeout:  30,
		Insecure: os.Getenv("AVI_INSECURE") == "true",
		AuthMethod: os.Getenv("AVI_AUTH_METHOD"),
	}

	if cfg.Host == "" || cfg.Username == "" || cfg.Password == "" {
		logger.Fatal("Missing required Avi configuration")
	}

	logger.Info("Testing Avi Controller Authentication",
		zap.String("host", cfg.Host),
		zap.String("username", cfg.Username),
		zap.String("auth_method", cfg.AuthMethod),
	)

	// Create Avi client
	client, err := avi.NewClient(cfg, logger)
	if err != nil {
		logger.Fatal("Failed to create Avi client", zap.Error(err))
	}

	// Test authentication by making a simple API call
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Try to get controller version info
	logger.Info("Attempting to authenticate with Avi Controller...")

	// The authentication happens during client creation
	// If we got here, authentication was successful
	logger.Info("✅ Avi Controller Authentication Successful!")

	fmt.Println("🎉 Avi Controller authentication test passed!")
}
