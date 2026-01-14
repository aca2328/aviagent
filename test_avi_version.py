#!/usr/bin/env python3

import requests
import json

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

def test_different_versions():
    """Test with different Avi version headers"""
    print("=== Testing Different Avi Versions ===")
    
    session_id, csrf_token = get_session()
    if not session_id or not csrf_token:
        return
    
    # Test different version formats
    versions_to_test = [
        "31.2.1",
        "31.2",
        "31",
        "22.1.6",  # Common stable version
        "22.1",
        "21.1.3",
        "20.1.8",
        "",  # No version header
    ]
    
    api_url = f"https://{AVI_HOST}/api/virtualservice?limit_by=1"
    
    for version in versions_to_test:
        cookies = {'sessionid': session_id}
        headers = {
            'X-CSRFToken': csrf_token,
            'Referer': f'https://{AVI_HOST}/',
        }
        
        if version:  # Only add version header if version is not empty
            headers['X-Avi-Version'] = version
        
        try:
            response = requests.get(
                api_url,
                cookies=cookies,
                headers=headers,
                verify=verify_ssl,
                timeout=5
            )
            
            print(f"Version '{version}': Status={response.status_code}")
            
            if response.status_code == 200:
                print(f"  ✅ SUCCESS with version '{version}'")
                data = response.json()
                print(f"  Retrieved {len(data.get('results', []))} virtual services")
                return version  # Return the working version
            elif response.status_code == 400:
                error_data = response.json()
                print(f"  ❌ ERROR: {error_data.get('error', 'Unknown error')}")
            else:
                print(f"  ❌ ERROR: Unexpected status {response.status_code}")
                
        except Exception as e:
            print(f"  ❌ EXCEPTION: {e}")
    
    return None

def test_controller_version():
    """Try to discover the controller's version"""
    print("\n=== Discovering Controller Version ===")
    
    # Try to get controller version info
    url = f"https://{AVI_HOST}/api/cluster/runtime"
    
    try:
        response = requests.get(url, verify=verify_ssl, timeout=5)
        if response.status_code == 200:
            data = response.json()
            print(f"Controller info: {json.dumps(data, indent=2)}")
        else:
            print(f"Could not get controller info: {response.text}")
    except Exception as e:
        print(f"Error getting controller info: {e}")

def test_with_working_version(working_version):
    """Test multiple calls with the working version"""
    if not working_version:
        print("\nNo working version found")
        return
    
    print(f"\n=== Testing with Working Version: {working_version} ===")
    
    session_id, csrf_token = get_session()
    if not session_id or not csrf_token:
        return
    
    api_url = f"https://{AVI_HOST}/api/virtualservice?limit_by=1"
    
    # Test multiple calls
    for i in range(3):
        cookies = {'sessionid': session_id}
        headers = {
            'X-CSRFToken': csrf_token,
            'Referer': f'https://{AVI_HOST}/',
            'X-Avi-Version': working_version
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
            
            print(f"Call {i+1}: Status={response.status_code}, Time={elapsed_time:.2f}s")
            
            if response.status_code == 200:
                print(f"  ✅ SUCCESS")
            else:
                print(f"  ❌ ERROR: {response.text}")
                
        except requests.exceptions.Timeout:
            print(f"  ❌ TIMEOUT after 2 seconds")
        except Exception as e:
            print(f"  ❌ EXCEPTION: {e}")

if __name__ == "__main__":
    print("Avi Controller Version Compatibility Test")
    print(f"Controller: {AVI_HOST}")
    print("=" * 60)
    
    # Test different versions
    working_version = test_different_versions()
    
    # Try to discover controller version
    test_controller_version()
    
    # Test with working version
    test_with_working_version(working_version)
    
    print("\n" + "=" * 60)
    if working_version:
        print(f"🎉 FOUND WORKING VERSION: {working_version}")
        print("Recommend updating the application configuration to use this version")
    else:
        print("❌ No working version found")
        print("The Avi controller may have specific version requirements")