package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show newsletter statistics",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		database := openDB(cfg)
		defer database.Close()

		lists, _ := database.ListLists()
		campaigns, _ := database.ListCampaigns()

		totalSubscribers := 0
		for _, l := range lists {
			count, _ := database.ListSubscriberCount(l.ID)
			totalSubscribers += count
		}

		totalSent := 0
		totalFailed := 0
		draftCount := 0
		sentCount := 0
		for _, c := range campaigns {
			totalSent += c.SentCount
			totalFailed += c.FailedCount
			switch c.Status {
			case "draft":
				draftCount++
			case "sent":
				sentCount++
			}
		}

		fmt.Println("  _       _    ____       _  __ _   ")
		fmt.Println(" | |     | |  |  _ \\ _ __(_)/ _| |_ ")
		fmt.Println(" | | _ _ | | _| | | | '__| | |_| __|")
		fmt.Println(" | || ' \\| / / |_| | |  | |  _| |_ ")
		fmt.Println(" |_||_||_|_\\_\\____/|_|  |_|_|  \\__|")
		fmt.Println()
		fmt.Println(" Newsletter Statistics")
		fmt.Println(" ---------------------")
		fmt.Println()
		fmt.Printf("  Lists:        %d\n", len(lists))
		fmt.Printf("  Subscribers:  %d (active)\n", totalSubscribers)
		fmt.Printf("  Campaigns:    %d total (%d draft, %d sent)\n", len(campaigns), draftCount, sentCount)
		fmt.Printf("  Emails sent:  %d\n", totalSent)
		fmt.Printf("  Failed:       %d\n", totalFailed)
		fmt.Println()

		if len(lists) > 0 {
			fmt.Println(" Lists:")
			for _, l := range lists {
				count, _ := database.ListSubscriberCount(l.ID)
				fmt.Printf("  - %s: %d subscribers\n", l.Name, count)
			}
		}

		if cfg.SMTP.Host != "" {
			fmt.Println()
			fmt.Printf(" SMTP: %s:%d (%s)\n", cfg.SMTP.Host, cfg.SMTP.Port, cfg.SMTP.From)
		} else {
			fmt.Println()
			fmt.Println(" SMTP: Not configured (run: inkdrift init)")
		}

		fmt.Printf(" API:  http://%s:%d\n", cfg.API.Host, cfg.API.Port)
		fmt.Printf(" DB:   %s\n", cfg.DB.Path)

		if cfg.Server.Domain != "" {
			fmt.Printf(" Domain: %s\n", cfg.Server.Domain)
		}
	},
}

func init() {
	rootCmd.AddCommand(statsCmd)
}

func exitIf(err error, msg string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", msg, err)
		os.Exit(1)
	}
}
