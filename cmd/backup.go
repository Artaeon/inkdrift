package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Manage database backups",
}

var backupCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a database backup",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()

		dir, _ := cmd.Flags().GetString("dir")
		if dir == "" {
			dir = "backups"
		}

		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating backup directory: %v\n", err)
			os.Exit(1)
		}

		timestamp := time.Now().Format("20060102-150405")
		backupName := fmt.Sprintf("inkdrift-%s.db", timestamp)
		backupPath := filepath.Join(dir, backupName)

		src, err := os.Open(cfg.DB.Path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer src.Close()

		dst, err := os.Create(backupPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating backup: %v\n", err)
			os.Exit(1)
		}
		defer dst.Close()

		bytes, err := io.Copy(dst, src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error copying database: %v\n", err)
			os.Exit(1)
		}

		// Also backup WAL file if it exists
		walPath := cfg.DB.Path + "-wal"
		if _, err := os.Stat(walPath); err == nil {
			if err := copyFile(walPath, backupPath+"-wal"); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not backup WAL file: %v\n", err)
			}
		}

		fmt.Printf("Backup created: %s (%.1f KB)\n", backupPath, float64(bytes)/1024)

		// Enforce retention: keep max 10 backups
		maxBackups, _ := cmd.Flags().GetInt("max")
		enforceRetention(dir, maxBackups)
	},
}

var backupListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List available backups",
	Run: func(cmd *cobra.Command, args []string) {
		dir, _ := cmd.Flags().GetString("dir")
		if dir == "" {
			dir = "backups"
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			fmt.Println("No backups found.")
			return
		}

		var backups []os.DirEntry
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "inkdrift-") && strings.HasSuffix(e.Name(), ".db") {
				backups = append(backups, e)
			}
		}

		if len(backups) == 0 {
			fmt.Println("No backups found. Create one with: inkdrift backup create")
			return
		}

		fmt.Printf("Backups in %s/:\n", dir)
		for _, b := range backups {
			info, err := b.Info()
			if err != nil {
				fmt.Printf("  %s\n", b.Name())
				continue
			}
			fmt.Printf("  %s  (%.1f KB)\n", b.Name(), float64(info.Size())/1024)
		}
		fmt.Printf("\n%d backup(s) total\n", len(backups))
	},
}

var backupRestoreCmd = &cobra.Command{
	Use:   "restore [backup-file]",
	Short: "Restore database from a backup",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		backupPath := args[0]

		// Validate backup path — must be a .db file, no path traversal
		absPath, err := filepath.Abs(backupPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid path: %v\n", err)
			os.Exit(1)
		}

		if !strings.HasSuffix(absPath, ".db") {
			fmt.Fprintln(os.Stderr, "Backup file must be a .db file")
			os.Exit(1)
		}

		if _, err := os.Stat(absPath); err != nil {
			fmt.Fprintf(os.Stderr, "Backup file not found: %s\n", backupPath)
			os.Exit(1)
		}

		force, _ := cmd.Flags().GetBool("force")
		if !force {
			if !confirm("This will replace the current database. Continue?") {
				fmt.Println("Cancelled.")
				return
			}
		}

		// Create a safety backup of current DB before restoring
		if _, err := os.Stat(cfg.DB.Path); err == nil {
			safetyPath := cfg.DB.Path + ".pre-restore"
			if err := copyFile(cfg.DB.Path, safetyPath); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not create safety backup: %v\n", err)
			} else {
				fmt.Printf("Safety backup: %s\n", safetyPath)
			}
		}

		if err := copyFile(absPath, cfg.DB.Path); err != nil {
			fmt.Fprintf(os.Stderr, "Error restoring: %v\n", err)
			os.Exit(1)
		}

		// Set restrictive permissions on restored database
		os.Chmod(cfg.DB.Path, 0o600)

		// Remove stale WAL/SHM files
		os.Remove(cfg.DB.Path + "-wal")
		os.Remove(cfg.DB.Path + "-shm")

		fmt.Printf("Database restored from %s\n", backupPath)
	},
}

func init() {
	rootCmd.AddCommand(backupCmd)
	backupCmd.AddCommand(backupCreateCmd)
	backupCmd.AddCommand(backupListCmd)
	backupCmd.AddCommand(backupRestoreCmd)

	backupCreateCmd.Flags().String("dir", "backups", "Backup directory")
	backupCreateCmd.Flags().Int("max", 10, "Maximum number of backups to keep")
	backupListCmd.Flags().String("dir", "backups", "Backup directory")
	backupRestoreCmd.Flags().BoolP("force", "f", false, "Skip confirmation")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("creating destination: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copying data: %w", err)
	}

	return out.Close()
}

func enforceRetention(dir string, maxBackups int) {
	if maxBackups <= 0 {
		maxBackups = 10
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	var backups []os.DirEntry
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "inkdrift-") && strings.HasSuffix(e.Name(), ".db") {
			backups = append(backups, e)
		}
	}

	if len(backups) <= maxBackups {
		return
	}

	// Sort by name (which is by timestamp) ascending
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Name() < backups[j].Name()
	})

	// Remove oldest
	toRemove := len(backups) - maxBackups
	for i := 0; i < toRemove; i++ {
		path := filepath.Join(dir, backups[i].Name())
		os.Remove(path)
		os.Remove(path + "-wal")
		fmt.Printf("Removed old backup: %s\n", backups[i].Name())
	}
}
