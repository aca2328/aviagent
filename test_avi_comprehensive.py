#!/usr/bin/env python3

import requests
import json
import time
import concurrent.futures

# Avi Controller Configuration
AVI_HOST = "35.164.151.144"
AVI_USERNAME = "aviagent"
AVI_PASSWORD = "uoPNu0h=aD"

# Disable SSL verification for testing
verify_ssl = False

def get_session():
    """Get session ID and CSRF token"""
    login_url = f"https://{AVI_HOST}/login"
    login_data = {"username": AVI_USERNAME, "password": AVI_PASSWORD}
    
    try:
        login_response = requests.post(login_url, json=login_data, verify=verify_ssl, timeout=10)
        
        if login_response.status_code == 200:
            session_id = None
            csrf_token = None
            
            # Try to get from cookies
            for cookie in login_response.cookies:
                if cookie.name in ["avi-sessionid", "sessionid"]:
                    session_id = cookie.value
                if cookie.name == "csrftoken":
                    csrf_token = cookie.value
            
            return session_id, csrf_token
        else:
            print(f"Login failed: {login_response.text}")
            return None, None
            
    except Exception as e:
        print(f"Login error: {e}")
        return None, None

def test_api_call_with_session(session_id, csrf_token, call_number):
    """Test API call with session"""
    api_url = f"https://{AVI_HOST}/api/virtualservice?limit_by=1"
    
    cookies = {'sessionid': session_id}
    headers = {
        'X-CSRFToken': csrf_token,
        'Referer': f'https://{AVI_HOST}/',
        'X-Avi-Version': '31.2.1'  # Add version header like the application
    }
    
    try:
        start_time = time.time()
        response = requests.get(
            api_url,
            cookies=cookies,
            headers=headers,
            verify=verify_ssl,
            timeout=2  # Same as application
        )
        elapsed_time = time.time() - start_time
        
        print(f"Call {call_number}: Status={response.status_code}, Time={elapsed_time:.2f}s")
        
        if response.status_code == 200:
            data = response.json()
            return "success", elapsed_time, response.status_code
        else:
            print(f"  Error: {response.text}")
            return "error", elapsed_time, response.status_code
            
    except requests.exceptions.Timeout:
        print(f"  Timeout after 2 seconds")
        return "timeout", 2.0, None
    except Exception as e:
        print(f"  Exception: {e}")
        return "exception", 0, None

def test_multiple_concurrent_calls():
    """Test multiple concurrent API calls to simulate health check behavior"""
    print("=== Testing Multiple Concurrent API Calls ===")
    
    # Get session first
    session_id, csrf_token = get_session()
    if not session_id or not csrf_token:
        print("Failed to get session")
        return
    
    print(f"Session ID: {session_id}")
    print(f"CSRF Token: {csrf_token}")
    
    # Test 5 concurrent calls (simulating multiple health checks)
    results = []
    
    with concurrent.futures.ThreadPoolExecutor(max_workers=5) as executor:
        futures = []
        for i in range(5):
            futures.append(executor.submit(
                test_api_call_with_session, session_id, csrf_token, i+1
            ))
        
        for future in concurrent.futures.as_completed(futures):
            results.append(future.result())
    
    # Analyze results
    success_count = sum(1 for r in results if r[0] == "success")
    error_count = sum(1 for r in results if r[0] == "error")
    timeout_count = sum(1 for r in results if r[0] == "timeout")
    
    print(f"\nResults: {success_count} success, {error_count} errors, {timeout_count} timeouts")
    
    # Show any errors
    for i, result in enumerate(results):
        if result[0] == "error":
            print(f"  Call {i+1} failed with status {result[2]}")

def test_with_different_parameters():
    """Test with different query parameters"""
    print("\n=== Testing with Different Parameters ===")
    
    session_id, csrf_token = get_session()
    if not session_id or not csrf_token:
        return
    
    # Test different parameter combinations
    test_cases = [
        {"params": {"limit_by": "1"}, "description": "limit_by=1 (like application)"},
        {"params": {"limit": "1"}, "description": "limit=1 (alternative)"},
        {"params": {"page_size": "1"}, "description": "page_size=1"},
        {"params": {}, "description": "no parameters"},
    ]
    
    for test_case in test_cases:
        api_url = f"https://{AVI_HOST}/api/virtualservice"
        
        cookies = {'sessionid': session_id}
        headers = {
            'X-CSRFToken': csrf_token,
            'Referer': f'https://{AVI_HOST}/',
            'X-Avi-Version': '31.2.1'
        }
        
        try:
            response = requests.get(
                api_url,
                params=test_case["params"],
                cookies=cookies,
                headers=headers,
                verify=verify_ssl,
                timeout=5
            )
            
            print(f"{test_case['description']}: Status={response.status_code}")
            
            if response.status_code != 200:
                print(f"  Error response: {response.text}")
                
        except Exception as e:
            print(f"{test_case['description']}: Exception - {e}")

def test_session_reuse():
    """Test reusing the same session multiple times"""
    print("\n=== Testing Session Reuse ===")
    
    # Get session
    session_id, csrf_token = get_session()
    if not session_id or not csrf_token:
        return
    
    # Make multiple calls with the same session
    for i in range(3):
        result = test_api_call_with_session(session_id, csrf_token, i+1)
        time.sleep(0.5)  # Small delay between calls
    
    print("Session reuse test completed")

if __name__ == "__main__":
    print("Comprehensive Avi Controller API Test")
    print(f"Controller: {AVI_HOST}")
    print("=" * 60)
    
    # Run all tests
    test_multiple_concurrent_calls()
    test_with_different_parameters()
    test_session_reuse()
    
    print("\n" + "=" * 60)
    print("Test completed - check for any 400 errors or timeouts")