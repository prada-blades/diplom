package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"diplom/internal/domain"
	"diplom/internal/repository"
)

type AuthService struct {
	store     *repository.MemoryStore
	jwtSecret []byte
}

type ResourceService struct {
	store *repository.MemoryStore
}

type BookingService struct {
	store *repository.MemoryStore
}

func NewAuthService(store *repository.MemoryStore, jwtSecret string) *AuthService {
	return &AuthService{store: store, jwtSecret: []byte(jwtSecret)}
}

func NewResourceService(store *repository.MemoryStore) *ResourceService {
	return &ResourceService{store: store}
}

func NewBookingService(store *repository.MemoryStore) *BookingService {
	return &BookingService{store: store}
}

func (s *AuthService) SeedAdmin(fullName, email, password string) error {
	_, err := s.store.GetUserByEmail(email)
	if err == nil {
		return nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return err
	}

	_, _, err = s.Register(fullName, email, password, domain.RoleAdmin)
	return err
}

func (s *AuthService) Register(fullName, email, password string, role domain.Role) (domain.User, string, error) {
	fullName = strings.TrimSpace(fullName)
	email = strings.TrimSpace(strings.ToLower(email))
	password = strings.TrimSpace(password)

	if fullName == "" || email == "" || password == "" {
		return domain.User{}, "", errors.New("full_name, email and password are required")
	}
	if role == "" {
		role = domain.RoleEmployee
	}

	user := domain.User{
		FullName:     fullName,
		Email:        email,
		PasswordHash: hashPassword(password),
		Role:         role,
		CreatedAt:    time.Now().UTC(),
	}

	created, err := s.store.CreateUser(user)
	if err != nil {
		return domain.User{}, "", err
	}

	token, err := s.createToken(created)
	if err != nil {
		return domain.User{}, "", err
	}

	return created, token, nil
}

func (s *AuthService) Login(email, password string) (domain.User, string, error) {
	user, err := s.store.GetUserByEmail(email)
	if err != nil {
		return domain.User{}, "", errors.New("invalid credentials")
	}
	if user.PasswordHash != hashPassword(password) {
		return domain.User{}, "", errors.New("invalid credentials")
	}

	token, err := s.createToken(user)
	if err != nil {
		return domain.User{}, "", err
	}

	return user, token, nil
}

func (s *AuthService) Authenticate(token string) (domain.User, error) {
	claims, err := s.parseToken(token)
	if err != nil {
		return domain.User{}, errors.New("invalid token")
	}

	user, err := s.store.GetUserByID(claims.UserID)
	if err != nil {
		return domain.User{}, errors.New("user not found")
	}

	return user, nil
}

type tokenClaims struct {
	UserID int64       `json:"user_id"`
	Role   domain.Role `json:"role"`
	Exp    int64       `json:"exp"`
}

func (s *AuthService) createToken(user domain.User) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claims := tokenClaims{
		UserID: user.ID,
		Role:   user.Role,
		Exp:    time.Now().Add(24 * time.Hour).Unix(),
	}

	payloadRaw, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	payload := base64.RawURLEncoding.EncodeToString(payloadRaw)
	unsigned := header + "." + payload
	signature := sign(unsigned, s.jwtSecret)

	return unsigned + "." + signature, nil
}

func (s *AuthService) parseToken(token string) (tokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return tokenClaims{}, errors.New("invalid token format")
	}

	unsigned := parts[0] + "." + parts[1]
	expected := sign(unsigned, s.jwtSecret)
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return tokenClaims{}, errors.New("invalid signature")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return tokenClaims{}, err
	}

	var claims tokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return tokenClaims{}, err
	}
	if time.Now().Unix() > claims.Exp {
		return tokenClaims{}, errors.New("token expired")
	}

	return claims, nil
}

func sign(input string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(input))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func hashPassword(password string) string {
	sum := sha256.Sum256([]byte(password))
	return fmt.Sprintf("%x", sum[:])
}

func (s *ResourceService) Create(name string, resourceType domain.ResourceType, location string, capacity int, description string) (domain.Resource, error) {
	if strings.TrimSpace(name) == "" || resourceType == "" || strings.TrimSpace(location) == "" {
		return domain.Resource{}, errors.New("name, type and location are required")
	}
	if resourceType != domain.ResourceMeetingRoom && resourceType != domain.ResourceWorkspace {
		return domain.Resource{}, errors.New("invalid resource type")
	}
	if resourceType == domain.ResourceMeetingRoom && capacity <= 0 {
		return domain.Resource{}, errors.New("meeting room capacity must be positive")
	}

	now := time.Now().UTC()
	resource := domain.Resource{
		Name:        strings.TrimSpace(name),
		Type:        resourceType,
		Location:    strings.TrimSpace(location),
		Capacity:    capacity,
		Description: strings.TrimSpace(description),
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	return s.store.CreateResource(resource)
}

func (s *ResourceService) Update(id int64, name string, resourceType domain.ResourceType, location string, capacity int, description string, isActive bool) (domain.Resource, error) {
	current, err := s.store.GetResource(id)
	if err != nil {
		return domain.Resource{}, err
	}
	if strings.TrimSpace(name) == "" || resourceType == "" || strings.TrimSpace(location) == "" {
		return domain.Resource{}, errors.New("name, type and location are required")
	}

	current.Name = strings.TrimSpace(name)
	current.Type = resourceType
	current.Location = strings.TrimSpace(location)
	current.Capacity = capacity
	current.Description = strings.TrimSpace(description)
	current.IsActive = isActive
	current.UpdatedAt = time.Now().UTC()

	return s.store.UpdateResource(id, current)
}

func (s *ResourceService) Disable(id int64) (domain.Resource, error) {
	resource, err := s.store.GetResource(id)
	if err != nil {
		return domain.Resource{}, err
	}

	resource.IsActive = false
	resource.UpdatedAt = time.Now().UTC()

	return s.store.UpdateResource(id, resource)
}

func (s *ResourceService) Get(id int64) (domain.Resource, error) {
	return s.store.GetResource(id)
}

func (s *ResourceService) List(resourceType domain.ResourceType, onlyActive bool) []domain.Resource {
	return s.store.ListResources(resourceType, onlyActive)
}

func (s *BookingService) Create(userID, resourceID int64, start, end time.Time, purpose string) (domain.Booking, error) {
	if !start.Before(end) {
		return domain.Booking{}, errors.New("start_time must be before end_time")
	}
	if start.Before(time.Now().UTC()) {
		return domain.Booking{}, errors.New("cannot book in the past")
	}

	resource, err := s.store.GetResource(resourceID)
	if err != nil {
		return domain.Booking{}, errors.New("resource not found")
	}
	if !resource.IsActive {
		return domain.Booking{}, errors.New("resource is inactive")
	}

	booking := domain.Booking{
		ResourceID: resourceID,
		UserID:     userID,
		StartTime:  start.UTC(),
		EndTime:    end.UTC(),
		Status:     domain.BookingActive,
		Purpose:    strings.TrimSpace(purpose),
		CreatedAt:  time.Now().UTC(),
	}

	return s.store.CreateBooking(booking)
}

func (s *BookingService) Cancel(requestUser domain.User, bookingID int64) (domain.Booking, error) {
	booking, err := s.store.GetBooking(bookingID)
	if err != nil {
		return domain.Booking{}, errors.New("booking not found")
	}
	if booking.Status == domain.BookingCancelled {
		return booking, nil
	}
	if requestUser.Role != domain.RoleAdmin && booking.UserID != requestUser.ID {
		return domain.Booking{}, errors.New("forbidden")
	}

	return s.store.CancelBooking(bookingID, time.Now().UTC())
}

func (s *BookingService) ListMy(userID int64) []domain.Booking {
	return s.store.ListBookingsByUser(userID)
}

func (s *BookingService) ListAll() []domain.Booking {
	return s.store.ListBookings()
}

func (s *BookingService) Availability(start, end time.Time, resourceType domain.ResourceType) ([]domain.Resource, error) {
	if !start.Before(end) {
		return nil, errors.New("start_time must be before end_time")
	}
	return s.store.ListAvailableResources(start.UTC(), end.UTC(), resourceType), nil
}

func (s *BookingService) Utilization(start, end time.Time) ([]domain.UtilizationReportItem, error) {
	if !start.Before(end) {
		return nil, errors.New("start_time must be before end_time")
	}

	bookings := s.store.ListBookings()
	resources := s.store.ListResources("", false)
	resourceByID := make(map[int64]domain.Resource, len(resources))
	for _, resource := range resources {
		resourceByID[resource.ID] = resource
	}

	totalMinutes := end.Sub(start).Minutes()
	if totalMinutes <= 0 {
		return nil, errors.New("invalid period")
	}

	report := make([]domain.UtilizationReportItem, 0, len(resources))
	for _, resource := range resources {
		var bookedMinutes int64
		for _, booking := range bookings {
			if booking.ResourceID != resource.ID || booking.Status != domain.BookingActive {
				continue
			}

			overlapStart := maxTime(start, booking.StartTime)
			overlapEnd := minTime(end, booking.EndTime)
			if overlapStart.Before(overlapEnd) {
				bookedMinutes += int64(overlapEnd.Sub(overlapStart).Minutes())
			}
		}

		report = append(report, domain.UtilizationReportItem{
			ResourceID:    resource.ID,
			ResourceName:  resource.Name,
			ResourceType:  string(resource.Type),
			BookedMinutes: bookedMinutes,
			Utilization:   float64(bookedMinutes) / totalMinutes * 100,
		})
	}

	_ = resourceByID
	return report, nil
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
