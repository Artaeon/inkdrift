package cmd

import (
	"fmt"
	"net"
	"strings"

	"github.com/spf13/cobra"
)

var deliverabilityCmd = &cobra.Command{
	Use:     "deliverability",
	Aliases: []string{"deliver", "dns-check"},
	Short:   "Check email deliverability and DNS records",
	Long:    `Checks SPF, DKIM, DMARC, and MX records for your sending domain.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()

		from := cfg.SMTP.From
		if from == "" {
			fmt.Println("SMTP not configured. Run: inkdrift init")
			return
		}

		parts := strings.SplitN(from, "@", 2)
		if len(parts) != 2 {
			fmt.Println("Invalid from email in config")
			return
		}
		domain := parts[1]

		fmt.Printf("Checking deliverability for: %s\n", domain)
		fmt.Println(strings.Repeat("=", 50))
		fmt.Println()

		score := 0
		total := 4

		// 1. MX Records
		fmt.Println("[MX Records]")
		mxRecords, err := net.LookupMX(domain)
		if err != nil || len(mxRecords) == 0 {
			fmt.Println("  FAIL - No MX records found")
			fmt.Println("  Emails won't be deliverable without MX records.")
		} else {
			score++
			fmt.Println("  PASS - MX records found:")
			for _, mx := range mxRecords {
				fmt.Printf("    %s (priority %d)\n", mx.Host, mx.Pref)
			}
		}
		fmt.Println()

		// 2. SPF Record
		fmt.Println("[SPF Record]")
		txtRecords, err := net.LookupTXT(domain)
		spfFound := false
		if err == nil {
			for _, txt := range txtRecords {
				if strings.HasPrefix(txt, "v=spf1") {
					spfFound = true
					score++
					fmt.Println("  PASS - SPF record found:")
					fmt.Printf("    %s\n", txt)

					if strings.Contains(txt, cfg.SMTP.Host) || strings.Contains(txt, "include:") {
						fmt.Println("    Your SMTP host appears to be authorized.")
					} else {
						fmt.Println("    WARNING: Your SMTP host may not be included.")
						fmt.Printf("    Consider adding: include:%s\n", cfg.SMTP.Host)
					}
				}
			}
		}
		if !spfFound {
			fmt.Println("  FAIL - No SPF record found")
			fmt.Println("  Add a TXT record to your domain:")
			fmt.Printf("    v=spf1 include:%s ~all\n", guessSPFInclude(cfg.SMTP.Host))
		}
		fmt.Println()

		// 3. DKIM
		fmt.Println("[DKIM Record]")
		dkimSelectors := []string{"default", "mail", "google", "selector1", "selector2", "k1"}
		dkimFound := false
		for _, sel := range dkimSelectors {
			dkimDomain := fmt.Sprintf("%s._domainkey.%s", sel, domain)
			records, err := net.LookupTXT(dkimDomain)
			if err == nil && len(records) > 0 {
				for _, r := range records {
					if strings.Contains(r, "v=DKIM1") || strings.Contains(r, "p=") {
						dkimFound = true
						score++
						fmt.Printf("  PASS - DKIM record found (selector: %s)\n", sel)
						break
					}
				}
				if dkimFound {
					break
				}
			}
		}
		if !dkimFound {
			fmt.Println("  WARN - No DKIM record found (checked common selectors)")
			fmt.Println("  DKIM signing is usually configured by your SMTP provider.")
			fmt.Println("  Check your provider's docs for DKIM setup instructions.")
		}
		fmt.Println()

		// 4. DMARC
		fmt.Println("[DMARC Record]")
		dmarcRecords, err := net.LookupTXT("_dmarc." + domain)
		dmarcFound := false
		if err == nil {
			for _, txt := range dmarcRecords {
				if strings.HasPrefix(txt, "v=DMARC1") {
					dmarcFound = true
					score++
					fmt.Println("  PASS - DMARC record found:")
					fmt.Printf("    %s\n", txt)
				}
			}
		}
		if !dmarcFound {
			fmt.Println("  FAIL - No DMARC record found")
			fmt.Println("  Add a TXT record for _dmarc." + domain + ":")
			fmt.Printf("    v=DMARC1; p=none; rua=mailto:dmarc@%s\n", domain)
		}
		fmt.Println()

		// Summary
		fmt.Println(strings.Repeat("=", 50))
		fmt.Printf("Score: %d/%d\n", score, total)
		fmt.Println()

		switch {
		case score == total:
			fmt.Println("Excellent! Your domain is well-configured for email delivery.")
		case score >= 3:
			fmt.Println("Good setup. Fix the remaining issues for best deliverability.")
		case score >= 2:
			fmt.Println("Fair. Missing records may cause emails to land in spam.")
		default:
			fmt.Println("Poor. Multiple issues need attention for reliable delivery.")
		}

		fmt.Println()
		fmt.Println("Additional tips:")
		fmt.Println("  - Use a dedicated sending domain (e.g., mail.yourdomain.com)")
		fmt.Println("  - Warm up new domains: start with small batches")
		fmt.Println("  - Always include an unsubscribe link (InkDrift does this)")
		fmt.Println("  - Keep bounce rates below 2%")
		fmt.Println("  - Send both HTML and plaintext (InkDrift does this)")
	},
}

func init() {
	rootCmd.AddCommand(deliverabilityCmd)
}

func guessSPFInclude(smtpHost string) string {
	host := strings.ToLower(smtpHost)
	switch {
	case strings.Contains(host, "hostinger"):
		return "_spf.hostinger.com"
	case strings.Contains(host, "hetzner") || strings.Contains(host, "your-server.de"):
		return "_spf.hetzner.com"
	case strings.Contains(host, "contabo"):
		return "mail.contabo.de"
	case strings.Contains(host, "gmail") || strings.Contains(host, "google"):
		return "_spf.google.com"
	case strings.Contains(host, "outlook") || strings.Contains(host, "office365"):
		return "spf.protection.outlook.com"
	default:
		return smtpHost
	}
}
