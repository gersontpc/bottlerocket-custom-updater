package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bottlerocket-os/bottlerocket-update-operator/bottlerocket-updater/internal/config"
	"github.com/bottlerocket-os/bottlerocket-update-operator/bottlerocket-updater/internal/operator"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Printf("invalid configuration: %v", err)
		os.Exit(1)
	}

	switch cfg.Mode {
	case config.ModeController:
		controller, err := operator.NewController(ctx, cfg)
		if err != nil {
			log.Printf("failed to create controller: %v", err)
			os.Exit(1)
		}
		if err := controller.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("controller stopped: %v", err)
			os.Exit(1)
		}
	case config.ModeWorker:
		worker, err := operator.NewWorker(cfg)
		if err != nil {
			log.Printf("failed to create worker: %v", err)
			os.Exit(1)
		}
		if err := worker.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("worker stopped: %v", err)
			os.Exit(1)
		}
	}
}
