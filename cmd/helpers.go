package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/artaeon/inkdrift/internal/config"
	"github.com/artaeon/inkdrift/internal/db"
)

func loadConfig() *config.Config {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
	return cfg
}

func openDB(cfg *config.Config) *db.DB {
	database, err := db.Open(cfg.DB.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	return database
}

func prompt(label string) string {
	fmt.Printf("%s: ", label)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	return strings.TrimSpace(scanner.Text())
}

func promptDefault(label, defaultVal string) string {
	fmt.Printf("%s [%s]: ", label, defaultVal)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	val := strings.TrimSpace(scanner.Text())
	if val == "" {
		return defaultVal
	}
	return val
}

func confirm(label string) bool {
	fmt.Printf("%s [y/N]: ", label)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	val := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return val == "y" || val == "yes"
}

// shortID safely truncates an ID for display
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
