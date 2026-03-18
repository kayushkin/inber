# MCP Tool Example

This directory contains an example MCP (Model Context Protocol) tool server that demonstrates how to create dynamic tools for inber.

## What's Here

- `example_tool_server.py` - A Python MCP server that provides 3 example tools:
  - `python_eval` - Execute Python code safely
  - `word_count` - Count words, lines, and characters in text
  - `base64_encode` - Encode text to base64

## How It Works

1. **MCP Protocol**: Tools communicate with inber via JSON-RPC over stdin/stdout
2. **Process Isolation**: Each tool server runs as a separate process
3. **Language Agnostic**: Tool servers can be written in any language
4. **Dynamic Loading**: Tools can be added without recompiling inber

## Testing the Example

### Manual Test

You can test the tool server manually:

```bash
cd examples/mcp-tool
echo '{"jsonrpc": "2.0", "id": "1", "method": "initialize", "params": {"protocolVersion": "2024-11-05"}}' | python3 example_tool_server.py
```

Expected response:
```json
{"jsonrpc": "2.0", "id": "1", "result": {"protocolVersion": "2024-11-05", "capabilities": {"tools": {}, "logging": {}}, "serverInfo": {"name": "example-tool-server", "version": "1.0.0"}}}
```

### List Tools

```bash
echo '{"jsonrpc": "2.0", "id": "2", "method": "tools/list", "params": {}}' | python3 example_tool_server.py
```

### Call a Tool

```bash
echo '{"jsonrpc": "2.0", "id": "3", "method": "tools/call", "params": {"name": "word_count", "arguments": {"text": "Hello world! This is a test."}}}' | python3 example_tool_server.py
```

## Integration with Inber

To use this tool server with inber, you would add it to your configuration:

```yaml
tools:
  builtin: ["shell", "read_file", "write_file"]  # compiled-in tools
  mcp:
    - name: "example-tools"
      command: ["python3", "examples/mcp-tool/example_tool_server.py"]
      tools: ["python_eval", "word_count", "base64_encode"]
```

Then inber would:
1. Spawn the Python process
2. Perform MCP handshake
3. Discover available tools
4. Create agent.Tool adapters
5. Register tools in the tool registry

## Creating Your Own Tools

To create your own MCP tool server:

1. Implement the MCP JSON-RPC protocol (initialize, tools/list, tools/call)
2. Define your tools with name, description, and JSON schema
3. Implement tool execution logic
4. Handle errors gracefully
5. Read from stdin and write JSON responses to stdout

### Tool Schema Example

```json
{
  "name": "my_tool",
  "description": "What this tool does",
  "inputSchema": {
    "type": "object",
    "properties": {
      "param1": {
        "type": "string",
        "description": "First parameter"
      },
      "param2": {
        "type": "number", 
        "description": "Second parameter"
      }
    },
    "required": ["param1"]
  }
}
```

## Benefits of MCP Tools

- **Language Freedom**: Write tools in Python, JavaScript, Rust, Go, etc.
- **Process Safety**: Tool crashes don't affect inber
- **Hot Swapping**: Update tools without rebuilding inber
- **Distribution**: Share tools independently
- **Security**: Process isolation and sandboxing
- **Testing**: Easy to test tools independently

## Next Steps

- Implement MCP client in inber (`tools/mcp/`)
- Add configuration support for MCP tools
- Create more example tools (data analysis, API clients, etc.)
- Add tool discovery and marketplace concepts