"""
Python-Go Bridge for Mistral Client
VMware Avi LLM Agent - Hybrid Integration
"""

import json
import sys
import traceback
from typing import Optional, List, Dict, Any

# Add site-packages to Python path for Docker environment
site_packages_path = '/usr/local/lib/python3.11/site-packages'
if site_packages_path not in sys.path:
    sys.path.append(site_packages_path)

from .client import PythonMistralClient
from .config import MistralConfig
from .models import LLMResponse


class PythonGoBridge:
    """
    Bridge between Python Mistral client and Go web server
    
    This class provides a simple interface for Go to call Python Mistral client
    using JSON serialization for cross-language communication.
    """
    
    def __init__(self):
        self.client = None
        self.initialized = False
    
    def initialize(self, config_json: str, avi_client_provider: Optional[callable] = None) -> str:
        """
        Initialize Python Mistral client from JSON configuration
        
        Args:
            config_json: JSON string with Mistral configuration
            avi_client_provider: Optional Avi client provider function
            
        Returns:
            str: JSON response with status and error (if any)
        """
        try:
            config_data = json.loads(config_json)
            config = MistralConfig.from_env_and_data(**config_data)
            
            self.client = PythonMistralClient(config=config, avi_client_provider=avi_client_provider)
            self.initialized = True
            
            return json.dumps({
                "status": "success",
                "message": "Python Mistral client initialized successfully",
                "model": config.default_model,
                "timeout": config.timeout
            })
            
        except Exception as e:
            return json.dumps({
                "status": "error",
                "message": str(e),
                "error_type": type(e).__name__,
                "traceback": traceback.format_exc()
            })
    
    def process_query(self, query_json: str) -> str:
        """
        Process natural language query
        
        Args:
            query_json: JSON string with query data
            
        Returns:
            str: JSON response with LLM response or error
        """
        # Initialize client if not already initialized (lazy initialization)
        if not self.initialized or not self.client:
            # Try to initialize with configuration from environment
            try:
                config = MistralConfig.from_env_and_data()  # Load from environment
                self.client = PythonMistralClient(config=config, avi_client_provider=None)
                self.initialized = True
            except Exception as e:
                return json.dumps({
                    "status": "error",
                    "message": f"Failed to initialize Python Mistral client: {str(e)}",
                    "error_type": type(e).__name__
                })
        
        try:
            query_data = json.loads(query_json)
            
            # Extract query parameters
            query = query_data.get("query", "")
            conversation_history = query_data.get("conversation_history", [])
            tools = query_data.get("tools", [])
            
            # Process query
            response = self.client.process_natural_language_query(
                query=query,
                conversation_history=conversation_history,
                tools=tools
            )
            
            # Convert response to JSON
            return json.dumps({
                "status": "success",
                "response": response.model_dump()
            })
            
        except Exception as e:
            return json.dumps({
                "status": "error",
                "message": str(e),
                "error_type": type(e).__name__,
                "traceback": traceback.format_exc()
            })
    
    def chat_completion(self, request_json: str) -> str:
        """
        Send chat completion request
        
        Args:
            request_json: JSON string with chat completion request
            
        Returns:
            str: JSON response with chat completion result or error
        """
        # Initialize client if not already initialized (lazy initialization)
        if not self.initialized or not self.client:
            # Try to initialize with configuration from environment
            try:
                config = MistralConfig.from_env_and_data()  # Load from environment
                self.client = PythonMistralClient(config=config, avi_client_provider=None)
                self.initialized = True
            except Exception as e:
                return json.dumps({
                    "status": "error",
                    "message": f"Failed to initialize Python Mistral client: {str(e)}",
                    "error_type": type(e).__name__
                })
        
        try:
            request_data = json.loads(request_json)
            
            # Extract request parameters
            messages = request_data.get("messages", [])
            tools = request_data.get("tools", None)
            model = request_data.get("model", None)
            temperature = request_data.get("temperature", None)
            max_tokens = request_data.get("max_tokens", None)
            
            # Send chat completion
            response = self.client.chat_completion(
                messages=messages,
                tools=tools,
                model=model,
                temperature=temperature,
                max_tokens=max_tokens
            )
            
            return json.dumps({
                "status": "success",
                "response": response.model_dump()
            })
            
        except Exception as e:
            return json.dumps({
                "status": "error",
                "message": str(e),
                "error_type": type(e).__name__,
                "traceback": traceback.format_exc()
            })
    
    def get_status(self) -> str:
        """
        Get bridge status
        
        Returns:
            str: JSON response with bridge status
        """
        return json.dumps({
            "status": "success",
            "initialized": self.initialized,
            "client_available": self.client is not None,
            "python_version": sys.version
        })


def main():
    """
    Main function for standalone bridge operation
    """
    import sys
    
    # Check if we have command line arguments (for non-interactive mode)
    if len(sys.argv) > 1:
        bridge = PythonGoBridge()
        cmd = sys.argv[1].lower()
        
        try:
            if cmd == "initialize":
                if len(sys.argv) > 2:
                    config_json = sys.argv[2]
                    result = bridge.initialize(config_json)
                    print(result)
                else:
                    print(json.dumps({"status": "error", "message": "Missing config JSON argument"}))
            elif cmd == "query":
                if len(sys.argv) > 2:
                    query_json = sys.argv[2]
                    result = bridge.process_query(query_json)
                    print(result)
                else:
                    print(json.dumps({"status": "error", "message": "Missing query JSON argument"}))
            elif cmd == "chat":
                if len(sys.argv) > 2:
                    request_json = sys.argv[2]
                    result = bridge.chat_completion(request_json)
                    print(result)
                else:
                    print(json.dumps({"status": "error", "message": "Missing request JSON argument"}))
            elif cmd == "status":
                result = bridge.get_status()
                print(result)
            else:
                print(json.dumps({"status": "error", "message": f"Unknown command: {cmd}"}))
                
        except Exception as e:
            print(json.dumps({
                "status": "error",
                "message": str(e),
                "error_type": type(e).__name__,
                "traceback": traceback.format_exc()
            }))
    else:
        # Interactive CLI mode (for testing)
        bridge = PythonGoBridge()
        
        print("Python-Go Bridge for Mistral Client")
        print("=" * 50)
        print("Commands: initialize, query, chat, status, exit")
        
        while True:
            try:
                cmd = input("> ").strip().lower()
                
                if cmd == "exit":
                    break
                elif cmd == "status":
                    print(bridge.get_status())
                elif cmd == "initialize":
                    config_json = input("Config JSON: ")
                    print(bridge.initialize(config_json))
                elif cmd == "query":
                    query_json = input("Query JSON: ")
                    print(bridge.process_query(query_json))
                elif cmd == "chat":
                    request_json = input("Request JSON: ")
                    print(bridge.chat_completion(request_json))
                else:
                    print("Unknown command")
                    
            except KeyboardInterrupt:
                break
            except Exception as e:
                print(f"Error: {str(e)}")


if __name__ == "__main__":
    main()