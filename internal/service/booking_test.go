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
	resourceService := NewResourceService(store, store, cache.NewNoop(), nil)
	bookingService := NewBookingService(store, store, cache.NewNoop(), nil)

	resource, err := resourceService.Create("Room A", domain.ResourceMeetingRoom, "HQ", 8, "Main room", nil, nil)
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
	resourceService := NewResourceService(store, store, cache.NewNoop(), nil)
	bookingService := NewBookingService(store, store, cache.NewNoop(), nil)

	resource, err := resourceService.Create("Desk 1", domain.ResourceWorkspace, "HQ", 0, "Window desk", nil, nil)
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
	resourceService := NewResourceService(store, store, cache.NewNoop(), nil)
	bookingService := NewBookingService(store, store, cache.NewNoop(), nil)

	resource, err := resourceService.Create("Room B", domain.ResourceMeetingRoom, "HQ", 4, "Small room", nil, nil)
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
	resourceService := NewResourceService(store, store, cache.NewNoop(), nil)
	bookingService := NewBookingService(store, store, cache.NewNoop(), nil)

	resource, err := resourceService.Create("Room C", domain.ResourceMeetingRoom, "HQ", 6, "Project room", nil, nil)
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

func TestResourceServiceDeactivationCancelsOnlyFutureBookings(t *testing.T) {
	store := repository.NewMemoryStore()
	resourceService := NewResourceService(store, store, cache.NewNoop(), nil)
	bookingService := NewBookingService(store, store, cache.NewNoop(), nil)

	resource, err := resourceService.Create("Room D", domain.ResourceMeetingRoom, "HQ", 8, "Main room", nil, nil)
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	currentStart := now.Add(-30 * time.Minute)
	currentEnd := now.Add(30 * time.Minute)
	futureStart := now.Add(2 * time.Hour)
	futureEnd := futureStart.Add(time.Hour)
	otherStart := now.Add(3 * time.Hour)
	otherEnd := otherStart.Add(time.Hour)

	currentBooking, err := store.CreateBooking(domain.Booking{
		ResourceID: resource.ID,
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
	futureBooking, err := bookingService.Create(2, resource.ID, futureStart, futureEnd, "future")
	if err != nil {
		t.Fatalf("create future booking: %v", err)
	}

	otherResource, err := resourceService.Create("Room E", domain.ResourceMeetingRoom, "HQ", 8, "Other room", nil, nil)
	if err != nil {
		t.Fatalf("create other resource: %v", err)
	}
	otherBooking, err := bookingService.Create(3, otherResource.ID, otherStart, otherEnd, "other")
	if err != nil {
		t.Fatalf("create other booking: %v", err)
	}

	result, err := resourceService.Disable(resource.ID)
	if err != nil {
		t.Fatalf("disable resource: %v", err)
	}
	if result.Resource.IsActive {
		t.Fatal("expected resource to be inactive")
	}
	if result.CancelledBookingsCount != 1 {
		t.Fatalf("expected 1 cancelled booking, got %d", result.CancelledBookingsCount)
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
	if gotFuture.CancelledAt == nil {
		t.Fatal("expected future booking cancelled_at to be set")
	}

	gotOther, err := store.GetBooking(otherBooking.ID)
	if err != nil {
		t.Fatalf("get other booking: %v", err)
	}
	if gotOther.Status != domain.BookingActive {
		t.Fatalf("expected other resource booking to remain active, got %s", gotOther.Status)
	}

	secondResult, err := resourceService.Disable(resource.ID)
	if err != nil {
		t.Fatalf("disable already inactive resource: %v", err)
	}
	if secondResult.CancelledBookingsCount != 0 {
		t.Fatalf("expected repeated disable to cancel 0 bookings, got %d", secondResult.CancelledBookingsCount)
	}
}

func TestResourceServiceUpdateWithoutDeactivationDoesNotCancelBookings(t *testing.T) {
	store := repository.NewMemoryStore()
	resourceService := NewResourceService(store, store, cache.NewNoop(), nil)
	bookingService := NewBookingService(store, store, cache.NewNoop(), nil)

	resource, err := resourceService.Create("Room F", domain.ResourceMeetingRoom, "HQ", 6, "Focus room", nil, nil)
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}

	start := time.Now().UTC().Add(4 * time.Hour).Truncate(time.Second)
	end := start.Add(time.Hour)
	booking, err := bookingService.Create(1, resource.ID, start, end, "future")
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}

	result, err := resourceService.Update(resource.ID, "Room F Updated", domain.ResourceMeetingRoom, "HQ", 6, "Updated", nil, nil, true)
	if err != nil {
		t.Fatalf("update resource: %v", err)
	}
	if result.CancelledBookingsCount != 0 {
		t.Fatalf("expected 0 cancelled bookings, got %d", result.CancelledBookingsCount)
	}

	gotBooking, err := store.GetBooking(booking.ID)
	if err != nil {
		t.Fatalf("get booking: %v", err)
	}
	if gotBooking.Status != domain.BookingActive {
		t.Fatalf("expected booking to remain active, got %s", gotBooking.Status)
	}
}
