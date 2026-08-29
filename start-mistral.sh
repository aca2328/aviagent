#!/bin/bash

# Function to check for existing .env file and handle user confirmation
check_env_file() {
    if [ -f ".env" ]; then
        echo "⚠️  An existing .env file was detected!"
        echo
        echo "Current .env file contents:"
        echo "----------------------------------------"
        # Show relevant lines from existing .env file
        grep -E "^LLM_PROVIDER|^MISTRAL_API_KEY|^AVI_HOST|^AVI_USERNAME" .env || echo "(No matching configuration found)"
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
                echo "📝 Starting VMware Avi LLM Agent with Mistral AI..."
                echo "📝 Using Mistral AI cloud service (no local Ollama required)"
                echo
                docker-compose --env-file .env up -d --scale ollama=0
                
                if [ $? -eq 0 ]; then
                    echo "✅ Application started successfully with existing configuration!"
                    echo
                    # Extract port from .env or use default
                    LOCAL_PORT=$(grep -E "^SERVER_PORT=" .env | cut -d'=' -f2 || echo "8080")
                    echo "🌐 Access the application at: http://localhost:$LOCAL_PORT"
                    echo "📊 Health check endpoint: http://localhost:$LOCAL_PORT/api/health"
                    echo "💬 API endpoint: http://localhost:$LOCAL_PORT/api/chat"
                    echo
                    echo "📋 To stop the application, run: docker-compose down"
                    echo "📋 To view logs, run: docker-compose logs -f avi-llm-agent"
                else
                    echo "❌ Failed to start the application with existing configuration"
                fi
                exit 0
                ;;
            *)
                echo "🔴 Operation cancelled. Existing .env file preserved."
                echo "📋 To use the existing configuration later, run: docker-compose --env-file .env up -d --scale ollama=0"
                exit 0
                ;;
        esac
=======
    fi
    return 0
}

# VMware Avi LLM Agent - Mistral AI Startup Script
# This script creates a .env file and starts the application with Mistral AI

echo "🚀 VMware Avi LLM Agent - Mistral AI Setup"
echo "========================================"
echo

# Check if docker and docker-compose are installed
if ! command -v docker &> /dev/null; then
    echo "❌ Docker is not installed. Please install Docker first."
    exit 1
fi

if ! command -v docker-compose &> /dev/null; then
    echo "❌ Docker Compose is not installed. Please install Docker Compose first."
    exit 1
fi

echo "✅ Docker and Docker Compose are installed"
echo

# Get user input for configuration
read -p "Enter your Mistral AI API Key: " MISTRAL_API_KEY
if [ -z "$MISTRAL_API_KEY" ]; then
    echo "❌ Mistral API Key is required"
    exit 1
fi

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

# Check for existing .env file before creating new one
check_env_file

# Create .env file
echo "📝 Creating .env file..."
cat > .env << EOF
# VMware Avi LLM Agent - Mistral AI Configuration
# Generated by start-mistral.sh

# LLM Provider Configuration
# The app's config validation only accepts "python" or "ollama" as literal
# values here, even though this is the Mistral setup path — "python" refers
# to the provider implementation (a Python bridge to the Mistral SDK), not
# the LLM itself. Setting this to "mistral" fails startup.
LLM_PROVIDER=python

# Mistral AI Configuration
MISTRAL_API_KEY=$MISTRAL_API_KEY
MISTRAL_API_BASE_URL=https://api.mistral.ai
MISTRAL_DEFAULT_MODEL=mistral-medium
MISTRAL_MODELS=mistral-tiny,mistral-small,mistral-medium,mistral-large
MISTRAL_TIMEOUT=60
MISTRAL_TEMPERATURE=0.7
MISTRAL_MAX_TOKENS=2048

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
echo "🚀 Starting VMware Avi LLM Agent with Mistral AI..."
echo "📝 Using Mistral AI cloud service (no local Ollama required)"
echo
docker-compose --env-file .env up -d --scale ollama=0

if [ $? -eq 0 ]; then
    echo "✅ Application started successfully!"
    echo
    echo "🌐 Access the application at: http://localhost:$SERVER_PORT"
    echo "📊 Health check endpoint: http://localhost:$SERVER_PORT/api/health"
    echo "💬 API endpoint: http://localhost:$SERVER_PORT/api/chat"
    echo
    echo "📋 To stop the application, run: docker-compose down"
    echo "📋 To view logs, run: docker-compose logs -f avi-llm-agent"
else
    echo "❌ Failed to start the application"
    exit 1
fi