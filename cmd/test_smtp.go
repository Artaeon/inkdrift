package cmd

import (
	"fmt"
	"os"

	"github.com/artaeon/inkdrift/internal/smtp"
	"github.com/spf13/cobra"
)

var testSMTPCmd = &cobra.Command{
	Use:   "test-smtp [email]",
	Short: "Send a test email to verify SMTP configuration",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()

		if cfg.SMTP.Host == "" {
			fmt.Fprintln(os.Stderr, "SMTP not configured. Run: inkdrift init")
			os.Exit(1)
		}

		to := ""
		if len(args) > 0 {
			to = args[0]
		} else {
			to = prompt("Send test email to")
		}

		fmt.Printf("Testing SMTP connection to %s:%d...\n", cfg.SMTP.Host, cfg.SMTP.Port)

		sender := smtp.NewSender(cfg.SMTP)

		if err := sender.TestConnection(); err != nil {
			fmt.Fprintf(os.Stderr, "Connection failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Connection: OK")

		fmt.Printf("Sending test email to %s...\n", to)
		err := sender.Send(smtp.Email{
			To:      to,
			Subject: "InkDrift Test Email",
			HTML: `<div style="font-family:sans-serif;padding:20px;">
<h2>InkDrift SMTP Test</h2>
<p>If you're reading this, your SMTP configuration is working correctly!</p>
<p style="color:#666;font-size:14px;">Sent from InkDrift Newsletter Service</p>
</div>`,
			Text: "InkDrift SMTP Test\n\nIf you're reading this, your SMTP configuration is working correctly!",
		})

		if err != nil {
			fmt.Fprintf(os.Stderr, "Send failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Test email sent successfully!")
	},
}

func init() {
	rootCmd.AddCommand(testSMTPCmd)
}
