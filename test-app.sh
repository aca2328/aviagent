#!/bin/bash

# Test script for VMware Avi LLM Agent
# Loads environment variables from .env and runs the application

echo "🚀 Testing VMware Avi LLM Agent with current .env configuration"
echo "================================================================"
echo

# Check if .env file exists
if [ ! -f ".env" ]; then
    echo "❌ .env file not found"
    exit 1
fi

# Load environment variables from .env file
set -a
source .env
set +a

echo "✅ Loaded environment variables:"
echo "  LLM Provider: $LLM_PROVIDER"
echo "  Avi Host: $AVI_HOST"
echo "  Server Port: $SERVER_PORT"
echo "  Log Level: $LOG_LEVEL"
echo

# Check if required variables are set
if [ -z "$LLM_PROVIDER" ]; then
    echo "❌ LLM_PROVIDER not set in .env file"
    exit 1
fi

if [ -z "$AVI_HOST" ]; then
    echo "❌ AVI_HOST not set in .env file"
    exit 1
fi

if [ -z "$AVI_USERNAME" ]; then
    echo "❌ AVI_USERNAME not set in .env file"
    exit 1
fi

if [ -z "$AVI_PASSWORD" ]; then
    echo "❌ AVI_PASSWORD not set in .env file"
    exit 1
fi

# Check if Mistral API key is set when using Mistral
if [ "$LLM_PROVIDER" = "mistral" ] && [ -z "$MISTRAL_API_KEY" ]; then
    echo "❌ MISTRAL_API_KEY not set but LLM_PROVIDER is mistral"
    exit 1
fi

echo "✅ All required environment variables are set"
echo

# Build the application
echo "🔨 Building application..."
export PATH=$PATH:/usr/local/go/bin
go build -o aviagent-test .

if [ $? -ne 0 ]; then
    echo "❌ Failed to build application"
    exit 1
fi

echo "✅ Application built successfully"
echo

# Run the application in the background
echo "🚀 Starting application..."
./aviagent-test -config config.yaml &
APP_PID=$!

echo "✅ Application started with PID: $APP_PID"
echo

# Wait a moment for the application to start
sleep 3

# Test the health endpoint
echo "📊 Testing health endpoint..."
HEALTH_RESPONSE=$(curl -s http://localhost:$SERVER_PORT/api/health)

if [ $? -eq 0 ]; then
    echo "✅ Health endpoint responded:"
    echo "$HEALTH_RESPONSE" | jq .
else
    echo "❌ Failed to connect to health endpoint"
    echo "Response: $HEALTH_RESPONSE"
fi

echo

# Test a simple chat request if health check passed
if [ -n "$HEALTH_RESPONSE" ]; then
    echo "💬 Testing chat endpoint..."
    CHAT_RESPONSE=$(curl -s -X POST http://localhost:$SERVER_PORT/api/chat \
        -H "Content-Type: application/json" \
        -d '{"message": "What is the current time?", "model": "mistral-medium"}')
    
    if [ $? -eq 0 ]; then
        echo "✅ Chat endpoint responded:"
        echo "$CHAT_RESPONSE" | jq .
    else
        echo "❌ Failed to connect to chat endpoint"
        echo "Response: $CHAT_RESPONSE"
    fi
fi

echo

# Clean up
echo "🧹 Cleaning up..."
kill $APP_PID 2>/dev/null
rm -f aviagent-test

echo "✅ Test completed"

echo
if [ -n "$HEALTH_RESPONSE" ]; then
    echo "🎉 Application is working correctly!"
else
    echo "⚠️  Application started but health check failed"
    echo "   This could be due to:"
    echo "   - Avi controller not reachable"
    echo "   - LLM provider not configured correctly"
    echo "   - Network connectivity issues"
fi