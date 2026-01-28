#!/usr/bin/env python3

import requests
import json
import time

# Avi Controller Configuration
AVI_HOST = "35.164.151.144"
AVI_USERNAME = "aviagent"
AVI_PASSWORD = "uoPNu0h=aD"

# Disable SSL verification for testing
verify_ssl = False

def test_session_based_api_call():
    """Test API call using session authentication (like the application)"""
    print("=== Testing Session-Based API Call (like application) ===")
    
    # Step 1: Login to get session
    login_url = f"https://{AVI_HOST}/login"
    login_data = {
        "username": AVI_USERNAME,
        "password": AVI_PASSWORD
    }
    
    try:
        # Login request
        print("1. Attempting login...")
        login_response = requests.post(
            login_url,
            json=login_data,
            verify=verify_ssl,
            timeout=10
        )
        
        print(f"   Login Status: {login_response.status_code}")
        
        if login_response.status_code == 200:
            # Extract session information
            session_id = None
            csrf_token = None
            
            # Try to get from cookies first
            for cookie in login_response.cookies:
                if cookie.name == "avi-sessionid" or cookie.name == "sessionid":
                    session_id = cookie.value
                if cookie.name == "csrftoken":
                    csrf_token = cookie.value
            
            # If not in cookies, try to parse from JSON response
            if not session_id or not csrf_token:
                try:
                    session_data = login_response.json()
                    if not session_id:
                        session_id = session_data.get('session_id')
                    if not csrf_token:
                        csrf_token = session_data.get('csrftoken')
                except:
                    pass
            
            print(f"   Session ID: {session_id}")
            print(f"   CSRF Token: {csrf_token}")
            
            if session_id and csrf_token:
                # Step 2: Make API call with session
                print("2. Making API call with session...")
                api_url = f"https://{AVI_HOST}/api/virtualservice?limit_by=1"
                
                cookies = {'sessionid': session_id}
                headers = {
                    'X-CSRFToken': csrf_token,
                    'Referer': f'https://{AVI_HOST}/'
                }
                
                start_time = time.time()
                api_response = requests.get(
                    api_url,
                    cookies=cookies,
                    headers=headers,
                    verify=verify_ssl,
                    timeout=10
                )
                elapsed_time = time.time() - start_time
                
                print(f"   API Status: {api_response.status_code}")
                print(f"   Response Time: {elapsed_time:.2f} seconds")
                
                if api_response.status_code == 200:
                    data = api_response.json()
                    print(f"   ✅ SUCCESS: Retrieved virtual services")
                    print(f"   Response: {json.dumps(data, indent=2)}")
                    return True, elapsed_time, data
                else:
                    print(f"   ❌ ERROR: {api_response.text}")
                    return False, elapsed_time, None
            else:
                print("   ❌ ERROR: Failed to get session ID or CSRF token")
                return False, 0, None
        else:
            print(f"   ❌ ERROR: Login failed - {login_response.text}")
            return False, 0, None
            
    except Exception as e:
        print(f"   ❌ ERROR: {e}")
        return False, 0, None

def test_with_application_timeout():
    """Test with the same 2-second timeout as the application"""
    print("\n=== Testing with Application Timeout (2 seconds) ===")
    
    # Step 1: Login
    login_url = f"https://{AVI_HOST}/login"
    login_data = {"username": AVI_USERNAME, "password": AVI_PASSWORD}
    
    try:
        login_response = requests.post(login_url, json=login_data, verify=verify_ssl, timeout=10)
        
        if login_response.status_code == 200:
            # Extract session info
            session_id = None
            csrf_token = None
            for cookie in login_response.cookies:
                if cookie.name in ["avi-sessionid", "sessionid"]:
                    session_id = cookie.value
                if cookie.name == "csrftoken":
                    csrf_token = cookie.value
            
            if session_id and csrf_token:
                # Step 2: API call with 2-second timeout
                api_url = f"https://{AVI_HOST}/api/virtualservice?limit_by=1"
                cookies = {'sessionid': session_id}
                headers = {'X-CSRFToken': csrf_token, 'Referer': f'https://{AVI_HOST}/'}
                
                start_time = time.time()
                api_response = requests.get(
                    api_url,
                    cookies=cookies,
                    headers=headers,
                    verify=verify_ssl,
                    timeout=2  # Same as application
                )
                elapsed_time = time.time() - start_time
                
                print(f"   Status: {api_response.status_code}")
                print(f"   Time: {elapsed_time:.2f} seconds")
                
                if api_response.status_code == 200:
                    print("   ✅ SUCCESS: Completed within timeout")
                    return True, elapsed_time
                else:
                    print(f"   ❌ ERROR: {api_response.text}")
                    return False, elapsed_time
            else:
                print("   ❌ ERROR: No session info")
                return False, 0
        else:
            print(f"   ❌ ERROR: Login failed")
            return False, 0
            
    except requests.exceptions.Timeout:
        print("   ❌ ERROR: Request timed out (2 seconds)")
        return False, 2.0
    except Exception as e:
        print(f"   ❌ ERROR: {e}")
        return False, 0

if __name__ == "__main__":
    print("Avi Controller Session Authentication Test")
    print(f"Controller: {AVI_HOST}")
    print(f"Username: {AVI_USERNAME}")
    print("=" * 60)
    
    # Test with normal timeout
    success1, time1, data1 = test_session_based_api_call()
    
    # Test with application timeout
    success2, time2 = test_with_application_timeout()
    
    print("\n" + "=" * 60)
    print("COMPARISON WITH APPLICATION:")
    print(f"Session API Call: {'✅ PASS' if success1 else '❌ FAIL'} ({time1:.2f}s)")
    print(f"App Timeout Test: {'✅ PASS' if success2 else '❌ FAIL'} ({time2:.2f}s)")
    
    if success1 and not success2:
        print("\n🔍 ROOT CAUSE ANALYSIS:")
        print(f"- Session authentication works (took {time1:.2f} seconds)")
        print("- Application fails because it uses a 2-second timeout")
        print("- The API call takes longer than 2 seconds to complete")
        print("- This causes 'context deadline exceeded' errors in the application")
        print("\n💡 RECOMMENDATION:")
        print("- Increase the health check timeout in the application")
        print("- Or optimize the API call to be faster")
        print("- Or use caching to avoid repeated slow calls")
    elif not success1:
        print("\n🔍 ROOT CAUSE ANALYSIS:")
        print("- Session authentication is failing")
        print("- This could be due to credentials, network, or controller issues")
    else:
        print("\n✅ Both tests passed - API is working correctly")