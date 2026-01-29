#!/usr/bin/env python3

"""
Test script for Python Mistral client
"""

import sys
import os

# Add the python_mistral directory to the Python path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), 'python_mistral'))

# Now we can import the modules
from client import PythonMistralClient
from config import MistralConfig

def test_mistral_client():
    """Test Mistral client connectivity"""
    try:
        print("Testing Python Mistral client...")
        
        # Load configuration
        config = MistralConfig.from_env_and_data()
        print(f"Configuration loaded: model={config.default_model}, timeout={config.timeout}")
        
        # Create client
        client = PythonMistralClient(config=config)
        print("Client created successfully")
        
        # Test simple chat completion
        messages = [{'role': 'user', 'content': 'Hello, test connection'}]
        print("Sending test message to Mistral API...")
        
        response = client.chat_completion(messages=messages)
        
        print('SUCCESS: Mistral API connection working')
        print(f'Response: {response.message[:100]}...')
        print(f'Model: {response.model}')
        print(f'Usage: {response.usage}')
        
        return True
        
    except Exception as e:
        print(f'ERROR: {type(e).__name__}: {str(e)}')
        import traceback
        traceback.print_exc()
        return False

if __name__ == "__main__":
    success = test_mistral_client()
    sys.exit(0 if success else 1)