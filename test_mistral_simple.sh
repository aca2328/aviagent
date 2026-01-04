#!/bin/bash

# Simple Mistral AI tool test - minimal version
# Tests if Mistral AI can select the list_virtual_services tool

# Get API key from .env
MISTRAL_API_KEY=$(grep "MISTRAL_API_KEY=" .env | cut -d '=' -f2)

# Simple curl command to test tool selection
echo "Testing Mistral AI tool selection..."

curl -s -X POST "https://api.mistral.ai/v1/chat/completions" \
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