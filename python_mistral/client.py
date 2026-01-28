"""
Python Mistral Client - Core Implementation
VMware Avi LLM Agent - Python Implementation
"""

import os
import sys
import json
import uuid
import logging
from typing import Optional, List, Dict, Any, Callable
from datetime import datetime

from tenacity import retry, stop_after_attempt, wait_exponential, retry_if_exception_type
try:
    # Try new API (mistralai >= 1.0.0)
    from mistralai.sdk import Mistral as MistralClient
    from mistralai import ChatCompletionResponse
except ImportError:
    try:
        # Try intermediate API
        from mistralai.client import MistralClient
        from mistralai.models.chat_completion import ChatCompletionResponse
    except ImportError:
        # Fallback to old API (mistralai < 0.4.2)
        from mistralai.client import MistralClient
        from mistralai import ChatCompletionResponse

from .models import (
    LLMResponse, 
    ToolCall, 
    ToolCallFunction, 
    Usage,
    MistralConfig
)

# Configure logging
import logging

# Configure logging to use stderr and simple format
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    stream=sys.stderr,
    force=True
)

# Create a logger
logger = logging.getLogger("mistral_client")


class PythonMistralClient:
    """
    Python implementation of Mistral AI client with fallback support
    
    This client uses the official Mistral AI Python SDK and provides
    comprehensive error handling and fallback mechanisms.
    """
    
    def __init__(self, config: MistralConfig, avi_client_provider: Optional[Callable] = None):
        """
        Initialize Python Mistral client
        
        Args:
            config: Mistral configuration
            avi_client_provider: Optional function to provide Avi client for fallback
        """
        self.config = config
        self.avi_client_provider = avi_client_provider
        
        # Initialize official Mistral client
        self.client = MistralClient(api_key=config.api_key)
        
        # Configure logging
        self.logger = logger
        self.logger.info("Python Mistral client initialized, model=%s, timeout=%s, debug=%s",
                        config.default_model, config.timeout, config.debug)
    
    @retry(
        stop=stop_after_attempt(3),
        wait=wait_exponential(multiplier=1, min=4, max=10),
        retry=retry_if_exception_type(Exception)
    )
    def chat_completion(
        self, 
        messages: List[Dict[str, str]], 
        tools: Optional[List[Dict[str, Any]]] = None,
        model: Optional[str] = None, 
        temperature: Optional[float] = None,
        max_tokens: Optional[int] = None
    ) -> LLMResponse:
        """
        Send chat completion request to Mistral AI with retry logic
        
        Args:
            messages: List of chat messages
            tools: Optional list of tools for function calling
            model: Optional model override
            temperature: Optional temperature override
            max_tokens: Optional max tokens override
            
        Returns:
            LLMResponse: Processed LLM response
            
        Raises:
            Exception: If all retry attempts fail
        """
        model = model or self.config.default_model
        temperature = temperature or self.config.temperature
        max_tokens = max_tokens or self.config.max_tokens

        self.logger.info("Sending Mistral API request, model=%s, message_count=%d, tool_count=%d, temperature=%s, max_tokens=%s",
                        model, len(messages), len(tools) if tools else 0, temperature, max_tokens)

        try:
            # Convert messages to Mistral format
            mistral_messages = [
                {"role": msg["role"], "content": msg["content"]}
                for msg in messages
            ]

            # Send request to Mistral
            response = self.client.chat(
                model=model,
                messages=mistral_messages,
                tools=tools,
                temperature=temperature,
                max_tokens=max_tokens
            )

            self.logger.info("Mistral API response received, model=%s, prompt_tokens=%d, completion_tokens=%d, total_tokens=%d",
                           response.model, response.usage.prompt_tokens, response.usage.completion_tokens, response.usage.total_tokens)

            return self._process_mistral_response(response)

        except Exception as e:
            self.logger.error("Mistral API request failed, error=%s, error_type=%s, model=%s",
                            str(e), type(e).__name__, model)
            raise

    def _process_mistral_response(self, response: ChatCompletionResponse) -> LLMResponse:
        """
        Process Mistral response into standard format
        
        Args:
            response: Raw Mistral API response
            
        Returns:
            LLMResponse: Processed response in standard format
        """
        tool_calls = []

        if response.choices and len(response.choices) > 0:
            choice = response.choices[0]
            
            if choice.message.tool_calls:
                for tool_call in choice.message.tool_calls:
                    tool_calls.append(ToolCall(
                        id=tool_call.id,
                        function=ToolCallFunction(
                            name=tool_call.function.name,
                            description="",
                            parameters=tool_call.function.arguments
                        ),
                        args=tool_call.function.arguments
                    ))

        return LLMResponse(
            message=response.choices[0].message.content if response.choices[0].message.content else "",
            tool_calls=tool_calls if tool_calls else None,
            model=response.model,
            usage=Usage(
                prompt_tokens=response.usage.prompt_tokens,
                completion_tokens=response.usage.completion_tokens,
                total_tokens=response.usage.total_tokens
            )
        )

    def process_natural_language_query(
        self, 
        query: str, 
        conversation_history: List[Dict[str, str]],
        tools: List[Dict[str, Any]]
    ) -> LLMResponse:
        """
        Process natural language query with fallback support
        
        This method handles the complete query processing pipeline including:
        - Message construction
        - Tool filtering for reliability
        - Mistral API calls with retry
        - Fallback to direct Avi API calls when Mistral fails
        
        Args:
            query: User query
            conversation_history: Previous conversation messages
            tools: Available tools for function calling
            
        Returns:
            LLMResponse: Processed response
            
        Raises:
            Exception: If both Mistral and fallback fail
        """
        # Build messages
        messages = [
            {"role": "system", "content": self._build_system_prompt()}
        ]

        # Add conversation history
        messages.extend(conversation_history)

        # Add user query
        messages.append({"role": "user", "content": query})

        self.logger.info("Processing natural language query, query=%s, history_count=%d, tool_count=%d",
                        query, len(conversation_history), len(tools))

        try:
            # Try with minimal tools first if we have many tools
            # This reduces payload size and improves reliability
            if len(tools) > 2:
                minimal_tools = self._filter_tools_for_query(query, tools)
                if minimal_tools and len(minimal_tools) < len(tools):
                    self.logger.info("Attempting minimal tool request for reliability, original_count=%d, minimal_count=%d",
                                    len(tools), len(minimal_tools))
                    
                    try:
                        return self.chat_completion(messages, minimal_tools)
                    except Exception as e:
                        self.logger.warning("Minimal tool request failed, falling back to full tool set, error=%s, error_type=%s",
                                       str(e), type(e).__name__)

            # Try with full tool set
            return self.chat_completion(messages, tools)

        except Exception as e:
            # Attempt fallback if available
            if self.avi_client_provider and self._can_handle_fallback(query):
                self.logger.warning("Mistral API failed, attempting fallback to direct Avi API call, error=%s, error_type=%s, query=%s",
                               str(e), type(e).__name__, query)
                
                try:
                    return self._handle_fallback_query(query)
                except Exception as fallback_error:
                    self.logger.error("Fallback Avi API call failed, original_error=%s, fallback_error=%s",
                                    str(e), str(fallback_error))
                    raise
            else:
                # No fallback available, re-raise original error
                raise

    def _build_system_prompt(self) -> str:
        """
        Build concise system prompt to reduce payload size
        
        Returns:
            str: System prompt for Mistral AI
        """
        return """You are an AI assistant for VMware Avi Load Balancer management with access to API tools.
        Use tools when users ask for specific information about virtual services, pools, or other resources.
        Be concise and provide clear, actionable responses.
        Always use tools for list/show/get operations on Avi resources."""

    def _filter_tools_for_query(self, query: str, tools: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        """
        Filter tools to reduce payload size and improve reliability
        
        Args:
            query: User query
            tools: Full list of tools
            
        Returns:
            List[Dict[str, Any]]: Filtered list of relevant tools
        """
        lower_query = query.lower()

        # For virtual service queries, return only VS-related tools
        if "virtual service" in lower_query:
            vs_tools = [
                "list_virtual_services", 
                "get_virtual_service", 
                "create_virtual_service",
                "update_virtual_service", 
                "delete_virtual_service"
            ]
            return [tool for tool in tools if tool.get("function", {}).get("name") in vs_tools]

        # For pool queries, return only pool-related tools
        if "pool" in lower_query:
            pool_tools = [
                "list_pools", 
                "get_pool", 
                "create_pool",
                "update_pool", 
                "delete_pool"
            ]
            return [tool for tool in tools if tool.get("function", {}).get("name") in pool_tools]

        # For service engine queries, return only SE-related tools
        if "service engine" in lower_query:
            se_tools = [
                "list_service_engines", 
                "get_service_engine"
            ]
            return [tool for tool in tools if tool.get("function", {}).get("name") in se_tools]

        # For health/status queries, return health-related tools
        if any(keyword in lower_query for keyword in ["health", "status", "monitor"]):
            health_tools = [
                "get_health_status",
                "list_health_monitors",
                "get_health_monitor"
            ]
            return [tool for tool in tools if tool.get("function", {}).get("name") in health_tools]

        # Return all tools if no specific pattern matches
        return tools

    def _can_handle_fallback(self, query: str) -> bool:
        """
        Determine if query can be handled by fallback to direct Avi API calls
        
        Args:
            query: User query
            
        Returns:
            bool: True if fallback is available for this query type
        """
        lower_query = query.lower()
        
        # Queries we can handle with direct Avi API calls
        fallback_patterns = [
            ("list", "virtual service"),
            ("show", "virtual service"),
            ("get", "virtual service"),
            ("list", "pool"),
            ("show", "pool"),
            ("list", "service engine"),
            ("show", "service engine"),
            ("health", "status"),
            ("current", "status")
        ]
        
        for verb, resource in fallback_patterns:
            if verb in lower_query and resource in lower_query:
                return True
        
        return False

    def _handle_fallback_query(self, query: str) -> LLMResponse:
        """
        Handle fallback to direct Avi API calls
        
        Args:
            query: User query
            
        Returns:
            LLMResponse: Response from Avi API
            
        Raises:
            Exception: If fallback fails
        """
        lower_query = query.lower()

        if "list" in lower_query and "virtual service" in lower_query:
            return self._handle_list_virtual_services_fallback()
        elif ("show" in lower_query or "get" in lower_query) and "virtual service" in lower_query:
            return self._handle_get_virtual_service_fallback()
        elif "list" in lower_query and "pool" in lower_query:
            return self._handle_list_pools_fallback()
        elif ("show" in lower_query or "get" in lower_query) and "pool" in lower_query:
            return self._handle_get_pool_fallback()
        elif "list" in lower_query and "service engine" in lower_query:
            return self._handle_list_service_engines_fallback()
        elif ("show" in lower_query or "get" in lower_query) and "service engine" in lower_query:
            return self._handle_get_service_engine_fallback()
        elif any(keyword in lower_query for keyword in ["health", "status", "monitor"]):
            return self._handle_health_status_fallback()
        else:
            raise ValueError(f"No fallback handler available for query: {query}")

    def _handle_list_virtual_services_fallback(self) -> LLMResponse:
        """
        Handle list virtual services fallback
        
        Returns:
            LLMResponse: Response with virtual services data
            
        Raises:
            Exception: If Avi client call fails
        """
        try:
            avi_client = self.avi_client_provider()
            if not avi_client:
                raise ValueError("Avi client not available")

            # Call Avi API directly
            result = avi_client.list_virtual_services(params={"limit_by": "10"})

            # Create tool call response
            tool_call = ToolCall(
                id=f"fallback-{uuid.uuid4()}",
                function=ToolCallFunction(
                    name="list_virtual_services",
                    description="List virtual services from Avi controller",
                    parameters=result
                ),
                args=result
            )

            return LLMResponse(
                message="Retrieved virtual services from Avi controller (fallback mode)",
                tool_calls=[tool_call],
                model="avi-fallback",
                usage=Usage(prompt_tokens=0, completion_tokens=0, total_tokens=0)
            )

        except Exception as e:
            self.logger.error("Fallback list virtual services failed, error=%s, error_type=%s", 
                            str(e), type(e).__name__)
            raise

    def _handle_get_virtual_service_fallback(self) -> LLMResponse:
        """
        Handle get virtual service fallback
        
        Note: This would need UUID extraction logic for specific resource queries
        
        Raises:
            NotImplementedError: UUID extraction not yet implemented
        """
        raise NotImplementedError("UUID extraction for specific resource queries not yet implemented")

    def _handle_list_pools_fallback(self) -> LLMResponse:
        """
        Handle list pools fallback
        
        Returns:
            LLMResponse: Response with pools data
            
        Raises:
            Exception: If Avi client call fails
        """
        try:
            avi_client = self.avi_client_provider()
            if not avi_client:
                raise ValueError("Avi client not available")

            result = avi_client.list_pools(params={"limit_by": "10"})

            tool_call = ToolCall(
                id=f"fallback-{uuid.uuid4()}",
                function=ToolCallFunction(
                    name="list_pools",
                    description="List pools from Avi controller",
                    parameters=result
                ),
                args=result
            )

            return LLMResponse(
                message="Retrieved pools from Avi controller (fallback mode)",
                tool_calls=[tool_call],
                model="avi-fallback",
                usage=Usage(prompt_tokens=0, completion_tokens=0, total_tokens=0)
            )

        except Exception as e:
            self.logger.error("Fallback list pools failed, error=%s, error_type=%s", 
                            str(e), type(e).__name__)
            raise

    def _handle_list_service_engines_fallback(self) -> LLMResponse:
        """
        Handle list service engines fallback
        
        Returns:
            LLMResponse: Response with service engines data
            
        Raises:
            Exception: If Avi client call fails
        """
        try:
            avi_client = self.avi_client_provider()
            if not avi_client:
                raise ValueError("Avi client not available")

            result = avi_client.list_service_engines(params={"limit_by": "10"})

            tool_call = ToolCall(
                id=f"fallback-{uuid.uuid4()}",
                function=ToolCallFunction(
                    name="list_service_engines",
                    description="List service engines from Avi controller",
                    parameters=result
                ),
                args=result
            )

            return LLMResponse(
                message="Retrieved service engines from Avi controller (fallback mode)",
                tool_calls=[tool_call],
                model="avi-fallback",
                usage=Usage(prompt_tokens=0, completion_tokens=0, total_tokens=0)
            )

        except Exception as e:
            self.logger.error("Fallback list service engines failed, error=%s, error_type=%s", 
                            str(e), type(e).__name__)
            raise

    def _handle_health_status_fallback(self) -> LLMResponse:
        """
        Handle health status fallback
        
        Returns:
            LLMResponse: Response with health status data
            
        Raises:
            Exception: If Avi client call fails
        """
        try:
            avi_client = self.avi_client_provider()
            if not avi_client:
                raise ValueError("Avi client not available")

            # Get overall system health
            result = avi_client.get_system_health()

            tool_call = ToolCall(
                id=f"fallback-{uuid.uuid4()}",
                function=ToolCallFunction(
                    name="get_health_status",
                    description="Get system health status from Avi controller",
                    parameters=result
                ),
                args=result
            )

            return LLMResponse(
                message="Retrieved system health status from Avi controller (fallback mode)",
                tool_calls=[tool_call],
                model="avi-fallback",
                usage=Usage(prompt_tokens=0, completion_tokens=0, total_tokens=0)
            )

        except Exception as e:
            self.logger.error("Fallback health status failed, error=%s, error_type=%s", 
                            str(e), type(e).__name__)
            raise