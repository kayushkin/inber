# User Configuration Guide

## Problem

When you install `inber` globally (e.g., via alias in WSL), running it outside of the project directory fails with "ANTHROPIC_API_KEY not set" because it can't find the project's `.env` file.

## Solution

`inber` now supports multiple configuration locations, checked in this priority order:

1. **Project `.env`** (current directory) — project-specific overrides
2. **User config** (`~/.config/inber/.env`) — your personal API key
3. **System environment** — system-wide `ANTHROPIC_API_KEY`

## Setup Options

### Option 1: User Config (Recommended for Aliases)

Create a user-level config that works from any directory:

```bash
inber config user
```

This will:
- Create `~/.config/inber/.env`
- Prompt you to enter your API key
- Save it with secure permissions (user read/write only)

You can also create it manually:

```bash
mkdir -p ~/.config/inber
echo "ANTHROPIC_API_KEY=sk-ant-..." > ~/.config/inber/.env
chmod 600 ~/.config/inber/.env
```

### Option 2: System Environment Variable

Add to your `~/.bashrc` or `~/.zshrc`:

```bash
export ANTHROPIC_API_KEY="sk-ant-api03-your-key-here"
```

Then reload:
```bash
source ~/.bashrc  # or source ~/.zshrc
```

### Option 3: Project-Specific Config

For project-specific keys (e.g., different API keys per project):

```bash
cd your-project
inber config init
# Edit .env and add your key
```

## Checking Your Config

View current configuration:

```bash
inber config show
```

This shows:
- Whether API key is set (with masked display)
- User config file location
- Project config location (if in a git repo)
- Default model and other settings

## Priority Example

If you have:
- System: `ANTHROPIC_API_KEY=key1`
- User config: `ANTHROPIC_API_KEY=key2`  
- Project .env: `ANTHROPIC_API_KEY=key3`

The **project .env wins** (key3), allowing you to override on a per-project basis.

## Security Notes

- User config files are created with `0600` permissions (user read/write only)
- The `.env` file in project directories should be added to `.gitignore`
- Never commit API keys to version control
- Use `inber config show` to verify which key is active (shows masked version)

## WSL / Linux Alias Setup

Once you have user config set up:

```bash
# Add to ~/.bashrc or ~/.zshrc
alias inber='/path/to/inber'

# Or if you built and installed it:
go install ./cmd/inber
# Then use it from anywhere (assuming ~/go/bin is in PATH)
```

## Troubleshooting

**"ANTHROPIC_API_KEY not set"**
- Run `inber config show` to check what's loaded
- Verify file exists: `cat ~/.config/inber/.env`
- Check permissions: `ls -la ~/.config/inber/.env` (should be `-rw-------`)

**Key not being picked up**
- Make sure there are no quotes around the key in the .env file
- Format should be: `ANTHROPIC_API_KEY=sk-ant-...` (no spaces, no quotes)
- Try setting it in environment and running: `ANTHROPIC_API_KEY=key inber "test"`

**Multiple configs conflicting**
- Remember: project `.env` > user config > system env
- Use `inber config show` to see which locations exist
- Temporarily rename files to test: `mv ~/.config/inber/.env ~/.config/inber/.env.bak`
