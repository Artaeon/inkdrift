package cmd

import (
	"fmt"
	"html/template"
	"os"
	"time"

	"github.com/artaeon/inkdrift/internal/render"
	"github.com/artaeon/inkdrift/internal/smtp"
	"github.com/spf13/cobra"
)

var campaignTestSendCmd = &cobra.Command{
	Use:   "test-send [campaign-id] [email]",
	Short: "Send a campaign to a single test address",
	Long:  `Sends the campaign to one email address for testing, without affecting campaign status or stats.`,
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		database := openDB(cfg)
		defer database.Close()

		if !cfg.SMTPConfigured() {
			fmt.Fprintln(os.Stderr, "SMTP not configured. Run: inkdrift init")
			os.Exit(1)
		}

		c, err := findCampaignByPrefix(database, args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Campaign not found: %s\n", args[0])
			os.Exit(1)
		}

		testEmail := args[1]

		list, err := database.GetList(c.ListID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		tmplBody := c.Body
		if c.TemplateID != "" {
			tmpl, err := database.GetTemplate(c.TemplateID)
			if err == nil {
				tmplBody = tmpl.Body
			}
		}

		ctx := render.Context{
			SubscriberName:  "Test User",
			SubscriberEmail: testEmail,
			UnsubscribeURL:  "https://example.com/unsubscribe?token=test",
			ListName:        list.Name,
			SenderName:      cfg.SMTP.FromName,
			Content:         template.HTML(c.Body),
			Year:            time.Now().Year(),
		}

		html, err := render.RenderHTML(tmplBody, ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error rendering: %v\n", err)
			os.Exit(1)
		}

		text := render.RenderText(html)

		fmt.Printf("Sending test of '%s' to %s...\n", c.Name, testEmail)

		sender := smtp.NewSender(cfg.SMTP)
		err = sender.Send(smtp.Email{
			To:      testEmail,
			Subject: fmt.Sprintf("[TEST] %s", c.Subject),
			HTML:    html,
			Text:    text,
			Headers: map[string]string{
				"X-Mailer": "InkDrift",
			},
		})

		if err != nil {
			fmt.Fprintf(os.Stderr, "Send failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Test email sent successfully!")
		fmt.Println("Note: campaign status and stats are not affected by test sends.")
	},
}

func init() {
	campaignCmd.AddCommand(campaignTestSendCmd)
}
