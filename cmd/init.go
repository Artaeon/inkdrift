package cmd

import (
	"fmt"
	"strconv"

	"github.com/artaeon/inkdrift/internal/config"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize InkDrift with interactive setup",
	Long:  `Walks you through configuring SMTP, API, and database settings.`,
	Run:   runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) {
	fmt.Println("  _       _    ____       _  __ _   ")
	fmt.Println(" | |     | |  |  _ \\ _ __(_)/ _| |_ ")
	fmt.Println(" | | _ _ | | _| | | | '__| | |_| __|")
	fmt.Println(" | || ' \\| / / |_| | |  | |  _| |_ ")
	fmt.Println(" |_||_||_|_\\_\\____/|_|  |_|_|  \\__|")
	fmt.Println()
	fmt.Println(" Welcome to InkDrift - Simple Newsletter Service")
	fmt.Println(" ------------------------------------------------")
	fmt.Println()

	cfg := config.DefaultConfig()

	// Server
	fmt.Println("[Server]")
	cfg.Server.Name = promptDefault("Newsletter name", cfg.Server.Name)
	cfg.Server.Domain = prompt("Domain (e.g., newsletter.example.com, leave empty for localhost)")
	fmt.Println()

	// SMTP
	fmt.Println("[SMTP Configuration]")
	fmt.Println("Configure your email provider (Hostinger, Hetzner, Contabo, Gmail, etc.)")
	cfg.SMTP.Host = prompt("SMTP Host (e.g., smtp.hostinger.com)")
	portStr := promptDefault("SMTP Port", "587")
	port, err := strconv.Atoi(portStr)
	if err == nil {
		cfg.SMTP.Port = port
	}
	cfg.SMTP.Username = prompt("SMTP Username (usually your email)")
	cfg.SMTP.Password = prompt("SMTP Password")
	cfg.SMTP.From = prompt("From Email")
	cfg.SMTP.FromName = promptDefault("From Name", cfg.Server.Name)
	cfg.SMTP.TLS = true
	fmt.Println()

	// API
	fmt.Println("[API Configuration]")
	apiPortStr := promptDefault("API Port", "3377")
	apiPort, err := strconv.Atoi(apiPortStr)
	if err == nil {
		cfg.API.Port = apiPort
	}
	cfg.API.APIKey = prompt("API Key (for admin endpoints, leave empty to disable)")
	cfg.API.CORS = promptDefault("CORS Origin", "*")
	fmt.Println()

	// Database
	fmt.Println("[Database]")
	cfg.DB.Path = promptDefault("Database path", cfg.DB.Path)
	fmt.Println()

	// Save
	configPath := promptDefault("Save config to", "inkdrift.toml")
	if err := config.Save(cfg, configPath); err != nil {
		fmt.Printf("Error saving config: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Printf("Config saved to %s\n", configPath)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Create a list:       inkdrift list create \"My Newsletter\"")
	fmt.Println("  2. Add subscribers:     inkdrift subscriber add user@example.com --list \"My Newsletter\"")
	fmt.Println("  3. Create a campaign:   inkdrift campaign create --list \"My Newsletter\"")
	fmt.Println("  4. Send it:             inkdrift campaign send <campaign-id>")
	fmt.Println("  5. Start the API:       inkdrift serve")
	fmt.Println()
	fmt.Println("To integrate with your website, POST to /api/v1/subscribe:")
	fmt.Println(`  fetch("https://your-domain:3377/api/v1/subscribe", {`)
	fmt.Println(`    method: "POST",`)
	fmt.Println(`    headers: { "Content-Type": "application/json" },`)
	fmt.Println(`    body: JSON.stringify({ email: "user@example.com", list: "My Newsletter" })`)
	fmt.Println(`  })`)
}
