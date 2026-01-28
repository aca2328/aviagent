#!/bin/bash

# Test script to send a chat message to the Avi LLM Agent
# This tests the Mistral client fix

echo "Testing Avi LLM Agent with chat message..."

# Send a chat request using curl
# This simulates the "show all virtual service" request
curl -X POST "http://localhost:8080/api/chat" \
  -H "Content-Type: application/json" \
  -d '{"message": "show all virtual service", "model": "mistral-medium"}'

echo ""
echo "Test completed."