// Command gateway-api is the main entry point for the Nexus AI Gateway data & control plane.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/auth"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/config"
	gateway "github.com/LukasdeSouza/nexus-ai-gateway/internal/http"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/http/handlers"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/observability"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/provider"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/provider/anthropic"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/provider/gemini"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/provider/openai"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/ratelimit"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/storage/postgres"
	redisstore "github.com/LukasdeSouza/nexus-ai-gateway/internal/storage/redis"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/tenant"
	"github.com/LukasdeSouza/nexus-ai-gateway/internal/usage"
)

func main() {
	// 1. Load configuration
	cfg, err := config.Load()
	if err != nil {
		panic("failed to load configuration: " + err.Error())
	}

	// 2. Initialize logger
	logger, err := observability.NewLogger(cfg.Observability.LogLevel, cfg.Server.Env)
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}
	defer logger.Sync()

	logger.Info("starting nexus ai gateway",
		zap.String("env", cfg.Server.Env),
		zap.Int("port", cfg.Server.Port),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 3. OpenTelemetry Tracing
	otelSDK, err := observability.Setup(ctx, cfg.Observability)
	if err != nil {
		logger.Warn("failed to setup OpenTelemetry tracing, continuing without it", zap.Error(err))
	} else {
		defer func() {
			shutdownCtx, sCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer sCancel()
			_ = otelSDK.Shutdown(shutdownCtx)
		}()
	}

	// 4. Prometheus Metrics
	metrics := observability.NewMetrics(nil)

	// 5. Database Connection & Schema Migrations
	pgPool, err := postgres.NewPool(ctx, cfg.Database)
	if err != nil {
		logger.Warn("database connection failed (will retry or continue in standalone mode)", zap.Error(err))
	} else {
		defer pgPool.Close()
		if err := postgres.RunMigrations(cfg.Database.URL, cfg.Database.MigrationsPath); err != nil {
			logger.Warn("migrations warning", zap.Error(err))
		}
	}

	// 6. Redis Connection
	var redisClient *redisstore.Client
	redisClient, err = redisstore.NewClient(cfg.Redis)
	if err != nil {
		logger.Warn("redis connection failed (rate limiting will fail-open)", zap.Error(err))
	} else {
		defer redisClient.Close()
	}

	// 7. Initialize Repositories
	var (
		orgRepo    *postgres.OrganizationRepo
		prjRepo    *postgres.ProjectRepo
		keyRepo    *postgres.APIKeyRepo
		pconnRepo  *postgres.ProviderRepo
		policyRepo *postgres.RoutingPolicyRepo
		aliasRepo  *postgres.ModelAliasRepo
		recRepo    *postgres.RequestRecordRepo
	)
	var taskRepo handlers.TaskStore
	if pgPool != nil {
		orgRepo = postgres.NewOrganizationRepo(pgPool)
		prjRepo = postgres.NewProjectRepo(pgPool)
		keyRepo = postgres.NewAPIKeyRepo(pgPool)
		pconnRepo = postgres.NewProviderRepo(pgPool)
		policyRepo = postgres.NewRoutingPolicyRepo(pgPool)
		aliasRepo = postgres.NewModelAliasRepo(pgPool)
		recRepo = postgres.NewRequestRecordRepo(pgPool)
		taskRepo = postgres.NewTaskRepo(pgPool)

		// Seed baseline model aliases
		if err := aliasRepo.SeedDefaults(ctx); err != nil {
			logger.Warn("failed to seed default model aliases", zap.Error(err))
		}
	}

	// 8. Register Providers
	reg := provider.NewRegistry()
	openaiKey := os.Getenv("OPENAI_API_KEY")
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	geminiKey := os.Getenv("GEMINI_API_KEY")

	reg.Register(openai.NewAdapter(cfg.Providers.OpenAIBaseURL, openaiKey))
	reg.Register(anthropic.NewAdapter(cfg.Providers.AnthropicBaseURL, anthropicKey))
	reg.Register(gemini.NewAdapter(cfg.Providers.GeminiBaseURL, geminiKey))

	// 9. Auth, Tenant, and Rate Limiting
	validator := auth.NewValidator(keyRepo, redisClient, 5*time.Minute)
	tenantResolver := tenant.NewResolver(prjRepo, orgRepo)
	limiter := ratelimit.NewLimiter(redisClient)
	usageProducer := usage.NewNoopProducer()

	// 10. Wire HTTP Handlers
	chatH := handlers.NewChatHandler(handlers.ChatHandlerConfig{
		Validator:      validator,
		TenantResolver: tenantResolver,
		RateLimiter:    limiter,
		ProviderReg:    reg,
		PolicyStore:    policyRepo,
		RequestStore:   recRepo,
		UsageProducer:  usageProducer,
		Metrics:        metrics,
		Logger:         logger,
		DefaultRateLimit: ratelimit.Limit{
			Requests: 120,
			Window:   time.Minute,
		},
	})

	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
	if supabaseKey == "" {
		supabaseKey = os.Getenv("SUPABASE_ANON_KEY")
	}

	cliAuthH := handlers.NewCliAuthHandler(handlers.CliAuthHandlerConfig{
		OrgStore:     orgRepo,
		ProjectStore: prjRepo,
		KeyStore:     keyRepo,
		SupabaseURL:  supabaseURL,
		SupabaseKey:  supabaseKey,
		Logger:       logger,
	})

	server := gateway.New(gateway.Dependencies{
		Config:               cfg,
		Logger:               logger,
		Metrics:              metrics,
		ChatHandler:          chatH.ServeHTTP,
		CliAuthHandler:       cliAuthH.ServeHTTP,
		CliRotateHandler:     cliAuthH.RotateKey,
		ModelsHandler:        handlers.NewModelsHandler(aliasRepo),
		OrganizationsHandler: handlers.NewOrganizationsRouter(orgRepo),
		ProjectsHandler:      handlers.NewProjectsRouter(prjRepo),
		APIKeysHandler:       handlers.NewAPIKeysRouter(keyRepo),
		ProvidersHandler:     handlers.NewProvidersRouter(pconnRepo),
		UsageHandler:         handlers.NewUsageHandler(recRepo),
		RequestsHandler:      handlers.NewRequestsHandler(recRepo),
		TasksHandler:         handlers.NewTasksRouter(taskRepo),
	})

	// 11. Handle Graceful Shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("shutdown signal received", zap.String("signal", sig.String()))
		cancel()
	}()

	if err := server.Start(ctx); err != nil {
		logger.Error("server terminated with error", zap.Error(err))
		os.Exit(1)
	}
	logger.Info("server exited cleanly")
}
