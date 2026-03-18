package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/artaeon/inkdrift/internal/campaign"
	"github.com/artaeon/inkdrift/internal/db"
	"github.com/artaeon/inkdrift/internal/smtp"
	"github.com/spf13/cobra"
)

var campaignCmd = &cobra.Command{
	Use:     "campaign",
	Aliases: []string{"campaigns"},
	Short:   "Manage campaigns",
}

var campaignCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new campaign",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		database := openDB(cfg)
		defer database.Close()

		name, _ := cmd.Flags().GetString("name")
		subject, _ := cmd.Flags().GetString("subject")
		body, _ := cmd.Flags().GetString("body")
		bodyFile, _ := cmd.Flags().GetString("body-file")
		listName, _ := cmd.Flags().GetString("list")

		if name == "" {
			name = prompt("Campaign name")
		}
		if subject == "" {
			subject = prompt("Email subject")
		}

		if bodyFile != "" {
			data, err := os.ReadFile(bodyFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading body file: %v\n", err)
				os.Exit(1)
			}
			body = string(data)
		}

		if body == "" {
			fmt.Println("Enter email body (HTML supported, end with empty line):")
			body = readMultiline()
		}

		listID := resolveListID(database, listName)

		c, err := database.CreateCampaign(name, subject, body, listID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Campaign created: %s (ID: %s)\n", c.Name, shortID(c.ID))
		fmt.Printf("Status: %s\n", c.Status)
		fmt.Printf("Send with: inkdrift campaign send %s\n", shortID(c.ID))
	},
}

var campaignListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List all campaigns",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		database := openDB(cfg)
		defer database.Close()

		campaigns, err := database.ListCampaigns()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if len(campaigns) == 0 {
			fmt.Println("No campaigns yet. Create one with: inkdrift campaign create")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tSUBJECT\tSTATUS\tSENT\tFAILED")
		fmt.Fprintln(w, "--\t----\t-------\t------\t----\t------")
		for _, c := range campaigns {
			id := c.ID
			if len(id) > 8 {
				id = id[:8]
			}
			subj := c.Subject
			if len(subj) > 30 {
				subj = subj[:30] + "..."
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%d\n",
				id, c.Name, subj, c.Status, c.SentCount, c.FailedCount)
		}
		w.Flush()
	},
}

var campaignSendCmd = &cobra.Command{
	Use:   "send [campaign-id]",
	Short: "Send a campaign to all subscribers",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		database := openDB(cfg)
		defer database.Close()

		campaignID := args[0]

		c, err := findCampaignByPrefix(database, campaignID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Campaign not found: %s\n", campaignID)
			os.Exit(1)
		}

		if cfg.SMTP.Host == "" {
			fmt.Fprintln(os.Stderr, "SMTP not configured. Run: inkdrift init")
			os.Exit(1)
		}

		list, err := database.GetList(c.ListID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		count, _ := database.ListSubscriberCount(c.ListID)
		fmt.Printf("Campaign: %s\n", c.Name)
		fmt.Printf("Subject:  %s\n", c.Subject)
		fmt.Printf("List:     %s (%d active subscribers)\n", list.Name, count)
		fmt.Println()

		dry, _ := cmd.Flags().GetBool("dry-run")
		if dry {
			fmt.Println("[DRY RUN] Would send to all active subscribers.")
			return
		}

		if !confirm("Send this campaign?") {
			fmt.Println("Cancelled.")
			return
		}

		fmt.Println()
		fmt.Println("Sending...")

		sender := campaign.NewSender(database, smtp.NewSender(cfg.SMTP), cfg)
		sender.OnSend(func(email string, err error) {
			if err != nil {
				fmt.Printf("  FAIL  %s: %v\n", email, err)
			} else {
				fmt.Printf("  SENT  %s\n", email)
			}
		})

		if err := sender.Send(c.ID); err != nil {
			fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
			os.Exit(1)
		}

		c, _ = database.GetCampaign(c.ID)
		fmt.Println()
		fmt.Printf("Done! Sent: %d, Failed: %d\n", c.SentCount, c.FailedCount)
	},
}

var campaignDeleteCmd = &cobra.Command{
	Use:   "delete [campaign-id]",
	Short: "Delete a campaign",
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

		force, _ := cmd.Flags().GetBool("force")
		if !force {
			if !confirm(fmt.Sprintf("Delete campaign '%s'?", c.Name)) {
				fmt.Println("Cancelled.")
				return
			}
		}

		if err := database.DeleteCampaign(c.ID); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Campaign '%s' deleted.\n", c.Name)
	},
}

func init() {
	rootCmd.AddCommand(campaignCmd)
	campaignCmd.AddCommand(campaignCreateCmd)
	campaignCmd.AddCommand(campaignListCmd)
	campaignCmd.AddCommand(campaignSendCmd)
	campaignCmd.AddCommand(campaignDeleteCmd)

	campaignCreateCmd.Flags().StringP("name", "n", "", "Campaign name")
	campaignCreateCmd.Flags().StringP("subject", "s", "", "Email subject")
	campaignCreateCmd.Flags().StringP("body", "b", "", "Email body (HTML)")
	campaignCreateCmd.Flags().String("body-file", "", "Read body from file")
	campaignCreateCmd.Flags().StringP("list", "l", "", "List name or ID")

	campaignSendCmd.Flags().Bool("dry-run", false, "Preview without sending")
	campaignDeleteCmd.Flags().BoolP("force", "f", false, "Skip confirmation")
}

func readMultiline() string {
	var lines []string
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" && len(lines) > 0 {
			break
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func findCampaignByPrefix(database *db.DB, prefix string) (*db.Campaign, error) {
	// Try exact match first
	c, err := database.GetCampaign(prefix)
	if err == nil {
		return c, nil
	}

	// Try prefix match
	campaigns, err := database.ListCampaigns()
	if err != nil {
		return nil, err
	}

	for _, c := range campaigns {
		if strings.HasPrefix(c.ID, prefix) {
			return &c, nil
		}
	}

	return nil, fmt.Errorf("not found")
}
