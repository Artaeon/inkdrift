package cmd

import (
	"fmt"
	"os"

	"github.com/artaeon/inkdrift/internal/api"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the InkDrift API server",
	Long: `Starts the REST API server for website integration.

Endpoints:
  POST /api/v1/subscribe      - Subscribe an email (public)
  GET  /api/v1/unsubscribe    - Unsubscribe by token (public)
  GET  /api/v1/confirm        - Confirm subscription (public)
  GET  /api/v1/lists          - List all lists (admin)
  POST /api/v1/lists          - Create a list (admin)
  GET  /api/v1/campaigns      - List campaigns (admin)
  GET  /api/v1/stats          - Dashboard stats (admin)
  GET  /health                - Health check`,
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
		fmt.Printf(" API server starting on port %d\n", cfg.API.Port)
		fmt.Println()

		server := api.NewServer(database, cfg)
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
