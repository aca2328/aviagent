package avi

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
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
	authMethod string // "session" or "basic"
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
// Session represents the authentication session with Avi controller
type Session struct {
	SessionID   string      `json:"sessionid"`
	CSRFToken   string      `json:"csrftoken"`
	Version     interface{} `json:"version"` // Can be string or object depending on Avi version
	SessionCookieName string `json:"session_cookie_name"`
	User        struct {
		Username string `json:"username"`
		UUID     string `json:"uuid"`
	} `json:"user"`
}

// GetVersionString extracts the version string from the Version field
func (s *Session) GetVersionString() string {
	switch v := s.Version.(type) {
	case string:
		return v
	case map[string]interface{}:
		if versionStr, ok := v["Version"].(string); ok {
			return versionStr
		}
		if versionStr, ok := v["version"].(string); ok {
			return versionStr
		}
		return "unknown"
	default:
		return fmt.Sprintf("%v", v)
	}
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

	// Determine authentication method
	authMethod := cfg.AuthMethod
	if authMethod == "" {
		authMethod = "session" // Default to session-based auth
	}

	client := &Client{
		config:     cfg,
		httpClient: httpClient,
		baseURL:    fmt.Sprintf("https://%s/api", cfg.Host),
		logger:     logger,
		cache:      newCache(30 * time.Second), // 30 second cache TTL
		authMethod: authMethod,
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

// authenticate performs authentication using the configured method (session or basic)
func (c *Client) authenticate() error {
	if c.authMethod == "basic" {
		return c.authenticateBasic()
	}
	// Default to session-based authentication
	return c.authenticateSession()
}

// authenticateSession performs session-based authentication (recommended method)
func (c *Client) authenticateSession() error {
	loginURL := fmt.Sprintf("https://%s/login", c.config.Host)
	
	c.logger.Info("Attempting Avi controller authentication",
		zap.String("login_url", loginURL),
		zap.String("username", c.config.Username),
		zap.String("version", c.config.Version),
		zap.Bool("insecure", c.config.Insecure))
	
	loginData := map[string]string{
		"username": c.config.Username,
		"password": c.config.Password,
	}

	jsonData, err := json.Marshal(loginData)
	if err != nil {
		return fmt.Errorf("failed to marshal login data: %w", err)
	}

	c.logger.Info("Avi login request details",
		zap.String("request_body", string(jsonData)),
		zap.String("content_type", "application/json"),
		zap.String("avi_version", c.config.Version))

	req, err := http.NewRequest("POST", loginURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create login request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Avi-Version", c.config.Version)
	
	c.logger.Info("Avi login request headers configured",
		zap.String("content_type", req.Header.Get("Content-Type")),
		zap.String("avi_version", req.Header.Get("X-Avi-Version")))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("Avi login request failed", zap.Error(err))
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	c.logger.Info("Avi login response received",
		zap.Int("status_code", resp.StatusCode),
		zap.String("status", resp.Status))

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		errorMsg := fmt.Sprintf("login failed with status %d: %s", resp.StatusCode, string(body))
		c.logger.Error("Avi login failed",
			zap.Int("status_code", resp.StatusCode),
			zap.String("error_response", string(body)))
		return errors.New(errorMsg)
	}

	// Parse session information from response
	// First try to get session from cookies (modern Avi controllers)
	sessionID := ""
	csrfToken := ""
	
	// Extract cookies from response
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "avi-sessionid" || cookie.Name == "sessionid" {
			sessionID = cookie.Value
		}
		if cookie.Name == "csrftoken" {
			csrfToken = cookie.Value
		}
	}
	
	// If cookies are empty, try to parse from JSON response (older controllers)
	if sessionID == "" || csrfToken == "" {
		var session Session
		if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
			body, _ := io.ReadAll(resp.Body)
			c.logger.Error("Failed to parse session response",
				zap.Error(err),
				zap.String("response_body", string(body)))
			return fmt.Errorf("failed to parse session response: %w", err)
		}
		
		// Use parsed values if cookies were empty
		if sessionID == "" {
			sessionID = session.SessionID
		}
		if csrfToken == "" {
			csrfToken = session.CSRFToken
		}
	}

	c.logger.Info("Session authentication successful - parsed session data",
		zap.String("session_id", sessionID),
		zap.String("csrf_token", csrfToken))

	// Create session object
	c.session = &Session{
		SessionID:   sessionID,
		CSRFToken:   csrfToken,
		Version:     "31.1.1", // We'll get this from the API
	}
	
	c.logger.Info("Session authentication successful")

	return nil
}

// authenticateBasic performs HTTP Basic Authentication
func (c *Client) authenticateBasic() error {
	c.logger.Info("Using Basic Authentication")
	
	// Basic auth doesn't require a separate login call
	// We'll include credentials in each request via Authorization header
	c.session = &Session{
		SessionID: "basic-auth",
		CSRFToken: "basic-auth",
		Version:   "basic-auth",
	}
	
	c.logger.Info("Basic authentication configured")
	return nil
}

// makeRequest performs an authenticated API request with context support and retry logic
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

	// Create request with retry logic
	var resp *http.Response
	var err error
	
	// Retry up to 3 times for transient errors
	for i := 0; i < 3; i++ {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("request cancelled: %w", ctx.Err())
		default:
			// Create new request for each attempt
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

			// Set authentication headers based on auth method
			if c.authMethod == "basic" {
				// Basic authentication
				req.SetBasicAuth(c.config.Username, c.config.Password)
				c.logger.Info("Using Basic Authentication for API request")
			} else {
				// Session-based authentication
				c.logger.Info("Using Session Authentication for API request",
					zap.String("csrf_token", c.session.CSRFToken),
					zap.String("session_id", c.session.SessionID))
				
				if c.session.CSRFToken != "" {
					req.Header.Set("X-CSRFToken", c.session.CSRFToken)
				}
				// Set session cookie
				req.AddCookie(&http.Cookie{
					Name:  "sessionid",
					Value: c.session.SessionID,
				})
			}

			c.logger.Debug("Making API request",
				zap.String("method", method),
				zap.String("endpoint", endpoint),
				zap.Any("params", params),
				zap.String("url", requestURL),
				zap.String("auth_method", c.authMethod),
				zap.Int("attempt", i+1))

			// Log API call headers and payload for debugging
			logHeaders := map[string]string{
				"Content-Type": req.Header.Get("Content-Type"),
				"X-Avi-Version": req.Header.Get("X-Avi-Version"),
				"X-Avi-Tenant": req.Header.Get("X-Avi-Tenant"),
			}
			if c.authMethod == "basic" {
				logHeaders["Authorization"] = "Basic <redacted>"
			} else if c.session != nil {
				logHeaders["Cookie"] = "sessionid=<redacted>"
			}
			
			// Capture payload for logging
			var logPayload interface{}
			if body != nil {
				if bodyReader != nil {
					if seeker, ok := bodyReader.(io.Seeker); ok {
						// Save current position
						currentPos, _ := seeker.Seek(0, io.SeekCurrent)
						seeker.Seek(0, io.SeekStart)
						bodyBytes, readErr := io.ReadAll(bodyReader)
						if readErr == nil {
							var jsonData interface{}
							if jsonErr := json.Unmarshal(bodyBytes, &jsonData); jsonErr == nil {
								logPayload = jsonData
							} else {
								logPayload = string(bodyBytes)
							}
						}
						// Restore position
						seeker.Seek(currentPos, io.SeekStart)
					}
				}
			}
			
			c.logger.Info("Avi API request details",
				zap.String("method", method),
				zap.String("endpoint", endpoint),
				zap.Any("headers", logHeaders),
				zap.Any("payload", logPayload))

			resp, err = c.httpClient.Do(req)
			if err == nil {
				// Log successful response details
				responseHeaders := map[string]string{
					"Content-Type": resp.Header.Get("Content-Type"),
					"Content-Length": resp.Header.Get("Content-Length"),
					"X-Avi-Version": resp.Header.Get("X-Avi-Version"),
				}
				
				// Capture response body for logging (limit size for performance)
				var responsePayload interface{}
				if resp.ContentLength > 0 && resp.ContentLength <= 1024*1024 { // Max 1MB
					responseBody, readErr := io.ReadAll(resp.Body)
					if readErr == nil {
						var jsonData interface{}
						if jsonErr := json.Unmarshal(responseBody, &jsonData); jsonErr == nil {
							responsePayload = jsonData
						} else {
							responsePayload = string(responseBody)
						}
						// Reset response body for later reading
						resp.Body = io.NopCloser(bytes.NewBuffer(responseBody))
					} else {
						c.logger.Warn("Failed to read response body for logging", zap.Error(readErr))
					}
				} else if resp.ContentLength > 1024*1024 {
					responsePayload = "<response too large for logging>"
				}
				
				c.logger.Info("Avi API response details",
					zap.String("method", method),
					zap.String("endpoint", endpoint),
					zap.Int("status_code", resp.StatusCode),
					zap.Any("response_headers", responseHeaders),
					zap.Any("response_payload", responsePayload))
				
				// Success, break out of retry loop
				break
			}

			// Check if error is retryable
			if isRetryableError(err) {
				c.logger.Warn("Retryable error, attempting retry",
					zap.String("method", method),
					zap.String("endpoint", endpoint),
					zap.Error(err),
					zap.Int("attempt", i+1))
				
				// Exponential backoff: 1s, 2s, 4s
				time.Sleep(time.Duration(1<<i) * time.Second)
				continue
			}

			// Non-retryable error, break immediately
			c.logger.Error("Non-retryable API request failed",
				zap.String("method", method),
				zap.String("endpoint", endpoint),
				zap.Error(err))
			return nil, fmt.Errorf("API request failed: %w", err)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("API request failed after retries: %w", err)
	}

	return resp, nil
}

// isRetryableError determines if an error is transient and worth retrying
func isRetryableError(err error) bool {
	// Network errors
	if strings.Contains(err.Error(), "connection reset") ||
	   strings.Contains(err.Error(), "timeout") ||
	   strings.Contains(err.Error(), "temporary failure") ||
	   strings.Contains(err.Error(), "i/o timeout") ||
	   strings.Contains(err.Error(), "broken pipe") {
		return true
	}

	// HTTP 5xx errors (we'll check these after getting response)
	// Rate limiting errors
	if strings.Contains(err.Error(), "429") ||
	   strings.Contains(err.Error(), "too many requests") {
		return true
	}

	// DNS resolution issues
	if strings.Contains(err.Error(), "no such host") ||
	   strings.Contains(err.Error(), "dial tcp") ||
	   strings.Contains(err.Error(), "lookup") {
		return true
	}

	return false
}

// ListVirtualServices retrieves all virtual services with enhanced error handling
func (c *Client) ListVirtualServices(ctx context.Context, params map[string]string) (interface{}, error) {
	// Generate cache key for this request
	cacheKey := c.getCacheKey("GET", "/virtualservice", params)

	// Try to get from cache first
	if cached, ok := c.getFromCache(cacheKey); ok {
		c.logger.Debug("Cache hit for virtual services", zap.String("key", cacheKey))
		return cached.(*APIResponse), nil
	}

	resp, err := c.makeRequest(ctx, "GET", "/virtualservice", nil, params)
	if err != nil {
		return nil, c.handleAPIError(err, "ListVirtualServices")
	}
	defer resp.Body.Close()

	// Enhanced status code handling
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, c.handleHTTPError(resp.StatusCode, body, "ListVirtualServices")
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

// handleAPIError provides consistent error handling for API request failures
func (c *Client) handleAPIError(err error, operation string) error {
	var apiError *APIError
	
	// Check if it's already an APIError
	if errors.As(err, &apiError) {
		return apiError
	}
	
	// Wrap the error with context
	return &APIError{
		Operation: operation,
		Message:   fmt.Sprintf("API request failed: %v", err),
		Cause:     err,
		Severity:  "high",
	}
}

// handleHTTPError provides consistent error handling for HTTP response errors
func (c *Client) handleHTTPError(statusCode int, body []byte, operation string) error {
	var errorMessage string
	var suggestions []string
	
	switch statusCode {
	case http.StatusUnauthorized:
		errorMessage = "Authentication failed"
		suggestions = []string{
			"Check your Avi controller credentials",
			"Verify the username and password are correct",
			"Ensure the user has proper permissions",
		}
	case http.StatusForbidden:
		errorMessage = "Access denied"
		suggestions = []string{
			"Check user permissions for the requested resource",
			"Verify the tenant configuration",
			"Contact your Avi administrator",
		}
	case http.StatusNotFound:
		errorMessage = "Resource not found"
		suggestions = []string{
			"Verify the resource UUID or name",
			"Check if the resource exists",
			"Review your query parameters",
		}
	case http.StatusTooManyRequests:
		errorMessage = "Rate limit exceeded"
		suggestions = []string{
			"Wait and try again later",
			"Check your request frequency",
			"Consider implementing client-side rate limiting",
		}
	case http.StatusInternalServerError:
		errorMessage = "Avi controller internal error"
		suggestions = []string{
			"Check Avi controller logs",
			"Verify controller health",
			"Retry the operation later",
		}
	case http.StatusServiceUnavailable:
		errorMessage = "Avi controller unavailable"
		suggestions = []string{
			"Check controller status",
			"Verify network connectivity",
			"Contact your infrastructure team",
		}
	default:
		errorMessage = fmt.Sprintf("Unexpected HTTP status: %d", statusCode)
		suggestions = []string{
			"Check the Avi controller documentation",
			"Review your request parameters",
			"Contact support if the issue persists",
		}
	}
	
	// Try to extract more details from response body
	if len(body) > 0 {
		var errorResponse map[string]interface{}
		if err := json.Unmarshal(body, &errorResponse); err == nil {
			if msg, ok := errorResponse["message"].(string); ok {
				errorMessage += ": " + msg
			}
		}
	}
	
	return &APIError{
		Operation:    operation,
		Message:      errorMessage,
		HTTPStatus:   statusCode,
		ResponseBody: string(body),
		Suggestions:  suggestions,
		Severity:     "high",
	}
}

// APIError represents a structured error from the Avi API client
type APIError struct {
	Operation    string
	Message      string
	Cause        error
	HTTPStatus   int
	ResponseBody string
	Suggestions  []string
	Severity     string // low, medium, high, critical
}

func (e *APIError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (operation: %s)", e.Message, e.Cause.Error(), e.Operation)
	}
	return fmt.Sprintf("%s (operation: %s)", e.Message, e.Operation)
}

func (e *APIError) Unwrap() error {
	return e.Cause
}

// GetVirtualService retrieves a specific virtual service by UUID
func (c *Client) GetVirtualService(ctx context.Context, uuid string, params map[string]string) (interface{}, error) {
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
func (c *Client) CreateVirtualService(ctx context.Context, vsData map[string]interface{}) (interface{}, error) {
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
func (c *Client) UpdateVirtualService(ctx context.Context, uuid string, vsData map[string]interface{}) (interface{}, error) {
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
func (c *Client) ListPools(ctx context.Context, params map[string]string) (interface{}, error) {
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
func (c *Client) GetPool(ctx context.Context, uuid string, params map[string]string) (interface{}, error) {
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
func (c *Client) CreatePool(ctx context.Context, poolData map[string]interface{}) (interface{}, error) {
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
func (c *Client) ListHealthMonitors(ctx context.Context, params map[string]string) (interface{}, error) {
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
func (c *Client) GetHealthMonitor(ctx context.Context, uuid string, params map[string]string) (interface{}, error) {
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
func (c *Client) ListServiceEngines(ctx context.Context, params map[string]string) (interface{}, error) {
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
func (c *Client) GetServiceEngine(ctx context.Context, uuid string, params map[string]string) (interface{}, error) {
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
func (c *Client) GetAnalytics(ctx context.Context, resourceType, uuid string, params map[string]string) (interface{}, error) {
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