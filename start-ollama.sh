#!/bin/bash

# Function to check for existing .env file and handle user confirmation
check_env_file() {
    if [ -f ".env" ]; then
        echo "⚠️  An existing .env file was detected!"
        echo
        echo "Current .env file contents:"
        echo "----------------------------------------"
        # Show relevant lines from existing .env file
        grep -E "^LLM_PROVIDER|^OLLAMA_HOST|^AVI_HOST|^AVI_USERNAME" .env || echo "(No matching configuration found)"
        echo "----------------------------------------"
        echo
        echo
        echo "Choose an option:"
        echo "  1. Overwrite existing .env file (creates backup)"
        echo "  2. Use existing .env file (start application now)"
        echo "  3. Cancel (do nothing)"
        echo
        read -p "Enter your choice (1-3, default: 3): " USER_CHOICE
        
        case "$USER_CHOICE" in
            1)
                echo "📝 Backing up existing .env file to .env.backup"
                cp .env .env.backup
                return 0
                ;;
            2)
                echo "🚀 Using existing .env file to start the application..."
                echo "📦 Make sure Ollama is running natively on this Mac first (see below)"
                echo
                ./container-run.sh up

                if [ $? -eq 0 ]; then
                    echo "✅ Application started successfully with existing configuration!"
                    echo
                    # Extract port from .env or use default
                    LOCAL_PORT=$(grep -E "^SERVER_PORT=" .env | cut -d'=' -f2 || echo "8088")
                    echo "🌐 Access the application at: http://localhost:$LOCAL_PORT"
                    echo "📊 Health check endpoint: http://localhost:$LOCAL_PORT/api/health"
                    echo "💬 API endpoint: http://localhost:$LOCAL_PORT/api/chat"
                    echo
                    echo "🔄 Pull models on the host, not the container: ollama pull llama3.2"
                    echo "📋 To list available models, run: ollama list"
                    echo
                    echo "📋 To stop the application, run: ./container-run.sh down"
                    echo "📋 To view logs, run: ./container-run.sh logs"
                else
                    echo "❌ Failed to start the application with existing configuration"
                fi
                exit 0
                ;;
            *)
                echo "🔴 Operation cancelled. Existing .env file preserved."
                echo "📋 To use the existing configuration later, run: ./container-run.sh up"
                exit 0
                ;;
        esac
=======
    fi
    return 0
}

# VMware Avi LLM Agent - Ollama Startup Script
# This script creates a .env file and starts the application with Ollama

echo "🚀 VMware Avi LLM Agent - Ollama Setup"
echo "===================================="
echo

# Check if Apple's `container` CLI is installed
if ! command -v container &> /dev/null; then
    echo "❌ Apple's container CLI is not installed. Install it from https://github.com/apple/container"
    exit 1
fi

echo "✅ container CLI is installed"
echo
echo "ℹ️  Ollama itself is not containerized here — run it natively on this Mac"
echo "   (the Ollama.app, or 'brew install ollama' + 'ollama serve') and set"
echo "   OLLAMA_HOST=0.0.0.0:11434 so the app container can reach it at the"
echo "   container's host gateway IP (this Mac's containerization network"
echo "   default gateway, e.g. 192.168.64.1 — check yours with:"
echo "   container run --rm alpine:latest sh -c 'cat /etc/resolv.conf')."
echo

# Get user input for configuration
read -p "Enter your Avi Controller Host (e.g., avi-controller.example.com): " AVI_HOST
if [ -z "$AVI_HOST" ]; then
    AVI_HOST="avi-controller.example.com"
    echo "📝 Using default Avi Host: $AVI_HOST"
fi

read -p "Enter your Avi Controller Username (default: admin): " AVI_USERNAME
if [ -z "$AVI_USERNAME" ]; then
    AVI_USERNAME="admin"
    echo "📝 Using default Avi Username: $AVI_USERNAME"
fi

read -s -p "Enter your Avi Controller Password: " AVI_PASSWORD
echo
if [ -z "$AVI_PASSWORD" ]; then
    echo "❌ Avi Password is required"
    exit 1
fi

read -p "Enter Avi Controller Version (default: 31.2.1): " AVI_VERSION
if [ -z "$AVI_VERSION" ]; then
    AVI_VERSION="31.2.1"
    echo "📝 Using default Avi Version: $AVI_VERSION"
fi

read -p "Enter Avi Tenant (default: admin): " AVI_TENANT
if [ -z "$AVI_TENANT" ]; then
    AVI_TENANT="admin"
    echo "📝 Using default Avi Tenant: $AVI_TENANT"
fi

read -p "Enable insecure SSL connection? (y/n, default: n): " AVI_INSECURE
if [ "$AVI_INSECURE" = "y" ] || [ "$AVI_INSECURE" = "Y" ]; then
    AVI_INSECURE="true"
else
    AVI_INSECURE="false"
fi

read -p "Enter application port (default: 8080): " SERVER_PORT
if [ -z "$SERVER_PORT" ]; then
    SERVER_PORT="8080"
    echo "📝 Using default Server Port: $SERVER_PORT"
fi

read -p "Enter log level (info, debug, warn, error, default: info): " LOG_LEVEL
if [ -z "$LOG_LEVEL" ]; then
    LOG_LEVEL="info"
    echo "📝 Using default Log Level: $LOG_LEVEL"
fi

read -p "Enter Ollama host, reachable from the container (default: http://192.168.64.1:11434): " OLLAMA_HOST
if [ -z "$OLLAMA_HOST" ]; then
    OLLAMA_HOST="http://192.168.64.1:11434"
    echo "📝 Using default Ollama Host: $OLLAMA_HOST"
fi

read -p "Enter default Ollama model (default: llama3.2): " OLLAMA_DEFAULT_MODEL
if [ -z "$OLLAMA_DEFAULT_MODEL" ]; then
    OLLAMA_DEFAULT_MODEL="llama3.2"
    echo "📝 Using default Ollama Model: $OLLAMA_DEFAULT_MODEL"
fi

# Check for existing .env file before creating new one
check_env_file

# Create .env file
echo "📝 Creating .env file..."
cat > .env << EOF
# VMware Avi LLM Agent - Ollama Configuration
# Generated by start-ollama.sh

# LLM Provider Configuration
LLM_PROVIDER=ollama

# Ollama Configuration
OLLAMA_HOST=$OLLAMA_HOST
OLLAMA_DEFAULT_MODEL=$OLLAMA_DEFAULT_MODEL
OLLAMA_MODELS=llama3.2,mistral,codellama,llama3.1
OLLAMA_TIMEOUT=60
OLLAMA_TEMPERATURE=0.7
OLLAMA_MAX_TOKENS=2048

# Avi Load Balancer Configuration
AVI_HOST=$AVI_HOST
AVI_USERNAME=$AVI_USERNAME
AVI_PASSWORD=$AVI_PASSWORD
AVI_VERSION=$AVI_VERSION
AVI_TENANT=$AVI_TENANT
AVI_TIMEOUT=30
AVI_INSECURE=$AVI_INSECURE

# Application Configuration
LOG_LEVEL=$LOG_LEVEL
LOG_FORMAT=json
SERVER_PORT=$SERVER_PORT
SERVER_READ_TIMEOUT=30
SERVER_WRITE_TIMEOUT=30
SERVER_IDLE_TIMEOUT=60
EOF

echo "✅ .env file created successfully"
echo

# Start the application
echo "🚀 Starting VMware Avi LLM Agent with Ollama..."
echo "📦 Make sure Ollama is already running natively on this Mac (see above)"
echo
PORT="$SERVER_PORT" ./container-run.sh up

if [ $? -eq 0 ]; then
    echo "✅ Application started successfully!"
    echo
    echo "🌐 Access the application at: http://localhost:$SERVER_PORT"
    echo "📊 Health check endpoint: http://localhost:$SERVER_PORT/api/health"
    echo "💬 API endpoint: http://localhost:$SERVER_PORT/api/chat"
    echo
    echo "🔄 Pull models on the host, not the container: ollama pull llama3.2"
    echo "📋 To list available models, run: ollama list"
    echo
    echo "📋 To stop the application, run: ./container-run.sh down"
    echo "📋 To view logs, run: ./container-run.sh logs"
else
    echo "❌ Failed to start the application"
    exit 1
fi