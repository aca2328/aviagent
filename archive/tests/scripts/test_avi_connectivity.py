#!/usr/bin/env python3

import requests
import json
import time
from requests.auth import HTTPBasicAuth

# Avi Controller Configuration
AVI_HOST = "35.164.151.144"
AVI_USERNAME = "aviagent"
AVI_PASSWORD = "uoPNu0h=aD"
AVI_VERSION = "31.2.1"

# Disable SSL verification for testing (not recommended for production)
verify_ssl = False

def test_basic_connectivity():
    """Test basic connectivity to the Avi controller"""
    print("=== Testing Basic Connectivity ===")
    
    # Test if we can reach the controller
    url = f"https://{AVI_HOST}/api/virtualservice"
    
    try:
        # First test with a simple GET request
        start_time = time.time()
        response = requests.get(
            url,
            auth=HTTPBasicAuth(AVI_USERNAME, AVI_PASSWORD),
            verify=verify_ssl,
            timeout=10
        )
        elapsed_time = time.time() - start_time
        
        print(f"Status Code: {response.status_code}")
        print(f"Response Time: {elapsed_time:.2f} seconds")
        print(f"Content-Type: {response.headers.get('Content-Type')}")
        
        if response.status_code == 200:
            data = response.json()
            print(f"Success! Retrieved {len(data.get('results', []))} virtual services")
            print(f"Total Count: {data.get('count', 0)}")
            return True, data
        else:
            print(f"Error: {response.text}")
            return False, None
            
    except requests.exceptions.Timeout:
        print("Error: Request timed out")
        return False, None
    except requests.exceptions.ConnectionError as e:
        print(f"Error: Connection failed - {e}")
        return False, None
    except Exception as e:
        print(f"Error: {e}")
        return False, None

def test_session_authentication():
    """Test session-based authentication"""
    print("\n=== Testing Session Authentication ===")
    
    # Step 1: Login to get session
    login_url = f"https://{AVI_HOST}/login"
    login_data = {
        "username": AVI_USERNAME,
        "password": AVI_PASSWORD
    }
    
    try:
        # Login request
        login_response = requests.post(
            login_url,
            json=login_data,
            verify=verify_ssl,
            timeout=10
        )
        
        print(f"Login Status Code: {login_response.status_code}")
        
        if login_response.status_code == 200:
            session_data = login_response.json()
            session_id = session_data.get('session_id')
            csrf_token = session_data.get('csrftoken')
            
            print(f"Session ID: {session_id}")
            print(f"CSRF Token: {csrf_token}")
            
            # Step 2: Use session to get virtual services
            if session_id and csrf_token:
                cookies = {'sessionid': session_id}
                headers = {
                    'X-CSRFToken': csrf_token,
                    'Referer': f'https://{AVI_HOST}/'
                }
                
                vs_url = f"https://{AVI_HOST}/api/virtualservice"
                vs_response = requests.get(
                    vs_url,
                    cookies=cookies,
                    headers=headers,
                    verify=verify_ssl,
                    timeout=10
                )
                
                print(f"VS Request Status Code: {vs_response.status_code}")
                
                if vs_response.status_code == 200:
                    vs_data = vs_response.json()
                    print(f"Success! Retrieved {len(vs_data.get('results', []))} virtual services")
                    return True, vs_data
                else:
                    print(f"VS Request Error: {vs_response.text}")
                    return False, None
        else:
            print(f"Login Error: {login_response.text}")
            return False, None
            
    except Exception as e:
        print(f"Session Auth Error: {e}")
        return False, None

def test_with_limit():
    """Test with limit parameter as used in health check"""
    print("\n=== Testing with Limit Parameter ===")
    
    url = f"https://{AVI_HOST}/api/virtualservice?limit_by=1"
    
    try:
        response = requests.get(
            url,
            auth=HTTPBasicAuth(AVI_USERNAME, AVI_PASSWORD),
            verify=verify_ssl,
            timeout=10
        )
        
        print(f"Status Code: {response.status_code}")
        
        if response.status_code == 200:
            data = response.json()
            print(f"Success! Retrieved limited virtual services")
            print(f"Results: {json.dumps(data, indent=2)}")
            return True, data
        else:
            print(f"Error: {response.text}")
            return False, None
            
    except Exception as e:
        print(f"Error: {e}")
        return False, None

def compare_with_app():
    """Compare results with the application's health endpoint"""
    print("\n=== Comparing with Application Health Endpoint ===")
    
    try:
        # Get health status from our application
        health_response = requests.get("http://localhost:8080/api/health", timeout=10)
        health_data = health_response.json()
        
        print(f"Application Health Status: {health_data.get('status')}")
        print(f"Avi Status: {health_data.get('avi_status')}")
        print(f"LLM Status: {health_data.get('llm_status')}")
        
        if health_data.get('avi_error'):
            print(f"Avi Error: {health_data.get('avi_error')}")
            
    except Exception as e:
        print(f"Error accessing application: {e}")

if __name__ == "__main__":
    print("Avi Controller Connectivity Test")
    print(f"Controller: {AVI_HOST}")
    print(f"Username: {AVI_USERNAME}")
    print("=" * 50)
    
    # Run all tests
    success1, data1 = test_basic_connectivity()
    success2, data2 = test_session_authentication()
    success3, data3 = test_with_limit()
    
    # Compare with application
    compare_with_app()
    
    print("\n" + "=" * 50)
    print("SUMMARY:")
    print(f"Basic Connectivity: {'✅ PASS' if success1 else '❌ FAIL'}")
    print(f"Session Authentication: {'✅ PASS' if success2 else '❌ FAIL'}")
    print(f"Limit Parameter Test: {'✅ PASS' if success3 else '❌ FAIL'}")