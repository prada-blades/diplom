// Нагрузочное тестирование REST API бронирования ресурсов.
// Запуск: go run . -rps 100 -duration 30s -base http://localhost:8080
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ── CLI flags ────────────────────────────────────────────────────────────────

var (
	baseURL      = flag.String("base", "http://localhost:8080", "base URL сервера")
	targetRPS    = flag.Int("rps", 100, "целевое число запросов в секунду")
	duration     = flag.Duration("duration", 30*time.Second, "длительность теста")
	workers      = flag.Int("workers", 50, "число параллельных горутин-воркеров")
	adminEmail   = flag.String("admin-email", "admin@corp.local", "e-mail администратора")
	adminPass    = flag.String("admin-pass", "admin123", "пароль администратора")
	outputFile   = flag.String("out", "", "сохранить CSV с латентностями (необязательно)")
	warmupSec    = flag.Int("warmup", 3, "прогрев в секундах (не учитывается в статистике)")
)

// ── Result ───────────────────────────────────────────────────────────────────

type result struct {
	scenario string
	latency  time.Duration
	status   int
	err      error
}

// ── Scenario ─────────────────────────────────────────────────────────────────

type scenario struct {
	name   string
	weight int // относительная вероятность выбора
	fn     func(c *client) result
}

// ── Client ───────────────────────────────────────────────────────────────────

type client struct {
	http        *http.Client
	base        string
	adminToken  string
	userTokens  []string
	resourceIDs []int64
	mu          sync.Mutex
}

func newClient(base string) *client {
	return &client{
		http: &http.Client{Timeout: 10 * time.Second},
		base: base,
	}
}

func (c *client) do(method, path string, body any, token string) (int, []byte, error) {
	var buf *bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		buf = bytes.NewBuffer(b)
	} else {
		buf = &bytes.Buffer{}
	}

	req, err := http.NewRequest(method, c.base+path, buf)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	var out bytes.Buffer
	out.ReadFrom(resp.Body)
	return resp.StatusCode, out.Bytes(), nil
}

// ── Setup: login + create test data ─────────────────────────────────────────

func (c *client) setup(adminEmail, adminPass string) error {
	// 1. Логин администратора
	status, body, err := c.do("POST", "/auth/login",
		map[string]string{"email": adminEmail, "password": adminPass}, "")
	if err != nil {
		return fmt.Errorf("admin login request: %w", err)
	}
	if status != 200 {
		return fmt.Errorf("admin login: HTTP %d — %s", status, body)
	}
	var lr struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &lr); err != nil {
		return fmt.Errorf("admin login parse: %w", err)
	}
	c.adminToken = lr.Token

	// 2. Регистрация 5 тестовых пользователей
	for i := 0; i < 5; i++ {
		email := fmt.Sprintf("loadtest_user%d_%d@test.local", i, time.Now().UnixNano())
		st, b, err := c.do("POST", "/auth/register", map[string]string{
			"full_name": fmt.Sprintf("Load Test User %d", i),
			"email":     email,
			"password":  "loadtest123",
		}, "")
		if err != nil {
			return fmt.Errorf("register user %d: %w", i, err)
		}
		if st != 201 {
			return fmt.Errorf("register user %d: HTTP %d — %s", i, st, b)
		}
		var rr struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal(b, &rr); err != nil {
			return fmt.Errorf("register user %d parse: %w", i, err)
		}
		c.userTokens = append(c.userTokens, rr.Token)
	}

	// 3. Создание 3 тестовых переговорных комнат
	for i := 0; i < 3; i++ {
		st, b, err := c.do("POST", "/resources", map[string]any{
			"name":     fmt.Sprintf("LoadTest Room %d", i),
			"type":     "meeting_room",
			"capacity": 10 + i*5,
			"equipment": []string{"projector", "whiteboard"},
		}, c.adminToken)
		if err != nil {
			return fmt.Errorf("create resource %d: %w", i, err)
		}
		if st != 201 {
			return fmt.Errorf("create resource %d: HTTP %d — %s", i, st, b)
		}
		var res struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(b, &res); err != nil {
			return fmt.Errorf("create resource %d parse: %w", i, err)
		}
		c.resourceIDs = append(c.resourceIDs, res.ID)
	}

	fmt.Printf("  Администратор: OK\n")
	fmt.Printf("  Тестовые пользователи: %d\n", len(c.userTokens))
	fmt.Printf("  Тестовые ресурсы: %v\n", c.resourceIDs)
	return nil
}

func (c *client) randomUserToken() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.userTokens[rand.Intn(len(c.userTokens))]
}

func (c *client) randomResourceID() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.resourceIDs[rand.Intn(len(c.resourceIDs))]
}

// ── Scenario implementations ─────────────────────────────────────────────────

func scenarioHealth(c *client) result {
	start := time.Now()
	st, _, err := c.do("GET", "/health", nil, "")
	return result{"GET /health", time.Since(start), st, err}
}

func scenarioListResources(c *client) result {
	start := time.Now()
	st, _, err := c.do("GET", "/resources", nil, c.adminToken)
	return result{"GET /resources", time.Since(start), st, err}
}

func scenarioAvailability(c *client) result {
	// проверяем доступность на завтра
	from := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02T09:00:00Z")
	to := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02T18:00:00Z")
	path := fmt.Sprintf("/availability?from=%s&to=%s", from, to)
	start := time.Now()
	st, _, err := c.do("GET", path, nil, c.randomUserToken())
	return result{"GET /availability", time.Since(start), st, err}
}

func scenarioMyBookings(c *client) result {
	start := time.Now()
	st, _, err := c.do("GET", "/bookings/my", nil, c.randomUserToken())
	return result{"GET /bookings/my", time.Since(start), st, err}
}

func scenarioCreateBooking(c *client) result {
	// Бронируем случайный ресурс через ~1-5 дней на 1 час
	offset := time.Duration(1+rand.Intn(5)) * 24 * time.Hour
	hourOffset := time.Duration(rand.Intn(8)) * time.Hour
	base := time.Now().UTC().Truncate(time.Hour).Add(offset + hourOffset + 9*time.Hour)
	body := map[string]any{
		"resource_id": c.randomResourceID(),
		"start_time":  base.Format(time.RFC3339),
		"end_time":    base.Add(time.Hour).Format(time.RFC3339),
	}
	start := time.Now()
	st, _, err := c.do("POST", "/bookings", body, c.randomUserToken())
	return result{"POST /bookings", time.Since(start), st, err}
}

func scenarioAdminBookings(c *client) result {
	start := time.Now()
	st, _, err := c.do("GET", "/admin/bookings", nil, c.adminToken)
	return result{"GET /admin/bookings", time.Since(start), st, err}
}

func scenarioUtilizationReport(c *client) result {
	from := time.Now().UTC().Add(-30 * 24 * time.Hour).Format("2006-01-02T00:00:00Z")
	to := time.Now().UTC().Format("2006-01-02T23:59:59Z")
	path := fmt.Sprintf("/admin/reports/utilization?from=%s&to=%s", from, to)
	start := time.Now()
	st, _, err := c.do("GET", path, nil, c.adminToken)
	return result{"GET /admin/reports/utilization", time.Since(start), st, err}
}

func scenarioRecommendations(c *client) result {
	from := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02T09:00:00Z")
	to := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02T18:00:00Z")
	path := fmt.Sprintf("/recommendations/schedule?from=%s&to=%s&duration=60&capacity=5", from, to)
	start := time.Now()
	st, _, err := c.do("GET", path, nil, c.randomUserToken())
	return result{"GET /recommendations/schedule", time.Since(start), st, err}
}

// ── Weighted scenario picker ─────────────────────────────────────────────────

func buildScenarios(c *client) []scenario {
	return []scenario{
		{"GET /health", 5, func(_ *client) result { return scenarioHealth(c) }},
		{"GET /resources", 15, func(_ *client) result { return scenarioListResources(c) }},
		{"GET /availability", 20, func(_ *client) result { return scenarioAvailability(c) }},
		{"GET /bookings/my", 20, func(_ *client) result { return scenarioMyBookings(c) }},
		{"POST /bookings", 20, func(_ *client) result { return scenarioCreateBooking(c) }},
		{"GET /admin/bookings", 8, func(_ *client) result { return scenarioAdminBookings(c) }},
		{"GET /admin/reports/utilization", 5, func(_ *client) result { return scenarioUtilizationReport(c) }},
		{"GET /recommendations/schedule", 7, func(_ *client) result { return scenarioRecommendations(c) }},
	}
}

func pickScenario(scenarios []scenario) scenario {
	total := 0
	for _, s := range scenarios {
		total += s.weight
	}
	r := rand.Intn(total)
	for _, s := range scenarios {
		r -= s.weight
		if r < 0 {
			return s
		}
	}
	return scenarios[len(scenarios)-1]
}

// ── Statistics ───────────────────────────────────────────────────────────────

type stats struct {
	mu        sync.Mutex
	latencies []time.Duration
	byScenario map[string]*scenarioStats
	errors    int64
	total     int64
}

type scenarioStats struct {
	count   int64
	errors  int64
	latSum  time.Duration
	latencies []time.Duration
}

func newStats() *stats {
	return &stats{byScenario: make(map[string]*scenarioStats)}
}

func (s *stats) record(r result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	atomic.AddInt64(&s.total, 1)
	s.latencies = append(s.latencies, r.latency)

	ss, ok := s.byScenario[r.scenario]
	if !ok {
		ss = &scenarioStats{}
		s.byScenario[r.scenario] = ss
	}
	ss.count++
	ss.latSum += r.latency
	ss.latencies = append(ss.latencies, r.latency)
	if r.err != nil || (r.status >= 500) {
		ss.errors++
		atomic.AddInt64(&s.errors, 1)
	}
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p/100.0*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func (s *stats) print(elapsed time.Duration) {
	sort.Slice(s.latencies, func(i, j int) bool { return s.latencies[i] < s.latencies[j] })

	total := int64(len(s.latencies))
	rps := float64(total) / elapsed.Seconds()
	errRate := float64(s.errors) / float64(total) * 100

	fmt.Println("\n╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║            РЕЗУЛЬТАТЫ НАГРУЗОЧНОГО ТЕСТИРОВАНИЯ              ║")
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	fmt.Printf("║  Длительность:     %-41s║\n", elapsed.Round(time.Millisecond))
	fmt.Printf("║  Всего запросов:   %-41d║\n", total)
	fmt.Printf("║  Реальный RPS:     %-41s║\n", fmt.Sprintf("%.1f", rps))
	fmt.Printf("║  Ошибок (5xx/net): %-41s║\n", fmt.Sprintf("%d (%.2f%%)", s.errors, errRate))
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	fmt.Printf("║  %-15s  %8s  %8s  %8s  %8s  ║\n", "Латентность", "min", "p50", "p95", "p99")
	fmt.Println("║" + strings.Repeat("─", 62) + "║")
	fmt.Printf("║  %-15s  %8s  %8s  %8s  %8s  ║\n",
		"Общая",
		percentile(s.latencies, 0).Round(time.Millisecond),
		percentile(s.latencies, 50).Round(time.Millisecond),
		percentile(s.latencies, 95).Round(time.Millisecond),
		percentile(s.latencies, 99).Round(time.Millisecond),
	)
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	fmt.Printf("║  %-32s  %5s  %5s  %7s  ║\n", "Сценарий", "RPS", "Ошибки", "p95 мс")
	fmt.Println("║" + strings.Repeat("─", 62) + "║")

	// sort scenarios by count desc
	type kv struct {
		k string
		v *scenarioStats
	}
	var sorted []kv
	for k, v := range s.byScenario {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].v.count > sorted[j].v.count })

	for _, kv := range sorted {
		sc := kv.v
		sort.Slice(sc.latencies, func(i, j int) bool { return sc.latencies[i] < sc.latencies[j] })
		scRPS := float64(sc.count) / elapsed.Seconds()
		p95ms := percentile(sc.latencies, 95).Milliseconds()
		errPct := fmt.Sprintf("%.1f%%", float64(sc.errors)/float64(sc.count)*100)
		fmt.Printf("║  %-32s  %5.1f  %5s  %7d  ║\n", kv.k, scRPS, errPct, p95ms)
	}
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
}

// ── CSV export ───────────────────────────────────────────────────────────────

func (s *stats) saveCSV(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintln(f, "scenario,latency_ms,status,error")
	s.mu.Lock()
	defer s.mu.Unlock()
	// We don't store per-request scenario after recording; this is approximate
	for _, l := range s.latencies {
		fmt.Fprintf(f, "all,%d,0,\n", l.Milliseconds())
	}
	return nil
}

// ── Rate limiter (token bucket) ──────────────────────────────────────────────

type rateLimiter struct {
	ticker *time.Ticker
}

func newRateLimiter(rps int) *rateLimiter {
	interval := time.Second / time.Duration(rps)
	return &rateLimiter{ticker: time.NewTicker(interval)}
}

func (rl *rateLimiter) Wait() { <-rl.ticker.C }
func (rl *rateLimiter) Stop() { rl.ticker.Stop() }

// ── Config file loader ────────────────────────────────────────────────────────

// loadConfigEnv читает config.env из директории, где находится бинарник (или CWD),
// и выставляет переменные окружения. Флаги командной строки имеют приоритет —
// вызывается до flag.Parse(), так что os.Setenv заполняет только незаданные значения.
func loadConfigEnv() {
	candidates := []string{"config.env", "loadtest/config.env"}
	var data []byte
	var chosen string
	for _, p := range candidates {
		b, err := os.ReadFile(p)
		if err == nil {
			data, chosen = b, p
			break
		}
	}
	if data == nil {
		return
	}
	fmt.Printf("Конфиг: %s\n", chosen)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	loadConfigEnv()

	// Подставляем значения из config.env как новые дефолты флагов.
	// flag.Parse() ниже перезапишет их, только если флаг передан явно в CLI.
	envFlags := map[string]string{
		"base":        "BASE_URL",
		"admin-email": "ADMIN_EMAIL",
		"admin-pass":  "ADMIN_PASSWORD",
		"rps":         "RPS",
		"duration":    "DURATION",
		"workers":     "WORKERS",
		"warmup":      "WARMUP",
		"out":         "OUTPUT_FILE",
	}
	for flagName, envKey := range envFlags {
		if v := os.Getenv(envKey); v != "" {
			flag.Set(flagName, v)
		}
	}

	flag.Parse()

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║        НАГРУЗОЧНОЕ ТЕСТИРОВАНИЕ — BOOKING API                ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Printf("\nЦелевой RPS : %d\n", *targetRPS)
	fmt.Printf("Длительность: %s\n", *duration)
	fmt.Printf("Воркеры     : %d\n", *workers)
	fmt.Printf("Сервер      : %s\n\n", *baseURL)

	c := newClient(*baseURL)

	fmt.Println("[ 1/3 ] Подготовка данных...")
	if err := c.setup(*adminEmail, *adminPass); err != nil {
		fmt.Fprintf(os.Stderr, "ОШИБКА при подготовке: %v\n", err)
		os.Exit(1)
	}

	scenarios := buildScenarios(c)
	st := newStats()
	rl := newRateLimiter(*targetRPS)
	defer rl.Stop()

	jobs := make(chan scenario, *targetRPS*2)
	var wg sync.WaitGroup

	// Запустить воркеры
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for sc := range jobs {
				r := sc.fn(nil)
				r.scenario = sc.name
				st.record(r)
			}
		}()
	}

	// Прогрев
	fmt.Printf("[ 2/3 ] Прогрев %d сек...\n", *warmupSec)
	warmupEnd := time.Now().Add(time.Duration(*warmupSec) * time.Second)
	for time.Now().Before(warmupEnd) {
		rl.Wait()
		sc := pickScenario(scenarios)
		jobs <- sc
	}
	// Сбрасываем статистику прогрева
	st = newStats()
	var requestsInTest int64

	fmt.Printf("[ 3/3 ] Тест %s @ %d RPS...\n", *duration, *targetRPS)
	testStart := time.Now()
	testEnd := testStart.Add(*duration)

	// Живой счётчик
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			elapsed := time.Since(testStart)
			n := atomic.LoadInt64(&requestsInTest)
			rps := float64(n) / elapsed.Seconds()
			fmt.Printf("    %.0fs / %s — запросов: %d, RPS: %.1f\n",
				elapsed.Seconds(), duration, n, rps)
		}
	}()

	for time.Now().Before(testEnd) {
		rl.Wait()
		sc := pickScenario(scenarios)
		jobs <- sc
		atomic.AddInt64(&requestsInTest, 1)
	}

	close(jobs)
	wg.Wait()

	elapsed := time.Since(testStart)
	st.print(elapsed)

	if *outputFile != "" {
		if err := st.saveCSV(*outputFile); err != nil {
			fmt.Fprintf(os.Stderr, "Ошибка записи CSV: %v\n", err)
		} else {
			fmt.Printf("\nЛатентности сохранены: %s\n", *outputFile)
		}
	}
}
