package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "inkdrift",
	Short: "InkDrift — simple newsletter service for any SMTP provider",
	Long: `InkDrift is a self-hosted newsletter service that works with any SMTP provider.
Configure your Hostinger, Hetzner, Contabo, or any other SMTP email and start
sending beautiful newsletters to your subscribers.

Features:
  - Simple SMTP configuration (works with any provider)
  - Subscriber management with lists
  - Campaign creation and scheduling
  - REST API for website integration (Next.js, React, etc.)
  - CLI for easy setup and management
  - Deployable with FleetDeck`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ./inkdrift.toml or /etc/inkdrift/config.toml)")
}
