package cmd

import (
	"fmt"
	"os"

	"github.com/artaeon/inkdrift/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management",
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Check configuration for issues",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()

		if cfgPath := config.ConfigPath(); cfgPath != "" {
			fmt.Printf("Config file: %s\n", cfgPath)
		} else {
			fmt.Println("Config file: none (using defaults + env vars)")
		}
		fmt.Println()

		// SMTP
		if cfg.SMTPConfigured() {
			fmt.Printf("  SMTP:       %s:%d (from: %s)\n", cfg.SMTP.Host, cfg.SMTP.Port, cfg.SMTP.From)
		} else {
			fmt.Println("  SMTP:       NOT CONFIGURED")
		}

		// Domain
		if cfg.Server.Domain != "" {
			fmt.Printf("  Domain:     %s\n", cfg.Server.Domain)
		} else {
			fmt.Println("  Domain:     not set (using localhost)")
		}

		// API
		if cfg.API.APIKey != "" {
			fmt.Printf("  API key:    configured (%d chars)\n", len(cfg.API.APIKey))
		} else {
			fmt.Println("  API key:    NOT SET (admin endpoints locked)")
		}

		fmt.Printf("  API:        %s:%d\n", cfg.API.Host, cfg.API.Port)
		fmt.Printf("  DB:         %s\n", cfg.DB.Path)
		fmt.Printf("  Rate limit: %d req/min\n", cfg.API.RateLimit)
		fmt.Println()

		warnings := cfg.Validate()
		if len(warnings) == 0 {
			fmt.Println("No issues found. Configuration looks good for production.")
		} else {
			fmt.Printf("Found %d issue(s):\n", len(warnings))
			for _, w := range warnings {
				fmt.Printf("  - %s\n", w)
			}
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configValidateCmd)
}
