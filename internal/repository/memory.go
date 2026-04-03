package repository

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"diplom/internal/domain"
)

type MemoryStore struct {
	mu             sync.RWMutex
	nextUserID     int64
	nextResourceID int64
	nextBookingID  int64
	users          map[int64]domain.User
	usersByEmail   map[string]int64
	resources      map[int64]domain.Resource
	bookings       map[int64]domain.Booking
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		nextUserID:     1,
		nextResourceID: 1,
		nextBookingID:  1,
		users:          make(map[int64]domain.User),
		usersByEmail:   make(map[string]int64),
		resources:      make(map[int64]domain.Resource),
		bookings:       make(map[int64]domain.Booking),
	}
}

func (s *MemoryStore) CreateUser(user domain.User) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	email := strings.ToLower(strings.TrimSpace(user.Email))
	if _, exists := s.usersByEmail[email]; exists {
		return domain.User{}, errors.New("email already exists")
	}

	user.ID = s.nextUserID
	s.nextUserID++
	user.Email = email
	s.users[user.ID] = user
	s.usersByEmail[email] = user.ID

	return user, nil
}

func (s *MemoryStore) GetUserByEmail(email string) (domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.usersByEmail[strings.ToLower(strings.TrimSpace(email))]
	if !ok {
		return domain.User{}, ErrNotFound
	}

	user, ok := s.users[id]
	if !ok {
		return domain.User{}, ErrNotFound
	}

	return user, nil
}

func (s *MemoryStore) GetUserByID(id int64) (domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[id]
	if !ok {
		return domain.User{}, ErrNotFound
	}

	return user, nil
}

func (s *MemoryStore) ListUsers() []domain.User {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]domain.User, 0, len(s.users))
	for _, user := range s.users {
		users = append(users, user)
	}

	sort.Slice(users, func(i, j int) bool {
		return users[i].ID < users[j].ID
	})

	return users
}

func (s *MemoryStore) UpdateUser(id int64, update domain.User) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.users[id]
	if !ok {
		return domain.User{}, ErrNotFound
	}

	email := strings.ToLower(strings.TrimSpace(update.Email))
	if email == "" {
		email = current.Email
	}
	if existingID, exists := s.usersByEmail[email]; exists && existingID != id {
		return domain.User{}, errors.New("email already exists")
	}

	delete(s.usersByEmail, current.Email)
	update.ID = current.ID
	update.Email = email
	update.CreatedAt = current.CreatedAt
	if update.PasswordHash == "" {
		update.PasswordHash = current.PasswordHash
	}

	s.users[id] = update
	s.usersByEmail[email] = id

	return update, nil
}

func (s *MemoryStore) CreateResource(resource domain.Resource) (domain.Resource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	resource.ID = s.nextResourceID
	s.nextResourceID++
	s.resources[resource.ID] = resource

	return resource, nil
}

func (s *MemoryStore) UpdateResource(id int64, update domain.Resource) (domain.Resource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.resources[id]
	if !ok {
		return domain.Resource{}, ErrNotFound
	}

	update.ID = current.ID
	update.CreatedAt = current.CreatedAt
	s.resources[id] = update

	return update, nil
}

func (s *MemoryStore) GetResource(id int64) (domain.Resource, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resource, ok := s.resources[id]
	if !ok {
		return domain.Resource{}, ErrNotFound
	}

	return resource, nil
}

func (s *MemoryStore) ListResources(resourceType domain.ResourceType, onlyActive bool) []domain.Resource {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resources := make([]domain.Resource, 0, len(s.resources))
	for _, resource := range s.resources {
		if resourceType != "" && resource.Type != resourceType {
			continue
		}
		if onlyActive && !resource.IsActive {
			continue
		}
		resources = append(resources, resource)
	}

	sort.Slice(resources, func(i, j int) bool {
		return resources[i].ID < resources[j].ID
	})

	return resources
}

func (s *MemoryStore) CreateBooking(booking domain.Booking) (domain.Booking, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, existing := range s.bookings {
		if existing.ResourceID != booking.ResourceID || existing.Status != domain.BookingActive {
			continue
		}
		if overlaps(existing.StartTime, existing.EndTime, booking.StartTime, booking.EndTime) {
			return domain.Booking{}, errors.New("resource already booked for selected time")
		}
	}

	booking.ID = s.nextBookingID
	s.nextBookingID++
	s.bookings[booking.ID] = booking

	return booking, nil
}

func (s *MemoryStore) GetBooking(id int64) (domain.Booking, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	booking, ok := s.bookings[id]
	if !ok {
		return domain.Booking{}, ErrNotFound
	}

	return booking, nil
}

func (s *MemoryStore) CancelBooking(id int64, cancelledAt time.Time) (domain.Booking, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	booking, ok := s.bookings[id]
	if !ok {
		return domain.Booking{}, ErrNotFound
	}

	booking.Status = domain.BookingCancelled
	booking.CancelledAt = &cancelledAt
	s.bookings[id] = booking

	return booking, nil
}

func (s *MemoryStore) ListBookingsByUser(userID int64) []domain.Booking {
	s.mu.RLock()
	defer s.mu.RUnlock()

	bookings := make([]domain.Booking, 0)
	for _, booking := range s.bookings {
		if booking.UserID == userID {
			bookings = append(bookings, booking)
		}
	}

	sort.Slice(bookings, func(i, j int) bool {
		return bookings[i].StartTime.Before(bookings[j].StartTime)
	})

	return bookings
}

func (s *MemoryStore) ListBookings() []domain.Booking {
	s.mu.RLock()
	defer s.mu.RUnlock()

	bookings := make([]domain.Booking, 0, len(s.bookings))
	for _, booking := range s.bookings {
		bookings = append(bookings, booking)
	}

	sort.Slice(bookings, func(i, j int) bool {
		return bookings[i].StartTime.Before(bookings[j].StartTime)
	})

	return bookings
}

func (s *MemoryStore) ListAvailableResources(start, end time.Time, resourceType domain.ResourceType) []domain.Resource {
	s.mu.RLock()
	defer s.mu.RUnlock()

	available := make([]domain.Resource, 0)
	for _, resource := range s.resources {
		if !resource.IsActive {
			continue
		}
		if resourceType != "" && resource.Type != resourceType {
			continue
		}

		conflict := false
		for _, booking := range s.bookings {
			if booking.ResourceID != resource.ID || booking.Status != domain.BookingActive {
				continue
			}
			if overlaps(booking.StartTime, booking.EndTime, start, end) {
				conflict = true
				break
			}
		}

		if !conflict {
			available = append(available, resource)
		}
	}

	sort.Slice(available, func(i, j int) bool {
		return available[i].ID < available[j].ID
	})

	return available
}

func overlaps(aStart, aEnd, bStart, bEnd time.Time) bool {
	return aStart.Before(bEnd) && bStart.Before(aEnd)
}
