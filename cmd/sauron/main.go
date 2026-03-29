package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"sauron/internal/config"
	"sauron/internal/localstart"
	"sauron/internal/server"
	"sauron/internal/tiamat"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Optional local file; does not override variables already set in the environment.
	if err := godotenv.Load(); err != nil {
		if !os.IsNotExist(err) {
			log.Printf("godotenv: %v", err)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	tc := tiamat.New(cfg.TiamatBaseURL, cfg.HubToken)

	ctl, err := localstart.New(cfg.TiamatStartScript, cfg.TiamatSystemdUnit, cfg.SystemdUserScope, cfg.TiamatStopScript)
	if err != nil {
		log.Fatal(err)
	}
	var hostCtl server.TiamatHostControl
	if ctl != nil {
		hostCtl = ctl
	}

	h := &server.Handler{
		Client:         tc,
		TokenSet:       cfg.HubToken != "",
		TiamatControl:  hostCtl,
	}

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           h.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("sauron listening on %s (Tiamat %s)", cfg.Listen, cfg.TiamatBaseURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
