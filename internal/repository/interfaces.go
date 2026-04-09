package repository

import (
	"errors"
	"time"

	"diplom/internal/domain"
)

var ErrNotFound = errors.New("not found")

type UserRepository interface {
	CreateUser(user domain.User) (domain.User, error)
	GetUserByEmail(email string) (domain.User, error)
	GetUserByID(id int64) (domain.User, error)
}

type ResourceRepository interface {
	CreateResource(resource domain.Resource) (domain.Resource, error)
	UpdateResource(id int64, update domain.Resource) (domain.Resource, error)
	GetResource(id int64) (domain.Resource, error)
	ListResources(resourceType domain.ResourceType, onlyActive bool) []domain.Resource
}

type BookingRepository interface {
	CreateBooking(booking domain.Booking) (domain.Booking, error)
	GetBooking(id int64) (domain.Booking, error)
	CancelBooking(id int64, cancelledAt time.Time) (domain.Booking, error)
	ListBookingsByUser(userID int64) []domain.Booking
	ListBookings() []domain.Booking
	ListAvailableResources(start, end time.Time, resourceType domain.ResourceType) []domain.Resource
}

type Store interface {
	UserRepository
	ResourceRepository
	BookingRepository
}
