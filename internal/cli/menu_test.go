package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"diplom/internal/bootstrap"
	"diplom/internal/cache"
	"diplom/internal/config"
	"diplom/internal/domain"
	"diplom/internal/repository"
	"diplom/internal/service"
)

func TestMenuHandlesInvalidChoiceAndEOF(t *testing.T) {
	menu, _, output := newTestMenu("9\n")

	if err := menu.Run(); err != nil {
		t.Fatalf("run menu: %v", err)
	}

	if !strings.Contains(output.String(), "Неверный выбор") {
		t.Fatalf("expected invalid choice warning, got %q", output.String())
	}
}

func TestMenuCreatesResource(t *testing.T) {
	menu, store, output := newTestMenu(
		"2\n3\nRoom A\n1\nHQ\n10\nBoard room\n0\n",
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

func TestMenuCreatesAndCancelsBooking(t *testing.T) {
	menu, store, _ := newTestMenu(
		"3\n2\n2\n1\n2099-05-01 10:00\n2099-05-01 11:00\nPlanning\n3\n1\n0\n",
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
	menu := NewMenu(
		bootstrap.Services{
			Auth:     authService,
			Resource: resourceService,
			Booking:  bookingService,
		},
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
