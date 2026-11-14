// Command studio-server runs the Rogue Fairy Studios Project Management & Cross-Tool Integration API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/roguefairystudios/intranet/studio-server/internal/auth"
	"github.com/roguefairystudios/intranet/studio-server/internal/config"
	"github.com/roguefairystudios/intranet/studio-server/internal/server"
	"github.com/roguefairystudios/intranet/studio-server/internal/store"
)

func main() {
	_ = godotenv.Load()

	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var st store.Store
	if cfg.DatabaseURL != "" {
		dbCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		pgStore, dbErr := store.OpenPostgres(dbCtx, cfg.DatabaseURL)
		cancel()
		if dbErr != nil {
			log.Error("failed to open PostgreSQL store", "error", dbErr)
			os.Exit(1)
		}
		st = pgStore
		log.Info("connected to PostgreSQL store")
	} else {
		st = store.NewMemory()
		log.Warn("STUDIO_DATABASE_URL / DATABASE_URL not set; running with ephemeral in-memory store")
	}
	defer st.Close()

	verifier := auth.NewMultiAppVerifier(auth.MultiAppVerifierOptions{
		BaseURL:          cfg.OIDCBaseURL,
		AllowedSlugs:     cfg.OIDCAllowedSlugs,
		ExplicitIssuers:  cfg.OIDCIssuers,
		AllowedAudiences: cfg.OIDCAudiences,
	})
	dirClient := auth.NewAuthentikAPIClient(cfg.AuthentikAPIURL, cfg.AuthentikAPIToken, nil)

	authenticator := &auth.Authenticator{
		Verifier:    verifier,
		InternalKey: cfg.InternalKey,
		StaticKeys:  cfg.StaticKeys,
		Keys:        st,
		AdminGroups: cfg.AdminGroups,
		Log:         log,
	}

	srv := server.New(server.Options{
		Config:        cfg,
		Store:         st,
		Authenticator: authenticator,
		Directory:     dirClient,
		Log:           log,
	})

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		log.Error("failed to bind TCP listener", "addr", cfg.Addr, "error", err)
		os.Exit(1)
	}

	httpServer := &http.Server{
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	log.Info("studio-server listening", "addr", ln.Addr().String(), "oidcBase", cfg.OIDCBaseURL)
	if os.Getenv("STUDIO_PRINT_LISTENER") == "1" {
		_, _ = fmt.Fprintf(os.Stdout, "STUDIO_READY addr=%s\n", ln.Addr().String())
	}

	errCh := make(chan error, 1)
	go func() {
		if serveErr := httpServer.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down studio-server")
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutCtx)
	case serveErr := <-errCh:
		if serveErr != nil {
			log.Error("http server terminated with error", "error", serveErr)
			os.Exit(1)
		}
	}
}
