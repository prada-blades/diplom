package service

import (
	"errors"
	"testing"
	"time"

	"diplom/internal/domain"
	"diplom/internal/repository"
)

type spyCache struct {
	values      map[string][]byte
	getHits     int
	setCalls    int
	deleteCalls []string
}

func newSpyCache() *spyCache {
	return &spyCache{values: make(map[string][]byte)}
}

func (c *spyCache) Get(key string) ([]byte, error) {
	value, ok := c.values[key]
	if !ok {
		return nil, errors.New("cache miss")
	}
	c.getHits++
	return value, nil
}

func (c *spyCache) Set(key string, value []byte, _ time.Duration) error {
	c.setCalls++
	c.values[key] = value
	return nil
}

func (c *spyCache) DeleteByPrefix(prefix string) error {
	c.deleteCalls = append(c.deleteCalls, prefix)
	for key := range c.values {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(c.values, key)
		}
	}
	return nil
}

func TestBookingServiceAvailabilityUsesCache(t *testing.T) {
	store := repository.NewMemoryStore()
	c := newSpyCache()
	resourceService := NewResourceService(store, c)
	bookingService := NewBookingService(store, store, c)

	resource, err := resourceService.Create("Room A", domain.ResourceMeetingRoom, "HQ", 8, "Room")
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}

	start := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	end := start.Add(time.Hour)

	first, err := bookingService.Availability(start, end, domain.ResourceMeetingRoom)
	if err != nil {
		t.Fatalf("first availability: %v", err)
	}
	if len(first) != 1 || first[0].ID != resource.ID {
		t.Fatalf("unexpected first availability response: %+v", first)
	}
	if c.setCalls == 0 {
		t.Fatal("expected cache set on first request")
	}

	second, err := bookingService.Availability(start, end, domain.ResourceMeetingRoom)
	if err != nil {
		t.Fatalf("second availability: %v", err)
	}
	if len(second) != 1 || second[0].ID != resource.ID {
		t.Fatalf("unexpected second availability response: %+v", second)
	}
	if c.getHits == 0 {
		t.Fatal("expected cache hit on second request")
	}
}

func TestBookingServiceCreateInvalidatesAvailabilityAndUtilizationCache(t *testing.T) {
	store := repository.NewMemoryStore()
	c := newSpyCache()
	resourceService := NewResourceService(store, c)
	bookingService := NewBookingService(store, store, c)

	resource, err := resourceService.Create("Room A", domain.ResourceMeetingRoom, "HQ", 8, "Room")
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}

	start := time.Now().UTC().Add(4 * time.Hour).Truncate(time.Second)
	end := start.Add(time.Hour)

	if _, err := bookingService.Availability(start, end, domain.ResourceMeetingRoom); err != nil {
		t.Fatalf("prime availability cache: %v", err)
	}
	if _, err := bookingService.UtilizationReport(start, end); err != nil {
		t.Fatalf("prime utilization cache: %v", err)
	}

	if _, err := bookingService.Create(1, resource.ID, start, end, "meeting"); err != nil {
		t.Fatalf("create booking: %v", err)
	}

	foundAvailability := false
	foundUtilization := false
	for _, prefix := range c.deleteCalls {
		if prefix == availabilityCachePrefix {
			foundAvailability = true
		}
		if prefix == utilizationCachePrefix {
			foundUtilization = true
		}
	}
	if !foundAvailability || !foundUtilization {
		t.Fatalf("expected both cache prefixes invalidated, got %v", c.deleteCalls)
	}
}
