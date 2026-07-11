# Ollama CLI

> A fast, local AI coding assistant powered by [Ollama](https://ollama.com). No cloud. No API keys. Just your machine.

Ollama CLI is a terminal-native chat interface for local large language models. It streams responses token-by-token, renders Markdown in the terminal, auto-saves generated code into project folders, runs shell commands with approval, and feeds errors back to the model so it can fix mistakes — all without leaving your terminal.

Built with Go and the [Charm](https://charm.sh) stack (Bubble Tea, Lipgloss, Glamour).

---

## Table of Contents

- [Features](#features)
- [Screenshots](#screenshots)
- [Requirements](#requirements)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Usage](#usage)
- [Slash Commands](#slash-commands)
- [Auto-Save & Project Workflow](#auto-save--project-workflow)
- [Configuration](#configuration)
- [Recommended Models](#recommended-models)
- [Keyboard Shortcuts](#keyboard-shortcuts)
- [Architecture](#architecture)
- [Building from Source](#building-from-source)
- [Cross-Platform Builds](#cross-platform-builds)
- [Troubleshooting](#troubleshooting)
- [Contributing](#contributing)
- [License](#license)

---

## Features

### Core chat experience

- **Streaming responses** — tokens appear immediately as Ollama generates them
- **Beautiful TUI** — full-screen terminal UI with spinner, viewport, and styled input
- **Live Markdown rendering** — code blocks, bold text, tables, and headings rendered with Glamour
- **Multiple themes** — dark, dracula, nord, monokai, ocean, forest, and sunset palettes
- **Model switching** — change models mid-session with `/model` or pick by number from `/models`
- **Conversation memory** — multi-turn chat with full context sent to Ollama
- **Input history** — navigate previous prompts with ↑ / ↓ (like a shell)
- **Retry & copy** — regenerate the last response or copy it to the clipboard

### Developer tools

- **Auto-save code blocks** — every fenced code block in an AI response is saved to `~/ollama-cli-projects/`
- **Multi-file projects** — annotate paths with `// file: src/main.py` (or `# file:` / `<!-- file: -->`)
- **Shell command execution** — run commands with `/run` and see captured stdout/stderr
- **Error-fix loop** — when a command fails, the full output is sent back to the model automatically
- **Smart run inference** — detects project type (npm, Go, Python, HTML, etc.) and suggests launch commands
- **Interactive terminal** — open a PowerShell window in your project dir with `/terminal`
- **File loading** — pull local files into context with `/file`
- **Token estimator** — approximate context usage with `/tokens`
- **Save conversations** — export full chats as Markdown with `/save`

### Platform support

- **Windows, macOS, and Linux** — cross-platform Go binary
- **Windows-aware execution** — uses `cmd.exe /C` for reliable `&&` chaining; `CREATE_NO_WINDOW` keeps subprocesses from breaking the TUI
- **Remote Ollama** — point at another machine with `--host`

---

## Screenshots

```
╔═══════════════════════════════════╗
║       🦙  Ollama CLI  🦙          ║
║  Local AI assistant — no cloud   ║
╚═══════════════════════════════════╝

✓  Connected to Ollama  ▸ qwen2.5-coder:7b
Type a message and press Enter. /help for commands.

You [14:32] Build a Python CLI that prints hello world

🦙 Ollama
Here's a simple Python CLI:

```python
# file: main.py
def main():
    print("Hello, world!")

if __name__ == "__main__":
    main()
```

💾 1 file(s) saved → C:\Users\you\ollama-cli-projects\2026-07-11\143245\
   📄 main.py

● Ready   ↑/↓ history  •  Ctrl+L=bottom  •  /help for commands
```

---

## Requirements

| Dependency | Notes |
|---|---|
| [Ollama](https://ollama.com) | Must be running (`ollama serve`) |
| A pulled model | e.g. `ollama pull qwen2.5-coder:7b` |
| Go 1.22+ | Only needed to build from source |
| Terminal with true-color support | Recommended for best theme rendering |

Optional (for running generated code):

- Python, Node.js, Go, Rust, etc. depending on what the model generates

---

## Installation

### From source

```bash
git clone https://github.com/gurtej9602/ollama-cli.git
cd ollama-cli
go build -ldflags="-s -w" -o ollama-cli .
```

### Go install

```bash
go install github.com/gurtej9602/ollama-cli@latest
```

### Pre-built binaries

Check the [Releases](https://github.com/gurtej9602/ollama-cli/releases) page for platform-specific binaries (when available).

---

## Quick Start

1. **Start Ollama** (if not already running):

   ```bash
   ollama serve
   ```

2. **Pull a model**:

   ```bash
   ollama pull qwen2.5-coder:7b
   ```

3. **Launch the CLI**:

   ```bash
   ollama-cli
   ```

4. **Ask a question** — type your prompt and press Enter.

5. **Explore commands** — type `/help` to see everything available.

---

## Usage

```bash
# Start chat (default subcommand)
ollama-cli

# Explicit chat subcommand
ollama-cli chat

# Use a specific model
ollama-cli --model qwen2.5-coder:7b
ollama-cli -m llama3.2

# Connect to a remote Ollama instance
ollama-cli --host http://192.168.1.100:11434

# Use a custom config file
ollama-cli --config /path/to/my-config.yaml
```

### Flags

| Flag | Short | Default | Description |
|---|---|---|---|
| `--model` | `-m` | `llama3.2` | Ollama model to use |
| `--host` | `-H` | `http://localhost:11434` | Ollama server URL |
| `--config` | | `~/.ollamacli.yaml` | Path to config file |

---

## Slash Commands

| Command | Description |
|---|---|
| `/help` | Show all available commands |
| `/models` | List locally available Ollama models |
| `/model <name\|number>` | Switch to a model by name or list index |
| `/clear` | Clear conversation history |
| `/file <path>` | Load a file into the chat context |
| `/system <text>` | Override the system prompt |
| `/append <text>` | Append text to every user message |
| `/run <cmd>` | Execute a shell command (errors auto-sent to LLM) |
| `/terminal [dir]` | Open an interactive terminal in project dir |
| `/copy` | Copy the last AI response to clipboard |
| `/save [name]` | Save conversation as Markdown |
| `/retry` | Regenerate the last AI response |
| `/cd <dir>` | Set working directory for commands |
| `/tokens` | Show approximate token usage |

---

## Auto-Save & Project Workflow

When the model responds with code blocks, Ollama CLI automatically extracts and saves them:

```
~/ollama-cli-projects/
└── 2026-07-11/
    └── 143245/
        ├── main.py
        ├── requirements.txt
        └── README.md
```

### Multi-file annotations

Tell the model (or write yourself) path comments on the first line of each block:

```python
# file: src/main.py
def hello():
    print("world")
```

```go
// file: cmd/root.go
package cmd
```

```html
<!-- file: index.html -->
<!DOCTYPE html>
<html>...</html>
```

If no annotation is present, filenames are inferred from the language (`main.py`, `index.html`, `script.js`, etc.).

### Error-fix loop

1. You ask the model to build something
2. Code is auto-saved to a timestamped project folder
3. You run `/run npm install && npm start` (or the CLI infers a launch command)
4. If the command fails, stderr/stdout is automatically sent back to the model
5. The model responds with corrected code, which is saved again

This creates a tight edit-run-fix loop without copy-pasting errors manually.

---

## Configuration

Create `~/.ollamacli.yaml` (or `%USERPROFILE%\.ollamacli.yaml` on Windows):

```yaml
# Default Ollama model
default_model: qwen2.5-coder:7b

# Ollama server URL
ollama_host: http://localhost:11434

# System prompt sent with every conversation
system_prompt: |
  You are an expert coding assistant running inside Ollama CLI.
  Be concise and practical. Prefer working, runnable code.

# UI theme: dark | dracula | nord | monokai | ocean | forest | sunset
theme: dark

# Auto-approve tool execution (future use)
auto_approve_tools: false
```

Config values can also be set via environment variables (Viper automatic env).

---

## Recommended Models

| Model | Speed | Best For |
|---|---|---|
| `llama3.2:3b` | Fast | Quick tasks, low RAM |
| `qwen2.5-coder:7b` | Good | **Code generation & debugging** |
| `llama3.2:8b` | Good | General conversation |
| `codellama:7b` | Good | Code completion |
| `deepseek-coder:6.7b` | Good | Complex coding tasks |

Pull any model with:

```bash
ollama pull <model-name>
```

---

## Keyboard Shortcuts

| Key | Action |
|---|---|
| `Enter` | Send message |
| `Shift+Enter` | New line in input |
| `↑` / `↓` | Navigate input history |
| `Ctrl+C` | Quit (or cancel streaming) |
| `Ctrl+L` | Jump to bottom of chat |

---

## Architecture

```
ollama-cli/
├── main.go                 # Entry point
├── cmd/
│   ├── root.go             # Cobra root command, config (Viper)
│   └── chat.go             # Chat subcommand → launches TUI
├── internal/
│   ├── ollama/
│   │   └── client.go       # HTTP client for Ollama /api/chat streaming
│   ├── tui/
│   │   ├── app.go          # Bubble Tea model, slash commands, streaming
│   │   └── styles.go       # Lipgloss themes and styles
│   └── tools/
│       ├── tools.go        # File read/write, directory listing
│       ├── runner.go       # Code execution, auto-save, run inference
│       ├── runner_windows.go
│       └── runner_other.go
├── go.mod
└── go.sum
```

### Tech stack

- **[Cobra](https://github.com/spf13/cobra)** — CLI framework
- **[Viper](https://github.com/spf13/viper)** — configuration
- **[Bubble Tea](https://github.com/charmbracelet/bubbletea)** — TUI framework
- **[Lipgloss](https://github.com/charmbracelet/lipgloss)** — terminal styling
- **[Glamour](https://github.com/charmbracelet/glamour)** — Markdown rendering
- **[Bubbles](https://github.com/charmbracelet/bubbles)** — TUI components (textarea, viewport, spinner)

---

## Building from Source

```bash
# Clone
git clone https://github.com/gurtej9602/ollama-cli.git
cd ollama-cli

# Download dependencies
go mod download

# Build (current platform)
go build -ldflags="-s -w" -o ollama-cli .

# Run
./ollama-cli
```

---

## Cross-Platform Builds

```bash
# Windows (amd64)
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o ollama-cli.exe .

# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o ollama-cli-mac .

# macOS (Intel)
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o ollama-cli-mac-intel .

# Linux (amd64)
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o ollama-cli-linux .
```

On Windows PowerShell:

```powershell
$env:GOOS="windows"; $env:GOARCH="amd64"
go build -ldflags="-s -w" -o ollama-cli.exe .
```

---

## Troubleshooting

### Cannot connect to Ollama

```
✗ cannot connect to Ollama: ollama not reachable at http://localhost:11434
```

- Make sure Ollama is running: `ollama serve`
- Check the host URL: `ollama-cli --host http://localhost:11434`
- On Windows, Ollama usually starts automatically from the system tray

### Model not found

```
✗ ollama returned HTTP 404 — is model "llama3.2" pulled?
```

Pull the model first:

```bash
ollama pull llama3.2
```

### Clipboard not working

The `/copy` command requires clipboard support. On Linux you may need `xclip` or `xsel` installed.

### Commands fail on Windows

Ollama CLI uses `cmd.exe /C` for shell commands (not PowerShell) so `&&` chaining works reliably. For interactive debugging, use `/terminal` to open a PowerShell window in your project folder.

### TUI looks broken

- Use a terminal that supports ANSI colors (Windows Terminal, iTerm2, Kitty, etc.)
- Try resizing the window — the layout adapts to terminal size

---

## Contributing

Contributions are welcome! To get started:

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/my-feature`
3. Make your changes and test locally
4. Commit: `git commit -m "Add my feature"`
5. Push and open a Pull Request

Please keep changes focused and match the existing code style.

---

## License

MIT License — see [LICENSE](LICENSE) for details.

---

<p align="center">
  Built with 🦙 and ☕ — run AI locally, keep your data private.
</p>
