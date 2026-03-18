package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var campaignShowCmd = &cobra.Command{
	Use:   "show [campaign-id]",
	Short: "Show campaign details",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		database := openDB(cfg)
		defer database.Close()

		c, err := findCampaignByPrefix(database, args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Campaign not found: %s\n", args[0])
			os.Exit(1)
		}

		listName := "(unknown)"
		list, err := database.GetList(c.ListID)
		if err == nil {
			listName = list.Name
		}

		tmplName := "(none)"
		if c.TemplateID != "" {
			tmpl, err := database.GetTemplate(c.TemplateID)
			if err == nil {
				tmplName = tmpl.Name
			}
		}

		fmt.Printf("ID:          %s\n", c.ID)
		fmt.Printf("Name:        %s\n", c.Name)
		fmt.Printf("Subject:     %s\n", c.Subject)
		fmt.Printf("Status:      %s\n", c.Status)
		fmt.Printf("List:        %s\n", listName)
		fmt.Printf("Template:    %s\n", tmplName)
		fmt.Printf("Sent:        %d\n", c.SentCount)
		fmt.Printf("Failed:      %d\n", c.FailedCount)
		fmt.Printf("Created:     %s\n", c.CreatedAt.Format("2006-01-02 15:04"))
		if c.SentAt != nil {
			fmt.Printf("Sent at:     %s\n", c.SentAt.Format("2006-01-02 15:04"))
		}
		fmt.Printf("Body size:   %d bytes\n", len(c.Body))
	},
}

func init() {
	campaignCmd.AddCommand(campaignShowCmd)
}
