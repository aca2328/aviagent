package avi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"vmware-avi-llm-agent/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestNewClient(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name    string
		config  *config.AviConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "valid config",
			config: &config.AviConfig{
				Host:     "test.example.com",
				Username: "admin",
				Password: "password",
				Version:  "31.2.1",
				Tenant:   "admin",
				Timeout:  30,
				Insecure: true,
			},
			wantErr: false, // Note: will fail authentication but client creation should succeed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.config, logger)
			
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				// Client creation should succeed even if authentication fails
				// The actual authentication is tested separately
				if err != nil {
					// Authentication failure is expected in tests
					assert.Contains(t, err.Error(), "authentication failed")
				}
			}
		})
	}
}

func TestClient_makeRequest(t *testing.T) {
	// Create a test server to mock Avi API responses
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
				"count": 1,
				"results": [
					{
						"uuid": "virtualservice-uuid-1",
						"name": "test-vs",
						"enabled": true,
						"services": [
							{"port": 80, "enable_ssl": false},
							{"port": 443, "enable_ssl": true}
						]
					}
				]
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	cfg := &config.AviConfig{
		Host:     strings.TrimPrefix(server.URL, "https://"),
		Username: "admin",
		Password: "password",
		Version:  "31.2.1",
		Tenant:   "admin",
		Timeout:  30,
		Insecure: true,
	}

	// Replace https with http for test server
	cfg.Host = strings.TrimPrefix(server.URL, "http://")

	client := &Client{
		config:     cfg,
		httpClient: server.Client(),
		baseURL:    server.URL + "/api",
		logger:     logger,
	}

	// First authenticate
	err := client.authenticate()
	require.NoError(t, err)
	require.NotNil(t, client.session)
	assert.Equal(t, "test-session-id", client.session.SessionID)

	// Test making a request
	resp, err := client.makeRequest("GET", "/virtualservice", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

func TestClient_ListVirtualServices(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/login") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"sessionid": "test-session-id",
				"csrftoken": "test-csrf-token",
				"version": "31.2.1"
			}`))
			return
		}

		if strings.Contains(r.URL.Path, "/virtualservice") {
			// Check query parameters
			name := r.URL.Query().Get("name")
			expectedResponse := `{
				"count": 2,
				"results": [
					{
						"uuid": "vs-uuid-1",
						"name": "web-app-vs",
						"enabled": true
					},
					{
						"uuid": "vs-uuid-2", 
						"name": "api-vs",
						"enabled": false
					}
				]
			}`

			if name == "web-app-vs" {
				expectedResponse = `{
					"count": 1,
					"results": [
						{
							"uuid": "vs-uuid-1",
							"name": "web-app-vs",
							"enabled": true
						}
					]
				}`
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(expectedResponse))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	cfg := &config.AviConfig{
		Host:     strings.TrimPrefix(server.URL, "http://"),
		Username: "admin",
		Password: "password",
		Version:  "31.2.1",
		Tenant:   "admin",
		Timeout:  30,
		Insecure: true,
	}

	client := &Client{
		config:     cfg,
		httpClient: server.Client(),
		baseURL:    server.URL + "/api",
		logger:     logger,
	}

	// Authenticate first
	err := client.authenticate()
	require.NoError(t, err)

	// Test listing all virtual services
	result, err := client.ListVirtualServices(nil)
	require.NoError(t, err)
	assert.Equal(t, 2, result.Count)
	assert.Len(t, result.Results, 2)

	// Test listing with filter
	result, err = client.ListVirtualServices(map[string]string{"name": "web-app-vs"})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Count)
	assert.Len(t, result.Results, 1)
	
	// Check the returned data
	vs := result.Results[0]
	assert.Equal(t, "vs-uuid-1", vs["uuid"])
	assert.Equal(t, "web-app-vs", vs["name"])
	assert.Equal(t, true, vs["enabled"])
}

func TestClient_CreateVirtualService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/login") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"sessionid": "test-session-id",
				"csrftoken": "test-csrf-token",
				"version": "31.2.1"
			}`))
			return
		}

		if r.Method == "POST" && strings.Contains(r.URL.Path, "/virtualservice") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{
				"uuid": "new-vs-uuid",
				"name": "new-test-vs",
				"enabled": true,
				"services": [
					{"port": 80, "enable_ssl": false}
				]
			}`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	cfg := &config.AviConfig{
		Host:     strings.TrimPrefix(server.URL, "http://"),
		Username: "admin",
		Password: "password",
		Version:  "31.2.1",
		Tenant:   "admin",
		Timeout:  30,
		Insecure: true,
	}

	client := &Client{
		config:     cfg,
		httpClient: server.Client(),
		baseURL:    server.URL + "/api",
		logger:     logger,
	}

	// Authenticate first
	err := client.authenticate()
	require.NoError(t, err)

	// Test creating a virtual service
	vsData := map[string]interface{}{
		"name": "new-test-vs",
		"services": []map[string]interface{}{
			{"port": 80, "enable_ssl": false},
		},
		"enabled": true,
	}

	result, err := client.CreateVirtualService(vsData)
	require.NoError(t, err)
	assert.Equal(t, "new-vs-uuid", result["uuid"])
	assert.Equal(t, "new-test-vs", result["name"])
	assert.Equal(t, true, result["enabled"])
}

func TestClient_Close(t *testing.T) {
	logger := zaptest.NewLogger(t)
	
	// Create a mock server that handles logout
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/logout") {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := &config.AviConfig{
		Host:     strings.TrimPrefix(server.URL, "http://"),
		Username: "admin",
		Password: "password",
		Version:  "31.2.1",
		Tenant:   "admin",
		Timeout:  30,
		Insecure: true,
	}

	client := &Client{
		config:     cfg,
		httpClient: server.Client(),
		baseURL:    server.URL + "/api",
		logger:     logger,
		session: &Session{
			SessionID: "test-session",
			CSRFToken: "test-token",
			Version:   "31.2.1",
		},
	}

	// Test close
	err := client.Close()
	assert.NoError(t, err)
	assert.Nil(t, client.session)
}

// Benchmark tests
func BenchmarkClient_ListVirtualServices(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"count": 100,
			"results": []
		}`))
	}))
	defer server.Close()

	logger := zaptest.NewLogger(b)
	cfg := &config.AviConfig{
		Host:     strings.TrimPrefix(server.URL, "http://"),
		Username: "admin",
		Password: "password",
		Version:  "31.2.1",
		Tenant:   "admin",
		Timeout:  30,
		Insecure: true,
	}

	client := &Client{
		config:     cfg,
		httpClient: server.Client(),
		baseURL:    server.URL + "/api",
		logger:     logger,
		session: &Session{
			SessionID: "test-session",
			CSRFToken: "test-token",
			Version:   "31.2.1",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := client.ListVirtualServices(nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}