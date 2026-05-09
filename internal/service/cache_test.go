package service

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"diplom/internal/cache"
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
	resourceService := NewResourceService(store, store, c, nil)
	bookingService := NewBookingService(store, store, c, nil)

	resource, err := resourceService.Create("Room A", domain.ResourceMeetingRoom, "HQ", 8, "Room", nil, nil)
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}

	start := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	end := start.Add(time.Hour)

	first, err := bookingService.Availability(start, end, domain.ResourceMeetingRoom, nil)
	if err != nil {
		t.Fatalf("first availability: %v", err)
	}
	if len(first) != 1 || first[0].ID != resource.ID {
		t.Fatalf("unexpected first availability response: %+v", first)
	}
	if c.setCalls == 0 {
		t.Fatal("expected cache set on first request")
	}

	second, err := bookingService.Availability(start, end, domain.ResourceMeetingRoom, nil)
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
	resourceService := NewResourceService(store, store, c, nil)
	bookingService := NewBookingService(store, store, c, nil)

	resource, err := resourceService.Create("Room A", domain.ResourceMeetingRoom, "HQ", 8, "Room", nil, nil)
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}

	start := time.Now().UTC().Add(4 * time.Hour).Truncate(time.Second)
	end := start.Add(time.Hour)

	if _, err := bookingService.Availability(start, end, domain.ResourceMeetingRoom, nil); err != nil {
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

func TestResourceServiceNormalizesImagesAndEquipment(t *testing.T) {
	store := repository.NewMemoryStore()
	resourceService := NewResourceService(store, store, cache.NewNoop(), nil)

	resource, err := resourceService.Create(
		"Room A",
		domain.ResourceMeetingRoom,
		"HQ",
		8,
		"Room",
		[]string{" https://example.com/a.jpg ", "https://example.com/a.jpg", "", "https://example.com/b.jpg"},
		[]string{" Projector ", "whiteboard", "projector", ""},
	)
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}

	if !reflect.DeepEqual(resource.ImageURLs, []string{"https://example.com/a.jpg", "https://example.com/b.jpg"}) {
		t.Fatalf("unexpected image urls: %v", resource.ImageURLs)
	}
	if !reflect.DeepEqual(resource.Equipment, []string{"projector", "whiteboard"}) {
		t.Fatalf("unexpected equipment: %v", resource.Equipment)
	}
}

func TestResourceServiceUpdateReplacesImagesAndEquipment(t *testing.T) {
	store := repository.NewMemoryStore()
	resourceService := NewResourceService(store, store, cache.NewNoop(), nil)

	resource, err := resourceService.Create(
		"Room A",
		domain.ResourceMeetingRoom,
		"HQ",
		8,
		"Room",
		[]string{"https://example.com/a.jpg"},
		[]string{"projector"},
	)
	if err != nil {
		t.Fatalf("create resource: %v", err)
	}

	updated, err := resourceService.Update(
		resource.ID,
		"Room A",
		domain.ResourceMeetingRoom,
		"HQ",
		8,
		"Room updated",
		[]string{"https://example.com/b.jpg"},
		[]string{"tv"},
		true,
	)
	if err != nil {
		t.Fatalf("update resource: %v", err)
	}

	if !reflect.DeepEqual(updated.Resource.ImageURLs, []string{"https://example.com/b.jpg"}) {
		t.Fatalf("unexpected image urls after update: %v", updated.Resource.ImageURLs)
	}
	if !reflect.DeepEqual(updated.Resource.Equipment, []string{"tv"}) {
		t.Fatalf("unexpected equipment after update: %v", updated.Resource.Equipment)
	}
}

func TestBookingServiceAvailabilityFiltersByEquipment(t *testing.T) {
	store := repository.NewMemoryStore()
	resourceService := NewResourceService(store, store, cache.NewNoop(), nil)
	bookingService := NewBookingService(store, store, cache.NewNoop(), nil)

	_, err := resourceService.Create("Room A", domain.ResourceMeetingRoom, "HQ", 8, "Room", nil, []string{"projector"})
	if err != nil {
		t.Fatalf("create room A: %v", err)
	}
	roomB, err := resourceService.Create("Room B", domain.ResourceMeetingRoom, "HQ", 8, "Room", nil, []string{"projector", "whiteboard"})
	if err != nil {
		t.Fatalf("create room B: %v", err)
	}

	start := time.Now().UTC().Add(8 * time.Hour).Truncate(time.Second)
	end := start.Add(time.Hour)

	items, err := bookingService.Availability(start, end, domain.ResourceMeetingRoom, []string{"projector", "whiteboard"})
	if err != nil {
		t.Fatalf("availability: %v", err)
	}
	if len(items) != 1 || items[0].ID != roomB.ID {
		t.Fatalf("unexpected filtered availability: %+v", items)
	}
}
