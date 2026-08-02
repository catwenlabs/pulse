package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/catwenlabs/pulse/internal/config"
	annotationdriver "github.com/catwenlabs/pulse/internal/drivers/annotations"
	"github.com/catwenlabs/pulse/internal/drivers/feed"
	filedriver "github.com/catwenlabs/pulse/internal/drivers/file"
	htmldriver "github.com/catwenlabs/pulse/internal/drivers/html"
	jsonapidriver "github.com/catwenlabs/pulse/internal/drivers/jsonapi"
	"github.com/catwenlabs/pulse/internal/drivers/push"
	"github.com/catwenlabs/pulse/internal/effect"
	"github.com/catwenlabs/pulse/internal/embedding"
	"github.com/catwenlabs/pulse/internal/events"
	"github.com/catwenlabs/pulse/internal/ingestion"
	"github.com/catwenlabs/pulse/internal/platform/httpclient"
	"github.com/catwenlabs/pulse/internal/preview"
	"github.com/catwenlabs/pulse/internal/scheduler"
	"github.com/catwenlabs/pulse/internal/security"
	"github.com/catwenlabs/pulse/internal/source"
	"github.com/catwenlabs/pulse/internal/storage/migrate"
	postgresstore "github.com/catwenlabs/pulse/internal/storage/postgres"
	"github.com/catwenlabs/pulse/internal/story"
	"github.com/catwenlabs/pulse/internal/transport/httpserver"
	"github.com/catwenlabs/pulse/internal/worker"
)

func main() {
	if err := run(); err != nil {
		slog.Error("pulse stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runContext(ctx, cfg)
}

func runContext(ctx context.Context, cfg config.Config, ready ...chan<- struct{}) error {
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open PostgreSQL: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping PostgreSQL: %w", err)
	}
	if err := migrate.Run(ctx, pool); err != nil {
		return fmt.Errorf("migrate PostgreSQL: %w", err)
	}
	changeHub := events.NewLibraryChangeHub()

	credentialCipher, err := security.NewCredentialCipher(cfg.MasterKey)
	if err != nil {
		return fmt.Errorf("configure source credential encryption: %w", err)
	}
	sourceStore := postgresstore.NewSourceStore(pool, credentialCipher)
	acquisitionStore := postgresstore.NewAcquisitionStore(pool)
	entryStore := postgresstore.NewEntryStore(pool, changeHub.PublishSource)
	storyStore := postgresstore.NewStoryStore(pool)
	opmlStore := postgresstore.NewOPMLStore(pool)
	organizationStore := postgresstore.NewOrganizationStore(pool)
	ruleStore := postgresstore.NewRuleStore(pool)
	safeHTTPClient := httpclient.New()
	registry, err := ingestion.NewRegistry(
		feed.New(safeHTTPClient),
		jsonapidriver.New(safeHTTPClient),
		htmldriver.New(safeHTTPClient),
		push.New(source.KindWebhook),
		push.NewManual(safeHTTPClient),
		filedriver.New(cfg.ImportRoots),
		annotationdriver.New(),
	)
	if err != nil {
		return fmt.Errorf("create driver registry: %w", err)
	}
	var embeddingProvider embedding.Provider
	if cfg.EmbeddingProvider == "ollama" {
		embeddingProvider, err = embedding.NewOllama(
			cfg.EmbeddingBaseURL,
			cfg.EmbeddingModel,
			nil,
		)
		if err != nil {
			return fmt.Errorf("configure embedding provider: %w", err)
		}
	}
	storyProcessor := story.NewProcessor(storyStore, embeddingProvider, changeHub.PublishSource)
	backend := httpserver.NewBackendWithEvents(
		sourceStore,
		acquisitionStore,
		entryStore,
		opmlStore,
		preview.New(registry),
		organizationStore,
		storyStore,
		storyProcessor,
		changeHub.PublishSource,
		ruleStore,
	)

	if slices.Contains(cfg.Roles, config.RoleWorker) {
		go func() {
			if err := storyProcessor.Run(ctx); err != nil {
				slog.Error("Story worker stopped", "error", err)
			}
		}()
		processor := ingestion.NewProcessor(acquisitionStore, sourceStore, entryStore, registry)
		owner, err := os.Hostname()
		if err != nil || owner == "" {
			owner = "pulse-worker"
		}
		runner := worker.New(processor, owner)
		go func() {
			if err := runner.Run(ctx); err != nil {
				slog.Error("worker stopped", "error", err)
			}
		}()
	}
	if slices.Contains(cfg.Roles, config.RoleScheduler) {
		runner := scheduler.New(sourceStore, acquisitionStore)
		go func() {
			if err := runner.Run(ctx); err != nil {
				slog.Error("scheduler stopped", "error", err)
			}
		}()
	}
	if slices.Contains(cfg.Roles, config.RoleEffect) {
		effectStore := postgresstore.NewEffectStore(pool)
		processor := effect.NewProcessor(effectStore, safeHTTPClient)
		owner, err := os.Hostname()
		if err != nil || owner == "" {
			owner = "pulse-effect-worker"
		}
		runner := worker.New(processor, owner+"-effects")
		go func() {
			if err := runner.Run(ctx); err != nil {
				slog.Error("effect worker stopped", "error", err)
			}
		}()
	}

	if !slices.Contains(cfg.Roles, config.RoleWeb) {
		signalReady(ready)
		<-ctx.Done()
		return nil
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpserver.NewHandlerWithWebAndEvents(backend, os.DirFS(cfg.WebDir), changeHub),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errs := make(chan error, 1)
	go func() {
		errs <- server.ListenAndServe()
	}()
	signalReady(ready)

	select {
	case err := <-errs:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown HTTP server: %w", err)
		}
		return nil
	}
}

func signalReady(ready []chan<- struct{}) {
	if len(ready) > 0 && ready[0] != nil {
		close(ready[0])
	}
}
