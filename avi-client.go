package avi

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"aviagent/internal/config"

	"go.uber.org/zap"
)

// Client represents the Avi Load Balancer API client
type Client struct {
	config     *config.AviConfig
	httpClient *http.Client
	baseURL    string
	logger     *zap.Logger
	session    *Session
}

// Session holds authentication session information
type Session struct {
	SessionID string `json:"sessionid"`
	CSRFToken string `json:"csrftoken"`
	Version   string `json:"version"`
}

// APIResponse represents a generic API response
type APIResponse struct {
	Count   int                      `json:"count"`
	Results []map[string]interface{} `json:"results"`
	Next    string                   `json:"next,omitempty"`
}

// NewClient creates a new Avi API client
func NewClient(cfg *config.AviConfig, logger *zap.Logger) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("avi config cannot be nil")
	}

	// Create HTTP client with custom transport for SSL handling
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.Insecure,
		},
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   time.Duration(cfg.Timeout) * time.Second,
	}

	client := &Client{
		config:     cfg,
		httpClient: httpClient,
		baseURL:    fmt.Sprintf("https://%s/api", cfg.Host),
		logger:     logger,
	}

	// Authenticate and create session
	if err := client.authenticate(); err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	return client, nil
}

// authenticate performs authentication and creates a session
func (c *Client) authenticate() error {
	loginURL := fmt.Sprintf("https://%s/login", c.config.Host)
	
	loginData := map[string]string{
		"username": c.config.Username,
		"password": c.config.Password,
	}

	jsonData, err := json.Marshal(loginData)
	if err != nil {
		return fmt.Errorf("failed to marshal login data: %w", err)
	}

	req, err := http.NewRequest("POST", loginURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create login request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Avi-Version", c.config.Version)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("login failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse session information from response
	var session Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return fmt.Errorf("failed to parse session response: %w", err)
	}

	c.session = &session
	c.logger.Info("Authentication successful", zap.String("version", session.Version))

	return nil
}

// makeRequest performs an authenticated API request
func (c *Client) makeRequest(method, endpoint string, body interface{}, params map[string]string) (*http.Response, error) {
	if c.session == nil {
		return nil, fmt.Errorf("not authenticated")
	}

	var bodyReader io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewBuffer(jsonData)
	}

	// Build URL with parameters
	requestURL := fmt.Sprintf("%s%s", c.baseURL, endpoint)
	if len(params) > 0 {
		values := url.Values{}
		for key, value := range params {
			values.Set(key, value)
		}
		requestURL += "?" + values.Encode()
	}

	req, err := http.NewRequest(method, requestURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set required headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Avi-Version", c.config.Version)
	req.Header.Set("X-Avi-Tenant", c.config.Tenant)
	if c.session.CSRFToken != "" {
		req.Header.Set("X-CSRFToken", c.session.CSRFToken)
	}

	// Set session cookie
	req.AddCookie(&http.Cookie{
		Name:  "sessionid",
		Value: c.session.SessionID,
	})

	return c.httpClient.Do(req)
}

// ListVirtualServices retrieves all virtual services
func (c *Client) ListVirtualServices(params map[string]string) (*APIResponse, error) {
	resp, err := c.makeRequest("GET", "/virtualservice", nil, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// GetVirtualService retrieves a specific virtual service by UUID
func (c *Client) GetVirtualService(uuid string, params map[string]string) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("/virtualservice/%s", uuid)
	resp, err := c.makeRequest("GET", endpoint, nil, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

// CreateVirtualService creates a new virtual service
func (c *Client) CreateVirtualService(vsData map[string]interface{}) (map[string]interface{}, error) {
	resp, err := c.makeRequest("POST", "/virtualservice", vsData, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

// UpdateVirtualService updates an existing virtual service
func (c *Client) UpdateVirtualService(uuid string, vsData map[string]interface{}) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("/virtualservice/%s", uuid)
	resp, err := c.makeRequest("PUT", endpoint, vsData, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

// DeleteVirtualService deletes a virtual service
func (c *Client) DeleteVirtualService(uuid string) error {
	endpoint := fmt.Sprintf("/virtualservice/%s", uuid)
	resp, err := c.makeRequest("DELETE", endpoint, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ListPools retrieves all pools
func (c *Client) ListPools(params map[string]string) (*APIResponse, error) {
	resp, err := c.makeRequest("GET", "/pool", nil, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// GetPool retrieves a specific pool by UUID
func (c *Client) GetPool(uuid string, params map[string]string) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("/pool/%s", uuid)
	resp, err := c.makeRequest("GET", endpoint, nil, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

// CreatePool creates a new pool
func (c *Client) CreatePool(poolData map[string]interface{}) (map[string]interface{}, error) {
	resp, err := c.makeRequest("POST", "/pool", poolData, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

// ScaleOutPool scales out a pool by adding servers
func (c *Client) ScaleOutPool(uuid string, params map[string]interface{}) error {
	endpoint := fmt.Sprintf("/pool/%s/scaleout", uuid)
	resp, err := c.makeRequest("POST", endpoint, params, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("scale out failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ScaleInPool scales in a pool by removing servers
func (c *Client) ScaleInPool(uuid string, params map[string]interface{}) error {
	endpoint := fmt.Sprintf("/pool/%s/scalein", uuid)
	resp, err := c.makeRequest("POST", endpoint, params, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("scale in failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ListHealthMonitors retrieves all health monitors
func (c *Client) ListHealthMonitors(params map[string]string) (*APIResponse, error) {
	resp, err := c.makeRequest("GET", "/healthmonitor", nil, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// GetHealthMonitor retrieves a specific health monitor by UUID
func (c *Client) GetHealthMonitor(uuid string, params map[string]string) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("/healthmonitor/%s", uuid)
	resp, err := c.makeRequest("GET", endpoint, nil, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

// ListServiceEngines retrieves all service engines
func (c *Client) ListServiceEngines(params map[string]string) (*APIResponse, error) {
	resp, err := c.makeRequest("GET", "/serviceengine", nil, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// GetServiceEngine retrieves a specific service engine by UUID
func (c *Client) GetServiceEngine(uuid string, params map[string]string) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("/serviceengine/%s", uuid)
	resp, err := c.makeRequest("GET", endpoint, nil, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

// GetAnalytics retrieves analytics data for a specific resource
func (c *Client) GetAnalytics(resourceType, uuid string, params map[string]string) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("/analytics/%s/%s", resourceType, uuid)
	resp, err := c.makeRequest("GET", endpoint, nil, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

// ExecuteGenericOperation performs a generic API operation
func (c *Client) ExecuteGenericOperation(method, endpoint string, body interface{}, params map[string]string) (interface{}, error) {
	// Ensure endpoint starts with /
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}

	resp, err := c.makeRequest(method, endpoint, body, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(responseBody))
	}

	// Try to parse as JSON
	var result interface{}
	if len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, &result); err != nil {
			// If JSON parsing fails, return raw string
			return string(responseBody), nil
		}
	}

	return result, nil
}

// Close closes the client and performs cleanup
func (c *Client) Close() error {
	// Perform logout if needed
	if c.session != nil {
		logoutURL := fmt.Sprintf("https://%s/logout", c.config.Host)
		req, err := http.NewRequest("POST", logoutURL, nil)
		if err == nil {
			req.Header.Set("X-Avi-Version", c.config.Version)
			req.AddCookie(&http.Cookie{
				Name:  "sessionid",
				Value: c.session.SessionID,
			})
			c.httpClient.Do(req) // Best effort, ignore errors
		}
		c.session = nil
	}
	return nil
}