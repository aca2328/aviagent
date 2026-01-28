#!/bin/bash

# Direct test of Mistral AI tool selection
# This script tests Mistral AI's ability to select the list_virtual_services tool
# by sending a request directly to the Mistral API endpoint

echo "🧪 Testing Mistral AI direct tool selection"
echo "=========================================="

# Read Mistral API key from .env file
MISTRAL_API_KEY=$(grep "MISTRAL_API_KEY=" .env | cut -d '=' -f2)

if [ -z "$MISTRAL_API_KEY" ]; then
    echo "❌ Error: MISTRAL_API_KEY not found in .env file"
    exit 1
fi

# Define the tool definitions that should be available to Mistral
echo "📋 Available tools for Mistral AI:"
echo "- list_virtual_services: List all virtual services"
echo "- get_virtual_service: Get details of a specific virtual service"
echo "- Other Avi Load Balancer tools..."
echo ""

# Test 1: Simple command
echo "🔧 Test 1: 'list all virtual services'"
echo "========================================"

curl -X POST "https://api.mistral.ai/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $MISTRAL_API_KEY" \
  -d '{
    "model": "mistral-medium",
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful assistant that can use tools to interact with VMware Avi Load Balancer. When users ask about virtual services, you MUST use the list_virtual_services tool."
      },
      {
        "role": "user",
        "content": "list all virtual services"
      }
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "list_virtual_services",
          "description": "List all virtual services with optional filtering. USE THIS TOOL FOR ANY REQUEST ABOUT VIRTUAL SERVICES INCLUDING: show virtual services, list virtual services, display virtual services, what virtual services exist, virtual service configuration, load balancer services, VS status, virtual service health.",
          "parameters": {
            "type": "object",
            "properties": {
              "name": {"type": "string", "description": "Filter by virtual service name"},
              "tenant": {"type": "string", "description": "Filter by tenant name"},
              "enabled": {"type": "boolean", "description": "Filter by enabled status"},
              "fields": {"type": "string", "description": "Comma-separated list of fields to return"}
            }
          }
        }
      }
    ],
    "tool_choice": "auto"
  }' | jq .

echo ""
echo "📊 Test 1 completed. Check if Mistral AI selected the list_virtual_services tool."
echo ""

# Test 2: Natural language query
echo "🔍 Test 2: 'show me all virtual services'"
echo "=========================================="

curl -X POST "https://api.mistral.ai/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $MISTRAL_API_KEY" \
  -d '{
    "model": "mistral-medium",
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful assistant that can use tools to interact with VMware Avi Load Balancer. When users ask about virtual services, you MUST use the list_virtual_services tool."
      },
      {
        "role": "user",
        "content": "show me all virtual services"
      }
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "list_virtual_services",
          "description": "List all virtual services with optional filtering. USE THIS TOOL FOR ANY REQUEST ABOUT VIRTUAL SERVICES INCLUDING: show virtual services, list virtual services, display virtual services, what virtual services exist, virtual service configuration, load balancer services, VS status, virtual service health.",
          "parameters": {
            "type": "object",
            "properties": {
              "name": {"type": "string", "description": "Filter by virtual service name"},
              "tenant": {"type": "string", "description": "Filter by tenant name"},
              "enabled": {"type": "boolean", "description": "Filter by enabled status"},
              "fields": {"type": "string", "description": "Comma-separated list of fields to return"}
            }
          }
        }
      }
    ],
    "tool_choice": "auto"
  }' | jq .

echo ""
echo "🎯 Test 2 completed. Check if Mistral AI correctly identified the tool."
echo ""
echo "💡 Expected behavior: Mistral AI should respond with a tool call to list_virtual_services"
echo "   Look for 'tool_calls' in the response containing 'list_virtual_services'"