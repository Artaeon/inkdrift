package cmd

import (
	"fmt"
	"html/template"
	"os"
	"strings"
	"time"

	"github.com/artaeon/inkdrift/internal/render"
	"github.com/spf13/cobra"
)

var campaignPreviewCmd = &cobra.Command{
	Use:   "preview [campaign-id]",
	Short: "Preview a campaign's rendered HTML",
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
			SubscriberName:  "Preview User",
			SubscriberEmail: "preview@example.com",
			UnsubscribeURL:  "https://example.com/unsubscribe?token=preview",
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

		output, _ := cmd.Flags().GetString("output")
		if output != "" {
			if err := os.WriteFile(output, []byte(html), 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Preview saved to %s (open in browser to view)\n", output)
			return
		}

		// Show text preview on terminal
		textMode, _ := cmd.Flags().GetBool("text")
		if textMode {
			fmt.Println(render.RenderText(html))
		} else {
			fmt.Printf("Subject: %s\n", c.Subject)
			fmt.Printf("List:    %s\n", list.Name)
			fmt.Printf("Status:  %s\n", c.Status)
			fmt.Println(strings.Repeat("-", 50))
			fmt.Println(html)
		}
	},
}

var campaignUpdateCmd = &cobra.Command{
	Use:   "update [campaign-id]",
	Short: "Update a campaign's body from a file",
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

		if c.Status != "draft" {
			fmt.Fprintf(os.Stderr, "Cannot update a %s campaign\n", c.Status)
			os.Exit(1)
		}

		bodyFile, _ := cmd.Flags().GetString("body-file")
		subject, _ := cmd.Flags().GetString("subject")

		if bodyFile != "" {
			data, err := os.ReadFile(bodyFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
				os.Exit(1)
			}
			c.Body = string(data)
		}

		if subject != "" {
			c.Subject = subject
		}

		_, err = database.UpdateCampaignBody(c.ID, c.Subject, c.Body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Campaign '%s' updated.\n", c.Name)
	},
}

func init() {
	campaignCmd.AddCommand(campaignPreviewCmd)
	campaignCmd.AddCommand(campaignUpdateCmd)

	campaignPreviewCmd.Flags().StringP("output", "o", "", "Save rendered HTML to file")
	campaignPreviewCmd.Flags().Bool("text", false, "Show plaintext version")

	campaignUpdateCmd.Flags().String("body-file", "", "HTML file for email body")
	campaignUpdateCmd.Flags().StringP("subject", "s", "", "Update subject line")
}
