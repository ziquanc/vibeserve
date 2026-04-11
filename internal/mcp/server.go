package mcp

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/vibeserve/vibeserve/internal/config"
	"github.com/vibeserve/vibeserve/internal/engine"
	"github.com/vibeserve/vibeserve/internal/llm"
	"github.com/vibeserve/vibeserve/internal/manifest"
	"github.com/vibeserve/vibeserve/internal/router"
	"github.com/vibeserve/vibeserve/internal/store"
)

// Server wraps the MCP server and the VibeServe engine.
type Server struct {
	eng      *engine.Engine
	store    *store.Store
	provider llm.Provider
	vibeDir  string
}

// Run creates the Engine, registers MCP tools, and starts stdio transport.
func Run(vibeDir string) error {
	srv, err := newServer(vibeDir)
	if err != nil {
		return err
	}

	s := mcpserver.NewMCPServer(
		"VibeServe",
		"1.0.0",
		mcpserver.WithToolCapabilities(true),
	)

	srv.registerTools(s)

	log.Println("[mcp] Starting VibeServe MCP server (stdio)")
	return mcpserver.ServeStdio(s)
}

func newServer(vibeDir string) (*Server, error) {
	if err := os.MkdirAll(vibeDir, 0o755); err != nil {
		return nil, fmt.Errorf("create vibe dir: %w", err)
	}

	// Load config.
	configPath := filepath.Join(vibeDir, "config.yaml")
	cfg, err := config.Load(configPath)
	if err != nil {
		cfg = config.DefaultConfig()
	}

	// Create LLM provider.
	var provider llm.Provider
	provider, err = createProvider(cfg)
	if err != nil {
		log.Printf("[mcp] Warning: LLM provider not available: %v", err)
		provider = nil
	}

	// Open store.
	dbPath := filepath.Join(vibeDir, "state.db")
	s, err := store.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Load manifest.
	manifestPath := filepath.Join(vibeDir, "manifest.json")
	var m *manifest.Manifest
	if _, statErr := os.Stat(manifestPath); statErr == nil {
		m, err = manifest.LoadFromFile(manifestPath)
		if err != nil {
			log.Printf("[mcp] Warning: failed to load manifest: %v", err)
		}
	}

	// Build trie + scripts from manifest.
	trie := router.NewTrie()
	scripts := make(map[string]string)
	if m != nil {
		for _, sc := range m.Scripts {
			scripts[sc.Name] = sc.Code
		}
		for _, r := range m.Routes {
			trie.Insert(r.Method, r.Path, r.Script)
		}
		if err := s.ApplySchemas(m.Schemas); err != nil {
			log.Printf("[mcp] Warning: schema apply: %v", err)
		}
		for _, seed := range m.Seeds {
			_ = s.Seed(seed.Table, seed.Rows)
		}
	}

	bus := engine.NewBus()
	eng := engine.NewEngine(engine.EngineConfig{
		Bus:      bus,
		Store:    s,
		Trie:     trie,
		Scripts:  scripts,
		Provider: provider,
		Manifest: m,
		VibeDir:  vibeDir,
		StoreOpener: func(dsn string) (engine.SchemaStore, error) {
			return store.New(dsn)
		},
	})

	return &Server{
		eng:      eng,
		store:    s,
		provider: provider,
		vibeDir:  vibeDir,
	}, nil
}

func createProvider(cfg *config.Config) (llm.Provider, error) {
	switch cfg.Provider {
	case "claude":
		apiKey := cfg.APIKey()
		if apiKey == "" {
			return nil, fmt.Errorf("provider 'claude' requires %s environment variable to be set", cfg.APIKeyEnv)
		}
		return llm.NewClaudeProvider(apiKey, cfg.Model), nil
	case "openai":
		apiKey := cfg.APIKey()
		if apiKey == "" {
			return nil, fmt.Errorf("provider 'openai' requires %s environment variable to be set", cfg.APIKeyEnv)
		}
		if cfg.BaseURL == "" {
			return nil, fmt.Errorf("provider 'openai' requires base_url in config")
		}
		var opts []llm.OpenAIOption
		if cfg.MaxTokens > 0 {
			opts = append(opts, llm.WithOpenAIMaxTokens(cfg.MaxTokens))
		}
		return llm.NewOpenAIProvider(apiKey, cfg.Model, cfg.BaseURL, opts...), nil
	case "ollama":
		return llm.NewOllamaProvider(cfg.OllamaHost, cfg.Model), nil
	default:
		return nil, fmt.Errorf("unknown provider %q (supported: claude, openai, ollama)", cfg.Provider)
	}
}

func (srv *Server) registerTools(s *mcpserver.MCPServer) {
	s.AddTool(
		mcplib.NewTool("list_routes",
			mcplib.WithDescription("List all API routes with their HTTP method, path, and description."),
		),
		srv.handleListRoutes,
	)

	s.AddTool(
		mcplib.NewTool("list_tables",
			mcplib.WithDescription("List all database tables with their columns, types, constraints, and row counts."),
		),
		srv.handleListTables,
	)

	s.AddTool(
		mcplib.NewTool("get_api_status",
			mcplib.WithDescription("Show the current API status: number of tables, routes, and project paths."),
		),
		srv.handleGetAPIStatus,
	)
}
