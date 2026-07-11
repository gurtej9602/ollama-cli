//go:build !windows

package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// setSysProcAttr is a no-op on non-Windows platforms.
func setSysProcAttr(cmd *exec.Cmd) {}

// openInNewTerminalImpl opens a new terminal emulator window on Linux/macOS,
// runs the code inside it, and waits for a key-press before closing.
func openInNewTerminalImpl(lang, code, tmpDir string) error {
	// Build the shell command that runs the code.
	var runCmd string

	switch lang {
	case "python", "python3", "py":
		pyBin, err := resolvePython()
		if err != nil {
			return err
		}
		src := filepath.Join(tmpDir, "script.py")
		if err := os.WriteFile(src, []byte(code), 0644); err != nil {
			return fmt.Errorf("failed to write script: %w", err)
		}
		runCmd = fmt.Sprintf(`"%s" "%s"`, pyBin, src)

	case "go", "golang":
		goBin, err := resolveGo()
		if err != nil {
			return err
		}
		goCode := code
		if !strings.Contains(goCode, "package main") {
			goCode = "package main\n\nimport \"fmt\"\n\nfunc main() {\n" + indentLines(goCode, "\t") + "\n}"
		}
		src := filepath.Join(tmpDir, "main.go")
		if err := os.WriteFile(src, []byte(goCode), 0644); err != nil {
			return fmt.Errorf("failed to write script: %w", err)
		}
		runCmd = fmt.Sprintf(`"%s" run "%s"`, goBin, src)

	case "javascript", "js", "node":
		nodeBin, err := resolveNode()
		if err != nil {
			return err
		}
		src := filepath.Join(tmpDir, "script.js")
		if err := os.WriteFile(src, []byte(code), 0644); err != nil {
			return fmt.Errorf("failed to write script: %w", err)
		}
		runCmd = fmt.Sprintf(`"%s" "%s"`, nodeBin, src)

	case "typescript", "ts":
		npx, ok := lookAny("npx")
		if !ok {
			return fmt.Errorf("npx not found — install Node.js from https://nodejs.org/")
		}
		src := filepath.Join(tmpDir, "script.ts")
		if err := os.WriteFile(src, []byte(code), 0644); err != nil {
			return fmt.Errorf("failed to write script: %w", err)
		}
		runCmd = fmt.Sprintf(`"%s" --yes ts-node "%s"`, npx, src)

	case "bash", "sh", "shell":
		bash, ok := lookAny("bash", "sh")
		if !ok {
			return fmt.Errorf("bash/sh not found in PATH")
		}
		src := filepath.Join(tmpDir, "script.sh")
		if err := os.WriteFile(src, []byte("#!/bin/bash\n"+code), 0755); err != nil {
			return fmt.Errorf("failed to write script: %w", err)
		}
		runCmd = fmt.Sprintf(`"%s" "%s"`, bash, src)

	case "ruby", "rb":
		rubyBin, ok := lookAny("ruby")
		if !ok {
			return fmt.Errorf("Ruby not found. Install from https://www.ruby-lang.org/")
		}
		src := filepath.Join(tmpDir, "script.rb")
		if err := os.WriteFile(src, []byte(code), 0644); err != nil {
			return fmt.Errorf("failed to write script: %w", err)
		}
		runCmd = fmt.Sprintf(`"%s" "%s"`, rubyBin, src)

	case "rust", "rs":
		rustc, ok := lookAny("rustc")
		if !ok {
			return fmt.Errorf("rustc not found. Install Rust from https://rustup.rs/")
		}
		src := filepath.Join(tmpDir, "main.rs")
		bin := filepath.Join(tmpDir, "main")
		if err := os.WriteFile(src, []byte(code), 0644); err != nil {
			return fmt.Errorf("failed to write script: %w", err)
		}
		runCmd = fmt.Sprintf(`"%s" "%s" -o "%s" && "%s"`, rustc, src, bin, bin)

	case "c":
		gcc, ok := lookAny("gcc", "cc")
		if !ok {
			return fmt.Errorf("gcc not found. Install build-essential or Xcode CLT.")
		}
		src := filepath.Join(tmpDir, "main.c")
		bin := filepath.Join(tmpDir, "main")
		if err := os.WriteFile(src, []byte(code), 0644); err != nil {
			return fmt.Errorf("failed to write script: %w", err)
		}
		runCmd = fmt.Sprintf(`"%s" "%s" -o "%s" && "%s"`, gcc, src, bin, bin)

	case "cpp", "c++":
		gpp, ok := lookAny("g++", "c++")
		if !ok {
			return fmt.Errorf("g++ not found. Install build-essential or Xcode CLT.")
		}
		src := filepath.Join(tmpDir, "main.cpp")
		bin := filepath.Join(tmpDir, "main")
		if err := os.WriteFile(src, []byte(code), 0644); err != nil {
			return fmt.Errorf("failed to write script: %w", err)
		}
		runCmd = fmt.Sprintf(`"%s" "%s" -o "%s" && "%s"`, gpp, src, bin, bin)

	default:
		return fmt.Errorf("unsupported language %q", lang)
	}

	// Build a shell wrapper that runs the program then waits for a key-press.
	wrapperPath := filepath.Join(tmpDir, "run.sh")
	wrapper := fmt.Sprintf(`#!/bin/bash
echo ""
echo " === Ollama CLI: running %s ==="
echo ""
%s
echo ""
echo "===================================================="
echo "  Program finished.  Press Enter to close..."
echo "===================================================="
read -r _
rm -rf "%s"
`, lang, runCmd, tmpDir)

	if err := os.WriteFile(wrapperPath, []byte(wrapper), 0755); err != nil {
		return fmt.Errorf("failed to write wrapper script: %w", err)
	}

	// Try common terminal emulators in priority order.
	type termSpec struct {
		bin  string
		args []string
	}
	candidates := []termSpec{
		{"gnome-terminal", []string{"--", "bash", wrapperPath}},
		{"xterm", []string{"-title", fmt.Sprintf("Ollama CLI - %s", lang), "-e", "bash", wrapperPath}},
		{"konsole", []string{"-e", "bash", wrapperPath}},
		{"xfce4-terminal", []string{"--command=bash " + wrapperPath}},
		{"lxterminal", []string{"-e", "bash " + wrapperPath}},
		{"open", []string{"-a", "Terminal", wrapperPath}}, // macOS
	}

	for _, t := range candidates {
		if _, ok := lookAny(t.bin); ok {
			cmd := exec.Command(t.bin, t.args...)
			if err := cmd.Start(); err == nil {
				return nil
			}
		}
	}

	return fmt.Errorf("no supported terminal emulator found (tried gnome-terminal, xterm, konsole, xfce4-terminal, lxterminal, Terminal.app)")
}

// OpenTerminalInDir opens a terminal emulator window in the given directory.
func OpenTerminalInDir(dir string) error {
	type termSpec struct {
		bin  string
		args []string
	}
	candidates := []termSpec{
		{"gnome-terminal", []string{"--working-directory=" + dir}},
		{"konsole", []string{"--workdir", dir}},
		{"xfce4-terminal", []string{"--working-directory=" + dir}},
		{"lxterminal", []string{"--working-directory=" + dir}},
		{"xterm", []string{"-e", fmt.Sprintf("bash -c 'cd %q; exec bash'", dir)}},
		{"open", []string{"-a", "Terminal", dir}}, // macOS
	}
	for _, t := range candidates {
		if _, ok := lookAny(t.bin); ok {
			cmd := exec.Command(t.bin, t.args...)
			if err := cmd.Start(); err == nil {
				return nil
			}
		}
	}
	return fmt.Errorf("no supported terminal emulator found")
}

// RunInVisibleTerminalCaptured opens a terminal window and runs command
// interactively with live output. The window stays open until the user presses
// Enter. Returns captured stdout+stderr and exit code for LLM feedback.
func RunInVisibleTerminalCaptured(command, workDir string, timeout time.Duration) CommandResult {
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	start := time.Now()
	res := CommandResult{Command: command}

	tmpDir, err := os.MkdirTemp("", "ollama-cli-vis-*")
	if err != nil {
		res.Error = fmt.Errorf("cannot create temp dir: %w", err)
		res.ExitCode = 1
		return res
	}

	outFile  := filepath.Join(tmpDir, "out.txt")
	exitFile := filepath.Join(tmpDir, "exit.txt")

	cdCmd := ""
	if workDir != "" {
		cdCmd = fmt.Sprintf("cd %q || { echo 'Cannot cd to directory'; exit 1; }\n", workDir)
	}

	// Bash wrapper: runs the command piped through `tee` so the user sees output
	// live AND it gets written to a file. Exit code is saved separately.
	wrapper := fmt.Sprintf(`#!/bin/bash
echo ''
echo '  ==== Ollama CLI ===='
echo '  $ %s'
echo ''

%s
( %s ) 2>&1 | tee '%s'
EXIT_CODE=${PIPESTATUS[0]}
echo "$EXIT_CODE" > '%s'

echo ''
if [ "$EXIT_CODE" -eq 0 ]; then
    echo '============================================'
    echo "  Completed successfully  (exit $EXIT_CODE)."
else
    echo '============================================'
    echo "  Command failed  (exit $EXIT_CODE)."
fi
echo '  Press Enter to close this window...'
echo '============================================'
read -r _
rm -rf '%s'
`, command, cdCmd, command, outFile, exitFile, tmpDir)

	wrapperPath := filepath.Join(tmpDir, "run.sh")
	if err := os.WriteFile(wrapperPath, []byte(wrapper), 0755); err != nil {
		os.RemoveAll(tmpDir)
		res.Error = fmt.Errorf("failed to write wrapper: %w", err)
		res.ExitCode = 1
		return res
	}

	// Try terminal emulators in priority order.
	type termSpec struct {
		bin  string
		args []string
	}
	candidates := []termSpec{
		{"gnome-terminal", []string{"--", "bash", wrapperPath}},
		{"konsole", []string{"-e", "bash", wrapperPath}},
		{"xfce4-terminal", []string{"--command=bash " + wrapperPath}},
		{"lxterminal", []string{"-e", "bash " + wrapperPath}},
		{"xterm", []string{"-e", "bash", wrapperPath}},
		{"open", []string{"-a", "Terminal", wrapperPath}}, // macOS
	}

	var termCmd *exec.Cmd
	for _, t := range candidates {
		if _, ok := lookAny(t.bin); ok {
			termCmd = exec.Command(t.bin, t.args...)
			break
		}
	}

	if termCmd == nil {
		// No terminal emulator found — fall back to silent capture.
		return RunCommandCaptured(command, workDir, timeout)
	}

	if err := termCmd.Start(); err != nil {
		os.RemoveAll(tmpDir)
		return RunCommandCaptured(command, workDir, timeout)
	}

	done := make(chan error, 1)
	go func() { done <- termCmd.Wait() }()

	select {
	case <-done:
	case <-time.After(timeout):
		termCmd.Process.Kill()
		<-done
		os.RemoveAll(tmpDir)
		res.Duration = time.Since(start)
		res.Error = fmt.Errorf("timed out after %s", timeout)
		res.ExitCode = 1
		return res
	}

	res.Duration = time.Since(start)

	if data, err := os.ReadFile(outFile); err == nil {
		res.Output = string(data)
	}
	if data, err := os.ReadFile(exitFile); err == nil {
		var code int
		if _, err := fmt.Sscan(strings.TrimSpace(string(data)), &code); err == nil {
			res.ExitCode = code
		}
	}
	if res.ExitCode != 0 && res.Error == nil {
		res.Error = fmt.Errorf("command exited with code %d", res.ExitCode)
	}

	os.RemoveAll(tmpDir) // clean up if the bash script didn't already
	return res
}

