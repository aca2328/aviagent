#!/bin/bash

# Simple test for Avi Controller Authentication
echo "🔐 Testing Avi Controller Authentication"
echo "========================================"
echo

# Load environment variables
if [ ! -f ".env" ]; then
    echo "❌ .env file not found"
    exit 1
fi

set -a
source .env
set +a

echo "✅ Loaded configuration:"
echo "  Avi Host: $AVI_HOST"
echo "  Avi Username: $AVI_USERNAME"
echo "  Auth Method: ${AVI_AUTH_METHOD:-session}"
echo

# Create a simple test that just checks if authentication works
cat > test_simple_auth.go << 'EOF'
package main

import (
	"fmt"
	"log"
	"os"

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
	)

	// Create Avi client - authentication happens here
	_, err = avi.NewClient(cfg, logger)
	if err != nil {
		logger.Fatal("Failed to create Avi client", zap.Error(err))
	}

	logger.Info("✅ Avi Controller Authentication Successful!")
	fmt.Println("🎉 Avi Controller authentication test passed!")
}
EOF

echo "🔨 Building test program..."
export PATH=$PATH:/usr/local/go/bin
go build -o test_simple_auth test_simple_auth.go

if [ $? -ne 0 ]; then
    echo "❌ Failed to build test program"
    exit 1
fi

echo "✅ Test program built successfully"
echo

echo "🚀 Running Avi authentication test..."
./test_simple_auth

# Clean up
rm -f test_simple_auth test_simple_auth.go

echo
echo "📋 Test completed"
