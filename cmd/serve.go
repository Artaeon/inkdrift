package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/artaeon/inkdrift/internal/api"
	"github.com/artaeon/inkdrift/internal/config"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the InkDrift API server",
	Long: `Starts the REST API server for website integration.

Public endpoints:
  POST /api/v1/subscribe       - Subscribe (double opt-in if SMTP configured)
  GET  /api/v1/unsubscribe     - Unsubscribe by token
  GET  /api/v1/confirm         - Confirm subscription (double opt-in)

Admin endpoints (requires API key):
  GET  /api/v1/lists           - List all lists
  POST /api/v1/lists           - Create a list
  GET  /api/v1/lists/:id/subscribers        - List subscribers (paginated)
  GET  /api/v1/lists/:id/subscribers/search - Search subscribers (?q=)
  GET  /api/v1/campaigns       - List campaigns
  GET  /api/v1/stats           - Dashboard stats
  GET  /health                 - Health check

Works out of the box with defaults. Configure via inkdrift.toml or environment variables.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		database := openDB(cfg)
		defer database.Close()

		port, _ := cmd.Flags().GetInt("port")
		if port > 0 {
			cfg.API.Port = port
		}

		fmt.Println("  _       _    ____       _  __ _   ")
		fmt.Println(" | |     | |  |  _ \\ _ __(_)/ _| |_ ")
		fmt.Println(" | | _ _ | | _| | | | '__| | |_| __|")
		fmt.Println(" | || ' \\| / / |_| | |  | |  _| |_ ")
		fmt.Println(" |_||_||_|_\\_\\____/|_|  |_|_|  \\__|")
		fmt.Println()

		// Show config source
		if cfgPath := config.ConfigPath(); cfgPath != "" {
			fmt.Printf(" Config:  %s\n", cfgPath)
		} else {
			fmt.Println(" Config:  using defaults (run inkdrift init to configure)")
		}
		fmt.Printf(" API:     http://%s:%d\n", cfg.API.Host, cfg.API.Port)
		fmt.Printf(" DB:      %s\n", cfg.DB.Path)

		if cfg.SMTPConfigured() {
			fmt.Printf(" SMTP:    %s:%d\n", cfg.SMTP.Host, cfg.SMTP.Port)
		}
		if cfg.Server.Domain != "" {
			fmt.Printf(" Domain:  %s\n", cfg.Server.Domain)
		}
		fmt.Println()

		// Show warnings for missing config
		warnings := cfg.Validate()
		if len(warnings) > 0 {
			for _, w := range warnings {
				log.Printf("WARNING: %s", w)
			}
			fmt.Println()
		}

		// Ensure at least one list exists for the subscribe endpoint
		lists, err := database.ListLists()
		if err == nil && len(lists) == 0 {
			log.Println("No subscriber lists found. Creating default list...")
			list, err := database.CreateList("Newsletter", "Default newsletter list", "", "")
			if err != nil {
				log.Printf("WARNING: could not create default list: %v", err)
			} else {
				log.Printf("Created default list: %s (ID: %s)", list.Name, shortID(list.ID))
			}
		}

		server := api.NewServer(database, cfg)
		log.Printf("InkDrift API server listening on :%d", cfg.API.Port)
		if err := server.ListenAndServe(); err != nil {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().IntP("port", "p", 0, "Override API port")
}
