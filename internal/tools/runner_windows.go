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

	escapedPsPath := psEscape(psPath)
	launchCmd := fmt.Sprintf(
		"Start-Process cmd -ArgumentList '/c powershell -NoProfile -ExecutionPolicy Bypass -File \"%s\"' -WindowStyle Normal -Wait",
		escapedPsPath,
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
	psFile   := filepath.Join(tmpDir, "run.ps1")

	// ── 1. Write the PowerShell script ───────────────────────────────────────
	// Using a .ps1 instead of a .bat gives us:
	//   • Native color / Unicode box-drawing for a beautiful UI.
	//   • Auto-close: the script simply ends — no pause needed.
	//   • Full system PATH set explicitly so gcc, MinGW, go, etc. all work.
	var b strings.Builder

	// Escape single quotes in strings that will be embedded in PS single-quoted literals.
	safeCmd  := strings.ReplaceAll(command,  "'", "''")
	safeDir  := strings.ReplaceAll(workDir,  "'", "''")
	safeExit := strings.ReplaceAll(exitFile, "'", "''")

	b.WriteString("$ErrorActionPreference = 'Continue'\r\n")
	b.WriteString("$host.UI.RawUI.WindowTitle = 'Ollama CLI'\r\n")
	// Rebuild PATH from both Machine and User hives so every tool is found.
	b.WriteString("$env:PATH = [Environment]::GetEnvironmentVariable('PATH','Machine') + ';' +" +
		" [Environment]::GetEnvironmentVariable('PATH','User')\r\n")
	if workDir != "" {
		b.WriteString(fmt.Sprintf("Set-Location -LiteralPath '%s'\r\n", safeDir))
	}
	b.WriteString("[Console]::OutputEncoding = [Text.Encoding]::UTF8\r\n")
	b.WriteString("Clear-Host\r\n")

	// Characters embedded directly — simplest, no [char] cast needed in PS.
	const (
		boxH  = "\u2550" // ═
		boxTL = "\u2554" // ╔
		boxTR = "\u2557" // ╗
		boxBL = "\u255A" // ╚
		boxBR = "\u255D" // ╝
		boxV  = "\u2551" // ║
		icoRun  = "\u25B6" // ▶
		icoOK   = "\u2714" // ✔
		icoFail = "\u2718" // ✘
		icoDash = "\u2014" // —
		icoTime = "\u23F1" // ⏱
		icoLogo = "\u26A1" // ⚡
	)
	sep := strings.Repeat(boxH, 66) // width=70 → inner = 66 chars

	// PS variables and helper written once at top of the script.
	b.WriteString(fmt.Sprintf("$cmdStr = '%s'\r\n", safeCmd))
	b.WriteString("function LPad([string]$s,[int]$n){ $s + (' ' * [Math]::Max(0,$n - $s.Length)) }\r\n")

	b.WriteString("Write-Host ''\r\n")
	b.WriteString(fmt.Sprintf("Write-Host '  %s%s%s' -ForegroundColor DarkCyan\r\n", boxTL, sep, boxTR))
	b.WriteString(fmt.Sprintf("Write-Host '  %s' -ForegroundColor DarkCyan -NoNewline\r\n", boxV))
	b.WriteString(fmt.Sprintf("Write-Host (LPad '  %s OLLAMA CLI' 66) -ForegroundColor White -NoNewline\r\n", icoLogo))
	b.WriteString(fmt.Sprintf("Write-Host '%s' -ForegroundColor DarkCyan\r\n", boxV))
	b.WriteString(fmt.Sprintf("Write-Host '  %s' -ForegroundColor DarkCyan -NoNewline\r\n", boxV))
	b.WriteString(fmt.Sprintf("Write-Host (LPad \"  %s  $cmdStr\" 66) -ForegroundColor Yellow -NoNewline\r\n", icoRun))
	b.WriteString(fmt.Sprintf("Write-Host '%s' -ForegroundColor DarkCyan\r\n", boxV))
	b.WriteString(fmt.Sprintf("Write-Host '  %s%s%s' -ForegroundColor DarkCyan\r\n", boxBL, sep, boxBR))
	b.WriteString("Write-Host ''\r\n")

	// ── run ─────────────────────────────────────────────────────────────────────
	b.WriteString("$t0 = Get-Date\r\n")
	// cmd /c passes the full command string to cmd.exe verbatim.
	b.WriteString("cmd /c $cmdStr\r\n")
	b.WriteString("$ec = $LASTEXITCODE; if ($null -eq $ec) { $ec = 0 }\r\n")
	b.WriteString("$dur = [Math]::Round(((Get-Date)-$t0).TotalSeconds, 2)\r\n")
	b.WriteString(fmt.Sprintf("\"$ec\" | Set-Content -LiteralPath '%s'\r\n", safeExit))

	// ── footer ─────────────────────────────────────────────────────────────────
	b.WriteString("Write-Host ''\r\n")
	b.WriteString(fmt.Sprintf("Write-Host '  %s%s%s' -ForegroundColor DarkCyan\r\n", boxTL, sep, boxTR))
	b.WriteString(fmt.Sprintf("Write-Host '  %s' -ForegroundColor DarkCyan -NoNewline\r\n", boxV))
	b.WriteString(fmt.Sprintf("if ($ec -eq 0) {\r\n  Write-Host (LPad '  %s  Finished successfully' 66) -ForegroundColor Green -NoNewline\r\n} else {\r\n  Write-Host (LPad \"  %s  Failed %s exit code $ec\" 66) -ForegroundColor Red -NoNewline\r\n}\r\n",
		icoOK, icoFail, icoDash))
	b.WriteString(fmt.Sprintf("Write-Host '%s' -ForegroundColor DarkCyan\r\n", boxV))
	b.WriteString(fmt.Sprintf("Write-Host '  %s' -ForegroundColor DarkCyan -NoNewline\r\n", boxV))
	b.WriteString(fmt.Sprintf("Write-Host (LPad \"  %s  $($dur)s\" 66) -ForegroundColor DarkGray -NoNewline\r\n", icoTime))
	b.WriteString(fmt.Sprintf("Write-Host '%s' -ForegroundColor DarkCyan\r\n", boxV))
	b.WriteString(fmt.Sprintf("Write-Host '  %s%s%s' -ForegroundColor DarkCyan\r\n", boxBL, sep, boxBR))
	b.WriteString("Write-Host ''\r\n")
	b.WriteString("Write-Host '  Program finished. Press any key to close...' -ForegroundColor Yellow\r\n")
	b.WriteString("try { $null = $Host.UI.RawUI.ReadKey('NoEcho,IncludeKeyDown') } catch { Read-Host }\r\n")
	
	// Script ends → powershell process exits → Start-Process -Wait unblocks → auto-close.
	b.WriteString("exit $ec\r\n")

	if err := os.WriteFile(psFile, []byte(b.String()), 0644); err != nil {
		os.RemoveAll(tmpDir)
		res.Error = fmt.Errorf("failed to write PS script: %w", err)
		res.ExitCode = 1
		return res
	}

	// ── 2. Launch a visible PowerShell window and wait for it to close ────────
	//
	// A hidden PowerShell process (Start-Process -Wait) launches the visible
	// PS window. When the script finishes the window closes automatically.
	launchCmd := fmt.Sprintf(
		`Start-Process cmd -ArgumentList '/c powershell -NoProfile -ExecutionPolicy Bypass -File "%s"' -WindowStyle Normal -Wait`,
		psEscape(psFile),
	)
	launcher := exec.Command(
		"powershell.exe",
		"-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden",
		"-Command", launchCmd,
	)
	launcher.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW — hide the PS launcher
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
