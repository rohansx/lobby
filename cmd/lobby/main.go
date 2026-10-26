// Command lobby is the Lobby waitlist server and its CLI. Run "lobby serve"
// to start the server; every other subcommand talks to a running instance.
// Symlinked or copied as "wl", it behaves as the CLI.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/rohansx/lobby/internal/app"
)

var version = "dev"

func main() {
	args := os.Args[1:]
	cmd := ""
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	asCLI := filepath.Base(os.Args[0]) == "wl"
	switch {
	case cmd == "serve" || (cmd == "" && !asCLI):
		if err := serve(); err != nil {
			fmt.Fprintln(os.Stderr, "lobby:", err)
			os.Exit(1)
		}
	case cmd == "version" || cmd == "--version":
		fmt.Println("lobby", version)
	case cmd == "" || cmd == "help" || cmd == "-h" || cmd == "--help":
		usage()
	default:
		if err := runCLI(cmd, args); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	}
}

func usage() {
	fmt.Print(`Lobby: a self-hosted waitlist for every project.

Server:
  lobby serve                        start the server (default when run as "lobby")

CLI (needs LOBBY_URL and LOBBY_KEY=sk_... for the project):
  lobby stats                        counts, confirm rate, top sources
  lobby signups [--status S] [--q Q] [--limit N]
  lobby invite --next N              invite the next N people in line
  lobby invite --ids 12,13           invite specific people
  lobby joined ID                    mark someone as joined
  lobby remove ID [--hard]           remove (or delete and suppress)
  lobby export [--format csv|ndjson] [--status all]
  lobby webhooks                     list webhooks
  lobby version

Server environment:
  LOBBY_BASE_URL      public URL of this instance (default http://localhost:8080)
  LOBBY_ADDR          listen address (default :8080)
  LOBBY_DATA          data directory holding lobby.db (default ./data)
  LOBBY_FROM_EMAIL    sender, ideally on a sending subdomain
  RESEND_API_KEY      send with Resend, or
  SMTP_URL            smtp://user:pass@host:587 or smtps://user:pass@host:465
  RESEND_WEBHOOK_SECRET  verify Resend bounce/complaint webhooks at /v1/hooks/resend
  LOBBY_TRUST_PROXY   0 to ignore X-Forwarded-For (default 1)
  LOBBY_CHECK_MX      0 to skip the MX lookup on signup (default 1)
`)
}

func serve() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	cfg := app.ConfigFromEnv()
	a, err := app.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer a.Close()
	srv, err := app.NewServer(a)
	if err != nil {
		return err
	}

	workerDone := make(chan struct{})
	go func() {
		a.RunWorker(ctx)
		close(workerDone)
	}()

	hs := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		slog.Info("lobby listening", "addr", cfg.Addr, "base_url", cfg.BaseURL, "data", cfg.DataDir, "version", version)
		errc <- hs.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	slog.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	hs.Shutdown(shutCtx)
	<-workerDone
	return nil
}
