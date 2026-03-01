package domain

import "time"

type Role string

const (
	RoleEmployee Role = "employee"
	RoleAdmin    Role = "admin"
)

type ResourceType string

const (
	ResourceMeetingRoom ResourceType = "meeting_room"
	ResourceWorkspace   ResourceType = "workspace"
)

type BookingStatus string

const (
	BookingActive    BookingStatus = "active"
	BookingCancelled BookingStatus = "cancelled"
)

type User struct {
	ID           int64     `json:"id"`
	FullName     string    `json:"full_name"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

type Resource struct {
	ID          int64        `json:"id"`
	Name        string       `json:"name"`
	Type        ResourceType `json:"type"`
	Location    string       `json:"location"`
	Capacity    int          `json:"capacity"`
	Description string       `json:"description"`
	IsActive    bool         `json:"is_active"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

type Booking struct {
	ID          int64         `json:"id"`
	ResourceID  int64         `json:"resource_id"`
	UserID      int64         `json:"user_id"`
	StartTime   time.Time     `json:"start_time"`
	EndTime     time.Time     `json:"end_time"`
	Status      BookingStatus `json:"status"`
	Purpose     string        `json:"purpose"`
	CreatedAt   time.Time     `json:"created_at"`
	CancelledAt *time.Time    `json:"cancelled_at,omitempty"`
}

type UtilizationReportItem struct {
	ResourceID    int64   `json:"resource_id"`
	ResourceName  string  `json:"resource_name"`
	ResourceType  string  `json:"resource_type"`
	BookedMinutes int64   `json:"booked_minutes"`
	Utilization   float64 `json:"utilization_percent"`
}
