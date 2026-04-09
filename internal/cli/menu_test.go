package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"diplom/internal/cache"
	"diplom/internal/config"
	"diplom/internal/domain"
	httpapi "diplom/internal/http"
	"diplom/internal/repository"
	"diplom/internal/service"
)

func TestMenuHandlesInvalidChoiceAndExit(t *testing.T) {
	menu, _, output := newTestMenu("9\n0\n")

	if err := menu.Run(); err != nil {
		t.Fatalf("run menu: %v", err)
	}

	if !strings.Contains(output.String(), "Неверный выбор") {
		t.Fatalf("expected invalid choice warning, got %q", output.String())
	}
}

func TestMenuCreatesResource(t *testing.T) {
	menu, store, output := newTestMenu(
		"2\n3\nRoom A\n1\nHQ\n10\nBoard room\n2\n0\n0\n",
	)

	if err := menu.Run(); err != nil {
		t.Fatalf("run menu: %v", err)
	}

	resources := store.ListResources("", false)
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	if resources[0].Name != "Room A" {
		t.Fatalf("unexpected resource: %+v", resources[0])
	}
	if !strings.Contains(output.String(), "Ресурс создан") {
		t.Fatalf("expected create confirmation, got %q", output.String())
	}
}

func TestMenuCreatesAndCancelsBookingAndShowsLogs(t *testing.T) {
	menu, store, output := newTestMenu(
		"3\n2\n2\n1\n2099-05-01 10:00\n2099-05-01 11:00\nPlanning\n3\n1\n0\n5\n\n0\n",
	)

	admin, err := menu.services.Auth.GetUserByEmail(menu.defaultAdmin.Email)
	if err != nil {
		t.Fatalf("get admin: %v", err)
	}
	employee, _, err := menu.services.Auth.Register("Employee", "employee@example.com", "pass123", domain.RoleEmployee)
	if err != nil {
		t.Fatalf("create employee: %v", err)
	}
	if _, err := menu.services.Resource.Create("Desk 7", domain.ResourceWorkspace, "Floor 2", 0, "Quiet zone"); err != nil {
		t.Fatalf("create resource: %v", err)
	}

	menu.logs.(*stubLogs).lines = append(menu.logs.(*stubLogs).lines, "server started")

	if err := menu.Run(); err != nil {
		t.Fatalf("run menu: %v", err)
	}

	bookings := store.ListBookings()
	if len(bookings) != 1 {
		t.Fatalf("expected 1 booking, got %d", len(bookings))
	}
	if bookings[0].UserID != employee.ID {
		t.Fatalf("unexpected booking user id: %d", bookings[0].UserID)
	}
	if bookings[0].Status != domain.BookingCancelled {
		t.Fatalf("expected cancelled booking, got %s", bookings[0].Status)
	}
	if admin.Role != domain.RoleAdmin {
		t.Fatalf("unexpected admin role: %s", admin.Role)
	}
	if !strings.Contains(output.String(), "server started") {
		t.Fatalf("expected log output, got %q", output.String())
	}
}

type stubLogs struct {
	lines []string
}

func (s *stubLogs) Logs() []string {
	return append([]string(nil), s.lines...)
}

func newTestMenu(input string) (*Menu, *repository.MemoryStore, *bytes.Buffer) {
	store := repository.NewMemoryStore()
	authService := service.NewAuthService(store, "secret")
	resourceService := service.NewResourceService(store, cache.NewNoop())
	bookingService := service.NewBookingService(store, store, cache.NewNoop())
	if err := authService.SeedAdmin("Admin", "admin@corp.local", "admin123"); err != nil {
		panic(err)
	}

	output := &bytes.Buffer{}
	logs := &stubLogs{}
	menu := NewMenu(
		httpapi.AppServices{
			Auth:     authService,
			Resource: resourceService,
			Booking:  bookingService,
		},
		logs,
		":8080",
		config.DefaultAdmin{
			FullName: "Admin",
			Email:    "admin@corp.local",
			Password: "admin123",
		},
		strings.NewReader(input),
		output,
	)

	time.Local = time.UTC
	return menu, store, output
}
