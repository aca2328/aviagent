#!/usr/bin/env python3

import requests
import json
import time
from requests.auth import HTTPBasicAuth

# Avi Controller Configuration
AVI_HOST = "35.164.151.144"
AVI_USERNAME = "aviagent"
AVI_PASSWORD = "uoPNu0h=aD"

# Disable SSL verification for testing
verify_ssl = False

def test_direct_api_call():
    """Test the exact same API call that the application makes"""
    print("=== Testing Direct API Call (same as application) ===")
    
    url = f"https://{AVI_HOST}/api/virtualservice?limit_by=1"
    
    try:
        start_time = time.time()
        response = requests.get(
            url,
            auth=HTTPBasicAuth(AVI_USERNAME, AVI_PASSWORD),
            verify=verify_ssl,
            timeout=5
        )
        elapsed_time = time.time() - start_time
        
        print(f"URL: {url}")
        print(f"Status Code: {response.status_code}")
        print(f"Response Time: {elapsed_time:.2f} seconds")
        print(f"Headers: {dict(response.headers)}")
        
        if response.status_code == 200:
            data = response.json()
            print(f"✅ SUCCESS: Retrieved virtual services")
            print(f"Response: {json.dumps(data, indent=2)}")
            return True, data
        else:
            print(f"❌ ERROR: {response.text}")
            return False, None
            
    except requests.exceptions.Timeout:
        print("❌ ERROR: Request timed out")
        return False, None
    except requests.exceptions.ConnectionError as e:
        print(f"❌ ERROR: Connection failed - {e}")
        return False, None
    except Exception as e:
        print(f"❌ ERROR: {e}")
        return False, None

def test_with_context_timeout():
    """Test with the same 2-second timeout as the application"""
    print("\n=== Testing with 2-second timeout (like application) ===")
    
    url = f"https://{AVI_HOST}/api/virtualservice?limit_by=1"
    
    try:
        start_time = time.time()
        response = requests.get(
            url,
            auth=HTTPBasicAuth(AVI_USERNAME, AVI_PASSWORD),
            verify=verify_ssl,
            timeout=2  # Same as application's context timeout
        )
        elapsed_time = time.time() - start_time
        
        print(f"Status Code: {response.status_code}")
        print(f"Response Time: {elapsed_time:.2f} seconds")
        
        if response.status_code == 200:
            print("✅ SUCCESS: Call completed within timeout")
            return True
        else:
            print(f"❌ ERROR: {response.text}")
            return False
            
    except requests.exceptions.Timeout:
        print("❌ ERROR: Request timed out (2 seconds)")
        return False
    except Exception as e:
        print(f"❌ ERROR: {e}")
        return False

def compare_with_app_health():
    """Compare with application health endpoint"""
    print("\n=== Application Health Check ===")
    
    try:
        health_response = requests.get("http://localhost:8080/api/health", timeout=10)
        health_data = health_response.json()
        
        print(f"Overall Status: {health_data.get('status')}")
        print(f"Avi Status: {health_data.get('avi_status')}")
        print(f"LLM Status: {health_data.get('llm_status')}")
        
        if health_data.get('avi_error'):
            error_msg = health_data.get('avi_error')
            if "context deadline exceeded" in error_msg:
                print("🔍 ANALYSIS: Application is timing out on Avi API calls")
            print(f"Avi Error: {error_msg}")
            
    except Exception as e:
        print(f"Error accessing application: {e}")

if __name__ == "__main__":
    print("Avi Controller API Test")
    print(f"Controller: {AVI_HOST}")
    print(f"Username: {AVI_USERNAME}")
    print("=" * 60)
    
    # Test direct API call
    success1, data1 = test_direct_api_call()
    
    # Test with application timeout
    success2 = test_with_context_timeout()
    
    # Compare with application
    compare_with_app_health()
    
    print("\n" + "=" * 60)
    print("ANALYSIS:")
    print(f"Direct API Call: {'✅ PASS' if success1 else '❌ FAIL'}")
    print(f"Timeout Test (2s): {'✅ PASS' if success2 else '❌ FAIL'}")
    
    if not success1 and not success2:
        print("\n🔍 ROOT CAUSE:")
        print("- The Avi controller is not responding to API requests")
        print("- This could be due to network connectivity, authentication, or controller issues")
        print("- The application's health check fails because it has a 2-second timeout")
    elif success1 and not success2:
        print("\n🔍 ROOT CAUSE:")
        print("- The API works but is slow (takes more than 2 seconds)")
        print("- The application's health check times out because of the strict 2-second limit")
        print("- Consider increasing the timeout or optimizing the API call")