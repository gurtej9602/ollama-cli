package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "ollama-cli",
	Short: "A local AI coding assistant powered by Ollama",
	Long: `
  ╔═══════════════════════════════════╗
  ║       🦙  Ollama CLI  🦙          ║
  ║  Local AI assistant — no cloud   ║
  ╚═══════════════════════════════════╝

A fast, local AI coding assistant powered by Ollama.
Streams responses token-by-token for instant feedback.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: $HOME/.ollamacli.yaml)")
	rootCmd.PersistentFlags().StringP("model", "m", "llama3.2", "Ollama model to use")
	rootCmd.PersistentFlags().StringP("host", "H", "http://localhost:11434", "Ollama host URL")

	viper.BindPFlag("model", rootCmd.PersistentFlags().Lookup("model"))
	viper.BindPFlag("host", rootCmd.PersistentFlags().Lookup("host"))
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".ollamacli")
	}

	viper.SetDefault("model", "llama3.2")
	viper.SetDefault("host", "http://localhost:11434")
	viper.SetDefault("system_prompt", `You are an expert coding assistant running inside Ollama CLI. You help users write, debug, and understand code.

IMPORTANT CONVENTIONS:
1. MULTI-FILE PROJECTS: When a project requires multiple files or folders, put EACH file in its own fenced code block and add a comment on the VERY FIRST LINE declaring its path, like:
   // file: src/main.py
   (use # file: for Python/Ruby/Shell, <!-- file: --> for HTML/CSS)
   All files are saved together under ~/ollama-cli-projects/<timestamp>/.

2. SHELL COMMANDS: When the user needs to run a command (e.g. npm install, pip install -r requirements.txt, go mod init), put it in a shell/bash/powershell code block. The CLI will ask the user to approve and run it, then show you the output.

3. ERROR FIXING: If code you write fails, the error output is automatically sent back to you. Read it carefully and provide the corrected code.

Be concise and practical. Always prefer working, runnable code over explanations.`)

	viper.SetDefault("theme", "dark")
	viper.SetDefault("auto_approve_tools", false)

	viper.AutomaticEnv()
	viper.ReadInConfig()
}
