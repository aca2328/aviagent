#!/bin/bash

# Comparison script to analyze differences between app requests and test script
# This script compares the logged requests/responses with the working test script

echo "🔍 Mistral Request/Response Comparison Tool"
echo "========================================="
echo ""

# Check if logs directory exists
if [ ! -d "mistral_comparison_logs" ]; then
    echo "❌ No mistral_comparison_logs directory found. Run the app first to generate logs."
    exit 1
fi

# Find the most recent request/response pair
echo "📁 Looking for recent log files..."
RECENT_REQUEST=$(ls -t mistral_comparison_logs/*_request.json 2>/dev/null | head -1)
RECENT_RESPONSE=$(ls -t mistral_comparison_logs/*_response.json 2>/dev/null | head -1)

if [ -z "$RECENT_REQUEST" ] || [ -z "$RECENT_RESPONSE" ]; then
    echo "❌ No recent request/response pair found in mistral_comparison_logs/"
    echo "Available files:"
    ls -la mistral_comparison_logs/
    exit 1
fi

echo "📄 Found recent files:"
echo "  Request:  $RECENT_REQUEST"
echo "  Response: $RECENT_RESPONSE"
echo ""

# Extract key information from the request
echo "🔧 Analyzing APP REQUEST..."
APP_MODEL=$(jq -r '.model' "$RECENT_REQUEST")
APP_TOOL_COUNT=$(jq -r '.tools | length' "$RECENT_REQUEST")
APP_SYSTEM_PROMPT=$(jq -r '.messages[0].content' "$RECENT_REQUEST")
APP_USER_QUERY=$(jq -r '.messages[1].content' "$RECENT_REQUEST")
APP_TOOL_CHOICE=$(jq -r '.tool_choice' "$RECENT_REQUEST")

echo "  Model: $APP_MODEL"
echo "  Tool Count: $APP_TOOL_COUNT"
echo "  Tool Choice: $APP_TOOL_CHOICE"
echo "  User Query: $APP_USER_QUERY"
echo ""

# Extract key information from the test script
echo "🔧 Analyzing TEST SCRIPT..."
TEST_MODEL="mistral-medium"
TEST_TOOL_COUNT=1
TEST_SYSTEM_PROMPT="You are an AI assistant specialized in VMware Avi Load Balancer management. You have access to tools that allow you to interact with the Avi Load Balancer API to perform management tasks and retrieve real-time data.

IMPORTANT RULES FOR TOOL USAGE:
1. ANY request for current system state, real-time data, or specific configurations MUST use the appropriate tool
2. ANY request that mentions \"show\", \"list\", \"get\", \"display\", \"current\", \"status\", \"health\", \"configuration\" MUST use tools
3. ANY request about pools, virtual services, health monitors, service engines, or analytics MUST use tools
4. Do NOT answer questions about the current system state with general knowledge - ALWAYS use tools
5. If a user asks for data that requires API access, you MUST call the appropriate tool function"
TEST_USER_QUERY="list all virtual services"
TEST_TOOL_CHOICE="auto"

echo "  Model: $TEST_MODEL"
echo "  Tool Count: $TEST_TOOL_COUNT"
echo "  Tool Choice: $TEST_TOOL_CHOICE"
echo "  User Query: $TEST_USER_QUERY"
echo ""

# Compare key differences
echo "🔍 COMPARISON RESULTS:"
echo "======================"

# Compare models
if [ "$APP_MODEL" != "$TEST_MODEL" ]; then
    echo "❌ MODEL MISMATCH:"
    echo "   App:     $APP_MODEL"
    echo "   Test:    $TEST_MODEL"
else
    echo "✅ Models match: $APP_MODEL"
fi

# Compare tool counts
if [ "$APP_TOOL_COUNT" != "$TEST_TOOL_COUNT" ]; then
    echo "❌ TOOL COUNT MISMATCH:"
    echo "   App:     $APP_TOOL_COUNT tools"
    echo "   Test:    $TEST_TOOL_COUNT tool"
else
    echo "✅ Tool counts match: $APP_TOOL_COUNT"
fi

# Compare tool choices
if [ "$APP_TOOL_CHOICE" != "$TEST_TOOL_CHOICE" ]; then
    echo "❌ TOOL CHOICE MISMATCH:"
    echo "   App:     $APP_TOOL_CHOICE"
    echo "   Test:    $TEST_TOOL_CHOICE"
else
    echo "✅ Tool choices match: $APP_TOOL_CHOICE"
fi

# Compare system prompts
if [ "$APP_SYSTEM_PROMPT" != "$TEST_SYSTEM_PROMPT" ]; then
    echo "❌ SYSTEM PROMPT DIFFERENCES:"
    echo ""

    # Create temporary files for diff
    echo "$APP_SYSTEM_PROMPT" > /tmp/app_prompt.txt
    echo "$TEST_SYSTEM_PROMPT" > /tmp/test_prompt.txt

    echo "   Differences found:"
    diff -u /tmp/test_prompt.txt /tmp/app_prompt.txt || true

    # Show character counts
    APP_PROMPT_LEN=$(echo "$APP_SYSTEM_PROMPT" | wc -c)
    TEST_PROMPT_LEN=$(echo "$TEST_SYSTEM_PROMPT" | wc -c)
    echo ""
    echo "   App prompt length:     $APP_PROMPT_LEN characters"
    echo "   Test prompt length:    $TEST_PROMPT_LEN characters"
    echo "   Difference:            $((APP_PROMPT_LEN - TEST_PROMPT_LEN)) characters"
else
    echo "✅ System prompts match exactly"
fi

# Analyze the response
echo ""
echo "📊 RESPONSE ANALYSIS:"
echo "====================="

# Check if response contains tool calls
TOOL_CALLS=$(jq -r '.choices[0].message.tool_calls // [] | length' "$RECENT_RESPONSE")
if [ "$TOOL_CALLS" -gt 0 ]; then
    echo "✅ Response contains $TOOL_CALLS tool call(s)"
    echo ""
    echo "Tool calls:"
    jq -r '.choices[0].message.tool_calls[] | "  - Function: \(.function.name)"' "$RECENT_RESPONSE"
else
    echo "❌ Response contains NO tool calls"
    echo ""

    # Check if there's a message instead
    MESSAGE=$(jq -r '.choices[0].message.content // ""' "$RECENT_RESPONSE")
    if [ -n "$MESSAGE" ]; then
        echo "Response message (first 200 chars):"
        echo "${MESSAGE:0:200}..."
    fi
fi

echo ""
echo "🎯 SUMMARY:"
echo "==========="
echo "The comparison shows the key differences between the application's requests"
echo "and the working test script. Focus on:"
echo "1. Model consistency"
echo "2. Tool definitions and count"
echo "3. System prompt exact match"
echo "4. Tool choice setting"
echo ""
echo "Check the individual log files for complete details:"
echo "  $RECENT_REQUEST"
echo "  $RECENT_RESPONSE"