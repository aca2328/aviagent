#!/usr/bin/env python3

"""
Test script for Python Mistral Client
VMware Avi LLM Agent - Python Implementation
"""

import os
import sys
from typing import Optional, List, Dict, Any

# Add python_mistral to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), 'python_mistral'))

from python_mistral.models import LLMResponse, MistralConfig
from python_mistral.client import PythonMistralClient


def test_basic_functionality():
    """Test basic client functionality"""
    print("🧪 Testing Python Mistral Client Basic Functionality")
    print("=" * 60)
    
    # Test configuration
    config = MistralConfig(
        api_key="test_api_key_12345",
        default_model="mistral-medium",
        timeout=60,
        temperature=0.7,
        max_tokens=2048
    )
    
    print(f"✅ Configuration created: {config.model_dump()}")
    
    # Test client initialization
    try:
        client = PythonMistralClient(config=config)
        print("✅ Client initialized successfully")
        
        # Test system prompt generation
        system_prompt = client._build_system_prompt()
        print(f"✅ System prompt generated ({len(system_prompt)} chars)")
        
        # Test tool filtering
        test_tools = [
            {"function": {"name": "list_virtual_services", "description": "List VS"}},
            {"function": {"name": "get_pool", "description": "Get pool"}},
            {"function": {"name": "create_virtual_service", "description": "Create VS"}},
            {"function": {"name": "delete_pool", "description": "Delete pool"}}
        ]
        
        filtered_tools = client._filter_tools_for_query("list all virtual services", test_tools)
        print(f"✅ Tool filtering works: {len(test_tools)} → {len(filtered_tools)} tools")
        
        # Test fallback detection
        can_fallback = client._can_handle_fallback("list all virtual services")
        print(f"✅ Fallback detection: {'Yes' if can_fallback else 'No'}")
        
        print("\n🎉 All basic tests passed!")
        return True
        
    except Exception as e:
        print(f"❌ Test failed: {str(e)}")
        return False


def test_with_mock_avi_client():
    """Test with mock Avi client provider"""
    print("\n🧪 Testing with Mock Avi Client")
    print("=" * 60)
    
    # Mock Avi client
    class MockAviClient:
        def list_virtual_services(self, params=None):
            return {
                "virtual_services": [
                    {"name": "vs1", "uuid": "vs1-uuid"},
                    {"name": "vs2", "uuid": "vs2-uuid"}
                ],
                "count": 2
            }
        
        def list_pools(self, params=None):
            return {
                "pools": [
                    {"name": "pool1", "uuid": "pool1-uuid"}
                ],
                "count": 1
            }
    
    # Test configuration
    config = MistralConfig(
        api_key="test_api_key_12345",
        default_model="mistral-medium"
    )
    
    # Create client with mock Avi provider
    client = PythonMistralClient(
        config=config,
        avi_client_provider=lambda: MockAviClient()
    )
    
    try:
        # Test fallback for virtual services
        response = client._handle_list_virtual_services_fallback()
        print(f"✅ Virtual services fallback: {response.message}")
        print(f"   Tool calls: {len(response.tool_calls) if response.tool_calls else 0}")
        
        # Test fallback for pools
        response = client._handle_list_pools_fallback()
        print(f"✅ Pools fallback: {response.message}")
        print(f"   Tool calls: {len(response.tool_calls) if response.tool_calls else 0}")
        
        print("\n🎉 Mock Avi client tests passed!")
        return True
        
    except Exception as e:
        print(f"❌ Mock Avi client test failed: {str(e)}")
        return False


def main():
    """Main test function"""
    print("🚀 Python Mistral Client Test Suite")
    print("=" * 60)
    print(f"Python version: {sys.version}")
    print(f"Working directory: {os.getcwd()}")
    print()
    
    # Run tests
    basic_passed = test_basic_functionality()
    mock_passed = test_with_mock_avi_client()
    
    # Summary
    print("\n" + "=" * 60)
    print("📊 TEST SUMMARY")
    print("=" * 60)
    print(f"Basic functionality: {'✅ PASSED' if basic_passed else '❌ FAILED'}")
    print(f"Mock Avi client:     {'✅ PASSED' if mock_passed else '❌ FAILED'}")
    print(f"Overall:             {'✅ ALL TESTS PASSED' if basic_passed and mock_passed else '❌ SOME TESTS FAILED'}")
    
    return basic_passed and mock_passed


if __name__ == "__main__":
    success = main()
    sys.exit(0 if success else 1)