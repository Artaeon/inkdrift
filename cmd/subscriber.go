package cmd

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/artaeon/inkdrift/internal/db"
	"github.com/spf13/cobra"
)

var subscriberCmd = &cobra.Command{
	Use:     "subscriber",
	Aliases: []string{"sub", "subscribers"},
	Short:   "Manage subscribers",
}

var subAddCmd = &cobra.Command{
	Use:   "add [email]",
	Short: "Add a subscriber to a list",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		database := openDB(cfg)
		defer database.Close()

		email := ""
		if len(args) > 0 {
			email = args[0]
		} else {
			email = prompt("Email")
		}

		name, _ := cmd.Flags().GetString("name")
		listName, _ := cmd.Flags().GetString("list")

		listID := resolveListID(database, listName)

		sub, err := database.AddSubscriber(email, name, listID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Subscriber added: %s (ID: %s)\n", sub.Email, shortID(sub.ID))
	},
}

var subListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List subscribers",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		database := openDB(cfg)
		defer database.Close()

		listName, _ := cmd.Flags().GetString("list")
		listID := resolveListID(database, listName)

		subs, err := database.ListSubscribers(listID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if len(subs) == 0 {
			fmt.Println("No subscribers yet.")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tEMAIL\tNAME\tSTATUS\tSUBSCRIBED")
		fmt.Fprintln(w, "--\t-----\t----\t------\t----------")
		for _, s := range subs {
			id := s.ID
			if len(id) > 8 {
				id = id[:8]
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				id, s.Email, s.Name, s.Status, s.SubscribedAt.Format("2006-01-02"))
		}
		w.Flush()
	},
}

var subRemoveCmd = &cobra.Command{
	Use:   "remove [email]",
	Short: "Remove a subscriber",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		database := openDB(cfg)
		defer database.Close()

		listName, _ := cmd.Flags().GetString("list")
		listID := resolveListID(database, listName)

		email := args[0]
		if err := database.UnsubscribeByEmail(email, listID); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Subscriber %s unsubscribed.\n", email)
	},
}

var subImportCmd = &cobra.Command{
	Use:   "import [file.csv]",
	Short: "Import subscribers from a CSV file (email,name)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		database := openDB(cfg)
		defer database.Close()

		listName, _ := cmd.Flags().GetString("list")
		listID := resolveListID(database, listName)

		// Check file size before reading (max 50MB)
		info, err := os.Stat(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if info.Size() > 50*1024*1024 {
			fmt.Fprintln(os.Stderr, "CSV file too large (max 50MB)")
			os.Exit(1)
		}

		f, err := os.Open(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()

		reader := csv.NewReader(f)
		records, err := reader.ReadAll()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading CSV: %v\n", err)
			os.Exit(1)
		}

		const maxImport = 100000
		if len(records) > maxImport {
			fmt.Fprintf(os.Stderr, "CSV has %d rows, max import is %d per batch\n", len(records), maxImport)
			os.Exit(1)
		}

		var entries []struct{ Email, Name string }
		for i, record := range records {
			if len(record) == 0 {
				continue
			}
			if i == 0 && (strings.EqualFold(record[0], "email") || strings.EqualFold(record[0], "e-mail")) {
				continue // skip header
			}
			email := strings.TrimSpace(strings.ToLower(record[0]))
			name := ""
			if len(record) > 1 {
				name = strings.TrimSpace(record[1])
			}
			if len(name) > 200 {
				name = name[:200]
			}
			if email != "" && strings.Contains(email, "@") && len(email) <= 254 {
				entries = append(entries, struct{ Email, Name string }{email, name})
			}
		}

		count, err := database.ImportSubscribers(listID, entries)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error importing: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Imported %d subscribers (from %d entries).\n", count, len(entries))
	},
}

var subExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export subscribers to CSV",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		database := openDB(cfg)
		defer database.Close()

		listName, _ := cmd.Flags().GetString("list")
		listID := resolveListID(database, listName)

		subs, err := database.ListSubscribers(listID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		output, _ := cmd.Flags().GetString("output")
		var w *csv.Writer
		if output != "" {
			f, err := os.Create(output)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error creating file: %v\n", err)
				os.Exit(1)
			}
			defer f.Close()
			w = csv.NewWriter(f)
		} else {
			w = csv.NewWriter(os.Stdout)
		}
		defer w.Flush()

		w.Write([]string{"email", "name", "status", "subscribed_at"})
		for _, s := range subs {
			w.Write([]string{s.Email, s.Name, s.Status, s.SubscribedAt.Format("2006-01-02")})
		}

		if output != "" {
			fmt.Printf("Exported %d subscribers to %s\n", len(subs), output)
		}
	},
}

func init() {
	rootCmd.AddCommand(subscriberCmd)
	subscriberCmd.AddCommand(subAddCmd)
	subscriberCmd.AddCommand(subListCmd)
	subscriberCmd.AddCommand(subRemoveCmd)
	subscriberCmd.AddCommand(subImportCmd)
	subscriberCmd.AddCommand(subExportCmd)

	for _, c := range []*cobra.Command{subAddCmd, subListCmd, subRemoveCmd, subImportCmd, subExportCmd} {
		c.Flags().StringP("list", "l", "", "List name or ID")
	}

	subAddCmd.Flags().StringP("name", "n", "", "Subscriber name")
	subExportCmd.Flags().StringP("output", "o", "", "Output file (default: stdout)")
}

func resolveListID(database *db.DB, nameOrID string) string {
	if nameOrID != "" {
		// Try as name first
		list, err := database.GetListByName(nameOrID)
		if err == nil {
			return list.ID
		}
		// Try as ID
		list, err = database.GetList(nameOrID)
		if err == nil {
			return list.ID
		}
		fmt.Fprintf(os.Stderr, "List not found: %s\n", nameOrID)
		os.Exit(1)
	}

	// Use first list as default
	lists, err := database.ListLists()
	if err != nil || len(lists) == 0 {
		fmt.Fprintln(os.Stderr, "No lists found. Create one with: inkdrift list create \"My Newsletter\"")
		os.Exit(1)
	}

	if len(lists) > 1 {
		fmt.Println("Multiple lists found. Please specify with --list:")
		for _, l := range lists {
			fmt.Printf("  - %s (%s)\n", l.Name, shortID(l.ID))
		}
		os.Exit(1)
	}

	return lists[0].ID
}
