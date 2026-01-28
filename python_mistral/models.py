"""
Python Mistral Client - Data Models
VMware Avi LLM Agent - Python Implementation
"""

from typing import Optional, List, Dict, Any
from pydantic import BaseModel, Field


class ToolCallFunction(BaseModel):
    """Represents a function that can be called by the LLM"""
    name: str
    description: str = ""
    parameters: Dict[str, Any] = Field(default_factory=dict)


class ToolCall(BaseModel):
    """Represents a tool call made by the LLM"""
    id: str
    type: str = "function"
    function: ToolCallFunction
    args: Optional[Dict[str, Any]] = None


class Usage(BaseModel):
    """Represents token usage statistics"""
    prompt_tokens: int = 0
    completion_tokens: int = 0
    total_tokens: int = 0


class LLMResponse(BaseModel):
    """Represents a processed LLM response"""
    message: str
    tool_calls: Optional[List[ToolCall]] = None
    model: str
    usage: Usage = Field(default_factory=Usage)


class MistralConfig(BaseModel):
    """Mistral AI configuration"""
    api_base_url: str = "https://api.mistral.ai"
    api_key: str
    default_model: str = "mistral-medium"
    models: List[str] = ["mistral-tiny", "mistral-small", "mistral-medium"]
    timeout: int = 60
    temperature: float = 0.7
    max_tokens: int = 2048
    debug: bool = False


class AviConfig(BaseModel):
    """VMware Avi Load Balancer configuration"""
    host: str
    username: str
    password: str
    version: str = "31.2.1"
    tenant: str = "admin"
    timeout: int = 30
    insecure: bool = False
    auth_method: str = "session"


class AppConfig(BaseModel):
    """Application configuration"""
    server_port: int = 8080
    provider: str = "mistral"
    mistral: MistralConfig
    avi: AviConfig
    log_level: str = "info"