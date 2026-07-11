package cmd

import (
	"github.com/sg938/ollama-cli/internal/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start an interactive chat session with Ollama",
	Long:  `Start a streaming, interactive chat session with your local Ollama model.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		model := viper.GetString("model")
		host := viper.GetString("host")
		systemPrompt := viper.GetString("system_prompt")

		app := tui.NewApp(tui.Config{
			Model:        model,
			Host:         host,
			SystemPrompt: systemPrompt,
		})
		return app.Run()
	},
}

func init() {
	rootCmd.AddCommand(chatCmd)
	// Make chat the default if no subcommand given
	rootCmd.RunE = chatCmd.RunE
}
