package llm

import (
	"fmt"
)

// GetAviToolDefinitions returns the tool definitions for Avi Load Balancer API functions
func GetAviToolDefinitions() []Tool {
	return []Tool{
		// Virtual Service Operations
		{
			Type: "function",
			Function: Function{
				Name:        "list_virtual_services",
				Description: "List all virtual services with optional filtering. Use this when users ask to see, list, or get information about virtual services.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Filter by virtual service name",
						},
						"tenant": map[string]interface{}{
							"type":        "string",
							"description": "Filter by tenant name",
						},
						"enabled": map[string]interface{}{
							"type":        "boolean",
							"description": "Filter by enabled status (true/false)",
						},
						"fields": map[string]interface{}{
							"type":        "string",
							"description": "Comma-separated list of fields to return (name,uuid,enabled,services,pool_ref)",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: Function{
				Name:        "get_virtual_service",
				Description: "Get details of a specific virtual service by UUID or name. Use this when users ask for detailed information about a specific virtual service.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"uuid": map[string]interface{}{
							"type":        "string",
							"description": "UUID of the virtual service (required)",
						},
						"fields": map[string]interface{}{
							"type":        "string",
							"description": "Comma-separated list of fields to return",
						},
					},
					"required": []string{"uuid"},
				},
			},
		},
		{
			Type: "function",
			Function: Function{
				Name:        "create_virtual_service",
				Description: "Create a new virtual service. Use this when users want to create or set up a new virtual service.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Name of the virtual service (required)",
						},
						"services": map[string]interface{}{
							"type":        "array",
							"description": "List of services with port and SSL configuration",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"port": map[string]interface{}{
										"type":        "integer",
										"description": "Service port number (80, 443, etc.)",
									},
									"enable_ssl": map[string]interface{}{
										"type":        "boolean",
										"description": "Enable SSL for this service",
									},
								},
							},
						},
						"pool_ref": map[string]interface{}{
							"type":        "string",
							"description": "Reference to the backend pool",
						},
						"vsvip_ref": map[string]interface{}{
							"type":        "string",
							"description": "Reference to the virtual service VIP",
						},
					},
					"required": []string{"name"},
				},
			},
		},
		{
			Type: "function",
			Function: Function{
				Name:        "update_virtual_service",
				Description: "Update an existing virtual service. Use this when users want to modify or change virtual service configuration.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"uuid": map[string]interface{}{
							"type":        "string",
							"description": "UUID of the virtual service to update (required)",
						},
						"name": map[string]interface{}{
							"type":        "string",
							"description": "New name for the virtual service",
						},
						"enabled": map[string]interface{}{
							"type":        "boolean",
							"description": "Enable or disable the virtual service",
						},
						"services": map[string]interface{}{
							"type":        "array",
							"description": "Updated list of services",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"port": map[string]interface{}{
										"type": "integer",
									},
									"enable_ssl": map[string]interface{}{
										"type": "boolean",
									},
								},
							},
						},
					},
					"required": []string{"uuid"},
				},
			},
		},
		{
			Type: "function",
			Function: Function{
				Name:        "delete_virtual_service",
				Description: "Delete a virtual service. Use this when users want to remove or delete a virtual service.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"uuid": map[string]interface{}{
							"type":        "string",
							"description": "UUID of the virtual service to delete (required)",
						},
					},
					"required": []string{"uuid"},
				},
			},
		},

		// Pool Operations
		{
			Type: "function",
			Function: Function{
				Name:        "list_pools",
				Description: "List all pools with optional filtering. Use this when users ask about backend pools, server pools, or load balancing pools.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Filter by pool name",
						},
						"enabled": map[string]interface{}{
							"type":        "boolean",
							"description": "Filter by enabled status",
						},
						"health_status": map[string]interface{}{
							"type":        "string",
							"description": "Filter by health status (up, down, partial)",
						},
						"fields": map[string]interface{}{
							"type":        "string",
							"description": "Comma-separated list of fields to return",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: Function{
				Name:        "get_pool",
				Description: "Get details of a specific pool by UUID. Use this for detailed pool information including servers and health status.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"uuid": map[string]interface{}{
							"type":        "string",
							"description": "UUID of the pool (required)",
						},
						"fields": map[string]interface{}{
							"type":        "string",
							"description": "Comma-separated list of fields to return",
						},
					},
					"required": []string{"uuid"},
				},
			},
		},
		{
			Type: "function",
			Function: Function{
				Name:        "create_pool",
				Description: "Create a new pool with backend servers. Use this when users want to create a new server pool or backend pool.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Name of the pool (required)",
						},
						"servers": map[string]interface{}{
							"type":        "array",
							"description": "List of backend servers",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"ip": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"addr": map[string]interface{}{
												"type":        "string",
												"description": "IP address of the server",
											},
											"type": map[string]interface{}{
												"type":        "string",
												"description": "IP address type (V4, V6, DNS)",
												"default":     "V4",
											},
										},
										"required": []string{"addr", "type"},
									},
									"port": map[string]interface{}{
										"type":        "integer",
										"description": "Server port number",
									},
									"enabled": map[string]interface{}{
										"type":        "boolean",
										"description": "Server enabled status",
										"default":     true,
									},
								},
								"required": []string{"ip"},
							},
						},
						"default_server_port": map[string]interface{}{
							"type":        "integer",
							"description": "Default port for servers",
							"default":     80,
						},
						"lb_algorithm": map[string]interface{}{
							"type":        "string",
							"description": "Load balancing algorithm (LB_ALGORITHM_ROUND_ROBIN, LB_ALGORITHM_LEAST_CONNECTIONS, LB_ALGORITHM_FASTEST_RESPONSE)",
							"default":     "LB_ALGORITHM_LEAST_CONNECTIONS",
						},
					},
					"required": []string{"name"},
				},
			},
		},
		{
			Type: "function",
			Function: Function{
				Name:        "scale_out_pool",
				Description: "Scale out a pool by adding more servers. Use this when users want to add capacity or scale out backend servers.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"uuid": map[string]interface{}{
							"type":        "string",
							"description": "UUID of the pool to scale out (required)",
						},
						"reason": map[string]interface{}{
							"type":        "string",
							"description": "Reason for scaling out",
							"default":     "Manual scale out operation",
						},
					},
					"required": []string{"uuid"},
				},
			},
		},
		{
			Type: "function",
			Function: Function{
				Name:        "scale_in_pool",
				Description: "Scale in a pool by removing servers. Use this when users want to reduce capacity or scale in backend servers.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"uuid": map[string]interface{}{
							"type":        "string",
							"description": "UUID of the pool to scale in (required)",
						},
						"servers": map[string]interface{}{
							"type":        "array",
							"description": "List of servers to remove",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"ip": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"addr": map[string]interface{}{
												"type": "string",
											},
											"type": map[string]interface{}{
												"type":    "string",
												"default": "V4",
											},
										},
									},
									"port": map[string]interface{}{
										"type": "integer",
									},
								},
							},
						},
						"reason": map[string]interface{}{
							"type":        "string",
							"description": "Reason for scaling in",
							"default":     "Manual scale in operation",
						},
					},
					"required": []string{"uuid"},
				},
			},
		},

		// Health Monitor Operations
		{
			Type: "function",
			Function: Function{
				Name:        "list_health_monitors",
				Description: "List all health monitors. Use this when users ask about health checks, monitoring, or health status.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Filter by health monitor name",
						},
						"type": map[string]interface{}{
							"type":        "string",
							"description": "Filter by health monitor type (HEALTH_MONITOR_HTTP, HEALTH_MONITOR_HTTPS, HEALTH_MONITOR_TCP, etc.)",
						},
						"fields": map[string]interface{}{
							"type":        "string",
							"description": "Comma-separated list of fields to return",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: Function{
				Name:        "get_health_monitor",
				Description: "Get details of a specific health monitor by UUID. Use this for detailed health monitor configuration.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"uuid": map[string]interface{}{
							"type":        "string",
							"description": "UUID of the health monitor (required)",
						},
						"fields": map[string]interface{}{
							"type":        "string",
							"description": "Comma-separated list of fields to return",
						},
					},
					"required": []string{"uuid"},
				},
			},
		},

		// Service Engine Operations
		{
			Type: "function",
			Function: Function{
				Name:        "list_service_engines",
				Description: "List all service engines. Use this when users ask about service engines, load balancer instances, or data plane components.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Filter by service engine name",
						},
						"se_group_ref": map[string]interface{}{
							"type":        "string",
							"description": "Filter by service engine group reference",
						},
						"fields": map[string]interface{}{
							"type":        "string",
							"description": "Comma-separated list of fields to return",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: Function{
				Name:        "get_service_engine",
				Description: "Get details of a specific service engine by UUID. Use this for detailed service engine information.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"uuid": map[string]interface{}{
							"type":        "string",
							"description": "UUID of the service engine (required)",
						},
						"fields": map[string]interface{}{
							"type":        "string",
							"description": "Comma-separated list of fields to return",
						},
					},
					"required": []string{"uuid"},
				},
			},
		},

		// Analytics Operations
		{
			Type: "function",
			Function: Function{
				Name:        "get_analytics",
				Description: "Get analytics and metrics data for virtual services, pools, or service engines. Use this when users ask about performance, metrics, statistics, or analytics data.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"resource_type": map[string]interface{}{
							"type":        "string",
							"description": "Type of resource (virtualservice, pool, serviceengine) (required)",
							"enum":        []string{"virtualservice", "pool", "serviceengine"},
						},
						"uuid": map[string]interface{}{
							"type":        "string",
							"description": "UUID of the resource (required)",
						},
						"metric": map[string]interface{}{
							"type":        "string",
							"description": "Specific metric to retrieve (connections, throughput, latency, errors)",
						},
						"time_range": map[string]interface{}{
							"type":        "string",
							"description": "Time range for metrics (1h, 6h, 24h, 7d)",
							"default":     "1h",
						},
					},
					"required": []string{"resource_type", "uuid"},
				},
			},
		},

		// Generic Operations
		{
			Type: "function",
			Function: Function{
				Name:        "execute_generic_operation",
				Description: "Execute a generic API operation when specific tools don't cover the user's request. Use this as a fallback for advanced or specific API calls.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"method": map[string]interface{}{
							"type":        "string",
							"description": "HTTP method (GET, POST, PUT, DELETE, PATCH) (required)",
							"enum":        []string{"GET", "POST", "PUT", "DELETE", "PATCH"},
						},
						"endpoint": map[string]interface{}{
							"type":        "string",
							"description": "API endpoint path (e.g., /virtualservice, /pool/uuid/scaleout) (required)",
						},
						"body": map[string]interface{}{
							"type":        "object",
							"description": "Request body for POST/PUT/PATCH operations",
						},
						"parameters": map[string]interface{}{
							"type":        "object",
							"description": "Query parameters as key-value pairs",
						},
					},
					"required": []string{"method", "endpoint"},
				},
			},
		},
	}
}

// GetToolByName returns a tool definition by name
func GetToolByName(name string) (*Tool, error) {
	tools := GetAviToolDefinitions()
	for _, tool := range tools {
		if tool.Function.Name == name {
			return &tool, nil
		}
	}
	return nil, fmt.Errorf("tool '%s' not found", name)
}

// GetToolNames returns a list of all available tool names
func GetToolNames() []string {
	tools := GetAviToolDefinitions()
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Function.Name
	}
	return names
}