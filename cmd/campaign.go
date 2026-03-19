package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

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
		templateName, _ := cmd.Flags().GetString("template")

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

		if strings.TrimSpace(body) == "" {
			fmt.Fprintln(os.Stderr, "Error: campaign body cannot be empty")
			os.Exit(1)
		}

		listID := resolveListID(database, listName)

		c, err := database.CreateCampaign(name, subject, body, listID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// Attach template if specified
		if templateName != "" {
			tmpl, err := database.GetTemplateByName(templateName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: template '%s' not found, campaign created without template\n", templateName)
			} else {
				if err := database.SetCampaignTemplate(c.ID, tmpl.ID); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to set template: %v\n", err)
				} else {
					fmt.Printf("Template: %s\n", tmpl.Name)
				}
			}
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
	Long: `Send a campaign email to all active subscribers in the campaign's list.

Use --retry to resend only to subscribers that failed in a previous send.
This is useful when a campaign was interrupted or had partial failures.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		database := openDB(cfg)
		defer database.Close()

		campaignID := args[0]
		retryMode, _ := cmd.Flags().GetBool("retry")

		c, err := findCampaignByPrefix(database, campaignID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Campaign not found: %s (run 'inkdrift campaign ls' to see available campaigns)\n", campaignID)
			os.Exit(1)
		}

		if !cfg.SMTPConfigured() {
			fmt.Fprintln(os.Stderr, "SMTP not configured. Run: inkdrift init")
			os.Exit(1)
		}

		if cfg.Server.Domain == "" {
			fmt.Fprintln(os.Stderr, "WARNING: Domain not configured — unsubscribe links will use localhost.")
			fmt.Fprintln(os.Stderr, "Set domain in inkdrift.toml or INKDRIFT_DOMAIN env for production use.")
			fmt.Println()
		}

		list, err := database.GetList(c.ListID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: list not found (may have been deleted)\n")
			os.Exit(1)
		}

		count, _ := database.ListSubscriberCount(c.ListID)
		fmt.Printf("Campaign:  %s\n", c.Name)
		fmt.Printf("Subject:   %s\n", c.Subject)
		fmt.Printf("Status:    %s\n", c.Status)
		fmt.Printf("List:      %s (%d active subscribers)\n", list.Name, count)
		fmt.Printf("SMTP:      %s:%d (from: %s)\n", cfg.SMTP.Host, cfg.SMTP.Port, cfg.SMTP.From)
		if retryMode {
			fmt.Printf("Mode:      RETRY (resending to failed/unsent subscribers only)\n")
		}
		fmt.Println()

		dry, _ := cmd.Flags().GetBool("dry-run")
		if dry {
			if retryMode {
				fmt.Println("[DRY RUN] Would resend to failed/unsent subscribers only.")
			} else {
				fmt.Printf("[DRY RUN] Would send to %d active subscribers.\n", count)
			}
			return
		}

		promptMsg := "Send this campaign?"
		if retryMode {
			promptMsg = "Retry failed sends for this campaign?"
		}
		if !confirm(promptMsg) {
			fmt.Println("Cancelled.")
			return
		}

		fmt.Println()

		start := time.Now()
		sender := campaign.NewSender(database, smtp.NewSender(cfg.SMTP), cfg)
		sender.OnSend(func(email string, idx, total int, err error) {
			pct := float64(idx) / float64(total) * 100
			if err != nil {
				fmt.Printf("  [%3.0f%%] FAIL  %s: %v\n", pct, email, err)
			} else {
				fmt.Printf("  [%3.0f%%] SENT  %s\n", pct, email)
			}
		})

		if retryMode {
			err = sender.ResendFailed(c.ID)
		} else {
			err = sender.Send(c.ID)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
			if strings.Contains(err.Error(), "not draft") || strings.Contains(err.Error(), "not in draft") {
				fmt.Fprintln(os.Stderr, "Hint: use --retry to resend a partial/failed campaign")
			}
			os.Exit(1)
		}

		elapsed := time.Since(start).Round(time.Second)
		fmt.Println()
		updated, err := database.GetCampaign(c.ID)
		if err != nil {
			fmt.Printf("Done in %s.\n", elapsed)
		} else {
			fmt.Printf("Done in %s! Sent: %d, Failed: %d, Status: %s\n", elapsed, updated.SentCount, updated.FailedCount, updated.Status)
			if updated.FailedCount > 0 {
				fmt.Println("Tip: use 'inkdrift campaign send --retry' to resend to failed subscribers")
			}
		}
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
	campaignCreateCmd.Flags().StringP("template", "t", "", "Template name to wrap email content")

	campaignSendCmd.Flags().Bool("dry-run", false, "Preview without sending")
	campaignSendCmd.Flags().Bool("retry", false, "Resend only to failed/unsent subscribers")
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
