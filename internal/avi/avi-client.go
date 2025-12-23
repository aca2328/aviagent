package avi

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
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
	cache      *Cache
}

// Cache represents a simple in-memory cache
type Cache struct {
	store      map[string]cacheEntry
	mu         sync.RWMutex
	cacheTTL   time.Duration
}

// cacheEntry represents a cached API response
type cacheEntry struct {
	data      interface{}
	expiresAt time.Time
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

	// Create HTTP client with optimized transport for SSL handling
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.Insecure,
			MinVersion:         tls.VersionTLS12, // Enforce minimum TLS version
		},
		MaxIdleConns:        100,              // Maximum number of idle connections
		IdleConnTimeout:     90 * time.Second,  // Timeout for idle connections
		TLSHandshakeTimeout: 10 * time.Second,  // Timeout for TLS handshake
		ExpectContinueTimeout: 1 * time.Second, // Timeout for expect continue
		DialContext: (&net.Dialer{              // Custom dialer with timeouts
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
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
		cache:      newCache(30 * time.Second), // 30 second cache TTL
	}

	// Authenticate and create session
	if err := client.authenticate(); err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	return client, nil
}

// newCache creates a new cache instance
func newCache(ttl time.Duration) *Cache {
	return &Cache{
		store:    make(map[string]cacheEntry),
		cacheTTL: ttl,
	}
}

// getCacheKey generates a cache key from method, endpoint, and parameters
func (c *Client) getCacheKey(method, endpoint string, params map[string]string) string {
	// Sort parameters for consistent key generation
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build parameter string
	paramStr := ""
	for _, k := range keys {
		paramStr += fmt.Sprintf("%s=%s&", k, params[k])
	}

	return fmt.Sprintf("%s:%s?%s", method, endpoint, paramStr)
}

// getFromCache retrieves data from cache if it exists and is not expired
func (c *Client) getFromCache(key string) (interface{}, bool) {
	if c.cache == nil {
		return nil, false
	}

	c.cache.mu.RLock()
	entry, ok := c.cache.store[key]
	c.cache.mu.RUnlock()

	if !ok {
		return nil, false
	}

	// Check if cache entry is expired
	if time.Now().After(entry.expiresAt) {
		c.cache.mu.Lock()
		delete(c.cache.store, key)
		c.cache.mu.Unlock()
		return nil, false
	}

	return entry.data, true
}

// setCache stores data in cache
func (c *Client) setCache(key string, data interface{}) {
	if c.cache == nil {
		return
	}

	c.cache.mu.Lock()
	c.cache.store[key] = cacheEntry{
		data:      data,
		expiresAt: time.Now().Add(c.cache.cacheTTL),
	}
	c.cache.mu.Unlock()
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

// makeRequest performs an authenticated API request with context support
func (c *Client) makeRequest(ctx context.Context, method, endpoint string, body interface{}, params map[string]string) (*http.Response, error) {
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

	req, err := http.NewRequestWithContext(ctx, method, requestURL, bodyReader)
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

	c.logger.Debug("Making API request",
		zap.String("method", method),
		zap.String("endpoint", endpoint),
		zap.Any("params", params),
		zap.String("url", requestURL))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("API request failed",
			zap.String("method", method),
			zap.String("endpoint", endpoint),
			zap.Error(err))
		return nil, fmt.Errorf("API request failed: %w", err)
	}

	return resp, nil
}

// ListVirtualServices retrieves all virtual services
func (c *Client) ListVirtualServices(ctx context.Context, params map[string]string) (*APIResponse, error) {
	// Generate cache key for this request
	cacheKey := c.getCacheKey("GET", "/virtualservice", params)

	// Try to get from cache first
	if cached, ok := c.getFromCache(cacheKey); ok {
		c.logger.Debug("Cache hit for virtual services", zap.String("key", cacheKey))
		return cached.(*APIResponse), nil
	}

	resp, err := c.makeRequest(ctx, "GET", "/virtualservice", nil, params)
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

	// Cache the result for future requests
	c.setCache(cacheKey, &result)
	c.logger.Debug("Cached virtual services response", zap.String("key", cacheKey))

	return &result, nil
}

// GetVirtualService retrieves a specific virtual service by UUID
func (c *Client) GetVirtualService(ctx context.Context, uuid string, params map[string]string) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("/virtualservice/%s", uuid)
	resp, err := c.makeRequest(ctx, "GET", endpoint, nil, params)
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
func (c *Client) CreateVirtualService(ctx context.Context, vsData map[string]interface{}) (map[string]interface{}, error) {
	resp, err := c.makeRequest(ctx, "POST", "/virtualservice", vsData, nil)
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
func (c *Client) UpdateVirtualService(ctx context.Context, uuid string, vsData map[string]interface{}) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("/virtualservice/%s", uuid)
	resp, err := c.makeRequest(ctx, "PUT", endpoint, vsData, nil)
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
func (c *Client) DeleteVirtualService(ctx context.Context, uuid string) error {
	endpoint := fmt.Sprintf("/virtualservice/%s", uuid)
	resp, err := c.makeRequest(ctx, "DELETE", endpoint, nil, nil)
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
func (c *Client) ListPools(ctx context.Context, params map[string]string) (*APIResponse, error) {
	// Generate cache key for this request
	cacheKey := c.getCacheKey("GET", "/pool", params)

	// Try to get from cache first
	if cached, ok := c.getFromCache(cacheKey); ok {
		c.logger.Debug("Cache hit for pools", zap.String("key", cacheKey))
		return cached.(*APIResponse), nil
	}

	resp, err := c.makeRequest(ctx, "GET", "/pool", nil, params)
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

	// Cache the result for future requests
	c.setCache(cacheKey, &result)
	c.logger.Debug("Cached pools response", zap.String("key", cacheKey))

	return &result, nil
}

// GetPool retrieves a specific pool by UUID
func (c *Client) GetPool(ctx context.Context, uuid string, params map[string]string) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("/pool/%s", uuid)
	resp, err := c.makeRequest(ctx, "GET", endpoint, nil, params)
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
func (c *Client) CreatePool(ctx context.Context, poolData map[string]interface{}) (map[string]interface{}, error) {
	resp, err := c.makeRequest(ctx, "POST", "/pool", poolData, nil)
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
func (c *Client) ScaleOutPool(ctx context.Context, uuid string, params map[string]interface{}) error {
	endpoint := fmt.Sprintf("/pool/%s/scaleout", uuid)
	resp, err := c.makeRequest(ctx, "POST", endpoint, params, nil)
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
func (c *Client) ScaleInPool(ctx context.Context, uuid string, params map[string]interface{}) error {
	endpoint := fmt.Sprintf("/pool/%s/scalein", uuid)
	resp, err := c.makeRequest(ctx, "POST", endpoint, params, nil)
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
func (c *Client) ListHealthMonitors(ctx context.Context, params map[string]string) (*APIResponse, error) {
	resp, err := c.makeRequest(ctx, "GET", "/healthmonitor", nil, params)
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
func (c *Client) GetHealthMonitor(ctx context.Context, uuid string, params map[string]string) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("/healthmonitor/%s", uuid)
	resp, err := c.makeRequest(ctx, "GET", endpoint, nil, params)
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
func (c *Client) ListServiceEngines(ctx context.Context, params map[string]string) (*APIResponse, error) {
	resp, err := c.makeRequest(ctx, "GET", "/serviceengine", nil, params)
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
func (c *Client) GetServiceEngine(ctx context.Context, uuid string, params map[string]string) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("/serviceengine/%s", uuid)
	resp, err := c.makeRequest(ctx, "GET", endpoint, nil, params)
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
func (c *Client) GetAnalytics(ctx context.Context, resourceType, uuid string, params map[string]string) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("/analytics/%s/%s", resourceType, uuid)
	resp, err := c.makeRequest(ctx, "GET", endpoint, nil, params)
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
func (c *Client) ExecuteGenericOperation(ctx context.Context, method, endpoint string, body interface{}, params map[string]string) (interface{}, error) {
	// Ensure endpoint starts with /
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}

	resp, err := c.makeRequest(ctx, method, endpoint, body, params)
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