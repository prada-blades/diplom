package http

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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
	if !strings.Contains(dashboardRec.Body.String(), "Панель администратора") {
		t.Fatal("expected russian dashboard content")
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

func TestAdminUICreateAndEditUser(t *testing.T) {
	app, session := newTestAdminApp(t)

	createForm := url.Values{
		"full_name": {"Иван Петров"},
		"email":     {"ivan@example.com"},
		"password":  {"secret456"},
		"role":      {"employee"},
	}
	createReq := httptest.NewRequest(http.MethodPost, "/admin/ui/users", strings.NewReader(createForm.Encode()))
	createReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createReq.AddCookie(session)
	createRec := httptest.NewRecorder()

	app.testMux().ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", createRec.Code)
	}

	users := app.authService.ListUsers()
	if len(users) != 3 {
		t.Fatalf("expected 3 users after create, got %d", len(users))
	}

	var created domain.User
	for _, user := range users {
		if user.Email == "ivan@example.com" {
			created = user
		}
	}
	if created.ID == 0 {
		t.Fatal("expected created user")
	}

	updateForm := url.Values{
		"full_name": {"Иван Петров Обновлённый"},
		"email":     {"ivan.updated@example.com"},
		"role":      {"admin"},
	}
	updateReq := httptest.NewRequest(http.MethodPost, "/admin/ui/users/"+strconvID(created.ID)+"/update", strings.NewReader(updateForm.Encode()))
	updateReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	updateReq.AddCookie(session)
	updateRec := httptest.NewRecorder()

	app.testMux().ServeHTTP(updateRec, updateReq)

	if updateRec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", updateRec.Code)
	}

	updated, err := app.authService.GetUser(created.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if updated.Email != "ivan.updated@example.com" || updated.Role != domain.RoleAdmin {
		t.Fatalf("unexpected updated user: %+v", updated)
	}

	if _, _, err := app.authService.Login("ivan.updated@example.com", "secret456"); err != nil {
		t.Fatalf("expected login with updated email to succeed: %v", err)
	}
}

func TestAdminUIUserPageRequiresAdminSession(t *testing.T) {
	app, _ := newTestAdminApp(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/ui/users", nil)
	rec := httptest.NewRecorder()
	app.testMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rec.Code)
	}
}

func TestAdminUICreateBookingForAnotherUser(t *testing.T) {
	app, session := newTestAdminApp(t)

	form := url.Values{
		"user_id":     {"2"},
		"resource_id": {"1"},
		"start_time":  {time.Now().Add(5 * time.Hour).Format("2006-01-02T15:04")},
		"end_time":    {time.Now().Add(6 * time.Hour).Format("2006-01-02T15:04")},
		"purpose":     {"Совещание"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/ui/bookings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(session)
	rec := httptest.NewRecorder()

	app.testMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rec.Code)
	}

	bookings := app.bookingService.ListAll()
	if len(bookings) != 2 {
		t.Fatalf("expected 2 bookings, got %d", len(bookings))
	}
	if bookings[1].UserID != 2 {
		t.Fatalf("expected booking for user 2, got %d", bookings[1].UserID)
	}
}

func TestAdminUIRejectsConflictingBooking(t *testing.T) {
	app, session := newTestAdminApp(t)
	bookings := app.bookingService.ListAll()
	existing := bookings[0]

	form := url.Values{
		"user_id":     {"2"},
		"resource_id": {"1"},
		"start_time":  {existing.StartTime.Local().Format("2006-01-02T15:04")},
		"end_time":    {existing.EndTime.Local().Format("2006-01-02T15:04")},
		"purpose":     {"Конфликт"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/ui/bookings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(session)
	rec := httptest.NewRecorder()

	app.testMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with form error, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Ресурс уже занят") {
		t.Fatal("expected conflict error in response")
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

func TestAdminUIBookingsListShowsUserData(t *testing.T) {
	app, session := newTestAdminApp(t)

	req := httptest.NewRequest(http.MethodGet, "/admin/ui/bookings", nil)
	req.AddCookie(session)
	rec := httptest.NewRecorder()

	app.testMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Admin User") || !strings.Contains(body, "admin@example.com") {
		t.Fatal("expected user name and email in bookings list")
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

func strconvID(id int64) string {
	return strconv.FormatInt(id, 10)
}
