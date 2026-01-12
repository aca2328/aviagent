#!/bin/bash

# Test Mistral AI with all 16 tools available
# This script tests if Mistral AI can correctly select the appropriate tool
# when given access to the complete set of Avi Load Balancer tools

# Get API key from .env
MISTRAL_API_KEY=$(grep "MISTRAL_API_KEY=" .env | cut -d '=' -f2)

if [ -z "$MISTRAL_API_KEY" ]; then
    echo "❌ Error: MISTRAL_API_KEY not found in .env file"
    exit 1
fi

echo "🧪 Testing Mistral AI with all 16 Avi Load Balancer tools"
echo "========================================================"
echo ""

# Test query for virtual services
echo "🔧 Testing: 'list all virtual services'"
echo "========================================"

curl -s -X POST "https://api.mistral.ai/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $MISTRAL_API_KEY" \
  -d '{
    "model": "mistral-medium",
    "messages": [
      {
        "role": "system",
        "content": 'You are an AI assistant specialized in VMware Avi Load Balancer management. You have access to tools that allow you to interact with the Avi Load Balancer API to perform management tasks and retrieve real-time data.
       IMPORTANT RULES FOR TOOL USAGE:
       1. ANY request for current system state, real-time data, or specific configurations MUST use the appropriate tool
       2. ANY request that mentions "show", "list", "get", "display", "current", "status", "health", "configuration" MUST use tools
       3. ANY request about pools, virtual services, health monitors, service engines, or analytics MUST use tools
       4. Do NOT answer questions about the current system state with general knowledge - ALWAYS use tools
       5. If a user asks for data that requires API access, you MUST call the appropriate tool function
       EXAMPLES THAT REQUIRE TOOLS:
       - "Show me all pools" -> Use list_pools tool
       - "List virtual services" -> Use list_virtual_services tool
       - "Show me all pools with their health status" -> Use list_pools tool with health_status parameter
       - "Get details about virtual service XYZ" -> Use get_virtual_service tool
       - "What pools are configured?" -> Use list_pools tool
       - "Show me service engine status" -> Use list_service_engines tool
       EXAMPLES THAT DON'T REQUIRE TOOLS:
       - "What is a virtual service?" (general knowledge)
       - "How do I configure a pool?" (general guidance)
       - "What are the benefits of load balancing?" (conceptual question)
       When using tools, always explain what you're doing and provide context for the results. If you need to use multiple tools, call them sequentially. Always be helpful, clear, and provide detailed context for your responses.'},
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
      },
      {
        "type": "function",
        "function": {
          "name": "get_virtual_service",
          "description": "Get details of a specific virtual service by UUID or name.",
          "parameters": {
            "type": "object",
            "properties": {
              "uuid": {"type": "string", "description": "UUID of the virtual service"},
              "fields": {"type": "string", "description": "Comma-separated list of fields to return"}
            },
            "required": ["uuid"]
          }
        }
      },
      {
        "type": "function",
        "function": {
          "name": "create_virtual_service",
          "description": "Create a new virtual service.",
          "parameters": {
            "type": "object",
            "properties": {
              "name": {"type": "string", "description": "Name of the virtual service"},
              "services": {"type": "array", "description": "List of services"},
              "pool_ref": {"type": "string", "description": "Reference to the backend pool"},
              "vsvip_ref": {"type": "string", "description": "Reference to the virtual service VIP"}
            },
            "required": ["name"]
          }
        }
      },
      {
        "type": "function",
        "function": {
          "name": "update_virtual_service",
          "description": "Update an existing virtual service.",
          "parameters": {
            "type": "object",
            "properties": {
              "uuid": {"type": "string", "description": "UUID of the virtual service"},
              "name": {"type": "string", "description": "New name for the virtual service"},
              "enabled": {"type": "boolean", "description": "Enable or disable the virtual service"},
              "services": {"type": "array", "description": "Updated list of services"}
            },
            "required": ["uuid"]
          }
        }
      },
      {
        "type": "function",
        "function": {
          "name": "delete_virtual_service",
          "description": "Delete a virtual service.",
          "parameters": {
            "type": "object",
            "properties": {
              "uuid": {"type": "string", "description": "UUID of the virtual service"}
            },
            "required": ["uuid"]
          }
        }
      },
      {
        "type": "function",
        "function": {
          "name": "list_pools",
          "description": "List all pools with optional filtering.",
          "parameters": {
            "type": "object",
            "properties": {
              "name": {"type": "string", "description": "Filter by pool name"},
              "enabled": {"type": "boolean", "description": "Filter by enabled status"},
              "health_status": {"type": "string", "description": "Filter by health status"},
              "fields": {"type": "string", "description": "Comma-separated list of fields to return"}
            }
          }
        }
      },
      {
        "type": "function",
        "function": {
          "name": "get_pool",
          "description": "Get details of a specific pool by UUID.",
          "parameters": {
            "type": "object",
            "properties": {
              "uuid": {"type": "string", "description": "UUID of the pool"},
              "fields": {"type": "string", "description": "Comma-separated list of fields to return"}
            },
            "required": ["uuid"]
          }
        }
      },
      {
        "type": "function",
        "function": {
          "name": "create_pool",
          "description": "Create a new pool with backend servers.",
          "parameters": {
            "type": "object",
            "properties": {
              "name": {"type": "string", "description": "Name of the pool"},
              "servers": {"type": "array", "description": "List of backend servers"},
              "default_server_port": {"type": "integer", "description": "Default port for servers"},
              "lb_algorithm": {"type": "string", "description": "Load balancing algorithm"}
            },
            "required": ["name"]
          }
        }
      },
      {
        "type": "function",
        "function": {
          "name": "scale_out_pool",
          "description": "Scale out a pool by adding more servers.",
          "parameters": {
            "type": "object",
            "properties": {
              "uuid": {"type": "string", "description": "UUID of the pool"},
              "reason": {"type": "string", "description": "Reason for scaling out"}
            },
            "required": ["uuid"]
          }
        }
      },
      {
        "type": "function",
        "function": {
          "name": "scale_in_pool",
          "description": "Scale in a pool by removing servers.",
          "parameters": {
            "type": "object",
            "properties": {
              "uuid": {"type": "string", "description": "UUID of the pool"},
              "servers": {"type": "array", "description": "List of servers to remove"},
              "reason": {"type": "string", "description": "Reason for scaling in"}
            },
            "required": ["uuid"]
          }
        }
      },
      {
        "type": "function",
        "function": {
          "name": "list_health_monitors",
          "description": "List all health monitors.",
          "parameters": {
            "type": "object",
            "properties": {
              "name": {"type": "string", "description": "Filter by health monitor name"},
              "type": {"type": "string", "description": "Filter by health monitor type"},
              "fields": {"type": "string", "description": "Comma-separated list of fields to return"}
            }
          }
        }
      },
      {
        "type": "function",
        "function": {
          "name": "get_health_monitor",
          "description": "Get details of a specific health monitor by UUID.",
          "parameters": {
            "type": "object",
            "properties": {
              "uuid": {"type": "string", "description": "UUID of the health monitor"},
              "fields": {"type": "string", "description": "Comma-separated list of fields to return"}
            },
            "required": ["uuid"]
          }
        }
      },
      {
        "type": "function",
        "function": {
          "name": "list_service_engines",
          "description": "List all service engines.",
          "parameters": {
            "type": "object",
            "properties": {
              "name": {"type": "string", "description": "Filter by service engine name"},
              "se_group_ref": {"type": "string", "description": "Filter by service engine group"},
              "fields": {"type": "string", "description": "Comma-separated list of fields to return"}
            }
          }
        }
      },
      {
        "type": "function",
        "function": {
          "name": "get_service_engine",
          "description": "Get details of a specific service engine by UUID.",
          "parameters": {
            "type": "object",
            "properties": {
              "uuid": {"type": "string", "description": "UUID of the service engine"},
              "fields": {"type": "string", "description": "Comma-separated list of fields to return"}
            },
            "required": ["uuid"]
          }
        }
      },
      {
        "type": "function",
        "function": {
          "name": "get_analytics",
          "description": "Get analytics and metrics data for virtual services, pools, or service engines.",
          "parameters": {
            "type": "object",
            "properties": {
              "resource_type": {"type": "string", "description": "Type of resource", "enum": ["virtualservice", "pool", "serviceengine"]},
              "uuid": {"type": "string", "description": "UUID of the resource"},
              "metric": {"type": "string", "description": "Specific metric to retrieve"},
              "time_range": {"type": "string", "description": "Time range for metrics"}
            },
            "required": ["resource_type", "uuid"]
          }
        }
      },
      {
        "type": "function",
        "function": {
          "name": "execute_generic_operation",
          "description": "Execute a generic API operation when specific tools don\u0027t cover the user\u0027s request.",
          "parameters": {
            "type": "object",
            "properties": {
              "method": {"type": "string", "description": "HTTP method", "enum": ["GET", "POST", "PUT", "DELETE", "PATCH"]},
              "endpoint": {"type": "string", "description": "API endpoint path"},
              "body": {"type": "object", "description": "Request body"},
              "parameters": {"type": "object", "description": "Query parameters"}
            },
            "required": ["method", "endpoint"]
          }
        }
      }
    ],
    "tool_choice": "auto"
  }' | jq .

echo ""
echo "📊 Test completed. Mistral AI should have selected the list_virtual_services tool."
echo "   Look for 'tool_calls' in the response containing 'list_virtual_services'"

echo ""
echo "💡 This test demonstrates Mistral AI's ability to select the correct tool"
echo "   from a comprehensive set of 16 available Avi Load Balancer tools."
