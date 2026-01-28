#!/bin/bash

# Test Avi LLM Agent with all 16 tools via the application API
# This script tests the complete application flow by sending a request to the local app
# which then communicates with Mistral AI

echo "🧪 Testing Avi LLM Agent with all 16 tools via application API"
echo "=============================================================="
echo ""

# Test query for virtual services through the application API
echo "🔧 Testing: 'list all virtual services' via local app"
echo "======================================================"

curl -s -X POST "http://localhost:8080/api/chat" \
  -H "Content-Type: application/json" \
  -d '{
    "message": "list all virtual services",
    "model": "mistral-medium"
  }' | jq .

echo ""
echo "📊 Test completed. The Avi LLM Agent should have:"
echo "   1. Received the request"
echo "   2. Sent it to Mistral AI with all 16 tools"
echo "   3. Received a tool call response from Mistral"
echo "   4. Executed the list_virtual_services tool"
echo "   5. Returned the virtual services data"
echo ""
echo "💡 Look for the virtual services data in the response."
echo "   The response should contain actual virtual service information from the Avi controller."