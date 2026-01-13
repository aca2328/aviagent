#!/bin/bash

# Test script for Mistral AI response parsing
# Demonstrates how to use the parse_mistral_response.sh script

echo "=== Mistral AI Response Parsing Test ==="
echo ""

# Test 1: List virtual services
echo "Test 1: List all virtual services"
echo "Command: ./parse_mistral_response.sh \"List all virtual services\" mistral-medium"
echo ""

# Test 2: Get specific virtual service
echo "Test 2: Get specific virtual service"
echo "Command: ./parse_mistral_response.sh \"Get details about virtual service vs-1\" mistral-medium"
echo ""

# Test 3: List pools
echo "Test 3: List pools with health status"
echo "Command: ./parse_mistral_response.sh \"Show me all pools with their health status\" mistral-medium"
echo ""

# Test 4: Complex query
echo "Test 4: Complex query with multiple tools"
echo "Command: ./parse_mistral_response.sh \"List all virtual services and their associated pools\" mistral-medium"
echo ""

echo "=== Test Script Complete ==="
echo "Run individual tests to see the complete Mistral AI responses with tool calls"