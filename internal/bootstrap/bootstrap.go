package bootstrap

import (
	"log/slog"
	"os"

	"diplom/internal/cache"
	"diplom/internal/config"
	"diplom/internal/repository/postgres"
	"diplom/internal/service"
)

type Services struct {
	Auth     *service.AuthService
	Resource *service.ResourceService
	Booking  *service.BookingService
}

type Bootstrap struct {
	Config   config.Config
	Logger   *slog.Logger
	Store    *postgres.Store
	Services Services
}

func New() (*Bootstrap, error) {
	cfg := config.Load()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	store, err := postgres.NewStore(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	if err := store.Migrate(); err != nil {
		_ = store.Close()
		return nil, err
	}

	appCache := cache.Cache(cache.NewNoop())
	if cfg.Redis.Enabled {
		redisCache, err := cache.NewRedis(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
		if err != nil {
			logger.Warn("redis unavailable, continuing without cache", "addr", cfg.Redis.Addr, "error", err)
		} else {
			appCache = redisCache
		}
	}

	authService := service.NewAuthService(store, cfg.JWTSecret)
	resourceService := service.NewResourceService(store, appCache)
	bookingService := service.NewBookingService(store, store, appCache)

	if err := authService.SeedAdmin(cfg.DefaultAdmin.FullName, cfg.DefaultAdmin.Email, cfg.DefaultAdmin.Password); err != nil {
		_ = store.Close()
		return nil, err
	}

	return &Bootstrap{
		Config: cfg,
		Logger: logger,
		Store:  store,
		Services: Services{
			Auth:     authService,
			Resource: resourceService,
			Booking:  bookingService,
		},
	}, nil
}

func (b *Bootstrap) Close() error {
	if b == nil || b.Store == nil {
		return nil
	}
	return b.Store.Close()
}
