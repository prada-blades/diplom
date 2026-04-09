package http

import (
	"context"
	"errors"
	nethttp "net/http"

	"diplom/internal/config"
	"diplom/internal/service"
)

type AppServices struct {
	Auth     *service.AuthService
	Resource *service.ResourceService
	Booking  *service.BookingService
}

func (a *App) Start() <-chan error {
	done := make(chan error, 1)

	go func() {
		a.logger.Info("server starting", "address", a.cfg.Address, "default_admin_email", a.cfg.DefaultAdmin.Email)
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
	closeErr := a.store.Close()
	if shutdownErr != nil {
		return shutdownErr
	}
	return closeErr
}

func (a *App) Services() AppServices {
	return AppServices{
		Auth:     a.authService,
		Resource: a.resourceService,
		Booking:  a.bookingService,
	}
}

func (a *App) Logs() []string {
	if a.logs == nil {
		return nil
	}
	return a.logs.Entries()
}

func (a *App) Address() string {
	return a.cfg.Address
}

func (a *App) DefaultAdmin() config.DefaultAdmin {
	return a.cfg.DefaultAdmin
}
