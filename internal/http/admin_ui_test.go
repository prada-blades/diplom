package http

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"diplom/internal/cache"
	"diplom/internal/config"
	"diplom/internal/domain"
	"diplom/internal/repository"
	"diplom/internal/service"
)

func TestAdminUILoginAndDashboard(t *testing.T) {
	app, _ := newTestAdminApp(t)

	loginForm := url.Values{
		"email":    {"admin@example.com"},
		"password": {"secret123"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/ui/login", strings.NewReader(loginForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	app.testMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rec.Code)
	}
	session := rec.Result().Cookies()
	if len(session) == 0 {
		t.Fatal("expected session cookie")
	}

	dashboardReq := httptest.NewRequest(http.MethodGet, "/admin/ui", nil)
	dashboardReq.AddCookie(session[0])
	dashboardRec := httptest.NewRecorder()

	app.testMux().ServeHTTP(dashboardRec, dashboardReq)

	if dashboardRec.Code != http.StatusOK {
		t.Fatalf("expected dashboard 200, got %d", dashboardRec.Code)
	}
	if !strings.Contains(dashboardRec.Body.String(), "Resource Control") {
		t.Fatal("expected dashboard content")
	}
}

func TestAdminUIRejectsProtectedPageWithoutSession(t *testing.T) {
	app, _ := newTestAdminApp(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/ui/resources", nil)
	rec := httptest.NewRecorder()
	app.testMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); !strings.HasPrefix(got, "/admin/ui/login") {
		t.Fatalf("unexpected redirect target: %s", got)
	}
}

func TestAdminUICreateResource(t *testing.T) {
	app, session := newTestAdminApp(t)

	form := url.Values{
		"name":        {"Room X"},
		"type":        {"meeting_room"},
		"location":    {"HQ"},
		"capacity":    {"10"},
		"description": {"Board room"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/ui/resources", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(session)
	rec := httptest.NewRecorder()

	app.testMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rec.Code)
	}

	resources := app.resourceService.List("", false)
	if len(resources) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(resources))
	}
}

func TestAdminUICancelBooking(t *testing.T) {
	app, session := newTestAdminApp(t)

	req := httptest.NewRequest(http.MethodPost, "/admin/ui/bookings/1/cancel", nil)
	req.AddCookie(session)
	rec := httptest.NewRecorder()

	app.testMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rec.Code)
	}

	bookings := app.bookingService.ListAll()
	if bookings[0].Status != domain.BookingCancelled {
		t.Fatalf("expected cancelled booking, got %s", bookings[0].Status)
	}
}

func newTestAdminApp(t *testing.T) (*App, *http.Cookie) {
	t.Helper()

	store := repository.NewMemoryStore()
	authService := service.NewAuthService(store, "test-secret")
	resourceService := service.NewResourceService(store, cache.NewNoop())
	bookingService := service.NewBookingService(store, store, cache.NewNoop())

	admin, token, err := authService.Register("Admin User", "admin@example.com", "secret123", domain.RoleAdmin)
	if err != nil {
		t.Fatalf("register admin: %v", err)
	}
	if _, _, err := authService.Register("Employee User", "employee@example.com", "secret123", domain.RoleEmployee); err != nil {
		t.Fatalf("register employee: %v", err)
	}

	resource, err := resourceService.Create("Room A", domain.ResourceMeetingRoom, "HQ", 8, "Main room")
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}

	start := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Minute)
	end := start.Add(1 * time.Hour)
	if _, err := bookingService.Create(admin.ID, resource.ID, start, end, "Review"); err != nil {
		t.Fatalf("create booking: %v", err)
	}

	app := &App{
		cfg: config.Config{
			Address:   ":8080",
			JWTSecret: "test-secret",
		},
		authService:     authService,
		resourceService: resourceService,
		bookingService:  bookingService,
	}

	return app, &http.Cookie{Name: adminSessionCookieName, Value: token, Path: "/admin/ui"}
}

func (a *App) testMux() *http.ServeMux {
	mux := http.NewServeMux()
	a.registerRoutes(mux)
	return mux
}
