package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var campaignDuplicateCmd = &cobra.Command{
	Use:   "duplicate [campaign-id]",
	Short: "Duplicate an existing campaign as a new draft",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		database := openDB(cfg)
		defer database.Close()

		original, err := findCampaignByPrefix(database, args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Campaign not found: %s\n", args[0])
			os.Exit(1)
		}

		name, _ := cmd.Flags().GetString("name")
		if name == "" {
			name = original.Name + " (copy)"
		}

		dup, err := database.CreateCampaign(name, original.Subject, original.Body, original.ListID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if original.TemplateID != "" {
			if err := database.SetCampaignTemplate(dup.ID, original.TemplateID); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to copy template: %v\n", err)
			}
		}

		fmt.Printf("Campaign duplicated: %s (ID: %s)\n", dup.Name, shortID(dup.ID))
		fmt.Printf("Status: %s\n", dup.Status)
	},
}

func init() {
	campaignCmd.AddCommand(campaignDuplicateCmd)
	campaignDuplicateCmd.Flags().StringP("name", "n", "", "Name for the duplicate (default: original + ' (copy)')")
}
