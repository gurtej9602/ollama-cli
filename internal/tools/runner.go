package tools

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// fileAnnotationRe matches lines like:
//
//	// file: src/main.py
//	# file: src/main.py
//	<!-- file: src/main.py -->
var fileAnnotationRe = regexp.MustCompile(`(?i)^\s*(?://|#|<!--)\s*file:\s*(.+?)\s*(?:-->)?\s*$`)

// ExtractFileAnnotation checks if the first non-empty line of code declares a
// file path (e.g. "// file: src/main.py"). Returns the path and true, or ("", false).
func ExtractFileAnnotation(code string) (string, bool) {
	for _, line := range strings.Split(code, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if m := fileAnnotationRe.FindStringSubmatch(line); len(m) == 2 {
			return filepath.FromSlash(strings.TrimSpace(m[1])), true
		}
		// First non-empty line is not an annotation
		break
	}
	return "", false
}

// StripFileAnnotation removes the first line if it is a file annotation comment.
func StripFileAnnotation(code string) string {
	lines := strings.SplitN(code, "\n", 2)
	if len(lines) < 2 {
		return code
	}
	if m := fileAnnotationRe.FindStringSubmatch(lines[0]); len(m) == 2 {
		return strings.TrimPrefix(lines[1], "\n")
	}
	return code
}

// ProjectFile is a single file in a multi-file project.
type ProjectFile struct {
	RelPath  string // relative path within the project (e.g. "src/main.py")
	Language string
	Code     string
}

// SaveProject writes all project files under ~/ollama-cli-projects/<timestamp>/
// and returns the project root directory.
func SaveProject(files []ProjectFile) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	ts := time.Now().Format("20060102_150405")
	root := filepath.Join(home, "ollama-cli-projects", ts)
	if err := os.MkdirAll(root, 0755); err != nil {
		return "", fmt.Errorf("cannot create project directory: %w", err)
	}
	for _, f := range files {
		dest := filepath.Join(root, f.RelPath)
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return root, fmt.Errorf("cannot create directory for %s: %w", f.RelPath, err)
		}
		if err := os.WriteFile(dest, []byte(f.Code), 0644); err != nil {
			return root, fmt.Errorf("cannot write %s: %w", f.RelPath, err)
		}
	}
	return root, nil
}

// DefaultFileName returns a sensible default filename for a code block based on its
// language and how many blocks of the same language have been seen before it.
func DefaultFileName(lang string, index int) string {
	sfx := ""
	if index > 0 {
		sfx = fmt.Sprintf("%d", index+1)
	}
	switch strings.ToLower(lang) {
	case "html":
		if index == 0 {
			return "index.html"
		}
		return fmt.Sprintf("page%d.html", index+1)
	case "css":
		if index == 0 {
			return "styles.css"
		}
		return fmt.Sprintf("styles%d.css", index+1)
	case "javascript", "js", "node":
		if index == 0 {
			return "script.js"
		}
		return fmt.Sprintf("script%d.js", index+1)
	case "typescript", "ts":
		return fmt.Sprintf("script%s.ts", sfx)
	case "python", "python3", "py":
		if index == 0 {
			return "main.py"
		}
		return fmt.Sprintf("module%d.py", index+1)
	case "go", "golang":
		if index == 0 {
			return "main.go"
		}
		return fmt.Sprintf("module%d.go", index+1)
	case "rust", "rs":
		return fmt.Sprintf("main%s.rs", sfx)
	case "c":
		return fmt.Sprintf("main%s.c", sfx)
	case "cpp", "c++":
		return fmt.Sprintf("main%s.cpp", sfx)
	case "ruby", "rb":
		return fmt.Sprintf("script%s.rb", sfx)
	case "json":
		if index == 0 {
			return "data.json"
		}
		return fmt.Sprintf("data%d.json", index+1)
	case "yaml", "yml":
		if index == 0 {
			return "config.yaml"
		}
		return fmt.Sprintf("config%d.yaml", index+1)
	case "toml":
		if index == 0 {
			return "config.toml"
		}
		return fmt.Sprintf("config%d.toml", index+1)
	case "bash", "sh", "shell":
		return fmt.Sprintf("script%s.sh", sfx)
	case "powershell", "ps1":
		return fmt.Sprintf("script%s.ps1", sfx)
	case "sql":
		return fmt.Sprintf("query%s.sql", sfx)
	case "dockerfile":
		return "Dockerfile"
	case "makefile":
		return "Makefile"
	default:
		ext := langExt(lang)
		if index == 0 {
			return "file." + ext
		}
		return fmt.Sprintf("file%d.%s", index+1, ext)
	}
}

// AutoSaveResponse extracts ALL code blocks from a markdown LLM response, assigns
// each a file path (using `// file:` annotations when present, or DefaultFileName),
// and saves them to ~/ollama-cli-projects/YYYY-MM-DD/HHMMSS/.
// Returns the project root directory, the saved files, and any error.
func AutoSaveResponse(markdown string) (string, []ProjectFile, error) {
	blocks := ExtractCodeBlocks(markdown)
	if len(blocks) == 0 {
		return "", nil, nil
	}

	langCount := map[string]int{}
	var files []ProjectFile

	for _, b := range blocks {
		lang := strings.ToLower(b.Language)
		code := b.Code
		var relPath string

		if p, ok := ExtractFileAnnotation(code); ok {
			relPath = p
			code = StripFileAnnotation(code)
		} else {
			idx := langCount[lang]
			relPath = DefaultFileName(lang, idx)
			langCount[lang]++
		}

		files = append(files, ProjectFile{
			RelPath:  relPath,
			Language: lang,
			Code:     code,
		})
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", nil, fmt.Errorf("cannot determine home directory: %w", err)
	}
	now := time.Now()
	root := filepath.Join(home, "ollama-cli-projects",
		now.Format("2006-01-02"), // date folder  e.g. 2026-07-10
		now.Format("150405"),     // time folder  e.g. 120812
	)

	if err := os.MkdirAll(root, 0755); err != nil {
		return "", nil, fmt.Errorf("cannot create project directory: %w", err)
	}

	for _, f := range files {
		dest := filepath.Join(root, f.RelPath)
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return root, files, fmt.Errorf("cannot create directory for %s: %w", f.RelPath, err)
		}
		if err := os.WriteFile(dest, []byte(f.Code), 0644); err != nil {
			return root, files, fmt.Errorf("cannot write %s: %w", f.RelPath, err)
		}
	}

	return root, files, nil
}

// VerifySavedFiles checks if the files written to projectRoot exist and their contents
// match the expected code from the LLM.
func VerifySavedFiles(root string, files []ProjectFile) error {
	for _, f := range files {
		dest := filepath.Join(root, f.RelPath)
		stat, err := os.Stat(dest)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("file %s was not created", f.RelPath)
			}
			return fmt.Errorf("error accessing %s: %w", f.RelPath, err)
		}
		if stat.IsDir() {
			return fmt.Errorf("%s is a directory but expected a file", f.RelPath)
		}
		data, err := os.ReadFile(dest)
		if err != nil {
			return fmt.Errorf("error reading %s: %w", f.RelPath, err)
		}
		if string(data) != f.Code {
			return fmt.Errorf("content mismatch in file %s", f.RelPath)
		}
	}
	return nil
}

// InferRunCommand returns the best shell command to launch/run a project given
// its files. The command is intended to be executed with workDir = projectRoot.
// Returns "" if no runnable entry point can be detected.
//
// Detection priority (high → low):
//  1. Explicit build/run scripts (Makefile, package.json, Cargo.toml, go.mod, …)
//  2. Named entry-point files (main.py, main.go, index.html, …)
//  3. Language fallback on first file
func InferRunCommand(files []ProjectFile, projectRoot string) string {
	fileSet := map[string]bool{}
	for _, f := range files {
		base := strings.ToLower(filepath.Base(f.RelPath))
		rel := strings.ToLower(filepath.ToSlash(f.RelPath))
		fileSet[base] = true
		fileSet[rel] = true
	}

	// Helper to find the correct relative path for a list of potential base names
	findRelPath := func(names []string) string {
		for _, name := range names {
			for _, f := range files {
				base := strings.ToLower(filepath.Base(f.RelPath))
				if base == name {
					return filepath.ToSlash(f.RelPath)
				}
			}
		}
		return ""
	}

	// ── 1. Makefile (explicit build instructions) ─────────────────────────────
	// Only use if there's also a recognisable source file (avoid false positives).
	if fileSet["makefile"] {
		for _, src := range []string{"main.c", "main.cpp", "main.go", "main.rs"} {
			if fileSet[src] {
				return "make"
			}
		}
	}

	// ── 2. npm / node project ─────────────────────────────────────────────────
	if fileSet["package.json"] {
		// Prefer dev server when a bundler config is present
		for _, cfg := range []string{
			"vite.config.js", "vite.config.ts",
			"next.config.js", "next.config.ts",
			"astro.config.mjs", "svelte.config.js",
			"nuxt.config.js", "nuxt.config.ts",
		} {
			if fileSet[cfg] {
				return "npm install && npm run dev"
			}
		}
		return "npm install && npm start"
	}

	// ── 3. Rust / Cargo ───────────────────────────────────────────────────────
	if fileSet["cargo.toml"] {
		return "cargo run"
	}

	// ── 4. Go module ─────────────────────────────────────────────────────────
	if fileSet["go.mod"] {
		return "go run ."
	}

	// ── 5. Java ───────────────────────────────────────────────────────────────
	if fileSet["pom.xml"] {
		return "mvn -q compile && mvn -q exec:java"
	}
	if fileSet["build.gradle"] || fileSet["build.gradle.kts"] {
		return "gradle run"
	}

	// ── 6. C# / .NET ─────────────────────────────────────────────────────────
	for base := range fileSet {
		if strings.HasSuffix(base, ".csproj") || strings.HasSuffix(base, ".fsproj") {
			return "dotnet run"
		}
	}

	// ── 7. Python with dependency file ───────────────────────────────────────
	pyEntry := findRelPath([]string{"main.py", "app.py", "server.py", "run.py", "__main__.py"})
	if fileSet["requirements.txt"] {
		if pyEntry == "" {
			pyEntry = "main.py"
		}
		return "pip install -r requirements.txt && python " + pyEntry
	}
	if fileSet["pyproject.toml"] || fileSet["setup.py"] || fileSet["setup.cfg"] {
		if pyEntry == "" {
			pyEntry = "main.py"
		}
		return "pip install -e . && python " + pyEntry
	}

	// ── 8. HTML project → open in default browser ────────────────────────────
	htmlEntry := findRelPath([]string{"index.html"})
	if htmlEntry != "" {
		if runtime.GOOS == "windows" {
			return "start " + htmlEntry
		} else if runtime.GOOS == "darwin" {
			return "open " + htmlEntry
		}
		return "xdg-open " + htmlEntry
	}

	// ── 9. Single Go file (no go.mod) ────────────────────────────────────────
	goEntry := findRelPath([]string{"main.go"})
	if goEntry != "" {
		return "go run " + goEntry
	}
	// Multiple .go files → run the whole package
	goCount := 0
	for _, f := range files {
		if strings.ToLower(filepath.Ext(f.RelPath)) == ".go" {
			goCount++
		}
	}
	if goCount > 1 {
		return "go run ."
	}

	// ── 10. Node.js (no package.json) ────────────────────────────────────────
	nodeEntry := findRelPath([]string{"server.js", "app.js", "index.js", "main.js", "script.js"})
	if nodeEntry != "" {
		return "node " + nodeEntry
	}

	// ── 11. TypeScript — prefer deno if available, else ts-node ──────────────
	tsEntry := findRelPath([]string{"server.ts", "app.ts", "index.ts", "main.ts", "script.ts", "mod.ts"})
	if tsEntry != "" {
		if _, ok := lookAny("deno", "deno.exe"); ok {
			return "deno run --allow-all " + tsEntry
		}
		return "npx ts-node " + tsEntry
	}

	// ── 12. Python (no dependency file) ──────────────────────────────────────
	if pyEntry != "" {
		return "python " + pyEntry
	}
	pyFallback := findRelPath([]string{"script.py", "app.py"})
	if pyFallback != "" {
		return "python " + pyFallback
	}

	// ── 13. Ruby ─────────────────────────────────────────────────────────────
	rubyEntry := findRelPath([]string{"main.rb", "app.rb", "server.rb", "script.rb"})
	if fileSet["gemfile"] {
		if rubyEntry != "" {
			return "bundle install && ruby " + rubyEntry
		}
	}
	if rubyEntry != "" {
		return "ruby " + rubyEntry
	}

	// ── 14. Compiled languages (single-file) ─────────────────────────────────
	rsEntry := findRelPath([]string{"main.rs"})
	if rsEntry != "" {
		dir := filepath.Dir(rsEntry)
		baseNoExt := strings.TrimSuffix(filepath.Base(rsEntry), filepath.Ext(rsEntry))
		outBin := filepath.ToSlash(filepath.Join(dir, baseNoExt))
		if runtime.GOOS == "windows" {
			outBinWin := filepath.FromSlash(outBin) + ".exe"
			return fmt.Sprintf(`rustc %s -o %s && .\%s`, rsEntry, outBinWin, outBinWin)
		}
		return fmt.Sprintf("rustc %s -o %s && ./%s", rsEntry, outBin, outBin)
	}
	cEntry := findRelPath([]string{"main.c"})
	if cEntry != "" {
		dir := filepath.Dir(cEntry)
		baseNoExt := strings.TrimSuffix(filepath.Base(cEntry), filepath.Ext(cEntry))
		outBin := filepath.ToSlash(filepath.Join(dir, baseNoExt))
		if runtime.GOOS == "windows" {
			outBinWin := filepath.FromSlash(outBin) + ".exe"
			return fmt.Sprintf(`gcc -std=c17 -Wall %s -o %s && .\%s`, cEntry, outBinWin, outBinWin)
		}
		return fmt.Sprintf("gcc -std=c17 -Wall %s -o %s && ./%s", cEntry, outBin, outBin)
	}
	cppEntry := findRelPath([]string{"main.cpp"})
	if cppEntry != "" {
		dir := filepath.Dir(cppEntry)
		baseNoExt := strings.TrimSuffix(filepath.Base(cppEntry), filepath.Ext(cppEntry))
		outBin := filepath.ToSlash(filepath.Join(dir, baseNoExt))
		if runtime.GOOS == "windows" {
			outBinWin := filepath.FromSlash(outBin) + ".exe"
			return fmt.Sprintf(`g++ -std=c++17 -Wall %s -o %s && .\%s`, cppEntry, outBinWin, outBinWin)
		}
		return fmt.Sprintf("g++ -std=c++17 -Wall %s -o %s && ./%s", cppEntry, outBin, outBin)
	}

	// ── 15. Shell scripts ─────────────────────────────────────────────────────
	shEntry := findRelPath([]string{"run.sh", "start.sh", "script.sh"})
	if shEntry != "" {
		return "bash " + shEntry
	}

	// ── 16. Fallback: first file whose language we can run ────────────────────
	for _, f := range files {
		rel := filepath.ToSlash(f.RelPath)
		ext := strings.ToLower(filepath.Ext(rel))
		switch ext {
		case ".py":
			return "python " + rel
		case ".go":
			return "go run " + rel
		case ".js":
			return "node " + rel
		case ".ts":
			return "npx ts-node " + rel
		case ".rb":
			return "ruby " + rel
		case ".html":
			if runtime.GOOS == "windows" {
				return "start " + rel
			}
			return "open " + rel
		case ".sh":
			return "bash " + rel
		}
		base := filepath.Base(f.RelPath)
		switch strings.ToLower(f.Language) {
		case "python", "python3", "py":
			return "python " + rel
		case "go", "golang":
			return "go run " + rel
		case "javascript", "js", "node":
			return "node " + rel
		case "typescript", "ts":
			return "npx ts-node " + rel
		case "html":
			if runtime.GOOS == "windows" {
				return "start " + rel
			}
			return "open " + rel
		case "ruby", "rb":
			return "ruby " + rel
		case "bash", "sh", "shell":
			return "bash " + base
		}
	}
	return ""
}


// IsLauncherCmd reports whether a command simply opens a file/app and returns
// immediately (e.g. `start index.html`). These produce no captured output.
func IsLauncherCmd(cmd string) bool {
	low := strings.ToLower(strings.TrimSpace(cmd))
	return strings.HasPrefix(low, "start ") ||
		strings.HasPrefix(low, "open ") ||
		strings.HasPrefix(low, "xdg-open ")
}

// CommandResult holds the captured output of a shell command.
type CommandResult struct {
	Command  string
	Output   string
	ExitCode int
	Duration time.Duration
	Error    error
}

// RunCommandCaptured executes an arbitrary shell command string with captured
// stdout+stderr. It does NOT open a new window.
//
// On Windows we use cmd.exe /C (not PowerShell) because the Windows built-in
// PowerShell 5.1 does not support the && operator, causing compound commands
// such as "npm install && npm start" to fail silently.
func RunCommandCaptured(command string, workDir string, timeout time.Duration) CommandResult {
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	start := time.Now()
	res := CommandResult{Command: command}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// cmd.exe /C natively supports && chaining and is always available.
		cmdExe := os.Getenv("COMSPEC")
		if cmdExe == "" {
			cmdExe = `C:\Windows\System32\cmd.exe`
		}
		cmd = exec.Command(cmdExe, "/C", command)
	} else {
		cmd = exec.Command("bash", "-c", command)
	}
	if workDir != "" {
		cmd.Dir = workDir
	}
	cmd.Env = os.Environ()
	setSysProcAttr(cmd)

	out, err := runWithTimeout(cmd, timeout)
	res.Output = out
	res.Duration = time.Since(start)
	if err != nil {
		res.Error = err
		// errors.As correctly unwraps through any fmt.Errorf %w chains.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			res.ExitCode = exitErr.ExitCode()
		} else {
			res.ExitCode = 1
		}
	}
	return res
}

// CodeBlock holds extracted code and its detected language
type CodeBlock struct {
	Language string
	Code     string
}

// codeBlockRe matches ```lang\ncode\n``` fences in markdown
var codeBlockRe = regexp.MustCompile("(?s)```([a-zA-Z0-9+#]*)\\n(.*?)```")

// ExtractCodeBlocks returns all code blocks found in markdown text
func ExtractCodeBlocks(markdown string) []CodeBlock {
	matches := codeBlockRe.FindAllStringSubmatch(markdown, -1)
	blocks := make([]CodeBlock, 0, len(matches))
	for _, m := range matches {
		lang := strings.ToLower(strings.TrimSpace(m[1]))
		code := strings.TrimSpace(m[2])
		if code == "" {
			continue
		}
		blocks = append(blocks, CodeBlock{Language: lang, Code: code})
	}
	return blocks
}

// ExtractLastCodeBlock returns the last runnable code block in markdown
func ExtractLastCodeBlock(markdown string) (*CodeBlock, bool) {
	blocks := ExtractCodeBlocks(markdown)
	for i := len(blocks) - 1; i >= 0; i-- {
		b := blocks[i]
		if isRunnable(b.Language) {
			return &b, true
		}
	}
	if len(blocks) > 0 {
		last := blocks[len(blocks)-1]
		return &last, true
	}
	return nil, false
}

// isRunnable returns true if we know how to execute this language
func isRunnable(lang string) bool {
	switch lang {
	case "python", "python3", "py",
		"go", "golang",
		"javascript", "js", "node",
		"typescript", "ts",
		"bash", "sh", "shell",
		"ruby", "rb",
		"rust", "rs",
		"c", "cpp", "c++",
		"java",
		"csharp", "cs", "c#",
		"kotlin", "kt",
		"php",
		"lua",
		"dart":
		return true
	}
	return false
}


// RunResult holds the output of a code execution
type RunResult struct {
	Language string
	Output   string
	ExitCode int
	Duration time.Duration
	Error    error
}

// ── Interpreter resolution ────────────────────────────────────────────────────

// lookAny returns the first binary name that resolves via exec.LookPath.
// On failure it returns ("", false).
func lookAny(candidates ...string) (string, bool) {
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			return p, true
		}
	}
	return "", false
}

// resolvePython returns the absolute path to a Python interpreter or an error.
func resolvePython() (string, error) {
	// Try in priority order:
	//   py      – Windows Python Launcher (installed at C:\Windows\py.exe, always in PATH)
	//   python  – standard name on Windows
	//   python3 – standard name on Linux/macOS
	candidates := []string{"py", "python", "python3"}

	// Also try common Windows AppData install paths
	if runtime.GOOS == "windows" {
		home, _ := os.UserHomeDir()
		for _, ver := range []string{"Python313", "Python312", "Python311", "Python310", "Python39", "Python38"} {
			candidates = append(candidates,
				filepath.Join(home, "AppData", "Local", "Programs", "Python", ver, "python.exe"),
				filepath.Join(home, "AppData", "Local", "Microsoft", "WindowsApps", "python.exe"),
				`C:\`+ver+`\python.exe`,
			)
		}
	}

	if bin, ok := lookAny(candidates...); ok {
		return bin, nil
	}
	return "", fmt.Errorf(
		"Python interpreter not found.\n" +
			"Install Python from https://www.python.org/downloads/ and make sure\n" +
			"it is added to your PATH (check 'Add Python to PATH' during install).")
}

// resolveNode returns the absolute path to node or an error.
func resolveNode() (string, error) {
	if bin, ok := lookAny("node", "node.exe"); ok {
		return bin, nil
	}
	return "", fmt.Errorf(
		"Node.js not found.\n" +
			"Install Node.js from https://nodejs.org/ and ensure it is in your PATH.")
}

// resolveGo returns the absolute path to the go binary.
func resolveGo() (string, error) {
	candidates := []string{"go"}
	if runtime.GOOS == "windows" {
		candidates = append(candidates,
			`C:\Program Files\Go\bin\go.exe`,
			`C:\Go\bin\go.exe`,
		)
	}
	if bin, ok := lookAny(candidates...); ok {
		return bin, nil
	}
	return "", fmt.Errorf("Go toolchain not found. Install from https://go.dev/dl/")
}

// ── Execution helpers ─────────────────────────────────────────────────────────

// makeCmd creates an exec.Cmd with proper environment and working directory so
// that scripts can reference files relative to their location.
// It also applies platform-specific process attributes (CREATE_NO_WINDOW on
// Windows) so the subprocess never touches the Bubble Tea console.
func makeCmd(tmpDir string, name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Dir = tmpDir
	cmd.Stdin = nil
	cmd.Env = os.Environ() // inherit full PATH / environment
	setSysProcAttr(cmd)
	return cmd
}

// makeInteractiveCmd creates a command that inherits the real terminal
// (stdin / stdout / stderr) from the parent process.
// Unlike makeCmd it does NOT apply CREATE_NO_WINDOW so the subprocess can
// accept keyboard input when Bubble Tea hands over the terminal via ExecProcess.
func makeInteractiveCmd(tmpDir, name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Dir = tmpDir
	cmd.Env = os.Environ()
	// stdin/stdout/stderr are left nil so tea.ExecProcess connects the real terminal
	return cmd
}

// compileAndRun compiles src with the given compiler args, then runs the
// resulting binary. Returns early with an error if compilation fails.
func compileAndRun(tmpDir, src, bin string, compilerArgs []string, start time.Time, timeout time.Duration) RunResult {
	result := RunResult{}

	// Build compiler command
	compileCmd := makeCmd(tmpDir, compilerArgs[0], append(compilerArgs[1:], src, "-o", bin)...)
	compileOut, err := compileCmd.CombinedOutput()
	if err != nil {
		result.Output = string(compileOut)
		result.Error = fmt.Errorf("compile error (%s): %w", compilerArgs[0], err)
		result.Duration = time.Since(start)
		return result
	}

	// Run the compiled binary
	runCmd := makeCmd(tmpDir, bin)
	out, runErr := runWithTimeout(runCmd, timeout)
	result.Output = out
	result.Duration = time.Since(start)
	if runErr != nil {
		result.Error = runErr
	}
	return result
}

// ── Main entry point ─────────────────────────────────────────────────────────

// RunCode writes code to a temp file and executes it based on language.
// It automatically strips any leading // file: annotation from the code.
// timeout: max execution time (0 = 30s default)
func RunCode(block *CodeBlock, timeout time.Duration) RunResult {
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	lang := strings.ToLower(block.Language)
	result := RunResult{Language: lang}

	// Strip any leading file-path annotation so it doesn't cause syntax errors
	cleanedCode := StripFileAnnotation(block.Code)
	cleanBlock := &CodeBlock{Language: block.Language, Code: cleanedCode}
	_ = cleanBlock // used below in place of block

	// Create a temp directory for the execution
	tmpDir, err := os.MkdirTemp("", "ollama-cli-run-*")
	if err != nil {
		result.Error = fmt.Errorf("cannot create temp dir: %w", err)
		return result
	}
	defer os.RemoveAll(tmpDir)

	start := time.Now()

	switch lang {

	// ── Python ───────────────────────────────────────────────────────────────
	case "python", "python3", "py":
		pyBin, err := resolvePython()
		if err != nil {
			result.Error = err
			return result
		}
		f := filepath.Join(tmpDir, "script.py")
		if err := os.WriteFile(f, []byte(cleanedCode), 0644); err != nil {
			result.Error = fmt.Errorf("failed to write script: %w", err)
			return result
		}
		// -u: unbuffered output so partial prints appear immediately in capture.
		cmd := makeCmd(tmpDir, pyBin, "-u", f)
		out, runErr := runWithTimeout(cmd, timeout)
		result.Output = out
		result.Duration = time.Since(start)
		result.Error = runErr

	// ── Go ────────────────────────────────────────────────────────────────────
	case "go", "golang":
		goBin, err := resolveGo()
		if err != nil {
			result.Error = err
			return result
		}
		code := cleanedCode
		if !strings.Contains(code, "package ") {
			// Snippet without package: wrap it, pulling in fmt if not already imported.
			imps := "\"fmt\""
			if strings.Contains(code, "fmt.") {
				imps = "\"fmt\""
			}
			code = "package main\n\nimport " + imps + "\n\nfunc main() {\n" + indentLines(code, "\t") + "\n}"
		}
		f := filepath.Join(tmpDir, "main.go")
		if err := os.WriteFile(f, []byte(code), 0644); err != nil {
			result.Error = fmt.Errorf("failed to write script: %w", err)
			return result
		}
		cmd := makeCmd(tmpDir, goBin, "run", f)
		out, runErr := runWithTimeout(cmd, timeout)
		result.Output = out
		result.Duration = time.Since(start)
		result.Error = runErr

	// ── JavaScript / Node ─────────────────────────────────────────────────────
	case "javascript", "js", "node":
		nodeBin, err := resolveNode()
		if err != nil {
			result.Error = err
			return result
		}
		f := filepath.Join(tmpDir, "script.js")
		if err := os.WriteFile(f, []byte(cleanedCode), 0644); err != nil {
			result.Error = fmt.Errorf("failed to write script: %w", err)
			return result
		}
		cmd := makeCmd(tmpDir, nodeBin, f)
		out, runErr := runWithTimeout(cmd, timeout)
		result.Output = out
		result.Duration = time.Since(start)
		result.Error = runErr

	// ── TypeScript ────────────────────────────────────────────────────────────
	case "typescript", "ts":
		f := filepath.Join(tmpDir, "script.ts")
		if err := os.WriteFile(f, []byte(cleanedCode), 0644); err != nil {
			result.Error = fmt.Errorf("failed to write script: %w", err)
			return result
		}
		// Prefer deno (zero-config TS runner), fall back to ts-node via npx.
		if deno, ok := lookAny("deno", "deno.exe"); ok {
			cmd := makeCmd(tmpDir, deno, "run", "--allow-all", f)
			out, runErr := runWithTimeout(cmd, timeout)
			result.Output = out
			result.Duration = time.Since(start)
			result.Error = runErr
			break
		}
		if _, ok := lookAny("npx", "npx.cmd"); !ok {
			result.Error = fmt.Errorf("neither deno nor npx found — install Deno (https://deno.land/) or Node.js (https://nodejs.org/)")
			return result
		}
		npx, _ := lookAny("npx", "npx.cmd")
		cmd := makeCmd(tmpDir, npx, "--yes", "ts-node", f)
		out, runErr := runWithTimeout(cmd, timeout)
		result.Output = out
		result.Duration = time.Since(start)
		result.Error = runErr

	// ── Bash / Shell ──────────────────────────────────────────────────────────
	case "bash", "sh", "shell":
		if runtime.GOOS == "windows" {
			// Use cmd.exe with a .bat file; cmd.exe is always available on Windows
			f := filepath.Join(tmpDir, "script.bat")
			bat := "@echo off\r\n" + cleanedCode
			if err := os.WriteFile(f, []byte(bat), 0644); err != nil {
				result.Error = fmt.Errorf("failed to write script: %w", err)
				return result
			}
			// Use the full path to cmd.exe to avoid PATH lookup issues
			cmdExe := os.Getenv("COMSPEC")
			if cmdExe == "" {
				cmdExe = `C:\Windows\System32\cmd.exe`
			}
			cmd := makeCmd(tmpDir, cmdExe, "/C", f)
			out, runErr := runWithTimeout(cmd, timeout)
			result.Output = out
			result.Duration = time.Since(start)
			result.Error = runErr
		} else {
			bash, ok := lookAny("bash", "sh")
			if !ok {
				result.Error = fmt.Errorf("bash/sh not found in PATH")
				return result
			}
			f := filepath.Join(tmpDir, "script.sh")
			if err := os.WriteFile(f, []byte("#!/bin/bash\n"+cleanedCode), 0755); err != nil {
				result.Error = fmt.Errorf("failed to write script: %w", err)
				return result
			}
			cmd := makeCmd(tmpDir, bash, f)
			out, runErr := runWithTimeout(cmd, timeout)
			result.Output = out
			result.Duration = time.Since(start)
			result.Error = runErr
		}

	// ── Ruby ──────────────────────────────────────────────────────────────────
	case "ruby", "rb":
		rubyBin, ok := lookAny("ruby", "ruby.exe")
		if !ok {
			result.Error = fmt.Errorf("Ruby not found. Install from https://www.ruby-lang.org/")
			return result
		}
		f := filepath.Join(tmpDir, "script.rb")
		if err := os.WriteFile(f, []byte(cleanedCode), 0644); err != nil {
			result.Error = fmt.Errorf("failed to write script: %w", err)
			return result
		}
		cmd := makeCmd(tmpDir, rubyBin, f)
		out, runErr := runWithTimeout(cmd, timeout)
		result.Output = out
		result.Duration = time.Since(start)
		result.Error = runErr

	// ── Rust ──────────────────────────────────────────────────────────────────
	case "rust", "rs":
		rustc, ok := lookAny("rustc", "rustc.exe")
		if !ok {
			result.Error = fmt.Errorf("rustc not found. Install Rust from https://rustup.rs/")
			return result
		}
		src := filepath.Join(tmpDir, "main.rs")
		bin := filepath.Join(tmpDir, "main")
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		if err := os.WriteFile(src, []byte(cleanedCode), 0644); err != nil {
			result.Error = fmt.Errorf("failed to write script: %w", err)
			return result
		}
		r := compileAndRun(tmpDir, src, bin, []string{rustc}, start, timeout)
		r.Language = lang
		return r

	// ── C ─────────────────────────────────────────────────────────────────────
	case "c":
		gcc, ok := lookAny("gcc", "gcc.exe", "cc", "clang")
		if !ok {
			result.Error = fmt.Errorf("gcc/clang not found. Install a C compiler (e.g. MinGW on Windows, Xcode CLT on macOS, build-essential on Ubuntu).")
			return result
		}
		src := filepath.Join(tmpDir, "main.c")
		bin := filepath.Join(tmpDir, "main")
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		if err := os.WriteFile(src, []byte(cleanedCode), 0644); err != nil {
			result.Error = fmt.Errorf("failed to write script: %w", err)
			return result
		}
		r := compileAndRun(tmpDir, src, bin, []string{gcc, "-std=c17", "-Wall"}, start, timeout)
		r.Language = lang
		return r

	// ── C++ ───────────────────────────────────────────────────────────────────
	case "cpp", "c++":
		gpp, ok := lookAny("g++", "g++.exe", "c++", "clang++")
		if !ok {
			result.Error = fmt.Errorf("g++/clang++ not found. Install a C++ compiler (e.g. MinGW on Windows, Xcode CLT on macOS, build-essential on Ubuntu).")
			return result
		}
		src := filepath.Join(tmpDir, "main.cpp")
		bin := filepath.Join(tmpDir, "main")
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		if err := os.WriteFile(src, []byte(cleanedCode), 0644); err != nil {
			result.Error = fmt.Errorf("failed to write script: %w", err)
			return result
		}
		r := compileAndRun(tmpDir, src, bin, []string{gpp, "-std=c++17", "-Wall"}, start, timeout)
		r.Language = lang
		return r

	// ── PHP ───────────────────────────────────────────────────────────────────
	case "php":
		php, ok := lookAny("php", "php.exe")
		if !ok {
			result.Error = fmt.Errorf("PHP not found. Install from https://www.php.net/downloads")
			return result
		}
		f := filepath.Join(tmpDir, "script.php")
		if err := os.WriteFile(f, []byte(cleanedCode), 0644); err != nil {
			result.Error = fmt.Errorf("failed to write script: %w", err)
			return result
		}
		cmd := makeCmd(tmpDir, php, f)
		out, runErr := runWithTimeout(cmd, timeout)
		result.Output = out
		result.Duration = time.Since(start)
		result.Error = runErr

	// ── Lua ───────────────────────────────────────────────────────────────────
	case "lua":
		lua, ok := lookAny("lua", "lua5.4", "lua5.3", "lua.exe")
		if !ok {
			result.Error = fmt.Errorf("Lua not found. Install from https://www.lua.org/download.html")
			return result
		}
		f := filepath.Join(tmpDir, "script.lua")
		if err := os.WriteFile(f, []byte(cleanedCode), 0644); err != nil {
			result.Error = fmt.Errorf("failed to write script: %w", err)
			return result
		}
		cmd := makeCmd(tmpDir, lua, f)
		out, runErr := runWithTimeout(cmd, timeout)
		result.Output = out
		result.Duration = time.Since(start)
		result.Error = runErr

	// ── Dart ──────────────────────────────────────────────────────────────────
	case "dart":
		dart, ok := lookAny("dart", "dart.exe")
		if !ok {
			result.Error = fmt.Errorf("Dart SDK not found. Install from https://dart.dev/get-dart")
			return result
		}
		f := filepath.Join(tmpDir, "script.dart")
		if err := os.WriteFile(f, []byte(cleanedCode), 0644); err != nil {
			result.Error = fmt.Errorf("failed to write script: %w", err)
			return result
		}
		cmd := makeCmd(tmpDir, dart, "run", f)
		out, runErr := runWithTimeout(cmd, timeout)
		result.Output = out
		result.Duration = time.Since(start)
		result.Error = runErr

	default:
		result.Error = fmt.Errorf("unsupported language %q — supported: python, go, js, ts, bash, ruby, rust, c, c++, php, lua, dart", lang)
		return result
	}

	return result
}

// ── Interactive (stdin-connected) execution ───────────────────────────────────

// InteractiveRun holds a prepared command and its temp-dir cleanup function.
// Use with tea.ExecProcess so Bubble Tea hands the real terminal to the process.
type InteractiveRun struct {
	Cmd     *exec.Cmd
	Cleanup func()
}

// PrepareInteractiveRun writes the code to a temp file and returns an
// InteractiveRun whose Cmd has stdin/stdout/stderr left unset so that
// tea.ExecProcess can connect the real terminal to it.
// The caller must invoke Cleanup() after the process exits.
func PrepareInteractiveRun(block *CodeBlock) (*InteractiveRun, error) {
	lang := strings.ToLower(block.Language)

	tmpDir, err := os.MkdirTemp("", "ollama-cli-interactive-*")
	if err != nil {
		return nil, fmt.Errorf("cannot create temp dir: %w", err)
	}
	cleanup := func() { os.RemoveAll(tmpDir) }

	var cmd *exec.Cmd

	switch lang {

	case "python", "python3", "py":
		pyBin, err := resolvePython()
		if err != nil {
			cleanup()
			return nil, err
		}
		f := filepath.Join(tmpDir, "script.py")
		if err := os.WriteFile(f, []byte(block.Code), 0644); err != nil {
			cleanup()
			return nil, fmt.Errorf("failed to write script: %w", err)
		}
		cmd = makeInteractiveCmd(tmpDir, pyBin, f)

	case "go", "golang":
		goBin, err := resolveGo()
		if err != nil {
			cleanup()
			return nil, err
		}
		code := block.Code
		if !strings.Contains(code, "package main") {
			code = "package main\n\nimport \"fmt\"\n\nfunc main() {\n" + indentLines(code, "\t") + "\n}"
		}
		f := filepath.Join(tmpDir, "main.go")
		if err := os.WriteFile(f, []byte(code), 0644); err != nil {
			cleanup()
			return nil, fmt.Errorf("failed to write script: %w", err)
		}
		cmd = makeInteractiveCmd(tmpDir, goBin, "run", f)

	case "javascript", "js", "node":
		nodeBin, err := resolveNode()
		if err != nil {
			cleanup()
			return nil, err
		}
		f := filepath.Join(tmpDir, "script.js")
		if err := os.WriteFile(f, []byte(block.Code), 0644); err != nil {
			cleanup()
			return nil, fmt.Errorf("failed to write script: %w", err)
		}
		cmd = makeInteractiveCmd(tmpDir, nodeBin, f)

	case "typescript", "ts":
		npx, ok := lookAny("npx", "npx.cmd")
		if !ok {
			cleanup()
			return nil, fmt.Errorf("npx not found — install Node.js from https://nodejs.org/")
		}
		f := filepath.Join(tmpDir, "script.ts")
		if err := os.WriteFile(f, []byte(block.Code), 0644); err != nil {
			cleanup()
			return nil, fmt.Errorf("failed to write script: %w", err)
		}
		cmd = makeInteractiveCmd(tmpDir, npx, "--yes", "ts-node", f)

	case "bash", "sh", "shell":
		if runtime.GOOS == "windows" {
			f := filepath.Join(tmpDir, "script.bat")
			if err := os.WriteFile(f, []byte("@echo off\r\n"+block.Code), 0644); err != nil {
				cleanup()
				return nil, fmt.Errorf("failed to write script: %w", err)
			}
			cmdExe := os.Getenv("COMSPEC")
			if cmdExe == "" {
				cmdExe = `C:\Windows\System32\cmd.exe`
			}
			cmd = makeInteractiveCmd(tmpDir, cmdExe, "/C", f)
		} else {
			bash, ok := lookAny("bash", "sh")
			if !ok {
				cleanup()
				return nil, fmt.Errorf("bash/sh not found in PATH")
			}
			f := filepath.Join(tmpDir, "script.sh")
			if err := os.WriteFile(f, []byte("#!/bin/bash\n"+block.Code), 0755); err != nil {
				cleanup()
				return nil, fmt.Errorf("failed to write script: %w", err)
			}
			cmd = makeInteractiveCmd(tmpDir, bash, f)
		}

	case "ruby", "rb":
		rubyBin, ok := lookAny("ruby", "ruby.exe")
		if !ok {
			cleanup()
			return nil, fmt.Errorf("Ruby not found. Install from https://www.ruby-lang.org/")
		}
		f := filepath.Join(tmpDir, "script.rb")
		if err := os.WriteFile(f, []byte(block.Code), 0644); err != nil {
			cleanup()
			return nil, fmt.Errorf("failed to write script: %w", err)
		}
		cmd = makeInteractiveCmd(tmpDir, rubyBin, f)

	case "rust", "rs":
		rustc, ok := lookAny("rustc", "rustc.exe")
		if !ok {
			cleanup()
			return nil, fmt.Errorf("rustc not found. Install Rust from https://rustup.rs/")
		}
		src := filepath.Join(tmpDir, "main.rs")
		bin := filepath.Join(tmpDir, "main")
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		if err := os.WriteFile(src, []byte(block.Code), 0644); err != nil {
			cleanup()
			return nil, fmt.Errorf("failed to write script: %w", err)
		}
		// Compile first (non-interactive), then run interactively
		rustc2 := makeCmd(tmpDir, rustc, src, "-o", bin)
		if out, err := rustc2.CombinedOutput(); err != nil {
			cleanup()
			return nil, fmt.Errorf("compile error:\n%s", string(out))
		}
		cmd = makeInteractiveCmd(tmpDir, bin)

	case "c":
		gcc, ok := lookAny("gcc", "gcc.exe", "cc")
		if !ok {
			cleanup()
			return nil, fmt.Errorf("gcc not found. Install a C compiler.")
		}
		src := filepath.Join(tmpDir, "main.c")
		bin := filepath.Join(tmpDir, "main")
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		if err := os.WriteFile(src, []byte(block.Code), 0644); err != nil {
			cleanup()
			return nil, fmt.Errorf("failed to write script: %w", err)
		}
		gccCmd := makeCmd(tmpDir, gcc, src, "-o", bin)
		if out, err := gccCmd.CombinedOutput(); err != nil {
			cleanup()
			return nil, fmt.Errorf("compile error:\n%s", string(out))
		}
		cmd = makeInteractiveCmd(tmpDir, bin)

	case "cpp", "c++":
		gpp, ok := lookAny("g++", "g++.exe", "c++")
		if !ok {
			cleanup()
			return nil, fmt.Errorf("g++ not found. Install a C++ compiler.")
		}
		src := filepath.Join(tmpDir, "main.cpp")
		bin := filepath.Join(tmpDir, "main")
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		if err := os.WriteFile(src, []byte(block.Code), 0644); err != nil {
			cleanup()
			return nil, fmt.Errorf("failed to write script: %w", err)
		}
		gppCmd := makeCmd(tmpDir, gpp, src, "-o", bin)
		if out, err := gppCmd.CombinedOutput(); err != nil {
			cleanup()
			return nil, fmt.Errorf("compile error:\n%s", string(out))
		}
		cmd = makeInteractiveCmd(tmpDir, bin)

	default:
		cleanup()
		return nil, fmt.Errorf("unsupported language %q — supported: python, go, js, ts, bash, ruby, rust, c, c++", lang)
	}

	return &InteractiveRun{Cmd: cmd, Cleanup: cleanup}, nil
}

// ── Code saving ───────────────────────────────────────────────────────────────

// langExt maps a language identifier to a canonical file extension.
func langExt(lang string) string {
	switch lang {
	case "python", "python3", "py":
		return "py"
	case "go", "golang":
		return "go"
	case "javascript", "js", "node":
		return "js"
	case "typescript", "ts":
		return "ts"
	case "bash", "sh", "shell":
		return "sh"
	case "ruby", "rb":
		return "rb"
	case "rust", "rs":
		return "rs"
	case "c":
		return "c"
	case "cpp", "c++":
		return "cpp"
	default:
		return "txt"
	}
}

// SaveCodeBlock writes a code block to ~/ollama-cli-code/ with a
// timestamped filename and returns the absolute path on success.
func SaveCodeBlock(block *CodeBlock) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}

	saveDir := filepath.Join(home, "ollama-cli-code")
	if err := os.MkdirAll(saveDir, 0755); err != nil {
		return "", fmt.Errorf("cannot create save directory: %w", err)
	}

	lang := strings.ToLower(block.Language)
	ext := langExt(lang)
	ts := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s.%s", ts, lang, ext)
	dest := filepath.Join(saveDir, filename)

	code := block.Code
	// For Go snippets without a package declaration, wrap them properly
	if (lang == "go" || lang == "golang") && !strings.Contains(code, "package main") {
		code = "package main\n\nimport \"fmt\"\n\nfunc main() {\n" + indentLines(code, "\t") + "\n}"
	}
	// For bash on all platforms, prepend shebang
	if lang == "bash" || lang == "sh" || lang == "shell" {
		if !strings.HasPrefix(code, "#!") {
			code = "#!/bin/bash\n" + code
		}
	}

	if err := os.WriteFile(dest, []byte(code), 0644); err != nil {
		return "", fmt.Errorf("failed to save code: %w", err)
	}
	return dest, nil
}

// OpenInNewTerminal runs the code block in a fresh OS terminal window that
// the user can interact with. The window shows a "Press any key to close..."
// banner after the program exits, so the output is never lost.
func OpenInNewTerminal(block *CodeBlock) error {
	lang := strings.ToLower(block.Language)

	// Scratch directory for wrapper scripts / compiled binaries.
	// Cleanup is handled by the wrapper script itself after the user dismisses.
	tmpDir, err := os.MkdirTemp("", "ollama-cli-term-*")
	if err != nil {
		return fmt.Errorf("cannot create temp dir: %w", err)
	}

	if err := openInNewTerminalImpl(lang, block.Code, tmpDir); err != nil {
		os.RemoveAll(tmpDir)
		return err
	}
	return nil
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// runWithTimeout runs a command and kills it after the given timeout.
// cmd.Stdin, cmd.Env, and platform-specific process attributes must already
// be set by the caller (makeCmd does this).
//
// Output and error are returned SEPARATELY so callers can display each
// independently without duplication. Do NOT embed output inside the error.
func runWithTimeout(cmd *exec.Cmd, timeout time.Duration) (string, error) {
	done := make(chan struct{})
	var out []byte
	var runErr error

	go func() {
		out, runErr = cmd.CombinedOutput()
		close(done)
	}()

	select {
	case <-done:
		// Return output and error separately — callers read both fields.
		return string(out), runErr
	case <-time.After(timeout):
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		<-done // wait for the goroutine so we capture any partial output
		return string(out), fmt.Errorf("timed out after %s", timeout)
	}
}

func indentLines(code, prefix string) string {
	lines := strings.Split(code, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n")
}


