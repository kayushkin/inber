# Changelog

## [Unreleased]

### Added - Multi-Agent Support (Phase 1)

- **Agent Registry** (`agent/registry/`)
  - YAML-based agent configuration system
  - Registry for managing multiple agents with isolated sessions and contexts
  - Per-agent tool scoping via configuration
  - Lazy agent initialization
  - Thread-safe registry operations

- **Agent Configurations** (`agents/`)
  - `orchestrator.yaml` - Task coordination and delegation agent
  - `coder.yaml` - Software engineering specialist with full shell access
  - `researcher.yaml` - Research and analysis specialist (read-only)

- **Enhanced Session Support**
  - Agent name tracking in sessions
  - Parent-child session relationships for sub-agents
  - Agent-specific log directories (`logs/agent-name/`)
  - Session IDs for tracking agent hierarchies

- **Documentation**
  - `docs/multi-agent-design.md` - Complete multi-agent architecture design
  - `docs/multi-agent-usage.md` - Usage guide and examples
  - `agent/registry/README.md` - Registry package documentation
  - `examples/multi-agent/main.go` - Working multi-agent example

### Changed

- **Session Constructor** - `session.New()` now accepts `agentName` and `parentID` parameters
- **Dependencies** - Added `gopkg.in/yaml.v3` for YAML config parsing

### Technical Details

**Phase 1 Complete:**
- ✅ Core registry implementation
- ✅ YAML config loading
- ✅ Agent registry with lazy initialization
- ✅ Per-agent context store mapping
- ✅ Per-agent session management
- ✅ Comprehensive tests for registry
- ✅ Example code and documentation

**Next Phases:**
- Phase 2: Tool scoping implementation
- Phase 3: Sub-agent spawning (`spawn_agent` tool)
- Phase 4: Context integration with message building
- Phase 5: CLI integration with `--agent` flag

## [0.1.0] - 2026-02-24

Initial release of inber framework.

### Features
- Core agent loop with Claude API integration
- Tool system (shell, read_file, write_file, edit_file, list_files)
- Tag-based context system
- Session logging (JSONL)
- REPL CLI interface
- Extended thinking support
