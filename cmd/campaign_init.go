package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var campaignInitCmd = &cobra.Command{
	Use:   "init [name]",
	Short: "Scaffold a new campaign directory with HTML template",
	Long: `Creates a campaign directory with a pre-built HTML email template
that you can edit in your favorite editor before creating the campaign.

Workflow:
  1. inkdrift campaign init "March Update"
  2. Edit campaigns/march-update/email.html
  3. inkdrift campaign create --name "March Update" --body-file campaigns/march-update/email.html
  4. inkdrift campaign preview <id> -o preview.html
  5. inkdrift campaign send <id>`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		slug := slugify(name)
		dir := filepath.Join("campaigns", slug)

		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		tmplChoice, _ := cmd.Flags().GetString("template")

		htmlContent := defaultCampaignHTML(name)
		if tmplChoice == "minimal" {
			htmlContent = minimalCampaignHTML(name)
		}

		emailPath := filepath.Join(dir, "email.html")
		if err := os.WriteFile(emailPath, []byte(htmlContent), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// Create a metadata file
		metaContent := fmt.Sprintf("name = %q\nsubject = %q\nlist = \"\"\n", name, name)
		metaPath := filepath.Join(dir, "campaign.toml")
		os.WriteFile(metaPath, []byte(metaContent), 0o644)

		fmt.Printf("Campaign scaffolded in %s/\n", dir)
		fmt.Println()
		fmt.Println("Files:")
		fmt.Printf("  %s  - edit your email content here\n", emailPath)
		fmt.Printf("  %s  - campaign metadata\n", metaPath)
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Printf("  1. Edit %s with your content\n", emailPath)
		fmt.Println("  2. Create the campaign:")
		fmt.Printf("     inkdrift campaign create --name %q --body-file %s --list \"Your List\"\n", name, emailPath)
	},
}

func init() {
	campaignCmd.AddCommand(campaignInitCmd)
	campaignInitCmd.Flags().StringP("template", "t", "default", "Template style: default, minimal")
}

func slugify(s string) string {
	var result []byte
	for _, c := range []byte(s) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-':
			result = append(result, c)
		case c >= 'A' && c <= 'Z':
			result = append(result, c+32)
		case c == ' ' || c == '_':
			result = append(result, '-')
		}
	}
	return string(result)
}

func defaultCampaignHTML(name string) string {
	return fmt.Sprintf(`<!-- InkDrift Campaign: %s -->
<!-- Edit this file with your newsletter content -->
<!-- Template variables: {{.SubscriberName}}, {{.UnsubscribeURL}}, {{.ListName}}, {{.Year}} -->

<h2>%s</h2>

<p>Hi {{.SubscriberName}},</p>

<p>Write your newsletter content here. You can use any HTML.</p>

<h3>What's New</h3>
<ul>
  <li>First update</li>
  <li>Second update</li>
  <li>Third update</li>
</ul>

<p>Thanks for reading!</p>

<!-- This content will be wrapped in your chosen email template (default or minimal) -->
<!-- The unsubscribe footer is added automatically -->
`, name, name)
}

func minimalCampaignHTML(name string) string {
	return fmt.Sprintf(`<!-- InkDrift Campaign: %s (minimal) -->

<p>Hi {{.SubscriberName}},</p>

<p>Your content here.</p>

<p>Best,<br>Your Name</p>
`, name)
}
