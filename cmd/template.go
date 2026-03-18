package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var templateCmd = &cobra.Command{
	Use:     "template",
	Aliases: []string{"templates", "tmpl"},
	Short:   "Manage email templates",
}

var tmplCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a template from a file",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		database := openDB(cfg)
		defer database.Close()

		name := ""
		if len(args) > 0 {
			name = args[0]
		} else {
			name = prompt("Template name")
		}

		file, _ := cmd.Flags().GetString("file")
		if file == "" {
			file = prompt("HTML file path")
		}

		data, err := os.ReadFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
			os.Exit(1)
		}

		t, err := database.CreateTemplate(name, string(data))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Template created: %s (ID: %s)\n", t.Name, shortID(t.ID))
	},
}

var tmplListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List all templates",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		database := openDB(cfg)
		defer database.Close()

		templates, err := database.ListTemplates()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if len(templates) == 0 {
			fmt.Println("No templates yet. Create one with: inkdrift template create")
			fmt.Println("Default templates are in the templates/ directory.")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tCREATED")
		fmt.Fprintln(w, "--\t----\t-------")
		for _, t := range templates {
			id := t.ID
			if len(id) > 8 {
				id = id[:8]
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", id, t.Name, t.CreatedAt.Format("2006-01-02"))
		}
		w.Flush()
	},
}

var tmplDeleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete a template",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		database := openDB(cfg)
		defer database.Close()

		if err := database.DeleteTemplate(args[0]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Template deleted.")
	},
}

func init() {
	rootCmd.AddCommand(templateCmd)
	templateCmd.AddCommand(tmplCreateCmd)
	templateCmd.AddCommand(tmplListCmd)
	templateCmd.AddCommand(tmplDeleteCmd)

	tmplCreateCmd.Flags().StringP("file", "f", "", "HTML template file")
}
