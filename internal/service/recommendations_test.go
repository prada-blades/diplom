package service

import (
	"testing"
	"time"

	"diplom/internal/domain"
	"diplom/internal/repository"
)

func TestRecommendScheduleReturnsPreferredSlotFirst(t *testing.T) {
	store := repository.NewMemoryStore()
	resourceService := NewResourceService(store)
	bookingService := NewBookingService(store, store)

	smallRoom, err := resourceService.Create("Small", domain.ResourceMeetingRoom, "HQ", 4, "Small room")
	if err != nil {
		t.Fatalf("create small room: %v", err)
	}
	_, err = resourceService.Create("Large", domain.ResourceMeetingRoom, "HQ", 10, "Large room")
	if err != nil {
		t.Fatalf("create large room: %v", err)
	}

	preferredStart := nextFutureSlot(48 * time.Hour)
	items, err := bookingService.RecommendSchedule(ScheduleRecommendationRequest{
		ResourceType:      domain.ResourceMeetingRoom,
		Participants:      4,
		DurationMinutes:   60,
		PreferredStart:    preferredStart,
		SearchWindowStart: preferredStart.Add(-1 * time.Hour),
		SearchWindowEnd:   preferredStart.Add(1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("recommend schedule: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected recommendations")
	}
	if items[0].ResourceID != smallRoom.ID {
		t.Fatalf("expected preferred small room first, got resource %d", items[0].ResourceID)
	}
	if !items[0].StartTime.Equal(preferredStart) {
		t.Fatalf("expected preferred slot first, got %s", items[0].StartTime)
	}
}

func TestRecommendScheduleFindsNearestSlotWhenPreferredIsBusy(t *testing.T) {
	store := repository.NewMemoryStore()
	resourceService := NewResourceService(store)
	bookingService := NewBookingService(store, store)

	room, err := resourceService.Create("Room A", domain.ResourceMeetingRoom, "HQ", 6, "Main room")
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}

	preferredStart := nextFutureSlot(72 * time.Hour)
	seedActiveBooking(t, store, room.ID, 1, preferredStart, preferredStart.Add(1*time.Hour))

	items, err := bookingService.RecommendSchedule(ScheduleRecommendationRequest{
		ResourceType:      domain.ResourceMeetingRoom,
		Participants:      4,
		DurationMinutes:   60,
		PreferredStart:    preferredStart,
		SearchWindowStart: preferredStart.Add(-1 * time.Hour),
		SearchWindowEnd:   preferredStart.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("recommend schedule: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected recommendations")
	}
	expectedStart := preferredStart.Add(-1 * time.Hour)
	if !items[0].StartTime.Equal(expectedStart) {
		t.Fatalf("expected nearest earlier slot %s, got %s", expectedStart, items[0].StartTime)
	}
}

func TestRecommendSchedulePrefersSmallerCapacityExcess(t *testing.T) {
	store := repository.NewMemoryStore()
	resourceService := NewResourceService(store)
	bookingService := NewBookingService(store, store)

	compactRoom, err := resourceService.Create("Compact", domain.ResourceMeetingRoom, "HQ", 4, "Compact room")
	if err != nil {
		t.Fatalf("create compact room: %v", err)
	}
	_, err = resourceService.Create("Oversized", domain.ResourceMeetingRoom, "HQ", 12, "Oversized room")
	if err != nil {
		t.Fatalf("create oversized room: %v", err)
	}

	preferredStart := nextFutureSlot(96 * time.Hour)
	items, err := bookingService.RecommendSchedule(ScheduleRecommendationRequest{
		ResourceType:      domain.ResourceMeetingRoom,
		Participants:      4,
		DurationMinutes:   60,
		PreferredStart:    preferredStart,
		SearchWindowStart: preferredStart,
		SearchWindowEnd:   preferredStart.Add(90 * time.Minute),
	})
	if err != nil {
		t.Fatalf("recommend schedule: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected recommendations")
	}
	if items[0].ResourceID != compactRoom.ID {
		t.Fatalf("expected compact room first, got resource %d", items[0].ResourceID)
	}
}

func TestRecommendSchedulePrefersLessUtilizedRoomWhenCapacityMatches(t *testing.T) {
	store := repository.NewMemoryStore()
	resourceService := NewResourceService(store)
	bookingService := NewBookingService(store, store)

	busyRoom, err := resourceService.Create("Busy", domain.ResourceMeetingRoom, "HQ", 6, "Busy room")
	if err != nil {
		t.Fatalf("create busy room: %v", err)
	}
	idleRoom, err := resourceService.Create("Idle", domain.ResourceMeetingRoom, "HQ", 6, "Idle room")
	if err != nil {
		t.Fatalf("create idle room: %v", err)
	}

	preferredStart := nextFutureSlot(120 * time.Hour)
	historyStart := preferredStart.Add(-7 * 24 * time.Hour)
	seedActiveBooking(t, store, busyRoom.ID, 1, historyStart, historyStart.Add(4*time.Hour))

	items, err := bookingService.RecommendSchedule(ScheduleRecommendationRequest{
		ResourceType:      domain.ResourceMeetingRoom,
		Participants:      4,
		DurationMinutes:   60,
		PreferredStart:    preferredStart,
		SearchWindowStart: preferredStart,
		SearchWindowEnd:   preferredStart.Add(90 * time.Minute),
	})
	if err != nil {
		t.Fatalf("recommend schedule: %v", err)
	}
	if len(items) < 2 {
		t.Fatalf("expected at least two recommendations, got %d", len(items))
	}
	if items[0].ResourceID != idleRoom.ID {
		t.Fatalf("expected less utilized room first, got resource %d", items[0].ResourceID)
	}
}

func TestRecommendScheduleAllowsAdjacentIntervals(t *testing.T) {
	store := repository.NewMemoryStore()
	resourceService := NewResourceService(store)
	bookingService := NewBookingService(store, store)

	room, err := resourceService.Create("Room Adjacent", domain.ResourceMeetingRoom, "HQ", 6, "Adjacent room")
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}

	preferredStart := nextFutureSlot(144 * time.Hour)
	seedActiveBooking(t, store, room.ID, 1, preferredStart.Add(-1*time.Hour), preferredStart)

	items, err := bookingService.RecommendSchedule(ScheduleRecommendationRequest{
		ResourceType:      domain.ResourceMeetingRoom,
		Participants:      4,
		DurationMinutes:   60,
		PreferredStart:    preferredStart,
		SearchWindowStart: preferredStart,
		SearchWindowEnd:   preferredStart.Add(90 * time.Minute),
	})
	if err != nil {
		t.Fatalf("recommend schedule: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected recommendations")
	}
	if !items[0].StartTime.Equal(preferredStart) {
		t.Fatalf("expected adjacent interval to stay available, got %s", items[0].StartTime)
	}
}

func TestRecommendScheduleReturnsEmptyWhenNoCandidatesExist(t *testing.T) {
	store := repository.NewMemoryStore()
	resourceService := NewResourceService(store)
	bookingService := NewBookingService(store, store)

	room, err := resourceService.Create("Room Busy", domain.ResourceMeetingRoom, "HQ", 6, "Busy room")
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}

	preferredStart := nextFutureSlot(168 * time.Hour)
	searchWindowStart := preferredStart.Add(-1 * time.Hour)
	searchWindowEnd := preferredStart.Add(1 * time.Hour)
	seedActiveBooking(t, store, room.ID, 1, searchWindowStart, searchWindowEnd)

	items, err := bookingService.RecommendSchedule(ScheduleRecommendationRequest{
		ResourceType:      domain.ResourceMeetingRoom,
		Participants:      4,
		DurationMinutes:   60,
		PreferredStart:    preferredStart,
		SearchWindowStart: searchWindowStart,
		SearchWindowEnd:   searchWindowEnd,
	})
	if err != nil {
		t.Fatalf("recommend schedule: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no recommendations, got %d", len(items))
	}
}

func TestUtilizationReportIncludesHourAndWeekdayStats(t *testing.T) {
	store := repository.NewMemoryStore()
	resourceService := NewResourceService(store)
	bookingService := NewBookingService(store, store)

	roomA, err := resourceService.Create("Room A", domain.ResourceMeetingRoom, "HQ", 6, "Room A")
	if err != nil {
		t.Fatalf("create room A: %v", err)
	}
	roomB, err := resourceService.Create("Room B", domain.ResourceMeetingRoom, "HQ", 6, "Room B")
	if err != nil {
		t.Fatalf("create room B: %v", err)
	}

	reportStart := nextMondayUTC().Add(9 * time.Hour)
	reportEnd := reportStart.Add(3 * time.Hour)
	seedActiveBooking(t, store, roomA.ID, 1, reportStart, reportStart.Add(1*time.Hour))
	seedActiveBooking(t, store, roomB.ID, 2, reportStart.Add(1*time.Hour), reportStart.Add(90*time.Minute))

	report, err := bookingService.UtilizationReport(reportStart, reportEnd)
	if err != nil {
		t.Fatalf("utilization report: %v", err)
	}
	if len(report.Items) != 2 {
		t.Fatalf("expected 2 utilization items, got %d", len(report.Items))
	}
	if len(report.Stats.ResourceLoads) != 2 {
		t.Fatalf("expected 2 resource stats, got %d", len(report.Stats.ResourceLoads))
	}
	if got := report.Stats.HourLoads[9].BookedMinutes; got != 60 {
		t.Fatalf("expected hour 9 to have 60 booked minutes, got %d", got)
	}
	if got := report.Stats.HourLoads[10].BookedMinutes; got != 30 {
		t.Fatalf("expected hour 10 to have 30 booked minutes, got %d", got)
	}
	if report.Stats.WeekdayLoads[0].Weekday != "monday" {
		t.Fatalf("expected first weekday to be monday, got %s", report.Stats.WeekdayLoads[0].Weekday)
	}
	if got := report.Stats.WeekdayLoads[0].BookedMinutes; got != 90 {
		t.Fatalf("expected monday to have 90 booked minutes, got %d", got)
	}
	if got := report.Stats.WeekdayLoads[0].SharePercent; got != 100 {
		t.Fatalf("expected monday share to be 100, got %v", got)
	}
}

func seedActiveBooking(t *testing.T, store *repository.MemoryStore, resourceID, userID int64, start, end time.Time) {
	t.Helper()

	_, err := store.CreateBooking(domain.Booking{
		ResourceID: resourceID,
		UserID:     userID,
		StartTime:  start.UTC(),
		EndTime:    end.UTC(),
		Status:     domain.BookingActive,
		Purpose:    "seed",
		CreatedAt:  start.UTC().Add(-1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("seed booking: %v", err)
	}
}

func nextFutureSlot(offset time.Duration) time.Time {
	base := time.Now().UTC().Add(offset).Truncate(recommendationSlotStep)
	if base.Minute()%15 != 0 || base.Second() != 0 || base.Nanosecond() != 0 {
		return base.Truncate(recommendationSlotStep).Add(recommendationSlotStep)
	}
	return base
}

func nextMondayUTC() time.Time {
	now := time.Now().UTC().Truncate(24 * time.Hour)
	for now.Weekday() != time.Monday {
		now = now.Add(24 * time.Hour)
	}
	return now.Add(7 * 24 * time.Hour)
}
