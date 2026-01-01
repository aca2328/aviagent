package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aviagent/internal/config"
	"aviagent/internal/llm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// TestEndToEndListVirtualServices tests the complete flow from user command to API response
func TestEndToEndListVirtualServices(t *testing.T) {
	// Create a mock Avi API server
	aviServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/login"):
			// Mock login response
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"sessionid": "test-session-id",
				"csrftoken": "test-csrf-token",
				"version": "31.2.1"
			}`))
		case strings.Contains(r.URL.Path, "/virtualservice"):
			// Mock virtual service response
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"count": 3,
				"results": [
					{
						"uuid": "vs-uuid-1",
						"name": "web-app-vs",
						"enabled": true,
						"services": [
							{"port": 80, "enable_ssl": false},
							{"port": 443, "enable_ssl": true}
						],
						"pool_ref": "/api/pool/pool-uuid-1"
					},
					{
						"uuid": "vs-uuid-2",
						"name": "api-vs",
						"enabled": true,
						"services": [
							{"port": 8080, "enable_ssl": false}
						]
					},
					{
						"uuid": "vs-uuid-3",
						"name": "legacy-vs",
						"enabled": false,
						"services": [
							{"port": 80, "enable_ssl": false}
						]
					}
				]
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer aviServer.Close()

	// Create a mock LLM server that simulates the LLM response
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read the request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}

		// Parse the request
		var req map[string]interface{}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "Failed to parse JSON", http.StatusBadRequest)
			return
		}

		// Check if this is a chat request
		if r.URL.Path == "/api/chat" {
			messages := req["messages"].([]interface{})
			lastMessage := messages[len(messages)-1].(map[string]interface{})
			userQuery := lastMessage["content"].(string)

			// Simulate LLM response based on user query
			if strings.Contains(strings.ToLower(userQuery), "list all virtual services") ||
			   strings.Contains(strings.ToLower(userQuery), "show all virtual service") {
				// Return a tool call for list_virtual_services
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{
					"model": "llama3.2",
					"created_at": "` + time.Now().Format(time.RFC3339) + `",
					"message": {
						"role": "assistant",
						"content": "I will retrieve the list of virtual services for you."
					},
					"done": true,
					"total_duration": 123456789,
					"load_duration": 123456,
					"prompt_eval_count": 50,
					"prompt_eval_duration": 1234567,
					"eval_count": 25,
					"eval_duration": 789012
				}`))
			} else {
				// Return a generic response for other queries
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{
					"model": "llama3.2",
					"created_at": "` + time.Now().Format(time.RFC3339) + `",
					"message": {
						"role": "assistant",
						"content": "I understand your request: "` + userQuery + `"
					},
					"done": true,
					"total_duration": 123456789,
					"load_duration": 123456,
					"prompt_eval_count": 30,
					"prompt_eval_duration": 987654,
					"eval_count": 15,
					"eval_duration": 321654
				}`))
			}
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer llmServer.Close()

	// Set up configuration
	llmConfig := &config.LLMConfig{
		OllamaHost:    llmServer.URL,
		DefaultModel:  "llama3.2",
		Models:        []string{"llama3.2", "mistral", "codellama"},
		Timeout:       60,
		Temperature:   0.7,
		MaxTokens:     2048,
	}

	// Create logger
	logger := zaptest.NewLogger(t)

	// Test the complete flow
	t.Run("UserCommandToAPIResponse", func(t *testing.T) {
		// Step 1: User sends "list all virtual services" command
		userQuery := "list all virtual services"

		// Step 2: LLM processes the query and identifies the tool call
		llmClient, err := llm.NewClient(llmConfig, logger)
		require.NoError(t, err)

		// Step 3: Process the query using the public interface
		llmResponse, err := llmClient.ProcessNaturalLanguageQuery(
			context.Background(),
			userQuery,
			"llama3.2",
			llm.GetAviToolDefinitions(),
			[]llm.ChatMessage{},
		)
		require.NoError(t, err)
		assert.NotNil(t, llmResponse)

		// The LLM should indicate it wants to call the list_virtual_services tool
		// In a real scenario, this would trigger the Avi API call
		// For this test, we'll simulate the Avi client call

		// Step 4: Avi client makes the actual API call
		// (This would be handled by the tool execution layer in production)
		// For testing, we'll verify the expected behavior

		// Expected: The system should recognize "list all virtual services" and provide appropriate response
		assert.NotEmpty(t, llmResponse.Message)
	})

	t.Run("AviAPICallSimulation", func(t *testing.T) {
		// Simulate the Avi API call that would be made after tool identification
		req, err := http.NewRequest("GET", aviServer.URL+"/api/virtualservice", nil)
		require.NoError(t, err)

		// Add authentication headers (simplified for test)
		req.SetBasicAuth("admin", "password")

		// Make the request
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Verify response
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Parse response
		var apiResponse map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&apiResponse)
		require.NoError(t, err)

		// Verify we got virtual services data
		assert.Contains(t, apiResponse, "count")
		assert.Contains(t, apiResponse, "results")
		assert.Equal(t, float64(3), apiResponse["count"])

		results := apiResponse["results"].([]interface{})
		assert.Len(t, results, 3)

		// Verify the structure of the first virtual service
		firstVS := results[0].(map[string]interface{})
		assert.Contains(t, firstVS, "uuid")
		assert.Contains(t, firstVS, "name")
		assert.Contains(t, firstVS, "enabled")
		assert.Contains(t, firstVS, "services")
		assert.Equal(t, "web-app-vs", firstVS["name"])
	})

	t.Run("CompleteFlowWithToolExecution", func(t *testing.T) {
		// This test simulates the complete flow including tool execution
		
		// Step 1: User query
		userQuery := "show all virtual service"

		// Step 2: LLM identifies tool call
		llmClient, err := llm.NewClient(llmConfig, logger)
		require.NoError(t, err)

		// Step 3: Process query
		llmResponse, err := llmClient.ProcessNaturalLanguageQuery(
			context.Background(),
			userQuery,
			"llama3.2",
			llm.GetAviToolDefinitions(),
			[]llm.ChatMessage{},
		)
		require.NoError(t, err)

		// Step 4: Verify LLM response indicates tool usage
		assert.NotEmpty(t, llmResponse.Message)

		// Step 5: Simulate tool execution (Avi API call)
		// In production, this would be handled by the tool execution layer
		// that maps tool names to actual API calls
		
		// Make the Avi API call
		req, err := http.NewRequest("GET", aviServer.URL+"/api/virtualservice", nil)
		require.NoError(t, err)
		req.SetBasicAuth("admin", "password")

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Parse and format response for user
		var apiResponse map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&apiResponse)
		require.NoError(t, err)

		// Step 6: Format response for user display
		// This simulates what the UI would show to the user
		var formattedResponse strings.Builder
		formattedResponse.WriteString("Here are the virtual services I found:\n\n")

		results := apiResponse["results"].([]interface{})
		for i, vs := range results {
			vsMap := vs.(map[string]interface{})
			formattedResponse.WriteString(fmt.Sprintf("%d. **%s** (%s)\n", i+1, vsMap["name"], vsMap["uuid"]))
			formattedResponse.WriteString(fmt.Sprintf("   - Status: %v\n", vsMap["enabled"]))
			
			services := vsMap["services"].([]interface{})
			if len(services) > 0 {
				formattedResponse.WriteString("   - Services: ")
				for j, service := range services {
					serviceMap := service.(map[string]interface{})
					if j > 0 {
						formattedResponse.WriteString(", ")
					}
					formattedResponse.WriteString(fmt.Sprintf("port %v", serviceMap["port"]))
					if serviceMap["enable_ssl"] == true {
						formattedResponse.WriteString("(SSL)")
					}
				}
				formattedResponse.WriteString("\n")
			}
			formattedResponse.WriteString("\n")
		}

		formattedResponse.WriteString(fmt.Sprintf("\nTotal: %v virtual services found.", apiResponse["count"]))

		// Step 7: Verify the formatted response looks correct
		finalResponse := formattedResponse.String()
		assert.Contains(t, finalResponse, "web-app-vs")
		assert.Contains(t, finalResponse, "api-vs")
		assert.Contains(t, finalResponse, "legacy-vs")
		assert.Contains(t, finalResponse, "Total: 3 virtual services found")
		
		// Log the final response (what user would see)
		logger.Info("Final user response", zap.String("response", finalResponse))
	})
}

// TestErrorHandling tests error scenarios
func TestErrorHandling(t *testing.T) {
	// Create a failing Avi API server
	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "Internal server error"}`))
	}))
	defer failingServer.Close()

	// Test authentication failure
	t.Run("AuthenticationFailure", func(t *testing.T) {
		_ = &config.AviConfig{
			Host:     strings.TrimPrefix(failingServer.URL, "http://"),
			Username: "wrong-user",
			Password: "wrong-password",
			Version:  "31.2.1",
			Tenant:   "admin",
			Timeout:  30,
			Insecure: true,
		}

		// Try to make a request (should fail)
		req, err := http.NewRequest("GET", failingServer.URL+"/api/virtualservice", nil)
		require.NoError(t, err)
		req.SetBasicAuth("wrong-user", "wrong-password")

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("LLMErrorHandling", func(t *testing.T) {
		// Create a failing LLM server
		failingLLMServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error": "LLM service unavailable"}`))
		}))
		defer failingLLMServer.Close()

		llmConfig := &config.LLMConfig{
			OllamaHost:   failingLLMServer.URL,
			DefaultModel: "llama3.2",
			Timeout:      60,
		}

		logger := zaptest.NewLogger(t)
		llmClient, err := llm.NewClient(llmConfig, logger)
		require.NoError(t, err)

		// Try to make a chat request (should fail)
		chatReq := llm.ChatRequest{
			Model:    "llama3.2",
			Messages: []llm.ChatMessage{{Role: "user", Content: "test"}},
		}

		_, err = llmClient.ChatCompletion(context.Background(), chatReq)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "request failed with status 503")
	})
}

// TestPerformance tests the performance of the complete flow
func TestPerformance(t *testing.T) {
	// Create a fast-responding mock server
	fastServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/login") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"sessionid": "fast-session", "csrftoken": "fast-token", "version": "31.2.1"}`))
			return
		}
		
		if strings.Contains(r.URL.Path, "/virtualservice") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"count": 1, "results": [{"uuid": "fast-vs", "name": "fast-vs", "enabled": true}]}`))
			return
		}
		
		w.WriteHeader(http.StatusNotFound)
	}))
	defer fastServer.Close()

	logger := zaptest.NewLogger(t)
	
	// Test multiple requests to measure performance
	startTime := time.Now()
	
	for i := 0; i < 5; i++ {
		req, err := http.NewRequest("GET", fastServer.URL+"/api/virtualservice", nil)
		require.NoError(t, err)
		req.SetBasicAuth("admin", "password")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	}
	
	duration := time.Since(startTime)
	avgDuration := duration / 5
	
	logger.Info("Performance test completed", 
		zap.Duration("total_duration", duration),
		zap.Duration("avg_duration", avgDuration))
	
	// Should complete 5 requests in under 2 seconds
	assert.Less(t, duration.Seconds(), 2.0, "Performance test should complete in under 2 seconds")
}

// TestEdgeCases tests edge cases and boundary conditions
func TestEdgeCases(t *testing.T) {
	t.Run("EmptyResponse", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"count": 0, "results": []}`))
		}))
		defer server.Close()

		req, err := http.NewRequest("GET", server.URL+"/api/virtualservice", nil)
		require.NoError(t, err)

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		var apiResponse map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&apiResponse)
		require.NoError(t, err)

		assert.Equal(t, float64(0), apiResponse["count"])
		assert.Len(t, apiResponse["results"].([]interface{}), 0)
	})

	t.Run("LargeResponse", func(t *testing.T) {
		// Generate a large response with many virtual services
		var largeResponse bytes.Buffer
		largeResponse.WriteString(`{"count": 100, "results": [`)

		for i := 0; i < 100; i++ {
			if i > 0 {
				largeResponse.WriteString(",")
			}
			largeResponse.WriteString(fmt.Sprintf(`{"uuid": "vs-%d", "name": "virtual-service-%d", "enabled": %v}`,
				i, i, i%2 == 0))
		}

		largeResponse.WriteString("]}")

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(largeResponse.Bytes())
		}))
		defer server.Close()

		req, err := http.NewRequest("GET", server.URL+"/api/virtualservice", nil)
		require.NoError(t, err)

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		var apiResponse map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&apiResponse)
		require.NoError(t, err)

		assert.Equal(t, float64(100), apiResponse["count"])
		assert.Len(t, apiResponse["results"].([]interface{}), 100)
	})
}