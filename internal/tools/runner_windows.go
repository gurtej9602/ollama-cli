//go:build windows

package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// setSysProcAttr sets Windows-specific process attributes so the subprocess
// does NOT inherit the Bubble Tea console and does NOT open a new window.
// Without this, child processes corrupt the TUI terminal on Windows.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
		HideWindow:    true,
	}
}

// psEscape escapes a string for use inside PowerShell single-quoted strings.
// The only special character in PS single-quoted strings is ' → ''.
func psEscape(s string) string {
	return strings.ReplaceAll(s, `'`, `''`)
}

// openInNewTerminalImpl writes the code to a temp file, builds a .ps1 script
// that runs it in a visible PowerShell window, and keeps that window open
// until the user presses any key.
func openInNewTerminalImpl(lang, code, tmpDir string) error {

	// ── Step 1: write the source file and resolve the run command ────────────

	var runBlock string // one or more lines of PowerShell to execute the code

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
		runBlock = fmt.Sprintf("& '%s' '%s'", psEscape(pyBin), psEscape(src))

	case "go", "golang":
		goBin, err := resolveGo()
		if err != nil {
			return err
		}
		goCode := code
		if !strings.Contains(goCode, "package ") {
			imps := "\"fmt\""
			if strings.Contains(goCode, "fmt.") {
				imps = "\"fmt\""
			}
			goCode = "package main\n\nimport " + imps + "\n\nfunc main() {\n" + indentLines(goCode, "\t") + "\n}"
		}
		src := filepath.Join(tmpDir, "main.go")
		if err := os.WriteFile(src, []byte(goCode), 0644); err != nil {
			return fmt.Errorf("failed to write script: %w", err)
		}
		runBlock = fmt.Sprintf("& '%s' run '%s'", psEscape(goBin), psEscape(src))

	case "javascript", "js", "node":
		nodeBin, err := resolveNode()
		if err != nil {
			return err
		}
		src := filepath.Join(tmpDir, "script.js")
		if err := os.WriteFile(src, []byte(code), 0644); err != nil {
			return fmt.Errorf("failed to write script: %w", err)
		}
		runBlock = fmt.Sprintf("& '%s' '%s'", psEscape(nodeBin), psEscape(src))

	case "typescript", "ts":
		npx, ok := lookAny("npx", "npx.cmd")
		if !ok {
			return fmt.Errorf("npx not found — install Node.js from https://nodejs.org/")
		}
		src := filepath.Join(tmpDir, "script.ts")
		if err := os.WriteFile(src, []byte(code), 0644); err != nil {
			return fmt.Errorf("failed to write script: %w", err)
		}
		runBlock = fmt.Sprintf("& '%s' --yes ts-node '%s'", psEscape(npx), psEscape(src))

	case "bash", "sh", "shell":
		if bash, ok := lookAny("bash", "bash.exe"); ok {
			src := filepath.Join(tmpDir, "script.sh")
			if err := os.WriteFile(src, []byte("#!/bin/bash\n"+code), 0755); err != nil {
				return fmt.Errorf("failed to write script: %w", err)
			}
			runBlock = fmt.Sprintf("& '%s' '%s'", psEscape(bash), psEscape(src))
		} else {
			src := filepath.Join(tmpDir, "script.bat")
			if err := os.WriteFile(src, []byte("@echo off\r\n"+code), 0644); err != nil {
				return fmt.Errorf("failed to write script: %w", err)
			}
			runBlock = fmt.Sprintf("cmd.exe /C '%s'", psEscape(src))
		}

	case "ruby", "rb":
		rubyBin, ok := lookAny("ruby", "ruby.exe")
		if !ok {
			return fmt.Errorf("Ruby not found. Install from https://www.ruby-lang.org/")
		}
		src := filepath.Join(tmpDir, "script.rb")
		if err := os.WriteFile(src, []byte(code), 0644); err != nil {
			return fmt.Errorf("failed to write script: %w", err)
		}
		runBlock = fmt.Sprintf("& '%s' '%s'", psEscape(rubyBin), psEscape(src))

	case "rust", "rs":
		rustc, ok := lookAny("rustc", "rustc.exe")
		if !ok {
			return fmt.Errorf("rustc not found. Install Rust from https://rustup.rs/")
		}
		src := filepath.Join(tmpDir, "main.rs")
		bin := filepath.Join(tmpDir, "main.exe")
		if err := os.WriteFile(src, []byte(code), 0644); err != nil {
			return fmt.Errorf("failed to write script: %w", err)
		}
		runBlock = fmt.Sprintf(
			"& '%s' '%s' -o '%s'\nif ($LASTEXITCODE -eq 0) { & '%s' } else { Write-Host 'Compilation failed.' -ForegroundColor Red }",
			psEscape(rustc), psEscape(src), psEscape(bin), psEscape(bin),
		)

	case "c":
		gcc, ok := lookAny("gcc", "gcc.exe", "cc", "clang")
		if !ok {
			return fmt.Errorf("gcc/clang not found. Install a C compiler (e.g. MinGW).")
		}
		src := filepath.Join(tmpDir, "main.c")
		bin := filepath.Join(tmpDir, "main.exe")
		if err := os.WriteFile(src, []byte(code), 0644); err != nil {
			return fmt.Errorf("failed to write script: %w", err)
		}
		runBlock = fmt.Sprintf(
			"& '%s' -std=c17 -Wall '%s' -o '%s'\nif ($LASTEXITCODE -eq 0) { & '%s' } else { Write-Host 'Compilation failed.' -ForegroundColor Red }",
			psEscape(gcc), psEscape(src), psEscape(bin), psEscape(bin),
		)

	case "cpp", "c++":
		gpp, ok := lookAny("g++", "g++.exe", "c++", "clang++")
		if !ok {
			return fmt.Errorf("g++/clang++ not found. Install a C++ compiler (e.g. MinGW).")
		}
		src := filepath.Join(tmpDir, "main.cpp")
		bin := filepath.Join(tmpDir, "main.exe")
		if err := os.WriteFile(src, []byte(code), 0644); err != nil {
			return fmt.Errorf("failed to write script: %w", err)
		}
		runBlock = fmt.Sprintf(
			"& '%s' -std=c++17 -Wall '%s' -o '%s'\nif ($LASTEXITCODE -eq 0) { & '%s' } else { Write-Host 'Compilation failed.' -ForegroundColor Red }",
			psEscape(gpp), psEscape(src), psEscape(bin), psEscape(bin),
		)

	case "php":
		php, ok := lookAny("php", "php.exe")
		if !ok {
			return fmt.Errorf("PHP not found. Install from https://www.php.net/downloads")
		}
		src := filepath.Join(tmpDir, "script.php")
		if err := os.WriteFile(src, []byte(code), 0644); err != nil {
			return fmt.Errorf("failed to write script: %w", err)
		}
		runBlock = fmt.Sprintf("& '%s' '%s'", psEscape(php), psEscape(src))

	case "lua":
		lua, ok := lookAny("lua", "lua5.4", "lua5.3", "lua.exe")
		if !ok {
			return fmt.Errorf("Lua not found. Install from https://www.lua.org/download.html")
		}
		src := filepath.Join(tmpDir, "script.lua")
		if err := os.WriteFile(src, []byte(code), 0644); err != nil {
			return fmt.Errorf("failed to write script: %w", err)
		}
		runBlock = fmt.Sprintf("& '%s' '%s'", psEscape(lua), psEscape(src))

	case "dart":
		dart, ok := lookAny("dart", "dart.exe")
		if !ok {
			return fmt.Errorf("Dart SDK not found. Install from https://dart.dev/get-dart")
		}
		src := filepath.Join(tmpDir, "script.dart")
		if err := os.WriteFile(src, []byte(code), 0644); err != nil {
			return fmt.Errorf("failed to write script: %w", err)
		}
		runBlock = fmt.Sprintf("& '%s' run '%s'", psEscape(dart), psEscape(src))


	default:
		return fmt.Errorf("unsupported language %q — supported: python, go, js, ts, bash, ruby, rust, c, c++, php, lua, dart", lang)
	}

	// ── Step 2: write the PowerShell runner script ───────────────────────────

	psScript := fmt.Sprintf(`
$ErrorActionPreference = 'Continue'
try { $Host.UI.RawUI.WindowTitle = 'Ollama CLI - %s' } catch {}
Write-Host ''
Write-Host ' === Ollama CLI: running %s ===' -ForegroundColor Cyan
Write-Host ''

%s

Write-Host ''
Write-Host '====================================================' -ForegroundColor Yellow
Write-Host '  Program finished.  Press any key to close...' -ForegroundColor Yellow
Write-Host '====================================================' -ForegroundColor Yellow

try {
    $null = $Host.UI.RawUI.ReadKey('NoEcho,IncludeKeyDown')
} catch {
    Read-Host
}
`, lang, lang, runBlock)

	psPath := filepath.Join(tmpDir, "run.ps1")
	if err := os.WriteFile(psPath, []byte(psScript), 0644); err != nil {
		return fmt.Errorf("failed to write PS script: %w", err)
	}

	// ── Step 3: launch a visible PowerShell window that runs the script ──────

	launchCmd := fmt.Sprintf(
		`Start-Process powershell -ArgumentList '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', '%s' -WindowStyle Normal -Wait`,
		psEscape(psPath),
	)

	launcher := exec.Command(
		"powershell.exe",
		"-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden",
		"-Command", launchCmd,
	)
	launcher.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW — hide the launcher
		HideWindow:    true,
	}

	if err := launcher.Start(); err != nil {
		return fmt.Errorf("failed to launch terminal: %w", err)
	}

	go func() {
		_ = launcher.Wait()
		os.RemoveAll(tmpDir)
	}()

	return nil
}

// OpenTerminalInDir opens a visible cmd.exe terminal window that starts in
// the given directory. The user can then run commands interactively.
// cmd.exe is used so all system PATH tools (gcc, MinGW, etc.) are available.
func OpenTerminalInDir(dir string) error {
	// A hidden PS launcher calls Start-Process to open a visible cmd.exe /K window.
	// /K sets the location and leaves the prompt open.
	psCmd := fmt.Sprintf(
		`Start-Process cmd -ArgumentList @('/K', 'cd /d "%s"') -WindowStyle Normal`,
		dir,
	)
	cmd := exec.Command(
		"powershell.exe",
		"-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden",
		"-Command", psCmd,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW — hide the launcher itself
		HideWindow:    true,
	}
	return cmd.Start()
}

// RunInVisibleTerminalCaptured runs a command in a new visible cmd.exe window
// as a fully interactive environment. cmd.exe is used (not PowerShell) so all
// system PATH tools like gcc/MinGW are available.
//
// After the command exits, cmd stays open (/K) so the user can run follow-up
// commands or inspect files. The TUI blocks until the user closes the window,
// then reads the exit code for LLM error-feedback.
//
// Strategy:
//  1. Write a .bat file: cd to workDir, run the command, save exit code to a
//     temp file, print a status banner.
//  2. Launch  cmd.exe /K <batFile>  via a hidden PS launcher (Start-Process -Wait).
//     /K runs the bat then leaves the cmd prompt open for interactive use.
//  3. Go blocks until the user closes the cmd window, then reads the exit code.
func RunInVisibleTerminalCaptured(command, workDir string, timeout time.Duration) CommandResult {
	if timeout == 0 {
		timeout = 10 * time.Minute // interactive sessions can be long
	}
	start := time.Now()
	res := CommandResult{Command: command}

	tmpDir, err := os.MkdirTemp("", "ollama-cli-vis-*")
	if err != nil {
		res.Error = fmt.Errorf("cannot create temp dir: %w", err)
		res.ExitCode = 1
		return res
	}
	// tmpDir cleanup happens in the goroutine after the window closes.

	exitFile := filepath.Join(tmpDir, "exit.txt")
	batFile  := filepath.Join(tmpDir, "run.bat")

	// ── 1. Write the Batch script ───────────────────────────────────────
	var b strings.Builder
	b.WriteString("@echo off\r\n")
	b.WriteString("title Ollama CLI\r\n")
	if workDir != "" {
		b.WriteString(fmt.Sprintf("cd /d \"%s\"\r\n", workDir))
	}
	b.WriteString("echo.\r\n")
	b.WriteString("echo   ============ OLLAMA CLI ============\r\n")
	b.WriteString(fmt.Sprintf("echo   * Running: %s\r\n", strings.ReplaceAll(command, "%", "%%")))
	b.WriteString("echo   ====================================\r\n")
	b.WriteString("echo.\r\n")
	b.WriteString(command + "\r\n")
	b.WriteString("set ec=%errorlevel%\r\n")
	b.WriteString(fmt.Sprintf("echo %%ec%% > \"%s\"\r\n", exitFile))
	b.WriteString("echo.\r\n")
	b.WriteString("echo   ====================================\r\n")
	b.WriteString("if %ec%==0 (\r\n")
	b.WriteString("  echo   * Finished successfully\r\n")
	b.WriteString(") else (\r\n")
	b.WriteString("  echo   * Failed with exit code %ec%\r\n")
	b.WriteString(")\r\n")
	b.WriteString("echo   ====================================\r\n")
	b.WriteString("echo.\r\n")
	b.WriteString("echo   Program finished. Press any key to close...\r\n")
	b.WriteString("pause > nul\r\n")
	b.WriteString("exit %ec%\r\n")

	if err := os.WriteFile(batFile, []byte(b.String()), 0644); err != nil {
		os.RemoveAll(tmpDir)
		res.Error = fmt.Errorf("failed to write Batch script: %w", err)
		res.ExitCode = 1
		return res
	}

	// ── 2. Launch a visible cmd window and wait for it to close ──────────
	//
	// Spawn cmd.exe to start /wait a visible cmd.exe window running the batch file.
	launcher := exec.Command(
		"cmd.exe",
		"/c", "start", "/wait", "cmd.exe", "/c", batFile,
	)
	launcher.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW — hide the launcher cmd
		HideWindow:    true,
	}

	if err := launcher.Start(); err != nil {
		os.RemoveAll(tmpDir)
		res.Error = fmt.Errorf("failed to launch terminal: %w", err)
		res.ExitCode = 1
		return res
	}

	done := make(chan error, 1)
	go func() {
		done <- launcher.Wait()
		os.RemoveAll(tmpDir) // clean up after window is closed
	}()

	// ── 3. Wait (with timeout) for the window to be closed ───────────────────
	select {
	case <-done:
		// User closed the cmd window — read results.
	case <-time.After(timeout):
		launcher.Process.Kill()
		<-done
		res.Duration = time.Since(start)
		res.Error = fmt.Errorf("timed out after %s", timeout)
		res.ExitCode = 1
		return res
	}

	res.Duration = time.Since(start)

	// ── 4. Read exit code ─────────────────────────────────────────────────────
	if data, err := os.ReadFile(exitFile); err == nil {
		var code int
		if _, err := fmt.Sscan(strings.TrimSpace(string(data)), &code); err == nil {
			res.ExitCode = code
		}
	}
	if res.ExitCode != 0 && res.Error == nil {
		res.Error = fmt.Errorf("command exited with code %d", res.ExitCode)
	}

	return res
}
