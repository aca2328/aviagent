# Mistral AI API Request Body Comparison

## Executive Summary

This document compares the API request bodies sent through the AviAgent application versus those sent directly to the Mistral endpoint through test scripts. The analysis reveals several key differences in structure, content, and approach.

## 1. Request Structure Comparison

### Direct Test Script Request (from test_mistral_tool_direct.sh)

```json
{
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
}
```

### Application Request (from internal/mistral/mistral-client.go)

The application constructs requests with these key characteristics:

1. **Enhanced System Prompt**: Uses a comprehensive system prompt with explicit tool usage rules
2. **Tool Selection Logic**: Implements `determineBestToolForQuery()` to map queries to specific tools
3. **Forced Tool Usage**: For certain query patterns, forces specific tool selection
4. **Multiple Tools**: Includes all available Avi tools (virtual services, pools, health monitors, etc.)
5. **Query Analysis**: Analyzes queries to determine if they should force tool usage

## 2. Key Differences

### System Prompt

**Test Script**: Simple, single-purpose system prompt focused on virtual services only.

**Application**: Comprehensive system prompt with:
- 5 explicit rules for when tools MUST be used
- Specific examples of queries requiring tools
- Examples of queries that don't require tools
- Clear guidance on tool selection criteria

### Tool Definitions

**Test Script**: Only includes `list_virtual_services` tool.

**Application**: Includes all Avi tools:
- Virtual Service operations (list, get, create, update, delete)
- Pool operations (list, get, create, scale out/in)
- Health Monitor operations (list, get)
- Service Engine operations (list, get)
- Analytics operations (get analytics)
- Generic operations (execute_generic_operation)

### Tool Selection Strategy

**Test Script**: Uses `"tool_choice": "auto"` - lets Mistral decide.

**Application**: Uses sophisticated logic:
1. Analyzes query for specific patterns
2. Forces tool usage for queries containing: "list", "show", "health status", "current status", "status"
3. Uses `determineBestToolForQuery()` to map queries to specific tools
4. Enhances system message with explicit tool usage requirements when forcing tools

### Query Processing

**Test Script**: Direct pass-through of user query.

**Application**: Multi-step processing:
1. Query analysis for tool forcing patterns
2. System message enhancement for forced tool usage
3. Conversation history integration
4. Comprehensive logging and flow tracking

## 3. Technical Implementation Differences

### Request Construction

**Test Script**: Simple JSON object with minimal fields.

**Application**: Complex request construction with:
- Message alternation validation
- Conversation history management
- Tool conversion between different formats
- Comprehensive error handling
- Detailed logging at each step

### Error Handling

**Test Script**: Basic error handling through curl/jq.

**Application**: Robust error handling with:
- Flow tracking and error logging
- Type conversion validation
- Parameter validation
- Context timeout management
- Retry logic for failed requests

## 4. Performance Implications

### Test Script
- **Pros**: Simple, fast, minimal overhead
- **Cons**: No conversation context, limited tool availability, no query analysis

### Application
- **Pros**: Comprehensive tool support, query analysis, conversation context, robust error handling
- **Cons**: Higher overhead due to analysis and logging, more complex processing

## 5. Recommendations

### For Test Scripts
1. **Adopt Application's System Prompt**: Use the comprehensive system prompt for better tool selection
2. **Include More Tools**: Add all relevant Avi tools for complete functionality
3. **Add Query Analysis**: Implement basic query pattern matching for better tool selection
4. **Add Error Handling**: Include proper error handling and retry logic

### For Application
1. **Performance Optimization**: Consider caching tool definitions and system prompts
2. **Simplified Testing Mode**: Add a simplified mode for testing that bypasses some analysis
3. **Tool Selection Debugging**: Add more detailed logging for tool selection decisions
4. **Configuration Options**: Allow configuration of tool selection strategy (auto vs. forced)

## 6. Example Application Request

Here's what an application request looks like when forcing tool usage:

```json
{
  "model": "mistral-medium",
  "messages": [
    {
      "role": "system",
      "content": "You are an AI assistant specialized in VMware Avi Load Balancer management... *** IMPORTANT: THIS QUERY REQUIRES TOOL USAGE - DO NOT ANSWER WITHOUT USING TOOLS ***"
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
        "description": "List all virtual services with optional filtering...",
        "parameters": {...}
      }
    },
    {
      "type": "function",
      "function": {
        "name": "get_virtual_service",
        "description": "Get details of a specific virtual service...",
        "parameters": {...}
      }
    },
    // ... all other Avi tools
  ],
  "tool_choice": {
    "type": "function",
    "function": {
      "name": "list_virtual_services"
    }
  },
  "temperature": 0.7,
  "max_tokens": 4096
}
```

## 7. Conclusion

The application provides a much more sophisticated and robust approach to Mistral API requests, with comprehensive tool support, advanced query analysis, and robust error handling. Test scripts should adopt some of these improvements for better reliability and functionality, while the application could benefit from performance optimizations and simplified testing modes.