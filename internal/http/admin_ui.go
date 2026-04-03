package http

import (
	"embed"
	"fmt"
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
		"formatTime":        formatAdminTime,
		"formatDateTime":    formatDateTime,
		"formatPercent":     formatPercent,
		"selectedResource":  selectedID,
		"selectedUser":      selectedID,
		"roleLabel":         roleLabel,
		"statusLabel":       statusLabel,
		"resourceTypeLabel": resourceTypeLabel,
		"weekdayLabel":      weekdayLabel,
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

	Users      []domain.User
	UserRoles  []domain.Role
	UserForm   adminUserForm
	EditUserID int64

	Bookings          []adminBookingView
	BookingCreateForm adminBookingCreateForm
	StatusFilter      string
	ResourceFilter    string
	UserFilter        string
	StartFilter       string
	EndFilter         string
	Report            *service.UtilizationReport
	ReportStart       string
	ReportEnd         string
}

type adminDashboardStats struct {
	UsersCount           int
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

type adminUserForm struct {
	ID       int64
	FullName string
	Email    string
	Password string
	Role     domain.Role
}

type adminBookingCreateForm struct {
	UserID     string
	ResourceID string
	StartTime  string
	EndTime    string
	Purpose    string
}

type adminBookingView struct {
	ID           int64
	ResourceID   int64
	ResourceName string
	UserID       int64
	UserName     string
	UserEmail    string
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
	mux.Handle("/admin/ui/users", a.requireAdminSession(nethttp.HandlerFunc(a.handleAdminUIUsers)))
	mux.Handle("/admin/ui/users/", a.requireAdminSession(nethttp.HandlerFunc(a.handleAdminUIUserAction)))
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
			Title:           "Вход в админку",
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
			Title:           "Вход в админку",
			ContentTemplate: "login_content",
			Error:           "Не удалось прочитать форму входа.",
		})
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	user, token, err := a.authService.Login(email, password)
	if err != nil || user.Role != domain.RoleAdmin {
		a.renderAdminTemplate(w, adminPageData{
			Title:           "Вход в админку",
			ContentTemplate: "login_content",
			Error:           "Неверные учётные данные администратора.",
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
	nethttp.Redirect(w, r, "/admin/ui/login?notice="+url.QueryEscape("Вы вышли из админки."), nethttp.StatusSeeOther)
}

func (a *App) handleAdminUIDashboard(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.Method != nethttp.MethodGet {
		nethttp.Error(w, "method not allowed", nethttp.StatusMethodNotAllowed)
		return
	}

	admin := currentUser(r)
	users := a.authService.ListUsers()
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
		Title:           "Панель администратора",
		ContentTemplate: "dashboard_content",
		ActiveTab:       "dashboard",
		Admin:           admin,
		Notice:          r.URL.Query().Get("notice"),
		Stats: adminDashboardStats{
			UsersCount:           len(users),
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
			data.Error = "Не удалось прочитать форму ресурса."
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
			data.Error = translateAdminError(err.Error())
			data.ResourceForm = form
			a.renderAdminTemplate(w, data)
			return
		}

		nethttp.Redirect(w, r, "/admin/ui/resources?notice="+url.QueryEscape("Ресурс создан."), nethttp.StatusSeeOther)
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
			data.Error = "Не удалось прочитать форму ресурса."
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
			data.Error = translateAdminError(err.Error())
			data.ResourceForm = form
			data.ResourceForm.IsActive = isActive
			data.EditResourceID = id
			a.renderAdminTemplate(w, data)
			return
		}

		nethttp.Redirect(w, r, "/admin/ui/resources?notice="+url.QueryEscape("Ресурс обновлён."), nethttp.StatusSeeOther)
	case "disable":
		if _, err := a.resourceService.Disable(id); err != nil {
			nethttp.Redirect(w, r, "/admin/ui/resources?notice="+url.QueryEscape("Не удалось отключить ресурс."), nethttp.StatusSeeOther)
			return
		}
		nethttp.Redirect(w, r, "/admin/ui/resources?notice="+url.QueryEscape("Ресурс отключён."), nethttp.StatusSeeOther)
	default:
		nethttp.NotFound(w, r)
	}
}

func (a *App) handleAdminUIUsers(w nethttp.ResponseWriter, r *nethttp.Request) {
	admin := currentUser(r)
	switch r.Method {
	case nethttp.MethodGet:
		data := a.newAdminUsersPage(admin)
		data.Notice = r.URL.Query().Get("notice")
		if editRaw := r.URL.Query().Get("edit"); editRaw != "" {
			if id, err := strconv.ParseInt(editRaw, 10, 64); err == nil {
				if user, err := a.authService.GetUser(id); err == nil {
					data.UserForm = adminUserForm{
						ID:       user.ID,
						FullName: user.FullName,
						Email:    user.Email,
						Role:     user.Role,
					}
					data.EditUserID = user.ID
				}
			}
		}
		a.renderAdminTemplate(w, data)
	case nethttp.MethodPost:
		if err := r.ParseForm(); err != nil {
			data := a.newAdminUsersPage(admin)
			data.Error = "Не удалось прочитать форму пользователя."
			a.renderAdminTemplate(w, data)
			return
		}

		form := parseAdminUserForm(r)
		if strings.TrimSpace(form.Password) == "" {
			data := a.newAdminUsersPage(admin)
			data.Error = "Пароль обязателен для нового пользователя."
			data.UserForm = form
			a.renderAdminTemplate(w, data)
			return
		}

		if _, _, err := a.authService.Register(form.FullName, form.Email, form.Password, form.Role); err != nil {
			data := a.newAdminUsersPage(admin)
			data.Error = translateAdminError(err.Error())
			data.UserForm = form
			a.renderAdminTemplate(w, data)
			return
		}

		nethttp.Redirect(w, r, "/admin/ui/users?notice="+url.QueryEscape("Пользователь создан."), nethttp.StatusSeeOther)
	default:
		nethttp.Error(w, "method not allowed", nethttp.StatusMethodNotAllowed)
	}
}

func (a *App) handleAdminUIUserAction(w nethttp.ResponseWriter, r *nethttp.Request) {
	if r.Method != nethttp.MethodPost {
		nethttp.Error(w, "method not allowed", nethttp.StatusMethodNotAllowed)
		return
	}

	id, action, ok := parseAdminEntityAction(r.URL.Path, "/admin/ui/users/")
	if !ok || action != "update" {
		nethttp.NotFound(w, r)
		return
	}

	admin := currentUser(r)
	if err := r.ParseForm(); err != nil {
		data := a.newAdminUsersPage(admin)
		data.Error = "Не удалось прочитать форму пользователя."
		a.renderAdminTemplate(w, data)
		return
	}

	form := parseAdminUserForm(r)
	form.ID = id
	if _, err := a.authService.UpdateUser(id, form.FullName, form.Email, form.Role); err != nil {
		data := a.newAdminUsersPage(admin)
		data.Error = translateAdminError(err.Error())
		data.UserForm = form
		data.EditUserID = id
		a.renderAdminTemplate(w, data)
		return
	}

	nethttp.Redirect(w, r, "/admin/ui/users?notice="+url.QueryEscape("Пользователь обновлён."), nethttp.StatusSeeOther)
}

func (a *App) handleAdminUIBookings(w nethttp.ResponseWriter, r *nethttp.Request) {
	admin := currentUser(r)
	switch r.Method {
	case nethttp.MethodGet:
		data := a.newAdminBookingsPage(admin)
		data.Notice = r.URL.Query().Get("notice")
		populateAdminBookingFilters(&data, r)
		a.populateBookingsList(&data)
		a.renderAdminTemplate(w, data)
	case nethttp.MethodPost:
		if err := r.ParseForm(); err != nil {
			data := a.newAdminBookingsPage(admin)
			data.Error = "Не удалось прочитать форму бронирования."
			a.populateBookingsList(&data)
			a.renderAdminTemplate(w, data)
			return
		}

		data := a.newAdminBookingsPage(admin)
		populateAdminBookingFilters(&data, r)
		data.BookingCreateForm = parseAdminBookingCreateForm(r)

		userID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("user_id")), 10, 64)
		if err != nil {
			data.Error = "Пользователь должен быть выбран."
			a.populateBookingsList(&data)
			a.renderAdminTemplate(w, data)
			return
		}
		resourceID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("resource_id")), 10, 64)
		if err != nil {
			data.Error = "Ресурс должен быть выбран."
			a.populateBookingsList(&data)
			a.renderAdminTemplate(w, data)
			return
		}

		start, err := parseAdminDateTime(strings.TrimSpace(r.FormValue("start_time")))
		if err != nil {
			data.Error = "Дата начала должна быть указана в локальном формате."
			a.populateBookingsList(&data)
			a.renderAdminTemplate(w, data)
			return
		}
		end, err := parseAdminDateTime(strings.TrimSpace(r.FormValue("end_time")))
		if err != nil {
			data.Error = "Дата окончания должна быть указана в локальном формате."
			a.populateBookingsList(&data)
			a.renderAdminTemplate(w, data)
			return
		}

		if _, err := a.bookingService.Create(userID, resourceID, start, end, strings.TrimSpace(r.FormValue("purpose"))); err != nil {
			data.Error = translateAdminError(err.Error())
			a.populateBookingsList(&data)
			a.renderAdminTemplate(w, data)
			return
		}

		nethttp.Redirect(w, r, "/admin/ui/bookings?notice="+url.QueryEscape("Бронирование создано."), nethttp.StatusSeeOther)
	default:
		nethttp.Error(w, "method not allowed", nethttp.StatusMethodNotAllowed)
	}
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
		nethttp.Redirect(w, r, "/admin/ui/bookings?notice="+url.QueryEscape("Не удалось отменить бронь."), nethttp.StatusSeeOther)
		return
	}

	nethttp.Redirect(w, r, "/admin/ui/bookings?notice="+url.QueryEscape("Бронь отменена."), nethttp.StatusSeeOther)
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
		Title:           "Отчёт по загрузке",
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
		data.Error = translateAdminError(err.Error())
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
			nethttp.Redirect(w, r, "/admin/ui/login?error="+url.QueryEscape("Войдите под учётной записью администратора."), nethttp.StatusSeeOther)
			return
		}

		next.ServeHTTP(w, r.WithContext(withUser(r.Context(), user)))
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
		Title:           "Ресурсы",
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

func (a *App) newAdminUsersPage(admin domain.User) adminPageData {
	return adminPageData{
		Title:           "Пользователи",
		ContentTemplate: "users_content",
		ActiveTab:       "users",
		Admin:           admin,
		Users:           a.authService.ListUsers(),
		UserRoles:       []domain.Role{domain.RoleEmployee, domain.RoleAdmin},
		UserForm: adminUserForm{
			Role: domain.RoleEmployee,
		},
	}
}

func (a *App) newAdminBookingsPage(admin domain.User) adminPageData {
	now := time.Now().Local().Add(2 * time.Hour).Truncate(15 * time.Minute)
	return adminPageData{
		Title:           "Бронирования",
		ContentTemplate: "bookings_content",
		ActiveTab:       "bookings",
		Admin:           admin,
		Users:           a.authService.ListUsers(),
		Resources:       a.resourceService.List("", false),
		BookingCreateForm: adminBookingCreateForm{
			StartTime: formatDateTimeLocal(now),
			EndTime:   formatDateTimeLocal(now.Add(time.Hour)),
		},
	}
}

func (a *App) populateBookingsList(data *adminPageData) {
	resourceByID := make(map[int64]domain.Resource, len(data.Resources))
	for _, resource := range data.Resources {
		resourceByID[resource.ID] = resource
	}
	userByID := make(map[int64]domain.User, len(data.Users))
	for _, user := range data.Users {
		userByID[user.ID] = user
	}

	filtered, err := filterAdminBookings(a.bookingService.ListAll(), resourceByID, userByID, data.StatusFilter, data.ResourceFilter, data.UserFilter, data.StartFilter, data.EndFilter)
	data.Bookings = filtered
	if err != nil {
		data.Error = err.Error()
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
			return form, adminFormError("Вместимость должна быть числом.")
		}
		form.Capacity = capacity
	}

	if form.Type == "" {
		form.Type = domain.ResourceMeetingRoom
	}

	return form, nil
}

func parseAdminUserForm(r *nethttp.Request) adminUserForm {
	role := domain.Role(strings.TrimSpace(r.FormValue("role")))
	if role == "" {
		role = domain.RoleEmployee
	}

	return adminUserForm{
		FullName: strings.TrimSpace(r.FormValue("full_name")),
		Email:    strings.TrimSpace(r.FormValue("email")),
		Password: r.FormValue("password"),
		Role:     role,
	}
}

func parseAdminBookingCreateForm(r *nethttp.Request) adminBookingCreateForm {
	return adminBookingCreateForm{
		UserID:     strings.TrimSpace(r.FormValue("user_id")),
		ResourceID: strings.TrimSpace(r.FormValue("resource_id")),
		StartTime:  strings.TrimSpace(r.FormValue("start_time")),
		EndTime:    strings.TrimSpace(r.FormValue("end_time")),
		Purpose:    strings.TrimSpace(r.FormValue("purpose")),
	}
}

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

func filterAdminBookings(bookings []domain.Booking, resourceByID map[int64]domain.Resource, userByID map[int64]domain.User, statusFilter, resourceFilter, userFilter, startFilter, endFilter string) ([]adminBookingView, error) {
	var (
		resourceID int64
		userID     int64
		err        error
		start      time.Time
		end        time.Time
	)

	if resourceFilter != "" {
		resourceID, err = strconv.ParseInt(resourceFilter, 10, 64)
		if err != nil {
			return nil, adminFormError("Фильтр ресурса должен содержать корректный ID.")
		}
	}
	if userFilter != "" {
		userID, err = strconv.ParseInt(userFilter, 10, 64)
		if err != nil {
			return nil, adminFormError("Фильтр пользователя должен содержать корректный ID.")
		}
	}
	if startFilter != "" {
		start, err = parseAdminDateTime(startFilter)
		if err != nil {
			return nil, adminFormError("Дата начала фильтра должна быть в локальном формате.")
		}
	}
	if endFilter != "" {
		end, err = parseAdminDateTime(endFilter)
		if err != nil {
			return nil, adminFormError("Дата окончания фильтра должна быть в локальном формате.")
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
		if userID != 0 && booking.UserID != userID {
			continue
		}
		if !start.IsZero() && booking.EndTime.Before(start) {
			continue
		}
		if !end.IsZero() && booking.StartTime.After(end) {
			continue
		}

		resourceName := fmt.Sprintf("Ресурс #%d", booking.ResourceID)
		if resource, ok := resourceByID[booking.ResourceID]; ok {
			resourceName = resource.Name
		}
		userName := fmt.Sprintf("Пользователь #%d", booking.UserID)
		userEmail := ""
		if user, ok := userByID[booking.UserID]; ok {
			userName = user.FullName
			userEmail = user.Email
		}

		filtered = append(filtered, adminBookingView{
			ID:           booking.ID,
			ResourceID:   booking.ResourceID,
			ResourceName: resourceName,
			UserID:       booking.UserID,
			UserName:     userName,
			UserEmail:    userEmail,
			StartTime:    booking.StartTime,
			EndTime:      booking.EndTime,
			Status:       booking.Status,
			Purpose:      booking.Purpose,
		})
	}

	return filtered, nil
}

func populateAdminBookingFilters(data *adminPageData, r *nethttp.Request) {
	data.StatusFilter = strings.TrimSpace(r.FormValue("status"))
	data.ResourceFilter = strings.TrimSpace(r.FormValue("resource_id"))
	data.UserFilter = strings.TrimSpace(r.FormValue("user_id"))
	data.StartFilter = strings.TrimSpace(r.FormValue("start"))
	data.EndFilter = strings.TrimSpace(r.FormValue("end"))
}

func parseAdminDateRange(startValue, endValue string) (time.Time, time.Time, error) {
	if startValue == "" && endValue == "" {
		end := time.Now().UTC().Truncate(time.Minute)
		start := end.Add(-7 * 24 * time.Hour)
		return start, end, nil
	}
	if startValue == "" || endValue == "" {
		return time.Time{}, time.Time{}, adminFormError("Нужно указать обе границы периода.")
	}

	start, err := parseAdminDateTime(startValue)
	if err != nil {
		return time.Time{}, time.Time{}, adminFormError("Начало периода должно быть в локальном формате.")
	}
	end, err := parseAdminDateTime(endValue)
	if err != nil {
		return time.Time{}, time.Time{}, adminFormError("Конец периода должен быть в локальном формате.")
	}
	if !start.Before(end) {
		return time.Time{}, time.Time{}, adminFormError("Начало периода должно быть раньше конца.")
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

func selectedID(id int64, value string) bool {
	if value == "" {
		return false
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	return err == nil && parsed == id
}

func formatAdminTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Local().Format("02.01.2006 15:04")
}

func formatDateTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Local().Format("02.01.2006 15:04")
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

func roleLabel(role any) string {
	switch typed := role.(type) {
	case domain.Role:
		switch typed {
		case domain.RoleAdmin:
			return "Администратор"
		case domain.RoleEmployee:
			return "Сотрудник"
		default:
			return string(typed)
		}
	case string:
		return roleLabel(domain.Role(typed))
	default:
		return fmt.Sprint(role)
	}
}

func statusLabel(status any) string {
	switch typed := status.(type) {
	case domain.BookingStatus:
		switch typed {
		case domain.BookingActive:
			return "Активна"
		case domain.BookingCancelled:
			return "Отменена"
		default:
			return string(typed)
		}
	case string:
		return statusLabel(domain.BookingStatus(typed))
	default:
		return fmt.Sprint(status)
	}
}

func resourceTypeLabel(resourceType any) string {
	switch typed := resourceType.(type) {
	case domain.ResourceType:
		switch typed {
		case domain.ResourceMeetingRoom:
			return "Переговорная"
		case domain.ResourceWorkspace:
			return "Рабочее место"
		default:
			return string(typed)
		}
	case string:
		return resourceTypeLabel(domain.ResourceType(typed))
	default:
		return fmt.Sprint(resourceType)
	}
}

func weekdayLabel(value string) string {
	switch strings.ToLower(value) {
	case "monday":
		return "Понедельник"
	case "tuesday":
		return "Вторник"
	case "wednesday":
		return "Среда"
	case "thursday":
		return "Четверг"
	case "friday":
		return "Пятница"
	case "saturday":
		return "Суббота"
	case "sunday":
		return "Воскресенье"
	default:
		return value
	}
}

func translateAdminError(message string) string {
	switch message {
	case "full_name, email and password are required":
		return "Нужно указать ФИО, email и пароль."
	case "full_name and email are required":
		return "Нужно указать ФИО и email."
	case "invalid credentials":
		return "Неверные учётные данные."
	case "invalid role":
		return "Указана некорректная роль."
	case "email already exists":
		return "Пользователь с таким email уже существует."
	case "name, type and location are required":
		return "Нужно указать название, тип и расположение."
	case "invalid resource type":
		return "Указан некорректный тип ресурса."
	case "meeting room capacity must be positive":
		return "Для переговорной вместимость должна быть больше нуля."
	case "resource not found":
		return "Ресурс не найден."
	case "resource is inactive":
		return "Ресурс отключён."
	case "start_time must be before end_time":
		return "Время начала должно быть раньше времени окончания."
	case "cannot book in the past":
		return "Нельзя создать бронь в прошлом."
	case "resource already booked for selected time":
		return "Ресурс уже занят на выбранное время."
	case "booking not found":
		return "Бронь не найдена."
	case "forbidden":
		return "Недостаточно прав для этой операции."
	default:
		return message
	}
}
