package http

import (
	"embed"
	"html/template"
	"io/fs"
	nethttp "net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"diplom/internal/domain"
	"diplom/internal/service"
)

const adminSessionCookieName = "admin_session"

//go:embed admin_templates/*.html admin_assets/*
var adminUIFiles embed.FS

var (
	adminTemplates = template.Must(template.New("admin").Funcs(template.FuncMap{
		"formatTime":       formatAdminTime,
		"formatDateTime":   formatDateTime,
		"formatPercent":    formatPercent,
		"datetimeLocal":    formatDateTimeLocal,
		"selectedResource": selectedResource,
	}).ParseFS(adminUIFiles, "admin_templates/*.html"))
	adminStaticFS = mustSubFS(adminUIFiles, "admin_assets")
)

type adminPageData struct {
	Title           string
	ContentTemplate string
	ActiveTab       string
	Admin           domain.User
	Error           string
	Notice          string

	Stats          adminDashboardStats
	Resources      []domain.Resource
	ResourceTypes  []domain.ResourceType
	ResourceForm   adminResourceForm
	EditResourceID int64
	Bookings       []adminBookingView
	StatusFilter   string
	ResourceFilter string
	StartFilter    string
	EndFilter      string
	Report         *service.UtilizationReport
	ReportStart    string
	ReportEnd      string
}

type adminDashboardStats struct {
	ResourcesCount       int
	ActiveResourcesCount int
	BookingsCount        int
	ActiveBookingsCount  int
}

type adminResourceForm struct {
	ID          int64
	Name        string
	Type        domain.ResourceType
	Location    string
	Capacity    int
	Description string
	IsActive    bool
}

type adminBookingView struct {
	ID           int64
	ResourceID   int64
	ResourceName string
	UserID       int64
	StartTime    time.Time
	EndTime      time.Time
	Status       domain.BookingStatus
	Purpose      string
}

func (a *App) registerAdminUIRoutes(mux *nethttp.ServeMux) {
	mux.Handle("/admin/static/", nethttp.StripPrefix("/admin/static/", nethttp.FileServer(nethttp.FS(adminStaticFS))))
	mux.HandleFunc("/admin/ui/login", a.handleAdminUILogin)
	mux.HandleFunc("/admin/ui/logout", a.handleAdminUILogout)
	mux.Handle("/admin/ui", a.requireAdminSession(nethttp.HandlerFunc(a.handleAdminUIDashboard)))
	mux.Handle("/admin/ui/resources", a.requireAdminSession(nethttp.HandlerFunc(a.handleAdminUIResources)))
	mux.Handle("/admin/ui/resources/", a.requireAdminSession(nethttp.HandlerFunc(a.handleAdminUIResourceAction)))
	mux.Handle("/admin/ui/bookings", a.requireAdminSession(nethttp.HandlerFunc(a.handleAdminUIBookings)))
	mux.Handle("/admin/ui/bookings/", a.requireAdminSession(nethttp.HandlerFunc(a.handleAdminUIBookingAction)))
	mux.Handle("/admin/ui/reports/utilization", a.requireAdminSession(nethttp.HandlerFunc(a.handleAdminUIReports)))
}

func (a *App) handleAdminUILogin(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.Method == nethttp.MethodGet {
		if _, ok := a.authenticatedAdminFromSession(r); ok {
			nethttp.Redirect(w, r, "/admin/ui", nethttp.StatusSeeOther)
			return
		}
		a.renderAdminTemplate(w, adminPageData{
			Title:           "Admin Login",
			ContentTemplate: "login_content",
			Error:           r.URL.Query().Get("error"),
			Notice:          r.URL.Query().Get("notice"),
		})
		return
	}

	if r.Method != nethttp.MethodPost {
		nethttp.Error(w, "method not allowed", nethttp.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		a.renderAdminTemplate(w, adminPageData{
			Title:           "Admin Login",
			ContentTemplate: "login_content",
			Error:           "Could not read login form.",
		})
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	user, token, err := a.authService.Login(email, password)
	if err != nil || user.Role != domain.RoleAdmin {
		a.renderAdminTemplate(w, adminPageData{
			Title:           "Admin Login",
			ContentTemplate: "login_content",
			Error:           "Invalid admin credentials.",
		})
		return
	}

	a.setAdminSessionCookie(w, token)
	nethttp.Redirect(w, r, "/admin/ui", nethttp.StatusSeeOther)
}

func (a *App) handleAdminUILogout(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.Method != nethttp.MethodPost {
		nethttp.Error(w, "method not allowed", nethttp.StatusMethodNotAllowed)
		return
	}

	a.clearAdminSessionCookie(w)
	nethttp.Redirect(w, r, "/admin/ui/login?notice="+url.QueryEscape("Signed out."), nethttp.StatusSeeOther)
}

func (a *App) handleAdminUIDashboard(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.Method != nethttp.MethodGet {
		nethttp.Error(w, "method not allowed", nethttp.StatusMethodNotAllowed)
		return
	}

	admin := currentUser(r)
	resources := a.resourceService.List("", false)
	bookings := a.bookingService.ListAll()
	activeResources := 0
	activeBookings := 0
	for _, resource := range resources {
		if resource.IsActive {
			activeResources++
		}
	}
	for _, booking := range bookings {
		if booking.Status == domain.BookingActive {
			activeBookings++
		}
	}

	a.renderAdminTemplate(w, adminPageData{
		Title:           "Admin Dashboard",
		ContentTemplate: "dashboard_content",
		ActiveTab:       "dashboard",
		Admin:           admin,
		Notice:          r.URL.Query().Get("notice"),
		Stats: adminDashboardStats{
			ResourcesCount:       len(resources),
			ActiveResourcesCount: activeResources,
			BookingsCount:        len(bookings),
			ActiveBookingsCount:  activeBookings,
		},
	})
}

func (a *App) handleAdminUIResources(w nethttp.ResponseWriter, r *nethttp.Request) {
	admin := currentUser(r)
	switch r.Method {
	case nethttp.MethodGet:
		data := a.newAdminResourcesPage(admin)
		data.Notice = r.URL.Query().Get("notice")
		if editRaw := r.URL.Query().Get("edit"); editRaw != "" {
			if id, err := strconv.ParseInt(editRaw, 10, 64); err == nil {
				if resource, err := a.resourceService.Get(id); err == nil {
					data.ResourceForm = adminResourceForm{
						ID:          resource.ID,
						Name:        resource.Name,
						Type:        resource.Type,
						Location:    resource.Location,
						Capacity:    resource.Capacity,
						Description: resource.Description,
						IsActive:    resource.IsActive,
					}
					data.EditResourceID = resource.ID
				}
			}
		}
		a.renderAdminTemplate(w, data)
	case nethttp.MethodPost:
		if err := r.ParseForm(); err != nil {
			data := a.newAdminResourcesPage(admin)
			data.Error = "Could not read resource form."
			a.renderAdminTemplate(w, data)
			return
		}

		form, err := parseAdminResourceForm(r)
		if err != nil {
			data := a.newAdminResourcesPage(admin)
			data.Error = err.Error()
			data.ResourceForm = form
			a.renderAdminTemplate(w, data)
			return
		}

		if _, err := a.resourceService.Create(form.Name, form.Type, form.Location, form.Capacity, form.Description); err != nil {
			data := a.newAdminResourcesPage(admin)
			data.Error = err.Error()
			data.ResourceForm = form
			a.renderAdminTemplate(w, data)
			return
		}

		nethttp.Redirect(w, r, "/admin/ui/resources?notice="+url.QueryEscape("Resource created."), nethttp.StatusSeeOther)
	default:
		nethttp.Error(w, "method not allowed", nethttp.StatusMethodNotAllowed)
	}
}

func (a *App) handleAdminUIResourceAction(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.Method != nethttp.MethodPost {
		nethttp.Error(w, "method not allowed", nethttp.StatusMethodNotAllowed)
		return
	}

	id, action, ok := parseAdminEntityAction(r.URL.Path, "/admin/ui/resources/")
	if !ok {
		nethttp.NotFound(w, r)
		return
	}

	admin := currentUser(r)
	switch action {
	case "update":
		if err := r.ParseForm(); err != nil {
			data := a.newAdminResourcesPage(admin)
			data.Error = "Could not read resource form."
			a.renderAdminTemplate(w, data)
			return
		}

		form, err := parseAdminResourceForm(r)
		form.ID = id
		if err != nil {
			data := a.newAdminResourcesPage(admin)
			data.Error = err.Error()
			data.ResourceForm = form
			data.EditResourceID = id
			a.renderAdminTemplate(w, data)
			return
		}

		isActive := r.FormValue("is_active") == "on"
		if _, err := a.resourceService.Update(id, form.Name, form.Type, form.Location, form.Capacity, form.Description, isActive); err != nil {
			data := a.newAdminResourcesPage(admin)
			data.Error = err.Error()
			data.ResourceForm = form
			data.ResourceForm.IsActive = isActive
			data.EditResourceID = id
			a.renderAdminTemplate(w, data)
			return
		}

		nethttp.Redirect(w, r, "/admin/ui/resources?notice="+url.QueryEscape("Resource updated."), nethttp.StatusSeeOther)
	case "disable":
		if _, err := a.resourceService.Disable(id); err != nil {
			nethttp.Redirect(w, r, "/admin/ui/resources?notice="+url.QueryEscape("Could not disable resource."), nethttp.StatusSeeOther)
			return
		}
		nethttp.Redirect(w, r, "/admin/ui/resources?notice="+url.QueryEscape("Resource disabled."), nethttp.StatusSeeOther)
	default:
		nethttp.NotFound(w, r)
	}
}

func (a *App) handleAdminUIBookings(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.Method != nethttp.MethodGet {
		nethttp.Error(w, "method not allowed", nethttp.StatusMethodNotAllowed)
		return
	}

	admin := currentUser(r)
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	resourceFilter := strings.TrimSpace(r.URL.Query().Get("resource_id"))
	startFilter := strings.TrimSpace(r.URL.Query().Get("start"))
	endFilter := strings.TrimSpace(r.URL.Query().Get("end"))

	bookings := a.bookingService.ListAll()
	resources := a.resourceService.List("", false)
	resourceByID := make(map[int64]domain.Resource, len(resources))
	for _, resource := range resources {
		resourceByID[resource.ID] = resource
	}

	filtered, err := filterAdminBookings(bookings, resourceByID, statusFilter, resourceFilter, startFilter, endFilter)
	data := adminPageData{
		Title:           "Admin Bookings",
		ContentTemplate: "bookings_content",
		ActiveTab:       "bookings",
		Admin:           admin,
		Notice:          r.URL.Query().Get("notice"),
		Resources:       resources,
		Bookings:        filtered,
		StatusFilter:    statusFilter,
		ResourceFilter:  resourceFilter,
		StartFilter:     startFilter,
		EndFilter:       endFilter,
	}
	if err != nil {
		data.Error = err.Error()
	}
	a.renderAdminTemplate(w, data)
}

func (a *App) handleAdminUIBookingAction(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.Method != nethttp.MethodPost {
		nethttp.Error(w, "method not allowed", nethttp.StatusMethodNotAllowed)
		return
	}

	id, action, ok := parseAdminEntityAction(r.URL.Path, "/admin/ui/bookings/")
	if !ok || action != "cancel" {
		nethttp.NotFound(w, r)
		return
	}

	if _, err := a.bookingService.Cancel(currentUser(r), id); err != nil {
		nethttp.Redirect(w, r, "/admin/ui/bookings?notice="+url.QueryEscape("Could not cancel booking."), nethttp.StatusSeeOther)
		return
	}

	nethttp.Redirect(w, r, "/admin/ui/bookings?notice="+url.QueryEscape("Booking cancelled."), nethttp.StatusSeeOther)
}

func (a *App) handleAdminUIReports(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.Method != nethttp.MethodGet {
		nethttp.Error(w, "method not allowed", nethttp.StatusMethodNotAllowed)
		return
	}

	admin := currentUser(r)
	startValue := strings.TrimSpace(r.URL.Query().Get("start"))
	endValue := strings.TrimSpace(r.URL.Query().Get("end"))
	start, end, err := parseAdminDateRange(startValue, endValue)

	data := adminPageData{
		Title:           "Utilization Report",
		ContentTemplate: "reports_content",
		ActiveTab:       "reports",
		Admin:           admin,
		ReportStart:     formatDateTimeLocal(start),
		ReportEnd:       formatDateTimeLocal(end),
	}
	if err != nil {
		data.Error = err.Error()
		a.renderAdminTemplate(w, data)
		return
	}

	report, err := a.bookingService.UtilizationReport(start, end)
	if err != nil {
		data.Error = err.Error()
		a.renderAdminTemplate(w, data)
		return
	}

	data.Report = &report
	a.renderAdminTemplate(w, data)
}

func (a *App) requireAdminSession(next nethttp.Handler) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		user, ok := a.authenticatedAdminFromSession(r)
		if !ok {
			a.clearAdminSessionCookie(w)
			nethttp.Redirect(w, r, "/admin/ui/login?error="+url.QueryEscape("Please sign in as admin."), nethttp.StatusSeeOther)
			return
		}

		ctx := r.Context()
		ctx = withUser(ctx, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *App) authenticatedAdminFromSession(r *nethttp.Request) (domain.User, bool) {
	cookie, err := r.Cookie(adminSessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return domain.User{}, false
	}

	user, err := a.authService.Authenticate(cookie.Value)
	if err != nil || user.Role != domain.RoleAdmin {
		return domain.User{}, false
	}

	return user, true
}

func (a *App) setAdminSessionCookie(w nethttp.ResponseWriter, token string) {
	nethttp.SetCookie(w, &nethttp.Cookie{
		Name:     adminSessionCookieName,
		Value:    token,
		Path:     "/admin/ui",
		HttpOnly: true,
		SameSite: nethttp.SameSiteLaxMode,
	})
}

func (a *App) clearAdminSessionCookie(w nethttp.ResponseWriter) {
	nethttp.SetCookie(w, &nethttp.Cookie{
		Name:     adminSessionCookieName,
		Value:    "",
		Path:     "/admin/ui",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: nethttp.SameSiteLaxMode,
	})
}

func (a *App) renderAdminTemplate(w nethttp.ResponseWriter, data adminPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := adminTemplates.ExecuteTemplate(w, "admin_layout", data); err != nil {
		nethttp.Error(w, "template render failed", nethttp.StatusInternalServerError)
	}
}

func (a *App) newAdminResourcesPage(admin domain.User) adminPageData {
	return adminPageData{
		Title:           "Manage Resources",
		ContentTemplate: "resources_content",
		ActiveTab:       "resources",
		Admin:           admin,
		Resources:       a.resourceService.List("", false),
		ResourceTypes: []domain.ResourceType{
			domain.ResourceMeetingRoom,
			domain.ResourceWorkspace,
		},
		ResourceForm: adminResourceForm{
			Type:     domain.ResourceMeetingRoom,
			IsActive: true,
		},
	}
}

func parseAdminResourceForm(r *nethttp.Request) (adminResourceForm, error) {
	form := adminResourceForm{
		Name:        strings.TrimSpace(r.FormValue("name")),
		Type:        domain.ResourceType(strings.TrimSpace(r.FormValue("type"))),
		Location:    strings.TrimSpace(r.FormValue("location")),
		Description: strings.TrimSpace(r.FormValue("description")),
		IsActive:    r.FormValue("is_active") == "on",
	}

	if rawCapacity := strings.TrimSpace(r.FormValue("capacity")); rawCapacity != "" {
		capacity, err := strconv.Atoi(rawCapacity)
		if err != nil {
			return form, errInvalidCapacity
		}
		form.Capacity = capacity
	}

	if form.Type == "" {
		form.Type = domain.ResourceMeetingRoom
	}

	return form, nil
}

var errInvalidCapacity = adminFormError("Capacity must be a number.")

type adminFormError string

func (e adminFormError) Error() string { return string(e) }

func parseAdminEntityAction(path, prefix string) (int64, string, bool) {
	raw := strings.TrimPrefix(path, prefix)
	parts := strings.Split(strings.Trim(raw, "/"), "/")
	if len(parts) != 2 {
		return 0, "", false
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, "", false
	}

	return id, parts[1], true
}

func filterAdminBookings(bookings []domain.Booking, resourceByID map[int64]domain.Resource, statusFilter, resourceFilter, startFilter, endFilter string) ([]adminBookingView, error) {
	var (
		resourceID int64
		err        error
		start      time.Time
		end        time.Time
	)

	if resourceFilter != "" {
		resourceID, err = strconv.ParseInt(resourceFilter, 10, 64)
		if err != nil {
			return nil, adminFormError("Resource filter must be a valid ID.")
		}
	}
	if startFilter != "" {
		start, err = parseAdminDateTime(startFilter)
		if err != nil {
			return nil, adminFormError("Start filter must use local date and time.")
		}
	}
	if endFilter != "" {
		end, err = parseAdminDateTime(endFilter)
		if err != nil {
			return nil, adminFormError("End filter must use local date and time.")
		}
	}

	filtered := make([]adminBookingView, 0, len(bookings))
	for _, booking := range bookings {
		if statusFilter != "" && string(booking.Status) != statusFilter {
			continue
		}
		if resourceID != 0 && booking.ResourceID != resourceID {
			continue
		}
		if !start.IsZero() && booking.EndTime.Before(start) {
			continue
		}
		if !end.IsZero() && booking.StartTime.After(end) {
			continue
		}

		resourceName := "Unknown resource"
		if resource, ok := resourceByID[booking.ResourceID]; ok {
			resourceName = resource.Name
		}

		filtered = append(filtered, adminBookingView{
			ID:           booking.ID,
			ResourceID:   booking.ResourceID,
			ResourceName: resourceName,
			UserID:       booking.UserID,
			StartTime:    booking.StartTime,
			EndTime:      booking.EndTime,
			Status:       booking.Status,
			Purpose:      booking.Purpose,
		})
	}

	return filtered, nil
}

func parseAdminDateRange(startValue, endValue string) (time.Time, time.Time, error) {
	if startValue == "" && endValue == "" {
		end := time.Now().UTC().Truncate(time.Minute)
		start := end.Add(-7 * 24 * time.Hour)
		return start, end, nil
	}
	if startValue == "" || endValue == "" {
		return time.Time{}, time.Time{}, adminFormError("Both report dates are required.")
	}

	start, err := parseAdminDateTime(startValue)
	if err != nil {
		return time.Time{}, time.Time{}, adminFormError("Report start must use local date and time.")
	}
	end, err := parseAdminDateTime(endValue)
	if err != nil {
		return time.Time{}, time.Time{}, adminFormError("Report end must use local date and time.")
	}
	if !start.Before(end) {
		return time.Time{}, time.Time{}, adminFormError("Report start must be before report end.")
	}

	return start, end, nil
}

func parseAdminDateTime(value string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02T15:04", value, time.Local)
}

func mustSubFS(root fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(root, dir)
	if err != nil {
		panic(err)
	}
	return sub
}

func selectedResource(resourceID int64, value string) bool {
	if value == "" {
		return false
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	return err == nil && parsed == resourceID
}

func formatAdminTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Local().Format("02 Jan 2006 15:04")
}

func formatDateTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Local().Format("02 Jan 2006 15:04 MST")
}

func formatDateTimeLocal(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Local().Format("2006-01-02T15:04")
}

func formatPercent(value float64) string {
	return strconv.FormatFloat(value, 'f', 1, 64)
}
