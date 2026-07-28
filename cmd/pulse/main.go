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

	"github.com/wenpengfei/pulse/internal/config"
	annotationdriver "github.com/wenpengfei/pulse/internal/drivers/annotations"
	"github.com/wenpengfei/pulse/internal/drivers/feed"
	filedriver "github.com/wenpengfei/pulse/internal/drivers/file"
	htmldriver "github.com/wenpengfei/pulse/internal/drivers/html"
	jsonapidriver "github.com/wenpengfei/pulse/internal/drivers/jsonapi"
	"github.com/wenpengfei/pulse/internal/drivers/push"
	"github.com/wenpengfei/pulse/internal/effect"
	"github.com/wenpengfei/pulse/internal/ingestion"
	"github.com/wenpengfei/pulse/internal/platform/httpclient"
	"github.com/wenpengfei/pulse/internal/preview"
	"github.com/wenpengfei/pulse/internal/scheduler"
	"github.com/wenpengfei/pulse/internal/security"
	"github.com/wenpengfei/pulse/internal/source"
	"github.com/wenpengfei/pulse/internal/storage/migrate"
	postgresstore "github.com/wenpengfei/pulse/internal/storage/postgres"
	"github.com/wenpengfei/pulse/internal/transport/httpserver"
	"github.com/wenpengfei/pulse/internal/worker"
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

	credentialCipher, err := security.NewCredentialCipher(cfg.MasterKey)
	if err != nil {
		return fmt.Errorf("configure source credential encryption: %w", err)
	}
	sourceStore := postgresstore.NewSourceStore(pool, credentialCipher)
	acquisitionStore := postgresstore.NewAcquisitionStore(pool)
	entryStore := postgresstore.NewEntryStore(pool)
	opmlStore := postgresstore.NewOPMLStore(pool)
	organizationStore := postgresstore.NewOrganizationStore(pool)
	ruleStore := postgresstore.NewRuleStore(pool)
	safeHTTPClient := httpclient.New()
	registry, err := ingestion.NewRegistry(
		feed.New(safeHTTPClient),
		jsonapidriver.New(safeHTTPClient),
		htmldriver.New(safeHTTPClient),
		push.New(source.KindWebhook),
		push.New(source.KindManual),
		filedriver.New(cfg.ImportRoots),
		annotationdriver.New(),
	)
	if err != nil {
		return fmt.Errorf("create driver registry: %w", err)
	}
	backend := httpserver.NewBackend(
		sourceStore,
		acquisitionStore,
		entryStore,
		opmlStore,
		preview.New(registry),
		organizationStore,
		ruleStore,
	)

	if slices.Contains(cfg.Roles, config.RoleWorker) {
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
		Handler:           httpserver.NewHandlerWithWeb(backend, os.DirFS(cfg.WebDir)),
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
