package http

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
	"time"

	"diplom/internal/cache"
	"diplom/internal/domain"
	"diplom/internal/repository"
	"diplom/internal/service"
)

func TestRegisterCreatesEmployeeRole(t *testing.T) {
	app, _ := newTestApp(t)

	resp := performRequest(t, app, nethttp.MethodPost, "/auth/register", map[string]any{
		"full_name": "Ivan Petrov",
		"email":     "ivan@example.com",
		"password":  "secret123",
	}, "")

	if resp.Code != nethttp.StatusCreated {
		t.Fatalf("expected status %d, got %d", nethttp.StatusCreated, resp.Code)
	}

	var payload struct {
		User domain.User `json:"user"`
	}
	decodeResponse(t, resp.Body, &payload)

	if payload.User.Role != domain.RoleEmployee {
		t.Fatalf("expected employee role, got %s", payload.User.Role)
	}
}

func TestMeReturnsRoleForAuthenticatedUsers(t *testing.T) {
	app, auth := newTestApp(t)

	adminToken := mustLogin(t, auth, "admin@corp.local", "admin123")
	employeeToken := mustLogin(t, auth, "employee@example.com", "password123")

	testCases := []struct {
		name  string
		token string
		role  domain.Role
	}{
		{name: "admin", token: adminToken, role: domain.RoleAdmin},
		{name: "employee", token: employeeToken, role: domain.RoleEmployee},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp := performRequest(t, app, nethttp.MethodGet, "/me", nil, tc.token)
			if resp.Code != nethttp.StatusOK {
				t.Fatalf("expected status %d, got %d", nethttp.StatusOK, resp.Code)
			}

			var user domain.User
			decodeResponse(t, resp.Body, &user)
			if user.Role != tc.role {
				t.Fatalf("expected role %s, got %s", tc.role, user.Role)
			}
		})
	}
}

func TestAdminEndpointsRequireAdminRole(t *testing.T) {
	app, auth := newTestApp(t)

	adminToken := mustLogin(t, auth, "admin@corp.local", "admin123")
	employeeToken := mustLogin(t, auth, "employee@example.com", "password123")

	resourceID := createResourceViaAPI(t, app, adminToken, "Room A")

	windowStart := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	windowEnd := windowStart.Add(1 * time.Hour)

	bookingResp := performRequest(t, app, nethttp.MethodPost, "/bookings", map[string]any{
		"resource_id": resourceID,
		"start_time":  time.Now().UTC().Add(3 * time.Hour).Truncate(time.Second).Format(time.RFC3339),
		"end_time":    time.Now().UTC().Add(4 * time.Hour).Truncate(time.Second).Format(time.RFC3339),
		"purpose":     "Team sync",
	}, employeeToken)
	if bookingResp.Code != nethttp.StatusCreated {
		t.Fatalf("expected booking create status %d, got %d", nethttp.StatusCreated, bookingResp.Code)
	}

	testCases := []struct {
		name        string
		method      string
		path        string
		token       string
		body        map[string]any
		wantStatus  int
		wantErrCode string
	}{
		{
			name:        "post resources requires authentication",
			method:      nethttp.MethodPost,
			path:        "/resources",
			body:        resourcePayload("Room B"),
			wantStatus:  nethttp.StatusUnauthorized,
			wantErrCode: "unauthorized",
		},
		{
			name:        "post resources forbids employee",
			method:      nethttp.MethodPost,
			path:        "/resources",
			token:       employeeToken,
			body:        resourcePayload("Room C"),
			wantStatus:  nethttp.StatusForbidden,
			wantErrCode: "forbidden",
		},
		{
			name:        "put resources forbids employee",
			method:      nethttp.MethodPut,
			path:        "/resources/1",
			token:       employeeToken,
			body:        resourcePayload("Room A Updated"),
			wantStatus:  nethttp.StatusForbidden,
			wantErrCode: "forbidden",
		},
		{
			name:        "delete resources forbids employee",
			method:      nethttp.MethodDelete,
			path:        "/resources/1",
			token:       employeeToken,
			wantStatus:  nethttp.StatusForbidden,
			wantErrCode: "forbidden",
		},
		{
			name:        "admin bookings forbids employee",
			method:      nethttp.MethodGet,
			path:        "/admin/bookings",
			token:       employeeToken,
			wantStatus:  nethttp.StatusForbidden,
			wantErrCode: "forbidden",
		},
		{
			name:        "admin utilization forbids employee",
			method:      nethttp.MethodGet,
			path:        "/admin/reports/utilization?start=" + windowStart.Format(time.RFC3339) + "&end=" + windowEnd.Format(time.RFC3339),
			token:       employeeToken,
			wantStatus:  nethttp.StatusForbidden,
			wantErrCode: "forbidden",
		},
		{
			name:       "admin can update resource",
			method:     nethttp.MethodPut,
			path:       "/resources/1",
			token:      adminToken,
			body:       resourcePayload("Room A Updated"),
			wantStatus: nethttp.StatusOK,
		},
		{
			name:       "admin can list all bookings",
			method:     nethttp.MethodGet,
			path:       "/admin/bookings",
			token:      adminToken,
			wantStatus: nethttp.StatusOK,
		},
		{
			name:       "admin can read utilization report",
			method:     nethttp.MethodGet,
			path:       "/admin/reports/utilization?start=" + windowStart.Format(time.RFC3339) + "&end=" + windowEnd.Format(time.RFC3339),
			token:      adminToken,
			wantStatus: nethttp.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp := performRequest(t, app, tc.method, tc.path, tc.body, tc.token)
			if resp.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d", tc.wantStatus, resp.Code)
			}

			if tc.wantErrCode == "" {
				return
			}

			var payload struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			decodeResponse(t, resp.Body, &payload)
			if payload.Error.Code != tc.wantErrCode {
				t.Fatalf("expected error code %q, got %q", tc.wantErrCode, payload.Error.Code)
			}
		})
	}
}

func TestResourcesSupportImagesAndEquipment(t *testing.T) {
	app, auth := newTestApp(t)
	adminToken := mustLogin(t, auth, "admin@corp.local", "admin123")

	resp := performRequest(t, app, nethttp.MethodPost, "/resources", map[string]any{
		"name":        "Room Media",
		"type":        "meeting_room",
		"location":    "HQ",
		"capacity":    10,
		"description": "Presentation room",
		"image_urls":  []string{"https://example.com/room-a.jpg", "https://example.com/room-b.jpg"},
		"equipment":   []string{"Projector", "whiteboard", "projector"},
	}, adminToken)
	if resp.Code != nethttp.StatusCreated {
		t.Fatalf("expected status %d, got %d", nethttp.StatusCreated, resp.Code)
	}

	var resource domain.Resource
	decodeResponse(t, resp.Body, &resource)
	if len(resource.ImageURLs) != 2 {
		t.Fatalf("expected 2 image urls, got %v", resource.ImageURLs)
	}
	if !reflect.DeepEqual(resource.Equipment, []string{"projector", "whiteboard"}) {
		t.Fatalf("unexpected equipment: %v", resource.Equipment)
	}
}

func TestResourcesFilterByEquipment(t *testing.T) {
	app, auth := newTestApp(t)
	adminToken := mustLogin(t, auth, "admin@corp.local", "admin123")

	createResourceWithAttrs(t, app, adminToken, "Room A", []string{"projector"}, nil)
	createResourceWithAttrs(t, app, adminToken, "Room B", []string{"projector", "whiteboard"}, nil)

	resp := performRequest(t, app, nethttp.MethodGet, "/resources?equipment=projector&equipment=whiteboard", nil, "")
	if resp.Code != nethttp.StatusOK {
		t.Fatalf("expected status %d, got %d", nethttp.StatusOK, resp.Code)
	}

	var payload struct {
		Items []domain.Resource `json:"items"`
	}
	decodeResponse(t, resp.Body, &payload)
	if len(payload.Items) != 1 || payload.Items[0].Name != "Room B" {
		t.Fatalf("unexpected filtered resources: %+v", payload.Items)
	}
}

func TestAvailabilityFilterByEquipment(t *testing.T) {
	app, auth := newTestApp(t)
	adminToken := mustLogin(t, auth, "admin@corp.local", "admin123")
	employeeToken := mustLogin(t, auth, "employee@example.com", "password123")

	roomA := createResourceWithAttrs(t, app, adminToken, "Room A", []string{"projector"}, nil)
	createResourceWithAttrs(t, app, adminToken, "Room B", []string{"projector", "whiteboard"}, nil)

	start := time.Now().UTC().Add(6 * time.Hour).Truncate(time.Second)
	end := start.Add(time.Hour)
	bookingResp := performRequest(t, app, nethttp.MethodPost, "/bookings", map[string]any{
		"resource_id": roomA,
		"start_time":  start.Format(time.RFC3339),
		"end_time":    end.Format(time.RFC3339),
		"purpose":     "busy",
	}, employeeToken)
	if bookingResp.Code != nethttp.StatusCreated {
		t.Fatalf("expected booking status %d, got %d", nethttp.StatusCreated, bookingResp.Code)
	}

	resp := performRequest(t, app, nethttp.MethodGet, "/availability?start="+start.Format(time.RFC3339)+"&end="+end.Format(time.RFC3339)+"&equipment=projector&equipment=whiteboard", nil, employeeToken)
	if resp.Code != nethttp.StatusOK {
		t.Fatalf("expected status %d, got %d", nethttp.StatusOK, resp.Code)
	}

	var payload struct {
		Items []domain.Resource `json:"items"`
	}
	decodeResponse(t, resp.Body, &payload)
	if len(payload.Items) != 1 || payload.Items[0].Name != "Room B" {
		t.Fatalf("unexpected availability resources: %+v", payload.Items)
	}
}

func TestDeleteResourceCancelsFutureBookingsAndReturnsCount(t *testing.T) {
	app, auth, store := newTestAppWithStore(t)
	adminToken := mustLogin(t, auth, "admin@corp.local", "admin123")
	employeeToken := mustLogin(t, auth, "employee@example.com", "password123")

	resourceID := createResourceViaAPI(t, app, adminToken, "Room Auto Cancel")
	now := time.Now().UTC().Truncate(time.Second)
	currentStart := now.Add(-30 * time.Minute)
	currentEnd := now.Add(30 * time.Minute)
	futureStart := now.Add(3 * time.Hour)
	futureEnd := futureStart.Add(time.Hour)

	// Seed an active in-progress booking directly in memory to avoid past-booking validation.
	currentBooking, err := store.CreateBooking(domain.Booking{
		ResourceID: resourceID,
		UserID:     1,
		StartTime:  currentStart,
		EndTime:    currentEnd,
		Status:     domain.BookingActive,
		Purpose:    "current",
		CreatedAt:  now.Add(-1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("seed current booking: %v", err)
	}
	futureResp := performRequest(t, app, nethttp.MethodPost, "/bookings", map[string]any{
		"resource_id": resourceID,
		"start_time":  futureStart.Format(time.RFC3339),
		"end_time":    futureEnd.Format(time.RFC3339),
		"purpose":     "future",
	}, employeeToken)
	if futureResp.Code != nethttp.StatusCreated {
		t.Fatalf("expected booking create status %d, got %d", nethttp.StatusCreated, futureResp.Code)
	}
	var futureBooking domain.Booking
	decodeResponse(t, futureResp.Body, &futureBooking)

	resp := performRequest(t, app, nethttp.MethodDelete, "/resources/"+strconv.FormatInt(resourceID, 10), nil, adminToken)
	if resp.Code != nethttp.StatusOK {
		t.Fatalf("expected status %d, got %d", nethttp.StatusOK, resp.Code)
	}

	var payload struct {
		Resource               domain.Resource `json:"resource"`
		CancelledBookingsCount int             `json:"cancelled_bookings_count"`
	}
	decodeResponse(t, resp.Body, &payload)
	if payload.CancelledBookingsCount != 1 {
		t.Fatalf("expected 1 cancelled booking, got %d", payload.CancelledBookingsCount)
	}
	if payload.Resource.IsActive {
		t.Fatal("expected returned resource to be inactive")
	}

	gotCurrent, err := store.GetBooking(currentBooking.ID)
	if err != nil {
		t.Fatalf("get current booking: %v", err)
	}
	if gotCurrent.Status != domain.BookingActive {
		t.Fatalf("expected current booking to remain active, got %s", gotCurrent.Status)
	}

	gotFuture, err := store.GetBooking(futureBooking.ID)
	if err != nil {
		t.Fatalf("get future booking: %v", err)
	}
	if gotFuture.Status != domain.BookingCancelled {
		t.Fatalf("expected future booking cancelled, got %s", gotFuture.Status)
	}
}

func TestPutResourceInactiveCancelsFutureBookingsAndBlocksNewOnes(t *testing.T) {
	app, auth, store := newTestAppWithStore(t)
	adminToken := mustLogin(t, auth, "admin@corp.local", "admin123")
	employeeToken := mustLogin(t, auth, "employee@example.com", "password123")

	resourceID := createResourceViaAPI(t, app, adminToken, "Room Put Inactive")
	start := time.Now().UTC().Add(4 * time.Hour).Truncate(time.Second)
	end := start.Add(time.Hour)

	createResp := performRequest(t, app, nethttp.MethodPost, "/bookings", map[string]any{
		"resource_id": resourceID,
		"start_time":  start.Format(time.RFC3339),
		"end_time":    end.Format(time.RFC3339),
		"purpose":     "future",
	}, employeeToken)
	if createResp.Code != nethttp.StatusCreated {
		t.Fatalf("expected booking create status %d, got %d", nethttp.StatusCreated, createResp.Code)
	}
	var booking domain.Booking
	decodeResponse(t, createResp.Body, &booking)

	updatePayload := resourcePayload("Room Put Inactive")
	updatePayload["is_active"] = false
	resp := performRequest(t, app, nethttp.MethodPut, "/resources/"+strconv.FormatInt(resourceID, 10), updatePayload, adminToken)
	if resp.Code != nethttp.StatusOK {
		t.Fatalf("expected status %d, got %d", nethttp.StatusOK, resp.Code)
	}

	var payload struct {
		Resource               domain.Resource `json:"resource"`
		CancelledBookingsCount int             `json:"cancelled_bookings_count"`
	}
	decodeResponse(t, resp.Body, &payload)
	if payload.CancelledBookingsCount != 1 {
		t.Fatalf("expected 1 cancelled booking, got %d", payload.CancelledBookingsCount)
	}
	if payload.Resource.IsActive {
		t.Fatal("expected updated resource to be inactive")
	}

	gotBooking, err := store.GetBooking(booking.ID)
	if err != nil {
		t.Fatalf("get booking: %v", err)
	}
	if gotBooking.Status != domain.BookingCancelled {
		t.Fatalf("expected booking cancelled, got %s", gotBooking.Status)
	}

	newResp := performRequest(t, app, nethttp.MethodPost, "/bookings", map[string]any{
		"resource_id": resourceID,
		"start_time":  start.Add(2 * time.Hour).Format(time.RFC3339),
		"end_time":    end.Add(2 * time.Hour).Format(time.RFC3339),
		"purpose":     "new",
	}, employeeToken)
	if newResp.Code != nethttp.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", nethttp.StatusBadRequest, newResp.Code)
	}

	var errPayload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	decodeResponse(t, newResp.Body, &errPayload)
	if errPayload.Error.Message != "resource is inactive" {
		t.Fatalf("expected inactive resource error, got %q", errPayload.Error.Message)
	}
}

func TestEquipmentEndpointReturnsSortedTags(t *testing.T) {
	app, auth := newTestApp(t)
	adminToken := mustLogin(t, auth, "admin@corp.local", "admin123")

	createResourceWithAttrs(t, app, adminToken, "Room A", []string{"Whiteboard", "projector"}, nil)
	createResourceWithAttrs(t, app, adminToken, "Room B", []string{"tv"}, nil)

	resp := performRequest(t, app, nethttp.MethodGet, "/equipment", nil, "")
	if resp.Code != nethttp.StatusOK {
		t.Fatalf("expected status %d, got %d", nethttp.StatusOK, resp.Code)
	}

	var payload struct {
		Items []string `json:"items"`
	}
	decodeResponse(t, resp.Body, &payload)
	if !reflect.DeepEqual(payload.Items, []string{"projector", "tv", "whiteboard"}) {
		t.Fatalf("unexpected equipment list: %v", payload.Items)
	}
}

func newTestApp(t *testing.T) (*App, *service.AuthService) {
	app, auth, _ := newTestAppWithStore(t)
	return app, auth
}

func newTestAppWithStore(t *testing.T) (*App, *service.AuthService, *repository.MemoryStore) {
	t.Helper()

	store := repository.NewMemoryStore()
	authService := service.NewAuthService(store, "test-secret")
	if err := authService.SeedAdmin("System Admin", "admin@corp.local", "admin123"); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	if _, _, err := authService.Register("Employee User", "employee@example.com", "password123", domain.RoleEmployee); err != nil {
		t.Fatalf("seed employee: %v", err)
	}

	app := &App{
		logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		authService:     authService,
		resourceService: service.NewResourceService(store, store, cache.NewNoop(), nil),
		bookingService:  service.NewBookingService(store, store, cache.NewNoop(), nil),
	}

	mux := nethttp.NewServeMux()
	app.registerRoutes(mux)
	app.server = &nethttp.Server{Handler: mux}

	return app, authService, store
}

func mustLogin(t *testing.T, auth *service.AuthService, email, password string) string {
	t.Helper()

	_, token, err := auth.Login(email, password)
	if err != nil {
		t.Fatalf("login %s: %v", email, err)
	}

	return token
}

func performRequest(t *testing.T, app *App, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()

	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		payload = bytes.NewReader(data)
	}

	req := httptest.NewRequest(method, path, payload)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	app.server.Handler.ServeHTTP(rec, req)
	return rec
}

func decodeResponse(t *testing.T, body *bytes.Buffer, dst any) {
	t.Helper()

	if err := json.NewDecoder(body).Decode(dst); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func createResourceViaAPI(t *testing.T, app *App, token, name string) int64 {
	t.Helper()

	resp := performRequest(t, app, nethttp.MethodPost, "/resources", resourcePayload(name), token)
	if resp.Code != nethttp.StatusCreated {
		t.Fatalf("expected status %d, got %d", nethttp.StatusCreated, resp.Code)
	}

	var resource domain.Resource
	decodeResponse(t, resp.Body, &resource)
	return resource.ID
}

func createResourceWithAttrs(t *testing.T, app *App, token, name string, equipment, imageURLs []string) int64 {
	t.Helper()

	payload := resourcePayload(name)
	payload["equipment"] = equipment
	payload["image_urls"] = imageURLs

	resp := performRequest(t, app, nethttp.MethodPost, "/resources", payload, token)
	if resp.Code != nethttp.StatusCreated {
		t.Fatalf("expected status %d, got %d", nethttp.StatusCreated, resp.Code)
	}

	var resource domain.Resource
	decodeResponse(t, resp.Body, &resource)
	return resource.ID
}

func resourcePayload(name string) map[string]any {
	return map[string]any{
		"name":        name,
		"type":        "meeting_room",
		"location":    "HQ",
		"capacity":    8,
		"description": "Main room",
	}
}
