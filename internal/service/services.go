package service

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"diplom/internal/cache"
	"diplom/internal/domain"
	"diplom/internal/repository"

	"github.com/golang-jwt/jwt/v5"
)

const (
	availabilityCachePrefix = "availability:"
	utilizationCachePrefix  = "utilization:"
	defaultCacheTTL         = 2 * time.Minute
)

type AuthService struct {
	users     repository.UserRepository
	jwtSecret []byte
}

type ResourceService struct {
	resources repository.ResourceRepository
	cache     cache.Cache
}

type BookingService struct {
	bookings  repository.BookingRepository
	resources repository.ResourceRepository
	cache     cache.Cache
}

func NewAuthService(users repository.UserRepository, jwtSecret string) *AuthService {
	return &AuthService{users: users, jwtSecret: []byte(jwtSecret)}
}

func NewResourceService(resources repository.ResourceRepository, c cache.Cache) *ResourceService {
	if c == nil {
		c = cache.NewNoop()
	}
	return &ResourceService{resources: resources, cache: c}
}

func NewBookingService(bookings repository.BookingRepository, resources repository.ResourceRepository, c cache.Cache) *BookingService {
	if c == nil {
		c = cache.NewNoop()
	}
	return &BookingService{bookings: bookings, resources: resources, cache: c}
}

func (s *AuthService) SeedAdmin(fullName, email, password string) error {
	_, err := s.users.GetUserByEmail(email)
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

	created, err := s.users.CreateUser(user)
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
	user, err := s.users.GetUserByEmail(email)
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

	user, err := s.users.GetUserByID(claims.UserID)
	if err != nil {
		return domain.User{}, errors.New("user not found")
	}

	return user, nil
}

type tokenClaims struct {
	UserID int64       `json:"user_id"`
	Role   domain.Role `json:"role"`
	jwt.RegisteredClaims
}

func (s *AuthService) createToken(user domain.User) (string, error) {
	claims := tokenClaims{
		UserID: user.ID,
		Role:   user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

func (s *AuthService) parseToken(token string) (tokenClaims, error) {
	var claims tokenClaims
	parsed, err := jwt.ParseWithClaims(token, &claims, func(parsed *jwt.Token) (any, error) {
		if parsed.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return tokenClaims{}, errors.New("token expired")
		}
		return tokenClaims{}, err
	}
	if !parsed.Valid {
		return tokenClaims{}, errors.New("invalid token")
	}

	return claims, nil
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

	created, err := s.resources.CreateResource(resource)
	if err != nil {
		return domain.Resource{}, err
	}

	s.invalidateReadCache()
	return created, nil
}

func (s *ResourceService) Update(id int64, name string, resourceType domain.ResourceType, location string, capacity int, description string, isActive bool) (domain.Resource, error) {
	current, err := s.resources.GetResource(id)
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

	updated, err := s.resources.UpdateResource(id, current)
	if err != nil {
		return domain.Resource{}, err
	}

	s.invalidateReadCache()
	return updated, nil
}

func (s *ResourceService) Disable(id int64) (domain.Resource, error) {
	resource, err := s.resources.GetResource(id)
	if err != nil {
		return domain.Resource{}, err
	}

	resource.IsActive = false
	resource.UpdatedAt = time.Now().UTC()

	updated, err := s.resources.UpdateResource(id, resource)
	if err != nil {
		return domain.Resource{}, err
	}

	s.invalidateReadCache()
	return updated, nil
}

func (s *ResourceService) Get(id int64) (domain.Resource, error) {
	return s.resources.GetResource(id)
}

func (s *ResourceService) List(resourceType domain.ResourceType, onlyActive bool) []domain.Resource {
	return s.resources.ListResources(resourceType, onlyActive)
}

func (s *BookingService) Create(userID, resourceID int64, start, end time.Time, purpose string) (domain.Booking, error) {
	if !start.Before(end) {
		return domain.Booking{}, errors.New("start_time must be before end_time")
	}
	if start.Before(time.Now().UTC()) {
		return domain.Booking{}, errors.New("cannot book in the past")
	}

	resource, err := s.resources.GetResource(resourceID)
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

	created, err := s.bookings.CreateBooking(booking)
	if err != nil {
		return domain.Booking{}, err
	}

	s.invalidateReadCache()
	return created, nil
}

func (s *BookingService) Cancel(requestUser domain.User, bookingID int64) (domain.Booking, error) {
	booking, err := s.bookings.GetBooking(bookingID)
	if err != nil {
		return domain.Booking{}, errors.New("booking not found")
	}
	if booking.Status == domain.BookingCancelled {
		return booking, nil
	}
	if requestUser.Role != domain.RoleAdmin && booking.UserID != requestUser.ID {
		return domain.Booking{}, errors.New("forbidden")
	}

	cancelled, err := s.bookings.CancelBooking(bookingID, time.Now().UTC())
	if err != nil {
		return domain.Booking{}, err
	}

	s.invalidateReadCache()
	return cancelled, nil
}

func (s *BookingService) ListMy(userID int64) []domain.Booking {
	return s.bookings.ListBookingsByUser(userID)
}

func (s *BookingService) ListAll() []domain.Booking {
	return s.bookings.ListBookings()
}

func (s *BookingService) Availability(start, end time.Time, resourceType domain.ResourceType) ([]domain.Resource, error) {
	if !start.Before(end) {
		return nil, errors.New("start_time must be before end_time")
	}

	key := buildAvailabilityCacheKey(start.UTC(), end.UTC(), resourceType)
	var items []domain.Resource
	if ok := loadCachedJSON(s.cache, key, &items); ok {
		return items, nil
	}

	items = s.bookings.ListAvailableResources(start.UTC(), end.UTC(), resourceType)
	storeCachedJSON(s.cache, key, items, defaultCacheTTL)
	return items, nil
}

func (s *BookingService) Utilization(start, end time.Time) ([]domain.UtilizationReportItem, error) {
	if !start.Before(end) {
		return nil, errors.New("start_time must be before end_time")
	}

	bookings := s.bookings.ListBookings()
	resources := s.resources.ListResources("", false)
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

func (s *BookingService) invalidateReadCache() {
	_ = s.cache.DeleteByPrefix(availabilityCachePrefix)
	_ = s.cache.DeleteByPrefix(utilizationCachePrefix)
}

func (s *ResourceService) invalidateReadCache() {
	_ = s.cache.DeleteByPrefix(availabilityCachePrefix)
	_ = s.cache.DeleteByPrefix(utilizationCachePrefix)
}

func buildAvailabilityCacheKey(start, end time.Time, resourceType domain.ResourceType) string {
	return availabilityCachePrefix + start.Format(time.RFC3339) + ":" + end.Format(time.RFC3339) + ":" + string(resourceType)
}

func buildUtilizationCacheKey(start, end time.Time) string {
	return utilizationCachePrefix + start.Format(time.RFC3339) + ":" + end.Format(time.RFC3339)
}

func loadCachedJSON(c cache.Cache, key string, dst any) bool {
	payload, err := c.Get(key)
	if err != nil {
		return false
	}

	return json.Unmarshal(payload, dst) == nil
}

func storeCachedJSON(c cache.Cache, key string, payload any, ttl time.Duration) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}

	_ = c.Set(key, data, ttl)
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
