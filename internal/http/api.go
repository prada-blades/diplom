package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	nethttp "net/http"
	"strconv"
	"strings"
	"time"

	"diplom/internal/cache"
	"diplom/internal/config"
	"diplom/internal/domain"
	"diplom/internal/repository/postgres"
	"diplom/internal/service"
)

type contextKey string

const userContextKey contextKey = "user"

type App struct {
	cfg             config.Config
	logger          *slog.Logger
	authService     *service.AuthService
	resourceService *service.ResourceService
	bookingService  *service.BookingService
	server          *nethttp.Server
}

func NewApp() (*App, error) {
	cfg := config.Load()
	logger := slog.Default()
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

	app := &App{
		cfg:             cfg,
		logger:          logger,
		authService:     authService,
		resourceService: resourceService,
		bookingService:  bookingService,
	}

	mux := nethttp.NewServeMux()
	app.registerRoutes(mux)
	app.server = &nethttp.Server{
		Addr:    cfg.Address,
		Handler: app.loggingMiddleware(mux),
	}

	return app, nil
}

func (a *App) Run() error {
	a.logger.Info("server starting", "address", a.cfg.Address, "default_admin_email", a.cfg.DefaultAdmin.Email)
	return a.server.ListenAndServe()
}

func (a *App) registerRoutes(mux *nethttp.ServeMux) {
	mux.HandleFunc("/health", a.handleHealth)
	mux.HandleFunc("/auth/register", a.handleRegister)
	mux.HandleFunc("/auth/login", a.handleLogin)
	mux.Handle("/me", a.requireAuth(nethttp.HandlerFunc(a.handleMe)))

	mux.HandleFunc("/resources", a.handleResources)
	mux.HandleFunc("/resources/", a.handleResourceByID)

	mux.Handle("/availability", a.requireAuth(nethttp.HandlerFunc(a.handleAvailability)))
	mux.Handle("/recommendations/schedule", a.requireAuth(nethttp.HandlerFunc(a.handleScheduleRecommendations)))
	mux.Handle("/bookings/my", a.requireAuth(nethttp.HandlerFunc(a.handleMyBookings)))
	mux.Handle("/bookings", a.requireAuth(nethttp.HandlerFunc(a.handleCreateBooking)))
	mux.Handle("/bookings/", a.requireAuth(nethttp.HandlerFunc(a.handleBookingByID)))

	mux.Handle("/admin/bookings", a.requireAdmin(nethttp.HandlerFunc(a.handleAdminBookings)))
	mux.Handle("/admin/reports/utilization", a.requireAdmin(nethttp.HandlerFunc(a.handleUtilizationReport)))
}

func (a *App) handleHealth(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.Method != nethttp.MethodGet {
		writeError(w, nethttp.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	writeJSON(w, nethttp.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().UTC(),
	})
}

type registerRequest struct {
	FullName string `json:"full_name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (a *App) handleRegister(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.Method != nethttp.MethodPost {
		writeError(w, nethttp.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	var req registerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, nethttp.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	user, token, err := a.authService.Register(req.FullName, req.Email, req.Password, domain.RoleEmployee)
	if err != nil {
		writeError(w, nethttp.StatusBadRequest, "register_failed", err.Error())
		return
	}

	writeJSON(w, nethttp.StatusCreated, map[string]any{
		"user":  user,
		"token": token,
	})
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (a *App) handleLogin(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.Method != nethttp.MethodPost {
		writeError(w, nethttp.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, nethttp.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	user, token, err := a.authService.Login(req.Email, req.Password)
	if err != nil {
		writeError(w, nethttp.StatusUnauthorized, "login_failed", err.Error())
		return
	}

	writeJSON(w, nethttp.StatusOK, map[string]any{
		"user":  user,
		"token": token,
	})
}

func (a *App) handleMe(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.Method != nethttp.MethodGet {
		writeError(w, nethttp.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	writeJSON(w, nethttp.StatusOK, currentUser(r))
}

type resourceRequest struct {
	Name        string              `json:"name"`
	Type        domain.ResourceType `json:"type"`
	Location    string              `json:"location"`
	Capacity    int                 `json:"capacity"`
	Description string              `json:"description"`
	IsActive    *bool               `json:"is_active,omitempty"`
}

func (a *App) handleResources(w nethttp.ResponseWriter, r *nethttp.Request) {
	switch r.Method {
	case nethttp.MethodGet:
		resourceType := domain.ResourceType(r.URL.Query().Get("type"))
		onlyActive := r.URL.Query().Get("include_inactive") != "true"
		writeJSON(w, nethttp.StatusOK, map[string]any{
			"items": a.resourceService.List(resourceType, onlyActive),
		})
	case nethttp.MethodPost:
		user, ok := a.authenticatedUserFromRequest(w, r)
		if !ok {
			return
		}
		if user.Role != domain.RoleAdmin {
			writeError(w, nethttp.StatusForbidden, "forbidden", "admin access required")
			return
		}

		var req resourceRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, nethttp.StatusBadRequest, "invalid_json", err.Error())
			return
		}

		resource, err := a.resourceService.Create(req.Name, req.Type, req.Location, req.Capacity, req.Description)
		if err != nil {
			writeError(w, nethttp.StatusBadRequest, "resource_create_failed", err.Error())
			return
		}

		writeJSON(w, nethttp.StatusCreated, resource)
	default:
		writeError(w, nethttp.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

func (a *App) handleResourceByID(w nethttp.ResponseWriter, r *nethttp.Request) {
	id, err := parseID(strings.TrimPrefix(r.URL.Path, "/resources/"))
	if err != nil {
		writeError(w, nethttp.StatusBadRequest, "invalid_resource_id", "invalid resource id")
		return
	}

	switch r.Method {
	case nethttp.MethodGet:
		resource, err := a.resourceService.Get(id)
		if err != nil {
			writeError(w, nethttp.StatusNotFound, "not_found", "resource not found")
			return
		}
		writeJSON(w, nethttp.StatusOK, resource)
	case nethttp.MethodPut:
		user, ok := a.authenticatedUserFromRequest(w, r)
		if !ok {
			return
		}
		if user.Role != domain.RoleAdmin {
			writeError(w, nethttp.StatusForbidden, "forbidden", "admin access required")
			return
		}

		var req resourceRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, nethttp.StatusBadRequest, "invalid_json", err.Error())
			return
		}

		isActive := true
		if req.IsActive != nil {
			isActive = *req.IsActive
		}

		resource, err := a.resourceService.Update(id, req.Name, req.Type, req.Location, req.Capacity, req.Description, isActive)
		if err != nil {
			writeError(w, nethttp.StatusBadRequest, "resource_update_failed", err.Error())
			return
		}

		writeJSON(w, nethttp.StatusOK, resource)
	case nethttp.MethodDelete:
		user, ok := a.authenticatedUserFromRequest(w, r)
		if !ok {
			return
		}
		if user.Role != domain.RoleAdmin {
			writeError(w, nethttp.StatusForbidden, "forbidden", "admin access required")
			return
		}

		resource, err := a.resourceService.Disable(id)
		if err != nil {
			writeError(w, nethttp.StatusNotFound, "not_found", "resource not found")
			return
		}

		writeJSON(w, nethttp.StatusOK, resource)
	default:
		writeError(w, nethttp.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

func (a *App) handleAvailability(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.Method != nethttp.MethodGet {
		writeError(w, nethttp.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	start, end, err := parseTimeWindow(r)
	if err != nil {
		writeError(w, nethttp.StatusBadRequest, "invalid_period", err.Error())
		return
	}

	resourceType := domain.ResourceType(r.URL.Query().Get("type"))
	items, err := a.bookingService.Availability(start, end, resourceType)
	if err != nil {
		writeError(w, nethttp.StatusBadRequest, "availability_failed", err.Error())
		return
	}

	writeJSON(w, nethttp.StatusOK, map[string]any{"items": items})
}

func (a *App) handleMyBookings(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.Method != nethttp.MethodGet {
		writeError(w, nethttp.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	user := currentUser(r)
	writeJSON(w, nethttp.StatusOK, map[string]any{
		"items": a.bookingService.ListMy(user.ID),
	})
}

type scheduleRecommendationRequest struct {
	ResourceType      domain.ResourceType `json:"resource_type"`
	Participants      int                 `json:"participants"`
	DurationMinutes   int                 `json:"duration_minutes"`
	PreferredStart    string              `json:"preferred_start"`
	SearchWindowStart string              `json:"search_window_start"`
	SearchWindowEnd   string              `json:"search_window_end"`
}

func (a *App) handleScheduleRecommendations(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.Method != nethttp.MethodPost {
		writeError(w, nethttp.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	var req scheduleRecommendationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, nethttp.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	preferredStart, err := time.Parse(time.RFC3339, req.PreferredStart)
	if err != nil {
		writeError(w, nethttp.StatusBadRequest, "invalid_preferred_start", "preferred_start must be RFC3339")
		return
	}
	searchWindowStart, err := time.Parse(time.RFC3339, req.SearchWindowStart)
	if err != nil {
		writeError(w, nethttp.StatusBadRequest, "invalid_search_window_start", "search_window_start must be RFC3339")
		return
	}
	searchWindowEnd, err := time.Parse(time.RFC3339, req.SearchWindowEnd)
	if err != nil {
		writeError(w, nethttp.StatusBadRequest, "invalid_search_window_end", "search_window_end must be RFC3339")
		return
	}

	items, err := a.bookingService.RecommendSchedule(service.ScheduleRecommendationRequest{
		ResourceType:      req.ResourceType,
		Participants:      req.Participants,
		DurationMinutes:   req.DurationMinutes,
		PreferredStart:    preferredStart,
		SearchWindowStart: searchWindowStart,
		SearchWindowEnd:   searchWindowEnd,
	})
	if err != nil {
		writeError(w, nethttp.StatusBadRequest, "recommendation_failed", err.Error())
		return
	}

	writeJSON(w, nethttp.StatusOK, map[string]any{"items": items})
}

type bookingRequest struct {
	ResourceID int64  `json:"resource_id"`
	StartTime  string `json:"start_time"`
	EndTime    string `json:"end_time"`
	Purpose    string `json:"purpose"`
}

func (a *App) handleCreateBooking(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.Method != nethttp.MethodPost {
		writeError(w, nethttp.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	var req bookingRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, nethttp.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	start, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		writeError(w, nethttp.StatusBadRequest, "invalid_start_time", "start_time must be RFC3339")
		return
	}
	end, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		writeError(w, nethttp.StatusBadRequest, "invalid_end_time", "end_time must be RFC3339")
		return
	}

	user := currentUser(r)
	booking, err := a.bookingService.Create(user.ID, req.ResourceID, start, end, req.Purpose)
	if err != nil {
		writeError(w, nethttp.StatusBadRequest, "booking_create_failed", err.Error())
		return
	}

	writeJSON(w, nethttp.StatusCreated, booking)
}

func (a *App) handleBookingByID(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.Method != nethttp.MethodDelete {
		writeError(w, nethttp.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	id, err := parseID(strings.TrimPrefix(r.URL.Path, "/bookings/"))
	if err != nil {
		writeError(w, nethttp.StatusBadRequest, "invalid_booking_id", "invalid booking id")
		return
	}

	booking, err := a.bookingService.Cancel(currentUser(r), id)
	if err != nil {
		status := nethttp.StatusBadRequest
		if err.Error() == "forbidden" {
			status = nethttp.StatusForbidden
		}
		if err.Error() == "booking not found" {
			status = nethttp.StatusNotFound
		}
		writeError(w, status, "booking_cancel_failed", err.Error())
		return
	}

	writeJSON(w, nethttp.StatusOK, booking)
}

func (a *App) handleAdminBookings(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.Method != nethttp.MethodGet {
		writeError(w, nethttp.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	writeJSON(w, nethttp.StatusOK, map[string]any{
		"items": a.bookingService.ListAll(),
	})
}

func (a *App) handleUtilizationReport(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.Method != nethttp.MethodGet {
		writeError(w, nethttp.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	start, end, err := parseTimeWindow(r)
	if err != nil {
		writeError(w, nethttp.StatusBadRequest, "invalid_period", err.Error())
		return
	}

	report, err := a.bookingService.UtilizationReport(start, end)
	if err != nil {
		writeError(w, nethttp.StatusBadRequest, "report_failed", err.Error())
		return
	}

	writeJSON(w, nethttp.StatusOK, report)
}

func (a *App) loggingMiddleware(next nethttp.Handler) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		a.logger.Info("request handled", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}

func (a *App) requireAuth(next nethttp.Handler) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		user, ok := a.authenticatedUserFromRequest(w, r)
		if !ok {
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *App) requireAdmin(next nethttp.Handler) nethttp.Handler {
	return a.requireAuth(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		user := currentUser(r)
		if user.Role != domain.RoleAdmin {
			writeError(w, nethttp.StatusForbidden, "forbidden", "admin access required")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func (a *App) authenticatedUserFromRequest(w nethttp.ResponseWriter, r *nethttp.Request) (domain.User, bool) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		writeError(w, nethttp.StatusUnauthorized, "unauthorized", "missing bearer token")
		return domain.User{}, false
	}

	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	user, err := a.authService.Authenticate(token)
	if err != nil {
		writeError(w, nethttp.StatusUnauthorized, "unauthorized", "invalid or expired token")
		return domain.User{}, false
	}

	return user, true
}

func currentUser(r *nethttp.Request) domain.User {
	user, _ := r.Context().Value(userContextKey).(domain.User)
	return user
}

func parseID(raw string) (int64, error) {
	trimmed := strings.Trim(raw, "/")
	if trimmed == "" || strings.Contains(trimmed, "/") {
		return 0, errors.New("invalid id")
	}
	return strconv.ParseInt(trimmed, 10, 64)
}

func parseTimeWindow(r *nethttp.Request) (time.Time, time.Time, error) {
	startRaw := r.URL.Query().Get("start")
	endRaw := r.URL.Query().Get("end")
	if startRaw == "" || endRaw == "" {
		return time.Time{}, time.Time{}, errors.New("start and end query parameters are required")
	}

	start, err := time.Parse(time.RFC3339, startRaw)
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("start must be RFC3339")
	}
	end, err := time.Parse(time.RFC3339, endRaw)
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("end must be RFC3339")
	}

	return start.UTC(), end.UTC(), nil
}

func decodeJSON(r *nethttp.Request, dst any) error {
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst)
}

func writeJSON(w nethttp.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w nethttp.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}
