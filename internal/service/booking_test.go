package service

import (
	"testing"
	"time"

	"diplom/internal/cache"
	"diplom/internal/domain"
	"diplom/internal/repository"
)

func TestBookingServiceCreateRejectsOverlappingBookings(t *testing.T) {
	store := repository.NewMemoryStore()
	resourceService := NewResourceService(store, cache.NewNoop())
	bookingService := NewBookingService(store, store, cache.NewNoop())

	resource, err := resourceService.Create("Room A", domain.ResourceMeetingRoom, "HQ", 8, "Main room")
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}

	start := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	end := start.Add(1 * time.Hour)

	_, err = bookingService.Create(1, resource.ID, start, end, "first booking")
	if err != nil {
		t.Fatalf("create initial booking: %v", err)
	}

	_, err = bookingService.Create(2, resource.ID, start.Add(30*time.Minute), end.Add(30*time.Minute), "overlap")
	if err == nil {
		t.Fatal("expected overlap error")
	}
	if got := err.Error(); got != "resource already booked for selected time" {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestBookingServiceCreateAllowsAdjacentBookings(t *testing.T) {
	store := repository.NewMemoryStore()
	resourceService := NewResourceService(store, cache.NewNoop())
	bookingService := NewBookingService(store, store, cache.NewNoop())

	resource, err := resourceService.Create("Desk 1", domain.ResourceWorkspace, "HQ", 0, "Window desk")
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}

	start := time.Now().UTC().Add(4 * time.Hour).Truncate(time.Second)
	mid := start.Add(1 * time.Hour)
	end := mid.Add(1 * time.Hour)

	_, err = bookingService.Create(1, resource.ID, start, mid, "first")
	if err != nil {
		t.Fatalf("create first booking: %v", err)
	}

	_, err = bookingService.Create(2, resource.ID, mid, end, "second")
	if err != nil {
		t.Fatalf("expected adjacent booking to succeed, got: %v", err)
	}
}

func TestBookingServiceCreateRejectsPastBooking(t *testing.T) {
	store := repository.NewMemoryStore()
	resourceService := NewResourceService(store, cache.NewNoop())
	bookingService := NewBookingService(store, store, cache.NewNoop())

	resource, err := resourceService.Create("Room B", domain.ResourceMeetingRoom, "HQ", 4, "Small room")
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}

	start := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	end := start.Add(1 * time.Hour)

	_, err = bookingService.Create(1, resource.ID, start, end, "past")
	if err == nil {
		t.Fatal("expected error for booking in the past")
	}
	if got := err.Error(); got != "cannot book in the past" {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestBookingServiceCancelRespectsOwnership(t *testing.T) {
	store := repository.NewMemoryStore()
	resourceService := NewResourceService(store, cache.NewNoop())
	bookingService := NewBookingService(store, store, cache.NewNoop())

	resource, err := resourceService.Create("Room C", domain.ResourceMeetingRoom, "HQ", 6, "Project room")
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}

	start := time.Now().UTC().Add(3 * time.Hour).Truncate(time.Second)
	end := start.Add(1 * time.Hour)

	booking, err := bookingService.Create(1, resource.ID, start, end, "owned booking")
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}

	_, err = bookingService.Cancel(domain.User{ID: 2, Role: domain.RoleEmployee}, booking.ID)
	if err == nil {
		t.Fatal("expected forbidden error")
	}
	if got := err.Error(); got != "forbidden" {
		t.Fatalf("unexpected error: %s", got)
	}

	cancelled, err := bookingService.Cancel(domain.User{ID: 1, Role: domain.RoleEmployee}, booking.ID)
	if err != nil {
		t.Fatalf("owner cancel should succeed: %v", err)
	}
	if cancelled.Status != domain.BookingCancelled {
		t.Fatalf("expected cancelled status, got %s", cancelled.Status)
	}
}
