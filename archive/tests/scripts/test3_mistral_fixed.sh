#!/bin/bash

# Fixed Test Mistral AI with proper JSON formatting
# Get API key from .env
MISTRAL_API_KEY=$(grep "MISTRAL_API_KEY=" .env | cut -d '=' -f2)

if [ -z "$MISTRAL_API_KEY" ]; then
    echo "❌ Error: MISTRAL_API_KEY not found in .env file"
    exit 1
fi

echo "🧪 Testing Mistral AI with fixed JSON formatting"
echo "=============================================="
echo ""

# Test query for virtual services
echo "🔧 Testing: 'list all virtual services'"
echo "======================================="

curl -s -X POST "https://api.mistral.ai/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $MISTRAL_API_KEY" \
  -d '{
    "model": "mistral-medium",
    "messages": [
      {
        "role": "system",
        "content": "You are an AI assistant specialized in VMware Avi Load Balancer management. You have access to tools that allow you to interact with the Avi Load Balancer API to perform management tasks and retrieve real-time data.\n\nIMPORTANT RULES FOR TOOL USAGE:\n1. ANY request for current system state, real-time data, or specific configurations MUST use the appropriate tool\n2. ANY request that mentions \"show\", \"list\", \"get\", \"display\", \"current\", \"status\", \"health\", \"configuration\" MUST use tools\n3. ANY request about pools, virtual services, health monitors, service engines, or analytics MUST use tools\n4. Do NOT answer questions about the current system state with general knowledge - ALWAYS use tools\n5. If a user asks for data that requires API access, you MUST call the appropriate tool function"
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
              "name": {
                "type": "string",
                "description": "Filter by virtual service name"
              },
              "tenant": {
                "type": "string",
                "description": "Filter by tenant name"
              },
              "enabled": {
                "type": "boolean",
                "description": "Filter by enabled status (true/false)"
              },
              "fields": {
                "type": "string",
                "description": "Comma-separated list of fields to return (name,uuid,enabled,services,pool_ref)"
              }
            }
          }
        }
      }
    ],
    "tool_choice": "auto"
  }' | jq .

echo ""
echo "📊 Test completed. Check for tool_calls in the response."
