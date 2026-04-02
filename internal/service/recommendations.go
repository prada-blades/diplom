package service

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"diplom/internal/domain"
)

const (
	recommendationSlotStep  = 15 * time.Minute
	recommendationLookback  = 30 * 24 * time.Hour
	recommendationLimit     = 3
	recommendationScoreEps  = 1e-9
	recommendationPrecision = 1_000_000
)

type ScheduleRecommendationRequest struct {
	ResourceType      domain.ResourceType `json:"resource_type"`
	Participants      int                 `json:"participants"`
	DurationMinutes   int                 `json:"duration_minutes"`
	PreferredStart    time.Time           `json:"preferred_start"`
	SearchWindowStart time.Time           `json:"search_window_start"`
	SearchWindowEnd   time.Time           `json:"search_window_end"`
}

type ScheduleRecommendationCandidate struct {
	ResourceID  int64     `json:"resource_id"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Score       float64   `json:"score"`
	Explanation string    `json:"explanation"`
}

type HourLoadStat struct {
	Hour          int     `json:"hour"`
	BookedMinutes int64   `json:"booked_minutes"`
	SharePercent  float64 `json:"share_percent"`
}

type WeekdayLoadStat struct {
	Weekday       string  `json:"weekday"`
	BookedMinutes int64   `json:"booked_minutes"`
	SharePercent  float64 `json:"share_percent"`
}

type UtilizationReportStats struct {
	ResourceLoads []domain.UtilizationReportItem `json:"resource_loads"`
	HourLoads     []HourLoadStat                 `json:"hour_loads"`
	WeekdayLoads  []WeekdayLoadStat              `json:"weekday_loads"`
}

type UtilizationReport struct {
	Items []domain.UtilizationReportItem `json:"items"`
	Stats UtilizationReportStats         `json:"stats"`
}

type scoredScheduleRecommendationCandidate struct {
	ScheduleRecommendationCandidate
	rawScore       float64
	timeDeviation  time.Duration
	capacityExcess float64
}

func (s *BookingService) RecommendSchedule(req ScheduleRecommendationRequest) ([]ScheduleRecommendationCandidate, error) {
	resourceType := domain.ResourceType(strings.TrimSpace(string(req.ResourceType)))
	if resourceType == "" {
		return nil, errors.New("resource_type is required")
	}
	if resourceType != domain.ResourceMeetingRoom {
		return nil, errors.New("schedule recommendations currently support meeting_room only")
	}
	if req.Participants <= 0 {
		return nil, errors.New("participants must be positive")
	}
	if req.DurationMinutes <= 0 || req.DurationMinutes%15 != 0 {
		return nil, errors.New("duration_minutes must be positive and divisible by 15")
	}
	if req.PreferredStart.IsZero() || req.SearchWindowStart.IsZero() || req.SearchWindowEnd.IsZero() {
		return nil, errors.New("preferred_start, search_window_start and search_window_end are required")
	}

	preferredStart := req.PreferredStart.UTC()
	searchWindowStart := req.SearchWindowStart.UTC()
	searchWindowEnd := req.SearchWindowEnd.UTC()
	if !searchWindowStart.Before(searchWindowEnd) {
		return nil, errors.New("search_window_start must be before search_window_end")
	}
	if preferredStart.Before(searchWindowStart) || !preferredStart.Before(searchWindowEnd) {
		return nil, errors.New("preferred_start must be inside the search window")
	}

	duration := time.Duration(req.DurationMinutes) * time.Minute
	latestStart := searchWindowEnd.Add(-duration)
	if latestStart.Before(searchWindowStart) {
		return nil, errors.New("search window must fit requested duration")
	}

	firstSlot := ceilToSlot(searchWindowStart, recommendationSlotStep)
	lastSlot := floorToSlot(latestStart, recommendationSlotStep)
	if lastSlot.Before(firstSlot) {
		return nil, errors.New("search window must contain at least one 15-minute slot")
	}

	resources := s.resources.ListResources(resourceType, true)
	if len(resources) == 0 {
		return []ScheduleRecommendationCandidate{}, nil
	}

	bookings := s.bookings.ListBookings()
	recentStart := preferredStart.Add(-recommendationLookback)
	recentUtilization := buildRecentUtilizationMap(bookings, recentStart, preferredStart)
	maxDeviation := maxDuration(
		absDuration(preferredStart.Sub(firstSlot)),
		absDuration(lastSlot.Sub(preferredStart)),
	)
	if maxDeviation <= 0 {
		maxDeviation = recommendationSlotStep
	}

	scored := make([]scoredScheduleRecommendationCandidate, 0)
	for _, resource := range resources {
		if resource.Capacity < req.Participants {
			continue
		}

		capacityExcess := relativeCapacityExcess(resource.Capacity, req.Participants)
		recentLoad := recentUtilization[resource.ID]
		for slotStart := firstSlot; !slotStart.After(lastSlot); slotStart = slotStart.Add(recommendationSlotStep) {
			slotEnd := slotStart.Add(duration)
			if hasBookingConflict(bookings, resource.ID, slotStart, slotEnd) {
				continue
			}

			timeDeviation := absDuration(slotStart.Sub(preferredStart))
			timeScore := normalizeDuration(timeDeviation, maxDeviation)
			score := 0.5*timeScore + 0.3*capacityExcess + 0.2*recentLoad

			scored = append(scored, scoredScheduleRecommendationCandidate{
				ScheduleRecommendationCandidate: ScheduleRecommendationCandidate{
					ResourceID:  resource.ID,
					StartTime:   slotStart,
					EndTime:     slotEnd,
					Score:       roundRecommendationScore(score),
					Explanation: buildRecommendationExplanation(timeDeviation, capacityExcess, recentLoad),
				},
				rawScore:       score,
				timeDeviation:  timeDeviation,
				capacityExcess: capacityExcess,
			})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		if math.Abs(scored[i].rawScore-scored[j].rawScore) > recommendationScoreEps {
			return scored[i].rawScore < scored[j].rawScore
		}
		if !scored[i].StartTime.Equal(scored[j].StartTime) {
			return scored[i].StartTime.Before(scored[j].StartTime)
		}
		if math.Abs(scored[i].capacityExcess-scored[j].capacityExcess) > recommendationScoreEps {
			return scored[i].capacityExcess < scored[j].capacityExcess
		}
		return scored[i].ResourceID < scored[j].ResourceID
	})

	if len(scored) > recommendationLimit {
		scored = scored[:recommendationLimit]
	}

	result := make([]ScheduleRecommendationCandidate, len(scored))
	for i := range scored {
		result[i] = scored[i].ScheduleRecommendationCandidate
	}

	return result, nil
}

func (s *BookingService) UtilizationReport(start, end time.Time) (UtilizationReport, error) {
	key := buildUtilizationCacheKey(start.UTC(), end.UTC())
	var report UtilizationReport
	if ok := loadCachedJSON(s.cache, key, &report); ok {
		return report, nil
	}

	items, err := s.Utilization(start, end)
	if err != nil {
		return UtilizationReport{}, err
	}

	stats := s.buildUtilizationReportStats(start.UTC(), end.UTC(), items)
	report = UtilizationReport{
		Items: items,
		Stats: stats,
	}
	storeCachedJSON(s.cache, key, report, defaultCacheTTL)
	return report, nil
}

func (s *BookingService) buildUtilizationReportStats(start, end time.Time, resourceLoads []domain.UtilizationReportItem) UtilizationReportStats {
	bookings := s.bookings.ListBookings()
	var hourBuckets [24]int64
	weekdayBuckets := map[time.Weekday]int64{
		time.Monday:    0,
		time.Tuesday:   0,
		time.Wednesday: 0,
		time.Thursday:  0,
		time.Friday:    0,
		time.Saturday:  0,
		time.Sunday:    0,
	}

	var totalBookedMinutes int64
	for _, booking := range bookings {
		if booking.Status != domain.BookingActive {
			continue
		}

		overlapStart := maxTime(start, booking.StartTime)
		overlapEnd := minTime(end, booking.EndTime)
		if !overlapStart.Before(overlapEnd) {
			continue
		}

		totalBookedMinutes += durationMinutes(overlapEnd.Sub(overlapStart))
		addMinutesByHour(hourBuckets[:], overlapStart, overlapEnd)
		addMinutesByWeekday(weekdayBuckets, overlapStart, overlapEnd)
	}

	resourceLoadsCopy := make([]domain.UtilizationReportItem, len(resourceLoads))
	copy(resourceLoadsCopy, resourceLoads)

	return UtilizationReportStats{
		ResourceLoads: resourceLoadsCopy,
		HourLoads:     buildHourLoadStats(hourBuckets, totalBookedMinutes),
		WeekdayLoads:  buildWeekdayLoadStats(weekdayBuckets, totalBookedMinutes),
	}
}

func buildRecentUtilizationMap(bookings []domain.Booking, windowStart, windowEnd time.Time) map[int64]float64 {
	utilization := make(map[int64]float64)
	totalMinutes := windowEnd.Sub(windowStart).Minutes()
	if totalMinutes <= 0 {
		return utilization
	}

	for _, booking := range bookings {
		if booking.Status != domain.BookingActive {
			continue
		}

		overlapStart := maxTime(windowStart, booking.StartTime)
		overlapEnd := minTime(windowEnd, booking.EndTime)
		if !overlapStart.Before(overlapEnd) {
			continue
		}

		utilization[booking.ResourceID] += overlapEnd.Sub(overlapStart).Minutes() / totalMinutes
		if utilization[booking.ResourceID] > 1 {
			utilization[booking.ResourceID] = 1
		}
	}

	return utilization
}

func buildRecommendationExplanation(timeDeviation time.Duration, capacityExcess, recentLoad float64) string {
	return fmt.Sprintf(
		"time deviation %d min, extra capacity %.1f%%, recent utilization %.1f%%",
		int(timeDeviation/time.Minute),
		capacityExcess*100,
		recentLoad*100,
	)
}

func buildHourLoadStats(hourBuckets [24]int64, totalBookedMinutes int64) []HourLoadStat {
	stats := make([]HourLoadStat, 0, len(hourBuckets))
	for hour := 0; hour < len(hourBuckets); hour++ {
		stats = append(stats, HourLoadStat{
			Hour:          hour,
			BookedMinutes: hourBuckets[hour],
			SharePercent:  sharePercent(hourBuckets[hour], totalBookedMinutes),
		})
	}
	return stats
}

func buildWeekdayLoadStats(weekdayBuckets map[time.Weekday]int64, totalBookedMinutes int64) []WeekdayLoadStat {
	orderedWeekdays := []time.Weekday{
		time.Monday,
		time.Tuesday,
		time.Wednesday,
		time.Thursday,
		time.Friday,
		time.Saturday,
		time.Sunday,
	}

	stats := make([]WeekdayLoadStat, 0, len(orderedWeekdays))
	for _, weekday := range orderedWeekdays {
		minutes := weekdayBuckets[weekday]
		stats = append(stats, WeekdayLoadStat{
			Weekday:       strings.ToLower(weekday.String()),
			BookedMinutes: minutes,
			SharePercent:  sharePercent(minutes, totalBookedMinutes),
		})
	}
	return stats
}

func addMinutesByHour(hourBuckets []int64, start, end time.Time) {
	cursor := start
	for cursor.Before(end) {
		nextHour := cursor.Truncate(time.Hour).Add(time.Hour)
		boundary := minTime(end, nextHour)
		hourBuckets[cursor.Hour()] += durationMinutes(boundary.Sub(cursor))
		cursor = boundary
	}
}

func addMinutesByWeekday(weekdayBuckets map[time.Weekday]int64, start, end time.Time) {
	cursor := start
	for cursor.Before(end) {
		nextDay := time.Date(cursor.Year(), cursor.Month(), cursor.Day()+1, 0, 0, 0, 0, cursor.Location())
		boundary := minTime(end, nextDay)
		weekdayBuckets[cursor.Weekday()] += durationMinutes(boundary.Sub(cursor))
		cursor = boundary
	}
}

func hasBookingConflict(bookings []domain.Booking, resourceID int64, start, end time.Time) bool {
	for _, booking := range bookings {
		if booking.ResourceID != resourceID || booking.Status != domain.BookingActive {
			continue
		}
		if overlaps(booking.StartTime, booking.EndTime, start, end) {
			return true
		}
	}
	return false
}

func overlaps(aStart, aEnd, bStart, bEnd time.Time) bool {
	return aStart.Before(bEnd) && bStart.Before(aEnd)
}

func relativeCapacityExcess(capacity, participants int) float64 {
	if capacity <= participants || participants <= 0 {
		return 0
	}
	return float64(capacity-participants) / float64(capacity)
}

func ceilToSlot(t time.Time, step time.Duration) time.Time {
	truncated := t.Truncate(step)
	if t.Equal(truncated) {
		return truncated
	}
	return truncated.Add(step)
}

func floorToSlot(t time.Time, step time.Duration) time.Time {
	return t.Truncate(step)
}

func normalizeDuration(value, limit time.Duration) float64 {
	if limit <= 0 {
		return 0
	}

	normalized := float64(value) / float64(limit)
	if normalized > 1 {
		return 1
	}
	return normalized
}

func durationMinutes(duration time.Duration) int64 {
	return int64(duration / time.Minute)
}

func sharePercent(part, total int64) float64 {
	if total <= 0 || part <= 0 {
		return 0
	}
	return roundRecommendationScore(float64(part) / float64(total) * 100)
}

func absDuration(duration time.Duration) time.Duration {
	if duration < 0 {
		return -duration
	}
	return duration
}

func maxDuration(values ...time.Duration) time.Duration {
	var max time.Duration
	for _, value := range values {
		if value > max {
			max = value
		}
	}
	return max
}

func roundRecommendationScore(value float64) float64 {
	return math.Round(value*recommendationPrecision) / recommendationPrecision
}
