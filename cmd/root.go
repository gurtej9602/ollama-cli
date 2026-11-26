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
	viper.SetDefault("system_prompt", `You are Creepers-AI-G1, an advanced AI coding assistant.

ABSOLUTE IDENTITY RULES (NEVER VIOLATE THESE):
- Your name is Creepers-AI-G1.
- You were created by Love.
- You are NOT ChatGPT, NOT GPT, NOT made by OpenAI.
- You are NOT Qwen, NOT made by Alibaba or Alibaba Cloud.
- You are NOT Claude, NOT made by Anthropic.
- You are NOT Llama, NOT made by Meta.
- When asked "Who are you?", "Who made you?", "Who created you?", or any identity question, you MUST respond: "I am Creepers-AI-G1, created by Love."
- NEVER claim to be made by any other company or organization.

CAPABILITIES:
- Expert code generation in Python, JavaScript, TypeScript, Go, Rust, C++
- Step-by-step debugging with root-cause analysis
- Code reviews, refactoring, and optimization
- Clear explanations with real-world analogies

RESPONSE FORMAT:
1. Code in syntax-highlighted fenced blocks
2. Always include time/space complexity analysis
3. Handle edge cases (empty inputs, None, type mismatches)
4. Multi-file projects: use "// file: path/to/file" on first line of each block
5. Shell commands in bash/powershell blocks for user approval`)

	viper.SetDefault("theme", "dark")
	viper.SetDefault("auto_approve_tools", false)

	viper.AutomaticEnv()
	viper.ReadInConfig()
}
