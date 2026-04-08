package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/manifest"
	"github.com/vibeserve/vibeserve/internal/router"
	"github.com/vibeserve/vibeserve/internal/runtime"
	"github.com/vibeserve/vibeserve/internal/store"
)

var version = "0.1.0"

func main() {
	rootCmd := &cobra.Command{
		Use:   "vibeserve",
		Short: "AI-powered stateful API backend from natural language",
	}

	rootCmd.AddCommand(upCmd())
	rootCmd.AddCommand(versionCmd())
	rootCmd.AddCommand(routesCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func upCmd() *cobra.Command {
	var port int
	var host string
	var manifestPath string

	cmd := &cobra.Command{
		Use:   "up",
		Short: "Start the API server from an existing manifest",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUp(manifestPath, host, port)
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 8080, "Server port")
	cmd.Flags().StringVar(&host, "host", "localhost", "Server host")
	cmd.Flags().StringVarP(&manifestPath, "manifest", "m", ".vibe/manifest.json", "Path to manifest.json")

	return cmd
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("vibeserve", version)
		},
	}
}

func routesCmd() *cobra.Command {
	var manifestPath string

	cmd := &cobra.Command{
		Use:   "routes",
		Short: "Print the route table from a manifest",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := manifest.LoadFromFile(manifestPath)
			if err != nil {
				return fmt.Errorf("load manifest: %w", err)
			}
			for _, r := range m.Routes {
				fmt.Printf("  %-6s %s  → %s\n", r.Method, r.Path, r.Script)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&manifestPath, "manifest", "m", ".vibe/manifest.json", "Path to manifest.json")
	return cmd
}

func runUp(manifestPath, host string, port int) error {
	m, err := manifest.LoadFromFile(manifestPath)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}
	log.Printf("Loaded manifest: %s (%d routes, %d schemas)", m.Name, len(m.Routes), len(m.Schemas))

	if err := manifest.Validate(m); err != nil {
		return fmt.Errorf("invalid manifest: %w", err)
	}
	log.Println("Manifest validated")

	bus := engine.NewBus()
	bus.Subscribe(engine.EventLogEmitted, func(e engine.Event) {
		if data, ok := e.Data.(map[string]string); ok {
			log.Printf("[%s] %s", data["level"], data["message"])
		}
	})

	os.MkdirAll(".vibe", 0o755)

	s, err := store.New(".vibe/state.db")
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	if err := s.ApplySchemas(m.Schemas); err != nil {
		return fmt.Errorf("apply schemas: %w", err)
	}
	for _, schema := range m.Schemas {
		log.Printf("Schema applied: %s (%d columns)", schema.Table, len(schema.Columns))
	}

	for _, seed := range m.Seeds {
		if err := s.Seed(seed.Table, seed.Rows); err != nil {
			return fmt.Errorf("seed %s: %w", seed.Table, err)
		}
		log.Printf("Seeded %s: %d rows", seed.Table, len(seed.Rows))
	}

	trie := router.NewTrie()
	scripts := make(map[string]string)
	for _, sc := range m.Scripts {
		scripts[sc.Name] = sc.Code
	}
	for _, r := range m.Routes {
		trie.Insert(r.Method, r.Path, r.Script)
		log.Printf("Route registered: %s %s → %s", r.Method, r.Path, r.Script)
	}

	rt := runtime.New(s, bus)
	handler := router.NewHandler(trie, scripts, rt, true)
	srv := router.NewServer(host, port, handler)

	manifestData, _ := json.MarshalIndent(m, "", "  ")
	os.WriteFile(".vibe/manifest.json", manifestData, 0o644)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	select {
	case err := <-errCh:
		return err
	case <-sigCh:
		log.Println("Shutting down...")
		return srv.Shutdown(context.Background())
	}
}
