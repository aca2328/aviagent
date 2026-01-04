# Health Endpoint Analysis - http://localhost:8080/api/health

## Current Implementation Analysis

The health endpoint at `/api/health` is implemented in `internal/web/web-server.go` (lines 822-902). Here's what it does:

### Health Check Logic

1. **Basic Status**: Always returns `"status": "healthy"` initially
2. **Avi Client Check**: 
   - Checks if Avi client is initialized
   - If available and not a health check probe, performs a quick test: `ListVirtualServices(ctx, map[string]string{"limit_by": "1"})`
   - Updates `avi_status` to "healthy", "unhealthy", "initializing", or "skipped"

3. **LLM Client Check**:
   - For Ollama: Calls `ListModels(ctx)` to verify connection
   - For Mistral: Just checks if `mistralClient != nil` (no actual API call)
   - Skips checks for known health check probes (kube-probe, GoogleHC)

## Potential Issues Identified

### 1. **Avi Client Initialization Issues**
- The Avi client uses lazy initialization (`getAviClient()`)
- If Avi controller is not reachable, `avi_status` becomes "unhealthy"
- This could cause the health endpoint to report issues even when the app itself is running fine

### 2. **LLM Connection Issues**
- For Ollama: Actually calls the API, which could fail if Ollama service is down
- For Mistral: Only checks client existence, doesn't verify actual API connectivity
- No fallback or retry logic for failed LLM connections

### 3. **Health Check Probe Detection**
- Only detects `kube-probe/1.27` and `GoogleHC/1.0` user agents
- Other health check systems might not be detected, causing unnecessary API calls

### 4. **Error Handling**
- Errors from Avi or LLM checks are added to the response but don't change the overall `"status": "healthy"`
- This could be misleading - the app might report "healthy" even when key components are failing

### 5. **Performance Issues**
- Avi health check calls `ListVirtualServices` which could be slow
- No caching of health status between requests
- Multiple concurrent health checks could overload the Avi controller

## Common Failure Scenarios

### Scenario 1: Avi Controller Unreachable
```json
{
  "status": "healthy",
  "avi_status": "unhealthy",
  "avi_error": "connection refused to avi-controller:443",
  "llm_status": "healthy"
}
```

### Scenario 2: Ollama Service Down
```json
{
  "status": "healthy", 
  "avi_status": "healthy",
  "llm_status": "unhealthy",
  "llm_error": "connection refused to localhost:11434"
}
```

### Scenario 3: Mistral Client Not Initialized
```json
{
  "status": "healthy",
  "avi_status": "healthy", 
  "llm_status": "unhealthy",
  "llm_error": "Mistral client not initialized"
}
```

## Recommended Fixes

### 1. **Improve Overall Status Logic**
```go
// Change from always "healthy" to comprehensive status
isHealthy := true
if status["avi_status"] == "unhealthy" {
    isHealthy = false
}
if status["llm_status"] == "unhealthy" {
    isHealthy = false
}

if isHealthy {
    status["status"] = "healthy"
} else {
    status["status"] = "degraded"
}
```

### 2. **Add Avi Client Connection Timeout**
```go
// Add timeout to Avi client initialization
ctx, cancel := context.WithTimeout(c.Request.Context(), 1*time.Second)
defer cancel()

aviClient, err := s.getAviClient()
if err != nil {
    status["avi_status"] = "unhealthy"
    status["avi_error"] = "timeout: " + err.Error()
}
```

### 3. **Improve Mistral Health Check**
```go
// For Mistral, add a lightweight API call to verify connectivity
if s.config.Provider == "mistral" && s.mistralClient != nil {
    // Try a lightweight API call (e.g., list models with limit=1)
    ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
    defer cancel()
    
    models, err := s.mistralClient.ListModels(ctx)
    if err != nil || len(models) == 0 {
        status["llm_status"] = "unhealthy"
        status["llm_error"] = "connection test failed: " + err.Error()
    } else {
        status["llm_status"] = "healthy"
    }
}
```

### 4. **Add Health Check Caching**
```go
// Add caching with short TTL (e.g., 30 seconds)
type HealthCache struct {
    LastCheck   time.Time
    AviStatus   string
    LLMStatus   string
    OverallStatus string
}

func (s *Server) getCachedHealthStatus() (*HealthCache, bool) {
    s.healthCacheMu.Lock()
    defer s.healthCacheMu.Unlock()
    
    if time.Since(s.healthCache.LastCheck) < 30*time.Second {
        return &s.healthCache, true
    }
    return nil, false
}
```

### 5. **Better Probe Detection**
```go
// Expand probe detection
func (s *Server) isHealthCheckProbe(c *gin.Context) bool {
    userAgent := c.Request.Header.Get("User-Agent")
    return strings.Contains(userAgent, "kube-probe") ||
           strings.Contains(userAgent, "GoogleHC") ||
           strings.Contains(userAgent, "healthcheck") ||
           strings.Contains(userAgent, "load-balancer-health-check") ||
           strings.Contains(userAgent, "ELB-HealthChecker") ||
           strings.Contains(userAgent, "AWS-HealthChecker")
}
```

## Debugging Steps

### 1. **Check Current Health Status**
```bash
curl -v http://localhost:8080/api/health
```

### 2. **Check with Different User Agents**
```bash
# Simulate kube-probe
curl -H "User-Agent: kube-probe/1.27" http://localhost:8080/api/health

# Simulate browser
curl -H "User-Agent: Mozilla/5.0" http://localhost:8080/api/health
```

### 3. **Check Component Connectivity**
```bash
# Test Avi controller connectivity
nc -zv avi-controller 443

# Test Ollama connectivity  
nc -zv localhost 11434

# Test Mistral API (if applicable)
curl -v https://api.mistral.ai/v1/models
```

### 4. **Check Application Logs**
```bash
# Look for health endpoint calls and errors
grep "HEALTH ENDPOINT CALLED" app.log
grep "avi_status.*unhealthy" app.log
grep "llm_status.*unhealthy" app.log
```

## Implementation Recommendations

### Short-term Fixes (Quick Wins)
1. **Fix the misleading status**: Change overall status based on component health
2. **Add timeouts**: Prevent health checks from hanging
3. **Improve probe detection**: Reduce unnecessary API calls

### Long-term Improvements
1. **Add health caching**: Reduce load on Avi controller
2. **Implement circuit breakers**: Prevent cascading failures
3. **Add metrics**: Track health check performance and failures
4. **Implement readiness vs liveness**: Separate component checks

## Expected Healthy Response

When everything is working correctly, the response should look like:

```json
{
  "status": "healthy",
  "timestamp": "2023-11-15T14:30:45Z",
  "provider": "mistral",
  "version": "1.1.0",
  "build_date": "2026-01-01",
  "app_name": "VMware Avi LLM Agent",
  "debug_server_ptr": "0xc0001a2b30",
  "avi_status": "healthy",
  "llm_status": "healthy"
}
```

## Troubleshooting Guide

### If Avi Status is Unhealthy
1. Check Avi controller is running and reachable
2. Verify network connectivity to Avi controller
3. Check Avi controller credentials in config.yaml
4. Test Avi API directly: `curl -k https://avi-controller/api/virtualservice`

### If LLM Status is Unhealthy
1. For Ollama: Check Ollama service is running
2. For Mistral: Verify API key and network access
3. Test LLM API directly based on provider
4. Check LLM configuration in config.yaml

### If Overall Status is Degraded
1. Check which component is failing (avi_status or llm_status)
2. Follow the specific troubleshooting for that component
3. Check application logs for detailed error messages
4. Consider restarting the application if the issue persists