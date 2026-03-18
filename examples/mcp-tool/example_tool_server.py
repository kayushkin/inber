#!/usr/bin/env python3
"""
Example MCP (Model Context Protocol) tool server for inber.

This demonstrates how to create external tools that can be dynamically loaded by inber.
The server implements the MCP protocol via JSON-RPC over stdin/stdout.
"""

import json
import sys
import os
import subprocess
from typing import Dict, Any, List


class MCPToolServer:
    def __init__(self):
        self.tools = {
            "python_eval": {
                "name": "python_eval",
                "description": "Execute Python code safely and return the result",
                "inputSchema": {
                    "type": "object",
                    "properties": {
                        "code": {
                            "type": "string",
                            "description": "Python code to execute"
                        }
                    },
                    "required": ["code"]
                }
            },
            "word_count": {
                "name": "word_count",  
                "description": "Count words, lines, and characters in text",
                "inputSchema": {
                    "type": "object",
                    "properties": {
                        "text": {
                            "type": "string",
                            "description": "Text to analyze"
                        }
                    },
                    "required": ["text"]
                }
            },
            "base64_encode": {
                "name": "base64_encode",
                "description": "Encode text to base64",
                "inputSchema": {
                    "type": "object", 
                    "properties": {
                        "text": {
                            "type": "string",
                            "description": "Text to encode"
                        }
                    },
                    "required": ["text"]
                }
            }
        }

    def handle_initialize(self, params: Dict[str, Any]) -> Dict[str, Any]:
        """Handle MCP initialization request"""
        return {
            "protocolVersion": "2024-11-05",
            "capabilities": {
                "tools": {},
                "logging": {}
            },
            "serverInfo": {
                "name": "example-tool-server",
                "version": "1.0.0"
            }
        }

    def handle_tools_list(self, params: Dict[str, Any]) -> Dict[str, Any]:
        """Return list of available tools"""
        return {
            "tools": list(self.tools.values())
        }

    def handle_tool_call(self, params: Dict[str, Any]) -> Dict[str, Any]:
        """Execute a tool and return results"""
        tool_name = params.get("name")
        arguments = params.get("arguments", {})

        if tool_name not in self.tools:
            raise Exception(f"Unknown tool: {tool_name}")

        try:
            if tool_name == "python_eval":
                result = self._python_eval(arguments["code"])
            elif tool_name == "word_count":
                result = self._word_count(arguments["text"]) 
            elif tool_name == "base64_encode":
                result = self._base64_encode(arguments["text"])
            else:
                raise Exception(f"Tool {tool_name} not implemented")

            return {
                "content": [
                    {
                        "type": "text",
                        "text": result
                    }
                ]
            }
        except Exception as e:
            return {
                "content": [
                    {
                        "type": "text", 
                        "text": f"Error: {str(e)}"
                    }
                ],
                "isError": True
            }

    def _python_eval(self, code: str) -> str:
        """Execute Python code safely"""
        try:
            # Basic safety: prevent dangerous imports and operations
            dangerous_keywords = ["import os", "import sys", "import subprocess", "__import__", "eval", "exec"]
            code_lower = code.lower()
            for keyword in dangerous_keywords:
                if keyword in code_lower:
                    return f"Error: Dangerous operation '{keyword}' not allowed"

            # Execute in restricted environment
            globals_dict = {
                "__builtins__": {
                    "len": len,
                    "str": str,
                    "int": int,
                    "float": float,
                    "list": list,
                    "dict": dict,
                    "range": range,
                    "sum": sum,
                    "max": max,
                    "min": min,
                    "abs": abs,
                    "round": round,
                    "print": print,
                }
            }
            locals_dict = {}
            
            result = eval(code, globals_dict, locals_dict)
            return str(result)
        except Exception as e:
            return f"Error: {str(e)}"

    def _word_count(self, text: str) -> str:
        """Count words, lines, and characters"""
        lines = text.split('\n')
        words = text.split()
        chars = len(text)
        chars_no_spaces = len(text.replace(' ', ''))
        
        return f"Lines: {len(lines)}\nWords: {len(words)}\nCharacters: {chars}\nCharacters (no spaces): {chars_no_spaces}"

    def _base64_encode(self, text: str) -> str:
        """Encode text to base64"""
        import base64
        encoded = base64.b64encode(text.encode('utf-8')).decode('utf-8')
        return encoded

    def handle_request(self, request: Dict[str, Any]) -> Dict[str, Any]:
        """Handle incoming JSON-RPC request"""
        method = request.get("method")
        params = request.get("params", {})
        request_id = request.get("id")

        try:
            if method == "initialize":
                result = self.handle_initialize(params)
            elif method == "tools/list":
                result = self.handle_tools_list(params)
            elif method == "tools/call":
                result = self.handle_tool_call(params)
            elif method == "notifications/initialized":
                # Notification, no response needed
                return None
            else:
                raise Exception(f"Unknown method: {method}")

            # Return successful response
            if request_id is not None:
                return {
                    "jsonrpc": "2.0",
                    "id": request_id,
                    "result": result
                }
            return None

        except Exception as e:
            # Return error response
            if request_id is not None:
                return {
                    "jsonrpc": "2.0", 
                    "id": request_id,
                    "error": {
                        "code": -32603,
                        "message": str(e)
                    }
                }
            return None

    def run(self):
        """Main server loop - read JSON-RPC from stdin, write to stdout"""
        for line in sys.stdin:
            line = line.strip()
            if not line:
                continue

            try:
                request = json.loads(line)
                response = self.handle_request(request)
                
                if response is not None:
                    print(json.dumps(response))
                    sys.stdout.flush()
            except json.JSONDecodeError:
                # Invalid JSON, ignore
                continue
            except Exception as e:
                # Send generic error
                error_response = {
                    "jsonrpc": "2.0",
                    "error": {
                        "code": -32603,
                        "message": f"Internal error: {str(e)}"
                    }
                }
                print(json.dumps(error_response))
                sys.stdout.flush()


if __name__ == "__main__":
    server = MCPToolServer()
    server.run()