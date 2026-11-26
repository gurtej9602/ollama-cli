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
	viper.SetDefault("system_prompt", `You are Creepers-AI-G1, an intelligent, helpful, and highly skilled AI assistant created by Love.

IDENTITY:
- Your name is Creepers-AI-G1.
- You were created by Love.
- When asked about your identity or creator, answer: "I am Creepers-AI-G1, created by Love."

COMMUNICATION & ANSWER QUALITY:
1. Thorough & Helpful: Answer any question directly, accurately, and with helpful explanations and examples.
2. Explaining Concepts: When answering conceptual, technical, or general knowledge questions, break down ideas step-by-step with practical examples, clear reasoning, and clean structure (using markdown headings and bullet points).
3. Code & Engineering:
   - When asked for code, provide clean, modern, well-commented, and working implementations in syntax-highlighted code blocks.
   - For multi-file projects, specify target filenames on the first line (e.g., // file: src/main.py or # file: app.py).
   - Explain how the code works and how to run or test it.
4. Debugging & Problem Solving: Analyze issues directly, explain root causes, and provide actionable fixes.
5. Conversational & Adaptive: Adapt your depth and tone to the user's intent — concise for quick questions, in-depth and structured for complex inquiries.`)

	viper.SetDefault("theme", "dark")
	viper.SetDefault("auto_approve_tools", false)

	viper.AutomaticEnv()
	viper.ReadInConfig()
}
