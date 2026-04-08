package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/vibeserve/vibeserve/internal/config"
	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/llm"
	"github.com/vibeserve/vibeserve/internal/manifest"
	"github.com/vibeserve/vibeserve/internal/router"
	"github.com/vibeserve/vibeserve/internal/runtime"
	"github.com/vibeserve/vibeserve/internal/snapshot"
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
	rootCmd.AddCommand(devCmd())
	rootCmd.AddCommand(undoCmd())

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

func devCmd() *cobra.Command {
	var port int
	var host string
	var manifestPath string
	var configPath string

	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Start the API server in interactive REPL mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDev(configPath, manifestPath, host, port)
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 0, "Server port (overrides config)")
	cmd.Flags().StringVar(&host, "host", "", "Server host (overrides config)")
	cmd.Flags().StringVarP(&manifestPath, "manifest", "m", ".vibe/manifest.json", "Path to manifest.json")
	cmd.Flags().StringVarP(&configPath, "config", "c", ".vibe/config.yaml", "Path to config.yaml")

	return cmd
}

func undoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "undo",
		Short: "Restore the most recent database snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			snap, err := snapshot.RestoreLatest(".vibe", ".vibe/state.db")
			if err != nil {
				return fmt.Errorf("undo failed: %w", err)
			}
			fmt.Printf("Restored snapshot #%d: %s\n", snap.ID, snap.Description)
			fmt.Println("Note: restart the server to apply the restored state.")
			return nil
		},
	}
}

func createProvider(cfg *config.Config) (llm.Provider, error) {
	switch cfg.Provider {
	case "claude":
		apiKey := cfg.APIKey()
		if apiKey == "" {
			return nil, fmt.Errorf("provider 'claude' requires %s environment variable to be set", cfg.APIKeyEnv)
		}
		return llm.NewClaudeProvider(apiKey, cfg.Model), nil
	case "ollama":
		return llm.NewOllamaProvider(cfg.OllamaHost, cfg.Model), nil
	default:
		return nil, fmt.Errorf("unknown provider %q (supported: claude, ollama)", cfg.Provider)
	}
}

func runDev(configPath, manifestPath, host string, port int) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Flag overrides take precedence over config
	if host != "" {
		cfg.Server.Host = host
	}
	if port != 0 {
		cfg.Server.Port = port
	}

	provider, err := createProvider(cfg)
	if err != nil {
		return fmt.Errorf("create provider: %w", err)
	}

	// Load or start fresh manifest
	var m *manifest.Manifest
	if _, statErr := os.Stat(manifestPath); statErr == nil {
		m, err = manifest.LoadFromFile(manifestPath)
		if err != nil {
			return fmt.Errorf("load manifest: %w", err)
		}
		log.Printf("Loaded manifest: %s (%d routes, %d schemas)", m.Name, len(m.Routes), len(m.Schemas))
	} else {
		m = nil
		log.Println("No manifest found; starting fresh")
	}

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

	if m != nil {
		if err := s.ApplySchemas(m.Schemas); err != nil {
			return fmt.Errorf("apply schemas: %w", err)
		}
		for _, seed := range m.Seeds {
			if err := s.Seed(seed.Table, seed.Rows); err != nil {
				return fmt.Errorf("seed %s: %w", seed.Table, err)
			}
		}
	}

	trie := router.NewTrie()
	scripts := make(map[string]string)
	if m != nil {
		for _, sc := range m.Scripts {
			scripts[sc.Name] = sc.Code
		}
		for _, r := range m.Routes {
			trie.Insert(r.Method, r.Path, r.Script)
		}
	}

	rt := runtime.New(s, bus)
	handler := router.NewHandler(trie, scripts, rt, cfg.Server.CORS)
	srv := router.NewServer(cfg.Server.Host, cfg.Server.Port, handler)

	eng := engine.NewEngine(engine.EngineConfig{
		Bus:      bus,
		Store:    s,
		Trie:     trie,
		Scripts:  scripts,
		Provider: provider,
		Manifest: m,
		VibeDir:  ".vibe",
		StoreOpener: func(dsn string) (engine.SchemaStore, error) {
			return store.New(dsn)
		},
	})

	// Start HTTP server in background
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- srv.Start()
	}()
	log.Printf("Server running at http://%s:%d", cfg.Server.Host, cfg.Server.Port)

	// Handle OS signals for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// REPL loop
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("VibeServe dev mode. Type a prompt, or: quit, exit, undo, routes, status")

	replDone := make(chan struct{})
	go func() {
		defer close(replDone)
		for {
			fmt.Print("vibe> ")
			if !scanner.Scan() {
				break
			}
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			switch strings.ToLower(line) {
			case "quit", "exit":
				cancel()
				return

			case "undo":
				if err := eng.Undo(); err != nil {
					fmt.Fprintf(os.Stderr, "undo error: %v\n", err)
				} else {
					fmt.Println("Undo successful.")
				}

			case "routes":
				cur := eng.Manifest()
				if cur == nil || len(cur.Routes) == 0 {
					fmt.Println("No routes defined.")
				} else {
					for _, r := range cur.Routes {
						fmt.Printf("  %-6s %s  → %s\n", r.Method, r.Path, r.Script)
					}
				}

			case "status":
				cur := eng.Manifest()
				if cur == nil {
					fmt.Println("No manifest loaded.")
				} else {
					fmt.Printf("Manifest: %s | Routes: %d | Schemas: %d\n",
						cur.Name, len(cur.Routes), len(cur.Schemas))
				}

			default:
				result, err := eng.Apply(ctx, line)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error: %v\n", err)
				} else {
					fmt.Print(engine.FormatChangeSummary(result))
				}
			}
		}
	}()

	select {
	case err := <-serverErrCh:
		cancel()
		<-replDone
		return err
	case <-sigCh:
		log.Println("Shutting down...")
		cancel()
		<-replDone
		return srv.Shutdown(context.Background())
	case <-ctx.Done():
		log.Println("Shutting down...")
		return srv.Shutdown(context.Background())
	}
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
