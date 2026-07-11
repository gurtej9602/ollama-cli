package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Executor manages available tools
type Executor struct{}

// NewExecutor creates a new tool executor
func NewExecutor() *Executor {
	return &Executor{}
}

// ReadFile reads the contents of a file
func ReadFile(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// WriteFile writes content to a file (creates parent dirs if needed)
func WriteFile(path, content string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return err
	}

	return os.WriteFile(abs, []byte(content), 0644)
}

// ListDir returns a simple tree of the given directory
func ListDir(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	err = walkDir(abs, abs, &sb, 0)
	return sb.String(), err
}

func walkDir(root, current string, sb *strings.Builder, depth int) error {
	entries, err := os.ReadDir(current)
	if err != nil {
		return err
	}

	for _, e := range entries {
		// Skip common ignored dirs
		name := e.Name()
		if name == ".git" || name == "node_modules" || name == "vendor" || name == "__pycache__" {
			continue
		}

		indent := strings.Repeat("  ", depth)
		if e.IsDir() {
			sb.WriteString(fmt.Sprintf("%s📁 %s/\n", indent, name))
			sub := filepath.Join(current, name)
			walkDir(root, sub, sb, depth+1)
		} else {
			sb.WriteString(fmt.Sprintf("%s📄 %s\n", indent, name))
		}
	}
	return nil
}

// RunCommand executes a shell command and returns output
func RunCommand(command string) (string, error) {
	var cmd *exec.Cmd

	// On Windows, use cmd.exe
	cmd = exec.Command("cmd", "/C", command)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("command failed: %w\nOutput: %s", err, string(output))
	}

	return string(output), nil
}
