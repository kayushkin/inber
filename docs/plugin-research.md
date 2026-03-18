# Plugin System Research for Inber

## Current State

Inber currently has a static tool system where tools are compiled into the binary at build time:

- **Tool Interface**: Clean `agent.Tool` with Name, Description, InputSchema, Run function
- **Registry**: `ToolRegistry` maps tool names to implementations
- **Built-ins**: File system, shell, browser, web search/fetch tools
- **Registration**: Happens in `init()` functions and at startup

**Current Pros**: Fast, simple, no dynamic loading complexity
**Current Cons**: Need to rebuild inber to add new tools, limited extensibility

## Dynamic Plugin Approaches

### 1. Go Plugins (plugin package)

**How it works**: 
- `.so` files on Linux/macOS, not supported on Windows
- Use `plugin.Open()` and `plugin.Lookup()` 

```go
// Example
p, err := plugin.Open("mytool.so")
symbol, err := p.Lookup("NewTool")
toolFunc := symbol.(func() agent.Tool)
tool := toolFunc()
```

**Pros**:
- Native Go solution
- Direct function calls (fast)
- Can share types between main app and plugin

**Cons**:
- Platform limitations (no Windows support)
- Complex build process (need to build plugins as `.so`)
- Plugin and main app must be compiled with same Go version
- Limited to single process
- Plugins can't be unloaded (memory leaks)

**Verdict**: Too many limitations for general use

### 2. MCP (Model Context Protocol) - Exec-based

**How it works**:
- Tools are separate executables that speak JSON over stdin/stdout
- inber spawns processes and communicates via standard MCP protocol
- Tools can be written in any language

```json
// Tool execution request
{"method": "call", "params": {"name": "shell", "arguments": {"command": "ls"}}}
// Tool response  
{"result": "file1.txt\nfile2.txt"}
```

**Pros**:
- Language agnostic (Python, JavaScript, Rust tools work)
- Process isolation (crashes don't kill inber)
- Can update tools without rebuilding inber
- Standard protocol (MCP is gaining adoption)
- Works across all platforms
- Can implement resource providers (file systems, APIs)

**Cons**:
- JSON serialization overhead
- Process spawn overhead  
- More complex error handling (process can die)
- Need MCP client implementation in inber

**Implementation effort**: Medium (need MCP client library)

### 3. Interface-based Runtime Registration

**How it works**:
- Tools implement a standard interface
- Tools register themselves via API calls or config
- Still requires recompilation but cleaner than current approach

```go
type ToolPlugin interface {
    Name() string
    Description() string 
    Schema() anthropic.ToolInputSchemaParam
    Execute(ctx context.Context, input string) (string, error)
}
```

**Pros**:
- Clean interface design
- Type safe
- Fast execution
- Easy testing

**Cons**:
- Still requires recompilation for new tools
- Not truly "dynamic"

**Verdict**: This is what we already have (could improve the interface)

### 4. WebAssembly (WASM)

**How it works**:
- Tools compiled to WASM modules
- Load and execute WASM at runtime
- Sandboxed execution environment

**Pros**:
- Platform independent 
- Sandboxed security
- Can support multiple languages (Rust, Go, TinyGo, AssemblyScript)

**Cons**:
- Performance overhead
- Limited system access from WASM
- Complex host function binding
- Still early ecosystem for Go WASM plugins
- Large implementation effort

**Verdict**: Promising but too complex for current needs

### 5. RPC/Network-based

**How it works**:
- Tools run as separate services
- Communicate via HTTP/gRPC/Unix sockets
- Similar to MCP but custom protocol

**Pros**:
- Language agnostic
- Can run tools on different machines
- Process isolation
- Hot-swappable

**Cons**:
- Network latency
- More complex deployment
- Need service discovery mechanism
- Custom protocol maintenance

## Recommendation: MCP-based Approach

**Why MCP**:
1. **Standard Protocol**: MCP is gaining adoption in the AI tooling space
2. **Language Agnostic**: Tools in Python, JS, Rust, etc.
3. **Process Isolation**: Tool crashes don't affect inber  
4. **Hot-swappable**: Update tools without rebuilding
5. **Cross-platform**: Works everywhere
6. **Future-proof**: As MCP grows, more tools become available

**Implementation Plan**:

1. **Add MCP Client**: Create `tools/mcp/` package with:
   - Process spawning and lifecycle management
   - JSON-RPC communication over stdin/stdout  
   - Tool discovery and registration
   - Error handling and process recovery

2. **MCP Tool Adapter**: Bridge between MCP tools and `agent.Tool` interface:
   ```go
   type MCPTool struct {
       name string
       process *MCPProcess
   }
   
   func (m *MCPTool) Run(ctx context.Context, input string) (string, error) {
       return m.process.Call(ctx, m.name, input)
   }
   ```

3. **Configuration**: Add tool discovery to config:
   ```yaml
   tools:
     builtin: ["shell", "read_file", "write_file"]  # compiled-in
     mcp:
       - name: "python-tools"
         command: ["python", "-m", "my_tool_server"]
         tools: ["data_analysis", "plot_chart"]
   ```

4. **Hybrid System**: Support both built-in and MCP tools:
   - Built-ins for core functionality (fast, reliable)
   - MCP for extensions and specialized tools

**Implementation Effort**: ~2-3 days

**Benefits**:
- Users can write tools in their preferred language
- Tools can be distributed independently  
- Safer execution (process isolation)
- Future-compatible with MCP ecosystem

## Next Steps

1. Research existing MCP Go client libraries
2. Create minimal MCP client implementation
3. Create example Python tool that implements MCP server
4. Add configuration and discovery mechanism
5. Update documentation and examples

**Files to create**:
- `tools/mcp/client.go` - MCP JSON-RPC client
- `tools/mcp/process.go` - Process lifecycle management  
- `tools/mcp/adapter.go` - MCP to agent.Tool bridge
- `examples/mcp-tool/` - Example Python MCP tool
- `docs/plugins.md` - Plugin development guide

This approach provides the best balance of flexibility, safety, and implementation complexity.