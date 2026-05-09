package postgres

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"diplom/internal/domain"
	"diplom/internal/repository"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Store struct {
	db *sql.DB
}

func NewStore(databaseURL string) (*Store, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Migrate() error {
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		content, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}

		statements := strings.Split(string(content), ";")
		for _, stmt := range statements {
			query := strings.TrimSpace(stmt)
			if query == "" {
				continue
			}

			if _, err := s.db.Exec(query); err != nil {
				return fmt.Errorf("run migration %s statement %q: %w", name, query, err)
			}
		}
	}

	return nil
}

func (s *Store) CreateUser(user domain.User) (domain.User, error) {
	const query = `
		INSERT INTO users (full_name, email, password_hash, role, created_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`

	err := s.db.QueryRow(
		query,
		user.FullName,
		strings.ToLower(strings.TrimSpace(user.Email)),
		user.PasswordHash,
		string(user.Role),
		user.CreatedAt,
	).Scan(&user.ID)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.User{}, errors.New("email already exists")
		}
		return domain.User{}, err
	}

	return user, nil
}

func (s *Store) GetUserByEmail(email string) (domain.User, error) {
	const query = `
		SELECT id, full_name, email, password_hash, role, created_at
		FROM users
		WHERE email = $1
	`

	var user domain.User
	var role string
	err := s.db.QueryRow(query, strings.ToLower(strings.TrimSpace(email))).Scan(
		&user.ID,
		&user.FullName,
		&user.Email,
		&user.PasswordHash,
		&role,
		&user.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.User{}, repository.ErrNotFound
		}
		return domain.User{}, err
	}

	user.Role = domain.Role(role)
	return user, nil
}

func (s *Store) GetUserByID(id int64) (domain.User, error) {
	const query = `
		SELECT id, full_name, email, password_hash, role, created_at
		FROM users
		WHERE id = $1
	`

	var user domain.User
	var role string
	err := s.db.QueryRow(query, id).Scan(
		&user.ID,
		&user.FullName,
		&user.Email,
		&user.PasswordHash,
		&role,
		&user.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.User{}, repository.ErrNotFound
		}
		return domain.User{}, err
	}

	user.Role = domain.Role(role)
	return user, nil
}

func (s *Store) CreateResource(resource domain.Resource) (domain.Resource, error) {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Resource{}, err
	}
	defer tx.Rollback()

	const query = `
		INSERT INTO resources (name, type, location, capacity, description, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`

	err = tx.QueryRowContext(
		ctx,
		query,
		resource.Name,
		string(resource.Type),
		resource.Location,
		resource.Capacity,
		resource.Description,
		resource.IsActive,
		resource.CreatedAt,
		resource.UpdatedAt,
	).Scan(&resource.ID)
	if err != nil {
		return domain.Resource{}, err
	}

	if err := s.syncResourceRelations(ctx, tx, resource); err != nil {
		return domain.Resource{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Resource{}, err
	}

	return s.GetResource(resource.ID)
}

func (s *Store) UpdateResource(id int64, update domain.Resource) (domain.Resource, error) {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Resource{}, err
	}
	defer tx.Rollback()

	const query = `
		UPDATE resources
		SET name = $2, type = $3, location = $4, capacity = $5, description = $6, is_active = $7, updated_at = $8
		WHERE id = $1
	`

	result, err := tx.ExecContext(
		ctx,
		query,
		id,
		update.Name,
		string(update.Type),
		update.Location,
		update.Capacity,
		update.Description,
		update.IsActive,
		update.UpdatedAt,
	)
	if err != nil {
		return domain.Resource{}, err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return domain.Resource{}, err
	}
	if rows == 0 {
		return domain.Resource{}, repository.ErrNotFound
	}

	update.ID = id
	if err := s.syncResourceRelations(ctx, tx, update); err != nil {
		return domain.Resource{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Resource{}, err
	}

	return s.GetResource(id)
}

func (s *Store) GetResource(id int64) (domain.Resource, error) {
	const query = `
		SELECT id, name, type, location, capacity, description, is_active, created_at, updated_at
		FROM resources
		WHERE id = $1
	`

	var resource domain.Resource
	var resourceType string
	err := s.db.QueryRow(query, id).Scan(
		&resource.ID,
		&resource.Name,
		&resourceType,
		&resource.Location,
		&resource.Capacity,
		&resource.Description,
		&resource.IsActive,
		&resource.CreatedAt,
		&resource.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Resource{}, repository.ErrNotFound
		}
		return domain.Resource{}, err
	}

	resource.Type = domain.ResourceType(resourceType)
	items := []domain.Resource{resource}
	if err := s.loadResourcesRelations(items); err != nil {
		return domain.Resource{}, err
	}
	return items[0], nil
}

func (s *Store) ListResources(resourceType domain.ResourceType, onlyActive bool, equipment []string) []domain.Resource {
	query := `
		SELECT id, name, type, location, capacity, description, is_active, created_at, updated_at
		FROM resources
		WHERE ($1 = '' OR type = $1)
		  AND ($2 = false OR is_active = true)
		ORDER BY id
	`

	rows, err := s.db.Query(query, string(resourceType), onlyActive)
	if err != nil {
		return nil
	}
	defer rows.Close()

	resources := make([]domain.Resource, 0)
	for rows.Next() {
		var resource domain.Resource
		var kind string
		if err := rows.Scan(
			&resource.ID,
			&resource.Name,
			&kind,
			&resource.Location,
			&resource.Capacity,
			&resource.Description,
			&resource.IsActive,
			&resource.CreatedAt,
			&resource.UpdatedAt,
		); err != nil {
			return nil
		}
		resource.Type = domain.ResourceType(kind)
		resources = append(resources, resource)
	}

	if err := s.loadResourcesRelations(resources); err != nil {
		return nil
	}
	if len(equipment) > 0 {
		resources = filterResourcesByEquipment(resources, equipment)
	}

	return resources
}

func (s *Store) ListEquipment() []string {
	rows, err := s.db.Query(`SELECT slug FROM equipment ORDER BY slug`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	items := make([]string, 0)
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil
		}
		items = append(items, slug)
	}
	return items
}

func (s *Store) CreateBooking(booking domain.Booking) (domain.Booking, error) {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Booking{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, booking.ResourceID); err != nil {
		return domain.Booking{}, err
	}

	var conflictID int64
	err = tx.QueryRowContext(
		ctx,
		`
			SELECT id
			FROM bookings
			WHERE resource_id = $1
			  AND status = $2
			  AND start_time < $4
			  AND $3 < end_time
			LIMIT 1
		`,
		booking.ResourceID,
		string(domain.BookingActive),
		booking.StartTime,
		booking.EndTime,
	).Scan(&conflictID)
	if err == nil {
		return domain.Booking{}, errors.New("resource already booked for selected time")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.Booking{}, err
	}

	err = tx.QueryRowContext(
		ctx,
		`
			INSERT INTO bookings (resource_id, user_id, start_time, end_time, status, purpose, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id
		`,
		booking.ResourceID,
		booking.UserID,
		booking.StartTime,
		booking.EndTime,
		string(booking.Status),
		booking.Purpose,
		booking.CreatedAt,
	).Scan(&booking.ID)
	if err != nil {
		return domain.Booking{}, err
	}

	if err := tx.Commit(); err != nil {
		return domain.Booking{}, err
	}

	return booking, nil
}

func (s *Store) GetBooking(id int64) (domain.Booking, error) {
	const query = `
		SELECT id, resource_id, user_id, start_time, end_time, status, purpose, created_at, cancelled_at
		FROM bookings
		WHERE id = $1
	`

	var booking domain.Booking
	var status string
	err := s.db.QueryRow(query, id).Scan(
		&booking.ID,
		&booking.ResourceID,
		&booking.UserID,
		&booking.StartTime,
		&booking.EndTime,
		&status,
		&booking.Purpose,
		&booking.CreatedAt,
		&booking.CancelledAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Booking{}, repository.ErrNotFound
		}
		return domain.Booking{}, err
	}

	booking.Status = domain.BookingStatus(status)
	return booking, nil
}

func (s *Store) CancelBooking(id int64, cancelledAt time.Time) (domain.Booking, error) {
	const query = `
		UPDATE bookings
		SET status = $2, cancelled_at = $3
		WHERE id = $1
	`

	result, err := s.db.Exec(query, id, string(domain.BookingCancelled), cancelledAt)
	if err != nil {
		return domain.Booking{}, err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return domain.Booking{}, err
	}
	if rows == 0 {
		return domain.Booking{}, repository.ErrNotFound
	}

	return s.GetBooking(id)
}

func (s *Store) CancelFutureBookingsByResource(resourceID int64, from time.Time) (int, error) {
	result, err := s.db.Exec(
		`
			UPDATE bookings
			SET status = $3, cancelled_at = $4
			WHERE resource_id = $1
			  AND status = $5
			  AND start_time > $2
		`,
		resourceID,
		from,
		string(domain.BookingCancelled),
		from,
		string(domain.BookingActive),
	)
	if err != nil {
		return 0, err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	return int(rows), nil
}

func (s *Store) ListBookingsByUser(userID int64) []domain.Booking {
	rows, err := s.db.Query(
		`
			SELECT id, resource_id, user_id, start_time, end_time, status, purpose, created_at, cancelled_at
			FROM bookings
			WHERE user_id = $1
			ORDER BY start_time
		`,
		userID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	return scanBookings(rows)
}

func (s *Store) ListBookings() []domain.Booking {
	rows, err := s.db.Query(
		`
			SELECT id, resource_id, user_id, start_time, end_time, status, purpose, created_at, cancelled_at
			FROM bookings
			ORDER BY start_time
		`,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	return scanBookings(rows)
}

func (s *Store) ListAvailableResources(start, end time.Time, resourceType domain.ResourceType, equipment []string) []domain.Resource {
	rows, err := s.db.Query(
		`
			SELECT r.id, r.name, r.type, r.location, r.capacity, r.description, r.is_active, r.created_at, r.updated_at
			FROM resources r
			WHERE r.is_active = true
			  AND ($3 = '' OR r.type = $3)
			  AND NOT EXISTS (
				SELECT 1
				FROM bookings b
				WHERE b.resource_id = r.id
				  AND b.status = $4
				  AND b.start_time < $2
				  AND $1 < b.end_time
			  )
			ORDER BY r.id
		`,
		start,
		end,
		string(resourceType),
		string(domain.BookingActive),
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	resources := make([]domain.Resource, 0)
	for rows.Next() {
		var resource domain.Resource
		var kind string
		if err := rows.Scan(
			&resource.ID,
			&resource.Name,
			&kind,
			&resource.Location,
			&resource.Capacity,
			&resource.Description,
			&resource.IsActive,
			&resource.CreatedAt,
			&resource.UpdatedAt,
		); err != nil {
			return nil
		}
		resource.Type = domain.ResourceType(kind)
		resources = append(resources, resource)
	}

	if err := s.loadResourcesRelations(resources); err != nil {
		return nil
	}
	if len(equipment) > 0 {
		resources = filterResourcesByEquipment(resources, equipment)
	}

	return resources
}

func (s *Store) syncResourceRelations(ctx context.Context, tx *sql.Tx, resource domain.Resource) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM resource_images WHERE resource_id = $1`, resource.ID); err != nil {
		return err
	}
	for idx, imageURL := range resource.ImageURLs {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO resource_images (resource_id, url, sort_order, created_at) VALUES ($1, $2, $3, $4)`,
			resource.ID,
			imageURL,
			idx,
			resource.UpdatedAt,
		); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM resource_equipment WHERE resource_id = $1`, resource.ID); err != nil {
		return err
	}
	for _, slug := range resource.Equipment {
		var equipmentID int64
		err := tx.QueryRowContext(
			ctx,
			`
				INSERT INTO equipment (slug, name, created_at)
				VALUES ($1, $2, $3)
				ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
				RETURNING id
			`,
			slug,
			slug,
			resource.UpdatedAt,
		).Scan(&equipmentID)
		if err != nil {
			return err
		}

		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO resource_equipment (resource_id, equipment_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			resource.ID,
			equipmentID,
		); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) loadResourcesRelations(resources []domain.Resource) error {
	if len(resources) == 0 {
		return nil
	}

	resourceIDs := make([]int64, 0, len(resources))
	resourceByID := make(map[int64]*domain.Resource, len(resources))
	for i := range resources {
		resources[i].ImageURLs = []string{}
		resources[i].Equipment = []string{}
		resourceIDs = append(resourceIDs, resources[i].ID)
		resourceByID[resources[i].ID] = &resources[i]
	}

	if err := s.loadResourceImages(resourceByID, resourceIDs); err != nil {
		return err
	}
	if err := s.loadResourceEquipment(resourceByID, resourceIDs); err != nil {
		return err
	}

	return nil
}

func (s *Store) loadResourceImages(resourceByID map[int64]*domain.Resource, resourceIDs []int64) error {
	query, args := buildInQuery(
		`SELECT resource_id, url FROM resource_images WHERE resource_id IN (%s) ORDER BY resource_id, sort_order, id`,
		resourceIDs,
	)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var resourceID int64
		var imageURL string
		if err := rows.Scan(&resourceID, &imageURL); err != nil {
			return err
		}
		resourceByID[resourceID].ImageURLs = append(resourceByID[resourceID].ImageURLs, imageURL)
	}

	return nil
}

func (s *Store) loadResourceEquipment(resourceByID map[int64]*domain.Resource, resourceIDs []int64) error {
	query, args := buildInQuery(
		`
			SELECT re.resource_id, e.slug
			FROM resource_equipment re
			JOIN equipment e ON e.id = re.equipment_id
			WHERE re.resource_id IN (%s)
			ORDER BY re.resource_id, e.slug
		`,
		resourceIDs,
	)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var resourceID int64
		var slug string
		if err := rows.Scan(&resourceID, &slug); err != nil {
			return err
		}
		resourceByID[resourceID].Equipment = append(resourceByID[resourceID].Equipment, slug)
	}

	return nil
}

func buildInQuery(template string, ids []int64) (string, []any) {
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	return fmt.Sprintf(template, strings.Join(placeholders, ",")), args
}

func filterResourcesByEquipment(resources []domain.Resource, equipment []string) []domain.Resource {
	filtered := make([]domain.Resource, 0, len(resources))
	for _, resource := range resources {
		owned := make(map[string]struct{}, len(resource.Equipment))
		for _, item := range resource.Equipment {
			owned[item] = struct{}{}
		}

		match := true
		for _, item := range equipment {
			if _, ok := owned[item]; !ok {
				match = false
				break
			}
		}
		if match {
			filtered = append(filtered, resource)
		}
	}
	return filtered
}

func scanBookings(rows *sql.Rows) []domain.Booking {
	bookings := make([]domain.Booking, 0)
	for rows.Next() {
		var booking domain.Booking
		var status string
		if err := rows.Scan(
			&booking.ID,
			&booking.ResourceID,
			&booking.UserID,
			&booking.StartTime,
			&booking.EndTime,
			&status,
			&booking.Purpose,
			&booking.CreatedAt,
			&booking.CancelledAt,
		); err != nil {
			return nil
		}
		booking.Status = domain.BookingStatus(status)
		bookings = append(bookings, booking)
	}

	sort.Slice(bookings, func(i, j int) bool {
		return bookings[i].StartTime.Before(bookings[j].StartTime)
	})

	return bookings
}

func isUniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "duplicate key value")
}
