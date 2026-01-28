#!/usr/bin/env python3
"""
Python Mistral Client - Main Module Entry Point
VMware Avi LLM Agent - Python Implementation
"""

import sys
import json
from .bridge import main

if __name__ == "__main__":
    # Pass command line arguments to the bridge main function
    main()