package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"lists"},
	Short:   "Manage subscriber lists",
}

var listCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a new subscriber list",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		database := openDB(cfg)
		defer database.Close()

		name := ""
		if len(args) > 0 {
			name = args[0]
		} else {
			name = prompt("List name")
		}

		desc, _ := cmd.Flags().GetString("description")
		if desc == "" {
			desc = prompt("Description (optional)")
		}

		list, err := database.CreateList(name, desc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("List created: %s (ID: %s)\n", list.Name, list.ID)
	},
}

var listListCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"show"},
	Short:   "Show all lists",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		database := openDB(cfg)
		defer database.Close()

		lists, err := database.ListLists()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if len(lists) == 0 {
			fmt.Println("No lists yet. Create one with: inkdrift list create \"My Newsletter\"")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tSUBSCRIBERS\tDESCRIPTION")
		fmt.Fprintln(w, "--\t----\t-----------\t-----------")
		for _, l := range lists {
			count, _ := database.ListSubscriberCount(l.ID)
			id := l.ID
			if len(id) > 8 {
				id = id[:8]
			}
			fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", id, l.Name, count, l.Description)
		}
		w.Flush()
	},
}

var listDeleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete a list and all its subscribers",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		database := openDB(cfg)
		defer database.Close()

		id := args[0]
		list, err := database.GetList(id)
		if err != nil {
			// Try by name
			list, err = database.GetListByName(id)
			if err != nil {
				fmt.Fprintf(os.Stderr, "List not found: %s\n", id)
				os.Exit(1)
			}
		}

		force, _ := cmd.Flags().GetBool("force")
		if !force {
			if !confirm(fmt.Sprintf("Delete list '%s' and all subscribers?", list.Name)) {
				fmt.Println("Cancelled.")
				return
			}
		}

		if err := database.DeleteList(list.ID); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("List '%s' deleted.\n", list.Name)
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.AddCommand(listCreateCmd)
	listCmd.AddCommand(listListCmd)
	listCmd.AddCommand(listDeleteCmd)

	listCreateCmd.Flags().StringP("description", "d", "", "List description")
	listDeleteCmd.Flags().BoolP("force", "f", false, "Skip confirmation")
}
