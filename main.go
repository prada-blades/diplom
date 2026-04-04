package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"diplom/internal/bootstrap"
	"diplom/internal/cli"
	httpapi "diplom/internal/http"
)

const (
	modeServer = "server"
	modeAdmin  = "admin"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		log.Fatalf("%v", err)
	}
}

func run(args []string, in io.Reader, out io.Writer) error {
	mode, err := parseMode(args)
	if err != nil {
		return err
	}

	switch mode {
	case modeServer:
		return runServer()
	case modeAdmin:
		return runAdmin(in, out)
	default:
		return fmt.Errorf("unsupported mode: %s", mode)
	}
}

func parseMode(args []string) (string, error) {
	if len(args) == 0 {
		return modeServer, nil
	}

	switch args[0] {
	case modeServer, modeAdmin:
		return args[0], nil
	default:
		return "", fmt.Errorf("unknown mode %q, use %q or %q", args[0], modeServer, modeAdmin)
	}
}

func runServer() error {
	boot, err := bootstrap.New()
	if err != nil {
		return fmt.Errorf("init server: %w", err)
	}

	app := httpapi.NewApp(boot)
	serverDone := app.Start()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	select {
	case err := <-serverDone:
		if err != nil {
			return fmt.Errorf("run server: %w", err)
		}
	case sig := <-signals:
		log.Printf("received signal: %s", sig)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := app.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown server: %w", err)
	}
	return nil
}

func runAdmin(in io.Reader, out io.Writer) error {
	boot, err := bootstrap.New()
	if err != nil {
		return fmt.Errorf("init admin: %w", err)
	}
	defer boot.Close()

	menu := cli.NewMenu(boot.Services, boot.Config.DefaultAdmin, in, out)
	if err := menu.Run(); err != nil {
		return fmt.Errorf("run admin: %w", err)
	}
	return nil
}
