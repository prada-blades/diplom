package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"diplom/internal/cli"
	httpapi "diplom/internal/http"
)

func main() {
	app, err := httpapi.NewApp()
	if err != nil {
		log.Fatalf("init app: %v", err)
	}

	serverDone := app.Start()
	menu := cli.NewMenu(app.Services(), app, app.Address(), app.DefaultAdmin(), os.Stdin, os.Stdout)
	menuDone := make(chan error, 1)
	go func() {
		menuDone <- menu.Run()
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	select {
	case err := <-serverDone:
		if err != nil {
			log.Fatalf("run app: %v", err)
		}
	case err := <-menuDone:
		if err != nil {
			log.Fatalf("run cli: %v", err)
		}
	case sig := <-signals:
		log.Printf("received signal: %s", sig)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := app.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown app: %v", err)
	}
}
