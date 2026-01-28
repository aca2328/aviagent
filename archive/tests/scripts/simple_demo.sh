#!/bin/bash

# Simple demo of Mistral AI response parsing
# Shows how the parsing functions work with sample data

echo "=== Mistral AI Response Parsing Demo ==="
echo ""
echo "Loading sample response from test_response.json..."
echo ""

# Load the sample response
response=$(cat test_response.json)

# Extract components using jq
echo "📄 MISTRAL AI RESPONSE"
echo "====================="
echo "Model: $(echo "$response" | jq -r '.model')"
echo "Timestamp: $(echo "$response" | jq -r '.created')"
echo ""
echo "💬 MESSAGE:"
echo "$(echo "$response" | jq -r '.message')"
echo ""

# Parse tool calls
echo "🔧 TOOL CALLS:"
tool_calls=$(echo "$response" | jq -r '.tool_calls // []')
tool_count=$(echo "$tool_calls" | jq 'length')

if [ "$tool_count" -eq 0 ]; then
    echo "None"
else
    echo "($tool_count total)"
    echo "====================="
    
    for ((i=0; i<tool_count; i++)); do
        tool_call=$(echo "$tool_calls" | jq -r ".[$i]")
        echo "Tool #$((i+1)):"
        echo "  ID: $(echo "$tool_call" | jq -r '.id')"
        echo "  Type: $(echo "$tool_call" | jq -r '.type')"
        echo "  Name: $(echo "$tool_call" | jq -r '.function.name')"
        echo "  Arguments: $(echo "$tool_call" | jq -r '.function.arguments')"
        echo ""
    done
fi

# Parse usage statistics
echo "📊 USAGE STATISTICS:"
echo "====================="
echo "$(echo "$response" | jq '.usage')"
echo ""

echo "=== Demo Complete ==="
echo ""
echo "✅ Successfully parsed Mistral AI response"
echo "✅ Extracted message, tool calls, and usage statistics"
echo "✅ Demonstrated structured output format"