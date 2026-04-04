package http

import (
	"context"
	"errors"
	nethttp "net/http"
)

func (a *App) Start() <-chan error {
	done := make(chan error, 1)

	go func() {
		a.boot.Logger.Info("server starting", "address", a.boot.Config.Address, "default_admin_email", a.boot.Config.DefaultAdmin.Email)
		err := a.server.ListenAndServe()
		if err != nil && !errors.Is(err, nethttp.ErrServerClosed) {
			done <- err
			return
		}
		done <- nil
	}()

	return done
}

func (a *App) Shutdown(ctx context.Context) error {
	shutdownErr := a.server.Shutdown(ctx)
	closeErr := a.boot.Close()
	if shutdownErr != nil {
		return shutdownErr
	}
	return closeErr
}
