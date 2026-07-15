package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	ollamaclient "github.com/gurtej9602/ollama-cli/internal/ollama"
	"github.com/gurtej9602/ollama-cli/internal/tools"
)

// Config holds the app configuration
type Config struct {
	Model        string
	Host         string
	SystemPrompt string
}

// App is the main Bubble Tea application
type App struct {
	cfg Config
}

// NewApp creates a new App
func NewApp(cfg Config) *App {
	return &App{cfg: cfg}
}

// Run launches the Bubble Tea TUI
func (a *App) Run() error {
	m, err := newModel(a.cfg)
	if err != nil {
		return err
	}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err = p.Run()
	return err
}

// ── Messages ──────────────────────────────────────────────────────────────────

type (
	tokenMsg      string // one streaming token from Ollama
	streamDoneMsg string // full response when streaming finishes
	errMsg        error  // any error
	connectedMsg  struct{}
	listModelsMsg []string
)

// cmdResultMsg wraps a captured shell/command result + whether it was a launcher.
type cmdResultMsg struct {
	result     tools.CommandResult
	isLauncher bool
}

// streamState holds the channels for a live stream
type streamState struct {
	tokenCh <-chan string
	doneCh  <-chan string
	errCh   <-chan error
	cancel  context.CancelFunc
}

// ── App states ────────────────────────────────────────────────────────────────

type appState int

const (
	stateConnecting  appState = iota
	stateReady
	stateStreaming
	stateError
	stateCmdApproval // waiting for user to approve running a command
	stateRunningCmd  // a shell command is currently executing (captured)
)

// ── Model ─────────────────────────────────────────────────────────────────────

type model struct {
	cfg         Config
	client      *ollamaclient.Client
	state       appState
	spinner     spinner.Model
	viewport    viewport.Model
	input       textarea.Model
	messages    []ollamaclient.Message
	currentResp *strings.Builder
	history     []string
	errorMsg    string
	width       int
	height      int
	renderer    *glamour.TermRenderer
	stream      *streamState
	// command approval
	pendingCmd        string
	pendingIsLauncher bool
	// project context
	projectRoot string
	modelList   []string
	// input history (shell-style ↑/↓ navigation)
	inputHistory []string
	historyIdx   int // -1 = not navigating
	draftInput   string
	appendPrompt string
	// last assistant reply
	lastAssistant string
	// session stats
	startTime     time.Time
	responseCount int
	// aliases: /name → expansion text
	aliases map[string]string
}

func newModel(cfg Config) (*model, error) {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(colorPrimary)

	ta := textarea.New()
	ta.Placeholder = "Message Ollama... (Enter=send, Shift+Enter=newline, ↑/↓=history, /help=commands)"
	ta.Focus()
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.CharLimit = 4096

	vp := viewport.New(80, 20)
	vp.SetContent(hintStyle.Render("Connecting to Ollama..."))

	client, err := ollamaclient.NewClient(cfg.Host, cfg.Model)
	if err != nil {
		return nil, fmt.Errorf("failed to create ollama client: %w", err)
	}

	m := &model{
		cfg:           cfg,
		client:        client,
		state:         stateConnecting,
		spinner:       s,
		viewport:      vp,
		input:         ta,
		currentResp:   &strings.Builder{},
		historyIdx:    -1,
		startTime:     time.Now(),
		aliases:       make(map[string]string),
	}
	m.updateRenderer()
	return m, nil
}

func (m *model) updateRenderer() {
	style := "dark"
	switch currentThemeName {
	case "light":
		style = "light"
	case "dracula":
		style = "dracula"
	case "nord":
		style = "nord"
	default:
		// purple, green, forest, sunset, monokai, etc. fall back to dark style for markdown rendering
		style = "dark"
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(100),
	)
	if err == nil {
		m.renderer = r
	}
}

// ── Init ──────────────────────────────────────────────────────────────────────

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, pingOllama(m.client))
}

func pingOllama(client *ollamaclient.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := client.Ping(ctx); err != nil {
			return errMsg(fmt.Errorf("cannot connect to Ollama: %w", err))
		}
		return connectedMsg{}
	}
}

// ── Streaming helpers ─────────────────────────────────────────────────────────

func startStream(client *ollamaclient.Client, messages []ollamaclient.Message, systemPrompt string) (tea.Cmd, *streamState) {
	tokenCh := make(chan string, 256)
	doneCh := make(chan string, 1)
	errCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())

	ss := &streamState{tokenCh: tokenCh, doneCh: doneCh, errCh: errCh, cancel: cancel}

	go func() {
		resp, err := client.StreamChat(ctx, messages, systemPrompt, func(token string) {
			select {
			case tokenCh <- token:
			case <-ctx.Done():
			}
		})
		close(tokenCh)
		if err != nil && ctx.Err() == nil {
			errCh <- err
		} else {
			doneCh <- resp
		}
	}()

	return waitForNext(ss), ss
}

func waitForNext(ss *streamState) tea.Cmd {
	return func() tea.Msg {
		select {
		case token, ok := <-ss.tokenCh:
			if !ok {
				select {
				case resp := <-ss.doneCh:
					return streamDoneMsg(resp)
				case err := <-ss.errCh:
					return errMsg(err)
				}
			}
			return tokenMsg(token)
		case resp := <-ss.doneCh:
			return streamDoneMsg(resp)
		case err := <-ss.errCh:
			return errMsg(err)
		}
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeComponents()
		m.refreshViewport()

	case connectedMsg:
		m.state = stateReady
		welcome := fmt.Sprintf("%s  Connected to Ollama  %s\n%s\n",
			successStyle.Render("✓"),
			modelBadgeStyle.Render("▸ "+m.cfg.Model),
			hintStyle.Render("Type a message and press Enter. /help for commands."),
		)
		m.history = append(m.history, welcome)
		m.refreshViewport()

	case errMsg:
		m.state = stateError
		m.errorMsg = msg.Error()
		m.history = append(m.history, errorStyle.Render("✗ "+m.errorMsg)+"\n")
		m.refreshViewport()

	case tokenMsg:
		m.currentResp.WriteString(string(msg))
		m.refreshViewport()
		m.viewport.GotoBottom()
		if m.stream != nil {
			cmds = append(cmds, waitForNext(m.stream))
		}

	// ── Stream complete: auto-save ALL files, infer run command ──────────────
	case streamDoneMsg:
		m.state = stateReady
		fullResponse := string(msg)
		m.lastAssistant = fullResponse
		m.responseCount++
		ts := timestampStyle.Render(" [" + time.Now().Format("15:04") + "]")
		rendered := m.renderMD(fullResponse)
		if len(m.history) > 0 {
			m.history[len(m.history)-1] = assistantLabelStyle.Render("🦙 Ollama") + ts + "\n" + rendered
		}
		m.messages = append(m.messages, ollamaclient.Message{Role: "assistant", Content: fullResponse})
		m.currentResp = &strings.Builder{}
		m.stream = nil

		// ── Auto-save every code block the LLM produced ──────────────────────
		projectRoot, projectFiles, saveErr := tools.AutoSaveResponse(fullResponse)
		if saveErr != nil {
			m.history = append(m.history, errorStyle.Render("⚠ Save error: "+saveErr.Error())+"\n")
		} else if projectRoot != "" {
			m.projectRoot = projectRoot

			// Show what was saved
			var sb strings.Builder
			sb.WriteString(successStyle.Render(fmt.Sprintf("💾 %d file(s) saved", len(projectFiles))))
			sb.WriteString(hintStyle.Render(" → " + projectRoot + "\n"))
			for _, f := range projectFiles {
				sb.WriteString(hintStyle.Render("   📄 " + f.RelPath + "\n"))
			}
			m.history = append(m.history, sb.String())

			// ── Verification Step ─────────────────────────────────────────────
			m.history = append(m.history, hintStyle.Render("🔍 Checking file structure...\n"))
			if verifyErr := tools.VerifySavedFiles(projectRoot, projectFiles); verifyErr != nil {
				m.history = append(m.history, errorStyle.Render("❌ Verification failed: "+verifyErr.Error())+"\n")
			} else {
				m.history = append(m.history, successStyle.Render("✓ File structure verified successfully!\n"))

				// ── Infer the correct run command for this project ────────────────
				if runCmd := tools.InferRunCommand(projectFiles, projectRoot); runCmd != "" {
					m.pendingCmd = runCmd
					m.pendingIsLauncher = tools.IsLauncherCmd(runCmd)
					m.state = stateCmdApproval
					m.history = append(m.history, runPromptStyleCmd(runCmd, projectRoot))
				}
			}
		}

		m.refreshViewport()
		m.viewport.GotoBottom()

	// ── Command result: show output, feed errors back to LLM ─────────────────
	case cmdResultMsg:
		result := msg.result
		hasError := result.Error != nil || result.ExitCode != 0
		outputText := strings.TrimSpace(result.Output)

		if msg.isLauncher && !hasError {
			// Browser/app open — just confirm, no LLM feedback needed
			m.history = append(m.history,
				successStyle.Render("✓ Opened in browser / app")+"\n")
			m.state = stateReady
			m.refreshViewport()
			m.viewport.GotoBottom()
			return m, tea.Batch(cmds...)
		}

		// ── Build a clean output display ──────────────────────────────────────
		displayOut := outputText
		if displayOut == "" {
			displayOut = "(no output)"
		}

		var display string
		if hasError {
			// Include the Go-level error only when it adds information beyond
			// what the process already printed (e.g. timeout, not-found).
			extraErr := ""
			if result.Error != nil {
				errStr := result.Error.Error()
				if !strings.Contains(outputText, errStr) {
					extraErr = "\n**System error:** " + errStr
				}
			}
			display = fmt.Sprintf("**Command failed** (exit code %d, %.1fs):\n```\n$ %s\n%s\n```%s\n",
				result.ExitCode, result.Duration.Seconds(),
				result.Command, displayOut, extraErr)
		} else {
			display = fmt.Sprintf("**Command succeeded** (%.1fs):\n```\n$ %s\n%s\n```\n",
				result.Duration.Seconds(), result.Command, displayOut)
		}
		m.history = append(m.history, toolStyle.Render("⚙ Run Result")+"\n"+m.renderMD(display))
		m.refreshViewport()
		m.viewport.GotoBottom()

		// ── On error: feed full context to LLM so it can fix the code ─────────
		if hasError {
			projectInfo := ""
			if m.projectRoot != "" {
				projectInfo = fmt.Sprintf(" in `%s`", m.projectRoot)
			}
			feedback := fmt.Sprintf(
				"I ran `%s`%s and it **failed** with exit code %d.\n\n"+
					"**Full output / error:**\n```\n%s\n```\n\n"+
					"Please carefully read the error above, identify the root cause, and provide the complete corrected code with all necessary fixes.",
				result.Command, projectInfo, result.ExitCode, displayOut,
			)
			m.messages = append(m.messages, ollamaclient.Message{Role: "user", Content: feedback})
			m.history = append(m.history, userLabelStyle.Render("You")+hintStyle.Render(" [error auto-sent for fix — waiting for Ollama...]\n"))
			m.history = append(m.history, assistantLabelStyle.Render("🦙 Ollama")+"\n")
			m.currentResp.Reset()
			m.refreshViewport()
			m.viewport.GotoBottom()
			m.state = stateStreaming
			streamCmd, ss := startStream(m.client, m.messages, m.cfg.SystemPrompt)
			m.stream = ss
			return m, tea.Batch(append(cmds, streamCmd, m.spinner.Tick)...)
		}

		// ── On success: return to ready — no need to notify the LLM ──────────
		m.state = stateReady
		return m, tea.Batch(cmds...)

	// ── Model list ────────────────────────────────────────────────────────────
	case listModelsMsg:
		m.modelList = []string(msg)
		var sb strings.Builder
		sb.WriteString("## Available Models\n\n")
		sb.WriteString("| # | Model |\n|---|----------|\n")
		for i, name := range msg {
			active := ""
			if name == m.cfg.Model {
				active = " ✓"
			}
			sb.WriteString(fmt.Sprintf("| %d | `%s`%s |\n", i+1, name, active))
		}
		sb.WriteString("\n_Switch with_ `/model <name>` _or_ `/model <number>`_._\n")
		m.history = append(m.history, m.renderMD(sb.String()))
		m.refreshViewport()
		m.viewport.GotoBottom()

	// ── Spinner ───────────────────────────────────────────────────────────────
	case spinner.TickMsg:
		if m.state == stateConnecting || m.state == stateStreaming || m.state == stateRunningCmd {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}

	// ── Key events ────────────────────────────────────────────────────────────
	case tea.KeyMsg:
		switch m.state {

		// ── Command approval ──
		case stateCmdApproval:
			switch strings.ToLower(msg.String()) {
			case "y":
				if len(m.history) > 0 {
					m.history = m.history[:len(m.history)-1]
				}
				cmd := m.pendingCmd
				workDir := m.projectRoot
				isLauncher := m.pendingIsLauncher
				m.history = append(m.history,
					toolStyle.Render("⚙ Launching terminal for: ")+hintStyle.Render(cmd)+"\n")
				m.history = append(m.history,
					hintStyle.Render("   A terminal window will open — interact with the program, then close it when done.\n"))
				m.refreshViewport()
				m.viewport.GotoBottom()
				m.state = stateRunningCmd
				return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
					res := tools.RunInVisibleTerminalCaptured(cmd, workDir, 120*time.Second)
					return cmdResultMsg{result: res, isLauncher: isLauncher}
				})
			case "n", tea.KeyEsc.String():
				m.state = stateReady
				if len(m.history) > 0 {
					m.history = m.history[:len(m.history)-1]
				}
				m.history = append(m.history, hintStyle.Render("Skipped.\n"))
				m.refreshViewport()
			}
			return m, tea.Batch(cmds...)

		case stateReady:
			switch msg.Type {
			case tea.KeyCtrlC:
				return m, tea.Quit

			case tea.KeyCtrlL:
				// Jump to bottom
				m.viewport.GotoBottom()
				return m, tea.Batch(cmds...)

			case tea.KeyUp:
				// Navigate input history backwards (older)
				if len(m.inputHistory) == 0 {
					break
				}
				if m.historyIdx == -1 {
					// Save current draft before navigating
					m.draftInput = m.input.Value()
					m.historyIdx = 0
				} else if m.historyIdx < len(m.inputHistory)-1 {
					m.historyIdx++
				}
				m.input.SetValue(m.inputHistory[m.historyIdx])
				return m, tea.Batch(cmds...)

			case tea.KeyDown:
				// Navigate input history forwards (newer / back to draft)
				if m.historyIdx == -1 {
					break
				}
				if m.historyIdx == 0 {
					// Back to the unsaved draft
					m.historyIdx = -1
					m.input.SetValue(m.draftInput)
				} else {
					m.historyIdx--
					m.input.SetValue(m.inputHistory[m.historyIdx])
				}
				return m, tea.Batch(cmds...)

			case tea.KeyEnter:
				if msg.Alt {
					break // Shift+Enter → newline
				}
				input := strings.TrimSpace(m.input.Value())
				if input == "" {
					break
				}
				m.input.Reset()
				// Reset history navigation
				m.historyIdx = -1
				m.draftInput = ""

				if strings.HasPrefix(input, "/") {
					cmds = append(cmds, m.handleSlash(input)...)
					break
				}

				// Check alias expansion: /name maps to its stored text
				if strings.HasPrefix(input, "/") {
					alias := strings.TrimPrefix(strings.Fields(input)[0], "/")
					if expansion, ok := m.aliases[alias]; ok {
						input = expansion
					}
				}

				// Push to input history (deduplicate consecutive duplicates)
				if len(m.inputHistory) == 0 || m.inputHistory[0] != input {
					m.inputHistory = append([]string{input}, m.inputHistory...)
					if len(m.inputHistory) > 100 {
						m.inputHistory = m.inputHistory[:100]
					}
				}

				inputForLLM := input
				if m.appendPrompt != "" {
					inputForLLM = input + " " + m.appendPrompt
				}

				ts := timestampStyle.Render(" [" + time.Now().Format("15:04") + "]")
				m.history = append(m.history, userLabelStyle.Render("You")+ts+" "+input+"\n")
				m.history = append(m.history, assistantLabelStyle.Render("🦙 Ollama")+"\n")
				m.messages = append(m.messages, ollamaclient.Message{Role: "user", Content: inputForLLM})
				m.currentResp.Reset()
				m.refreshViewport()
				m.viewport.GotoBottom()

				m.state = stateStreaming
				streamCmd, ss := startStream(m.client, m.messages, m.cfg.SystemPrompt)
				m.stream = ss
				cmds = append(cmds, streamCmd, m.spinner.Tick)
			}

			var inputCmd tea.Cmd
			m.input, inputCmd = m.input.Update(msg)
			cmds = append(cmds, inputCmd)

		case stateStreaming:
			if msg.Type == tea.KeyCtrlC {
				if m.stream != nil {
					m.stream.cancel()
				}
				m.state = stateReady
				m.currentResp = &strings.Builder{}
				m.stream = nil
				m.history = append(m.history, hintStyle.Render("(cancelled)\n"))
				m.refreshViewport()
			}
		}
	}

	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	header := headerStyle.Width(m.width - 4).Render(
		"🦙 Ollama CLI   " + modelBadgeStyle.Render("▸ "+m.cfg.Model) +
			themeBadgeStyle.Render(" ◈ "+currentThemeName),
	)

	vp := m.viewport.View()

	var status string
	switch m.state {
	case stateConnecting:
		status = m.spinner.View() + " Connecting to Ollama..."
	case stateStreaming:
		status = m.spinner.View() + " " + assistantLabelStyle.Render("Streaming...") +
			hintStyle.Render("   Ctrl+C to cancel")
	case stateRunningCmd:
		status = m.spinner.View() + " " + toolStyle.Render("Executing command...") +
			hintStyle.Render("   please wait")
	case stateReady:
		status = successStyle.Render("● Ready") +
			hintStyle.Render("   ↑/↓ history  •  Ctrl+L=bottom  •  /help for commands")
	case stateCmdApproval:
		status = toolStyle.Render("▶ Run?") +
			hintStyle.Render("   Y to run  •  N or Esc to skip")
	case stateError:
		status = errorStyle.Render("✗ " + m.errorMsg)
	}
	statusBar := statusBarStyle.Width(m.width - 2).Render(status)

	var inputView string
	if m.state == stateReady {
		inputView = inputFocusedStyle.Width(m.width - 4).Render(m.input.View())
	} else {
		inputView = inputStyle.Width(m.width - 4).Render(m.input.View())
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, vp, statusBar, inputView)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (m *model) resizeComponents() {
	headerH := 3
	statusH := 2
	inputH := m.input.Height() + 2
	vpH := m.height - headerH - statusH - inputH - 2
	if vpH < 5 {
		vpH = 5
	}
	m.viewport.Width = m.width - 2
	m.viewport.Height = vpH
	m.input.SetWidth(m.width - 4)
}

func (m *model) refreshViewport() {
	var sb strings.Builder
	for _, block := range m.history {
		sb.WriteString(block)
	}
	if m.state == stateStreaming && m.currentResp.Len() > 0 {
		sb.WriteString(m.renderMD(m.currentResp.String()))
	}
	m.viewport.SetContent(sb.String())
}

func (m *model) renderMD(raw string) string {
	out, err := m.renderer.Render(raw)
	if err != nil {
		return raw
	}
	return out
}

// ── Slash commands ────────────────────────────────────────────────────────────

func (m *model) handleSlash(input string) []tea.Cmd {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return nil
	}

	switch parts[0] {
	case "/help":
		help := `## Commands

| Command | Description |
|---|---|
| ` + "`/models`" + ` | List all locally available models |
| ` + "`/model <name>`" + ` | Switch to a specific model by name or number |
| ` + "`/clear`" + ` | Clear conversation history |
| ` + "`/file <path>`" + ` | Load a file into context |
| ` + "`/system <text>`" + ` | Set system prompt |
| ` + "`/append <text>`" + ` | Set a prompt to append to all user messages |
| ` + "`/theme [name]`" + ` | Switch theme or list themes |
| ` + "`/run <cmd>`" + ` | Run a shell command (output shown + errors auto-fixed by LLM) |
| ` + "`/terminal [dir]`" + ` | Open an interactive terminal in the project dir (or [dir]) |
| ` + "`/copy`" + ` | Copy last AI response to clipboard |
| ` + "`/save [name]`" + ` | Save full conversation as Markdown |
| ` + "`/retry`" + ` | Regenerate the last AI response |
| ` + "`/cd <dir>`" + ` | Change the working directory for commands |
| ` + "`/tokens`" + ` | Show approximate token count for this conversation |
| ` + "`Ctrl+C`" + ` | Quit (or cancel streaming) |
| ` + "`Ctrl+L`" + ` | Jump to bottom of chat |
| ` + "`↑ / ↓`" + ` | Navigate your input history (like a terminal) |

**Auto-save**: Every time the LLM responds with code, ALL files are automatically saved to
` + "`~/ollama-cli-projects/YYYY-MM-DD/HHMMSS/`" + ` — even without pressing Y.

**Smart run**: After saving, the CLI infers the correct launch command for the project
(e.g. ` + "`start index.html`" + ` for web, ` + "`python main.py`" + ` for Python, ` + "`npm install && npm start`" + ` for Node).

**Error loop**: When a command fails, the full error output is automatically fed back to the
LLM which identifies the root cause and provides corrected code.

**Interactive terminal**: Use ` + "`/terminal`" + ` to open a PowerShell window in your project
folder to run commands manually, inspect files, or debug interactively.
`
		m.history = append(m.history, m.renderMD(help))
		m.refreshViewport()

	case "/clear":
		m.messages = nil
		m.projectRoot = ""
		m.history = []string{hintStyle.Render("Conversation cleared.\n")}
		m.refreshViewport()

	case "/models":
		m.history = append(m.history, hintStyle.Render("Fetching available models...\n"))
		m.refreshViewport()
		client := m.client
		return []tea.Cmd{func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			models, err := client.ListModels(ctx)
			if err != nil {
				return errMsg(fmt.Errorf("could not list models: %w", err))
			}
			return listModelsMsg(models)
		}}

	case "/model":
		if len(parts) < 2 {
			m.history = append(m.history, errorStyle.Render("Usage: /model <name> or /model <number>\n"))
			m.refreshViewport()
			return nil
		}
		arg := strings.Join(parts[1:], " ")
		if idx, err := strconv.Atoi(strings.TrimSpace(arg)); err == nil {
			if len(m.modelList) == 0 {
				m.history = append(m.history, errorStyle.Render("Run /models first.\n"))
				m.refreshViewport()
				return nil
			}
			if idx < 1 || idx > len(m.modelList) {
				m.history = append(m.history, errorStyle.Render(fmt.Sprintf("Choose 1–%d.\n", len(m.modelList))))
				m.refreshViewport()
				return nil
			}
			arg = m.modelList[idx-1]
		}
		m.client.SetModel(arg)
		m.cfg.Model = arg
		m.history = append(m.history, successStyle.Render(fmt.Sprintf("✓ Switched to: %s\n", arg)))
		m.refreshViewport()

	case "/file":
		if len(parts) < 2 {
			m.history = append(m.history, errorStyle.Render("Usage: /file <path>\n"))
			m.refreshViewport()
			return nil
		}
		path := parts[1]
		return []tea.Cmd{func() tea.Msg {
			content, err := tools.ReadFile(path)
			if err != nil {
				return errMsg(fmt.Errorf("cannot read %s: %w", path, err))
			}
			return tokenMsg(fmt.Sprintf("📁 **Loaded:** `%s`\n\n```\n%s\n```\n", path, content))
		}}

	case "/system":
		if len(parts) < 2 {
			m.history = append(m.history, errorStyle.Render("Usage: /system <prompt>\n"))
			m.refreshViewport()
			return nil
		}
		m.cfg.SystemPrompt = strings.Join(parts[1:], " ")
		m.history = append(m.history, successStyle.Render("✓ System prompt updated\n"))
		m.refreshViewport()

	case "/append":
		if len(parts) < 2 {
			m.history = append(m.history, errorStyle.Render("Usage: /append <prompt to append>\n"))
			m.refreshViewport()
			return nil
		}
		m.appendPrompt = strings.Join(parts[1:], " ")
		m.history = append(m.history, successStyle.Render("✓ Append prompt updated\n"))
		m.refreshViewport()

	case "/theme":
		if len(parts) < 2 {
			themesList := ListThemes()
			var sb strings.Builder
			sb.WriteString("## Available Themes\n\n")
			for _, name := range themesList {
				active := ""
				if name == currentThemeName {
					active = " ✓"
				}
				sb.WriteString(fmt.Sprintf("- `%s`%s\n", name, active))
			}
			sb.WriteString("\n_Switch with_ `/theme <name>`_._\n")
			m.history = append(m.history, m.renderMD(sb.String()))
			m.refreshViewport()
			m.viewport.GotoBottom()
			return nil
		}
		themeName := strings.TrimSpace(parts[1])
		if SetTheme(themeName) {
			m.history = append(m.history, successStyle.Render(fmt.Sprintf("✓ Switched theme to: %s\n", themeName)))
			m.updateRenderer()
		} else {
			m.history = append(m.history, errorStyle.Render(fmt.Sprintf("Unknown theme: %s\n", themeName)))
		}
		m.refreshViewport()
		m.viewport.GotoBottom()
		return nil

	case "/run":
		if len(parts) < 2 {
			m.history = append(m.history, errorStyle.Render("Usage: /run <shell command>\n"))
			m.refreshViewport()
			return nil
		}
		cmd := strings.Join(parts[1:], " ")
		workDir := m.projectRoot
		isLauncher := tools.IsLauncherCmd(cmd)
		m.history = append(m.history, toolStyle.Render("⚙ Running: ")+hintStyle.Render(cmd)+"\n")
		m.refreshViewport()
		m.viewport.GotoBottom()
		m.state = stateRunningCmd
		return []tea.Cmd{m.spinner.Tick, func() tea.Msg {
			res := tools.RunCommandCaptured(cmd, workDir, 120*time.Second)
			return cmdResultMsg{result: res, isLauncher: isLauncher}
		}}

	case "/terminal":
		// Open an interactive terminal window in the project root (or a given dir).
		dir := m.projectRoot
		if len(parts) >= 2 {
			target := strings.Join(parts[1:], " ")
			if strings.HasPrefix(target, "~") {
				home, _ := os.UserHomeDir()
				target = filepath.Join(home, target[1:])
			}
			if abs, err := filepath.Abs(target); err == nil {
				dir = abs
			}
		}
		if dir == "" {
			var err error
			dir, err = os.UserHomeDir()
			if err != nil {
				m.history = append(m.history, errorStyle.Render("⚠ Cannot determine directory\n"))
				m.refreshViewport()
				return nil
			}
		}
		finalDir := dir
		m.history = append(m.history,
			successStyle.Render("✓ Opening terminal in: ")+hintStyle.Render(finalDir+"\n"))
		m.refreshViewport()
		m.viewport.GotoBottom()
		go func() { tools.OpenTerminalInDir(finalDir) }()
		return nil

	case "/copy":
		if m.lastAssistant == "" {
			m.history = append(m.history, errorStyle.Render("Nothing to copy yet.\n"))
			m.refreshViewport()
			return nil
		}
		if err := clipboard.WriteAll(m.lastAssistant); err != nil {
			m.history = append(m.history, errorStyle.Render("⚠ Clipboard error: "+err.Error()+"\n"))
		} else {
			m.history = append(m.history, successStyle.Render("✓ Last response copied to clipboard!\n"))
		}
		m.refreshViewport()
		m.viewport.GotoBottom()

	case "/save":
		name := ""
		if len(parts) >= 2 {
			name = strings.Join(parts[1:], "_")
		}
		path, err := saveConversation(m.messages, name)
		if err != nil {
			m.history = append(m.history, errorStyle.Render("⚠ Save error: "+err.Error()+"\n"))
		} else {
			m.history = append(m.history, successStyle.Render("✓ Saved → ")+hintStyle.Render(path+"\n"))
		}
		m.refreshViewport()
		m.viewport.GotoBottom()

	case "/retry":
		// Re-send the last user message to get a fresh response
		var lastUser string
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].Role == "user" {
				lastUser = m.messages[i].Content
				break
			}
		}
		if lastUser == "" {
			m.history = append(m.history, errorStyle.Render("Nothing to retry.\n"))
			m.refreshViewport()
			return nil
		}
		// Drop last assistant reply from messages so we get a fresh one
		if len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == "assistant" {
			m.messages = m.messages[:len(m.messages)-1]
		}
		m.history = append(m.history, hintStyle.Render("↻ Retrying last message...\n"))
		m.history = append(m.history, assistantLabelStyle.Render("🦙 Ollama")+"\n")
		m.currentResp.Reset()
		m.refreshViewport()
		m.viewport.GotoBottom()
		m.state = stateStreaming
		streamCmd, ss := startStream(m.client, m.messages, m.cfg.SystemPrompt)
		m.stream = ss
		return []tea.Cmd{streamCmd, m.spinner.Tick}

	case "/cd":
		if len(parts) < 2 {
			cwd := m.projectRoot
			if cwd == "" {
				cwd = "(not set)"
			}
			m.history = append(m.history, hintStyle.Render("Working dir: "+cwd+"\n"))
			m.refreshViewport()
			return nil
		}
		target := strings.Join(parts[1:], " ")
		// Expand ~ to home dir
		if strings.HasPrefix(target, "~") {
			home, _ := os.UserHomeDir()
			target = filepath.Join(home, target[1:])
		}
		abs, err := filepath.Abs(target)
		if err != nil {
			m.history = append(m.history, errorStyle.Render("⚠ "+err.Error()+"\n"))
			m.refreshViewport()
			return nil
		}
		if info, err := os.Stat(abs); err != nil || !info.IsDir() {
			m.history = append(m.history, errorStyle.Render("⚠ Not a valid directory: "+abs+"\n"))
			m.refreshViewport()
			return nil
		}
		m.projectRoot = abs
		m.history = append(m.history, successStyle.Render("✓ Working dir → ")+hintStyle.Render(abs+"\n"))
		m.refreshViewport()
		m.viewport.GotoBottom()

	case "/tokens":
		total := 0
		for _, msg := range m.messages {
			// Rough approximation: 1 token ≈ 4 chars
			total += len(msg.Content) / 4
		}
		systemTokens := len(m.cfg.SystemPrompt) / 4
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("## Token Usage\n\n"))
		sb.WriteString(fmt.Sprintf("| | Tokens |\n|---|---:|\n"))
		sb.WriteString(fmt.Sprintf("| System prompt | ~%d |\n", systemTokens))
		sb.WriteString(fmt.Sprintf("| Conversation | ~%d |\n", total))
		sb.WriteString(fmt.Sprintf("| **Total** | **~%d** |\n", total+systemTokens))
		sb.WriteString(fmt.Sprintf("\n_(%d messages in context)_\n", len(m.messages)))
		m.history = append(m.history, m.renderMD(sb.String()))
		m.refreshViewport()
		m.viewport.GotoBottom()

	default:
		m.history = append(m.history, errorStyle.Render(fmt.Sprintf("Unknown command: %s  (try /help)\n", parts[0])))
		m.refreshViewport()
	}

	return nil
}

// saveConversation writes the full chat to a Markdown file and returns the path.
func saveConversation(messages []ollamaclient.Message, name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "ollama-cli-projects", "conversations")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	ts := time.Now().Format("2006-01-02_150405")
	fileName := ts
	if name != "" {
		fileName = ts + "_" + name
	}
	path := filepath.Join(dir, fileName+".md")

	var sb strings.Builder
	sb.WriteString("# Ollama CLI Conversation\n\n")
	sb.WriteString(fmt.Sprintf("_Saved: %s_\n\n---\n\n", time.Now().Format("2006-01-02 15:04:05")))
	for _, msg := range messages {
		switch msg.Role {
		case "user":
			sb.WriteString("### 🧑 You\n\n")
		case "assistant":
			sb.WriteString("### 🦙 Ollama\n\n")
		default:
			sb.WriteString(fmt.Sprintf("### %s\n\n", msg.Role))
		}
		sb.WriteString(msg.Content)
		sb.WriteString("\n\n---\n\n")
	}

	return path, os.WriteFile(path, []byte(sb.String()), 0644)
}
