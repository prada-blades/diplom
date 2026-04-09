package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"diplom/internal/config"
	"diplom/internal/domain"
	httpapi "diplom/internal/http"
)

const menuTimeLayout = "2006-01-02 15:04"

type LogSource interface {
	Logs() []string
}

type Menu struct {
	reader       *bufio.Reader
	out          io.Writer
	services     httpapi.AppServices
	logs         LogSource
	address      string
	defaultAdmin config.DefaultAdmin
}

func NewMenu(services httpapi.AppServices, logs LogSource, address string, defaultAdmin config.DefaultAdmin, in io.Reader, out io.Writer) *Menu {
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stdout
	}

	return &Menu{
		reader:       bufio.NewReader(in),
		out:          out,
		services:     services,
		logs:         logs,
		address:      address,
		defaultAdmin: defaultAdmin,
	}
}

func (m *Menu) Run() error {
	for {
		m.printMainMenu()

		choice, err := m.readLine()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		switch choice {
		case "1":
			if err := m.runUsersMenu(); err != nil {
				return err
			}
		case "2":
			if err := m.runResourcesMenu(); err != nil {
				return err
			}
		case "3":
			if err := m.runBookingsMenu(); err != nil {
				return err
			}
		case "4":
			if err := m.runReportsMenu(); err != nil {
				return err
			}
		case "5":
			if err := m.showLogs(); err != nil {
				return err
			}
		case "0":
			fmt.Fprintln(m.out, "Завершение работы администратора.")
			return nil
		default:
			m.printError("Неверный выбор. Введите цифру из меню.")
		}
	}
}

func (m *Menu) printMainMenu() {
	fmt.Fprintln(m.out)
	fmt.Fprintln(m.out, "=== Администратор CLI ===")
	fmt.Fprintf(m.out, "HTTP API: %s\n", m.address)
	fmt.Fprintf(m.out, "Администратор по умолчанию: %s\n", m.defaultAdmin.Email)
	fmt.Fprintln(m.out, "1. Пользователи")
	fmt.Fprintln(m.out, "2. Ресурсы")
	fmt.Fprintln(m.out, "3. Бронирования")
	fmt.Fprintln(m.out, "4. Отчёты")
	fmt.Fprintln(m.out, "5. Логи")
	fmt.Fprintln(m.out, "0. Выход")
	fmt.Fprint(m.out, "Выбор: ")
}

func (m *Menu) runUsersMenu() error {
	for {
		fmt.Fprintln(m.out)
		fmt.Fprintln(m.out, "=== Пользователи ===")
		fmt.Fprintln(m.out, "1. Показать пользователей")
		fmt.Fprintln(m.out, "2. Создать пользователя")
		fmt.Fprintln(m.out, "0. Назад")
		fmt.Fprint(m.out, "Выбор: ")

		choice, err := m.readLine()
		if err != nil {
			return err
		}

		switch choice {
		case "1":
			m.listUsers()
		case "2":
			if err := m.createUser(); err != nil {
				m.printError(err.Error())
			}
		case "0":
			return nil
		default:
			m.printError("Неверный выбор. Введите цифру из меню.")
		}
	}
}

func (m *Menu) runResourcesMenu() error {
	for {
		fmt.Fprintln(m.out)
		fmt.Fprintln(m.out, "=== Ресурсы ===")
		fmt.Fprintln(m.out, "1. Показать активные ресурсы")
		fmt.Fprintln(m.out, "2. Показать все ресурсы")
		fmt.Fprintln(m.out, "3. Создать ресурс")
		fmt.Fprintln(m.out, "4. Отключить ресурс")
		fmt.Fprintln(m.out, "0. Назад")
		fmt.Fprint(m.out, "Выбор: ")

		choice, err := m.readLine()
		if err != nil {
			return err
		}

		switch choice {
		case "1":
			m.listResources(true)
		case "2":
			m.listResources(false)
		case "3":
			if err := m.createResource(); err != nil {
				m.printError(err.Error())
			}
		case "4":
			if err := m.disableResource(); err != nil {
				m.printError(err.Error())
			}
		case "0":
			return nil
		default:
			m.printError("Неверный выбор. Введите цифру из меню.")
		}
	}
}

func (m *Menu) runBookingsMenu() error {
	for {
		fmt.Fprintln(m.out)
		fmt.Fprintln(m.out, "=== Бронирования ===")
		fmt.Fprintln(m.out, "1. Показать все бронирования")
		fmt.Fprintln(m.out, "2. Создать бронирование")
		fmt.Fprintln(m.out, "3. Отменить бронирование")
		fmt.Fprintln(m.out, "0. Назад")
		fmt.Fprint(m.out, "Выбор: ")

		choice, err := m.readLine()
		if err != nil {
			return err
		}

		switch choice {
		case "1":
			m.listBookings()
		case "2":
			if err := m.createBooking(); err != nil {
				m.printError(err.Error())
			}
		case "3":
			if err := m.cancelBooking(); err != nil {
				m.printError(err.Error())
			}
		case "0":
			return nil
		default:
			m.printError("Неверный выбор. Введите цифру из меню.")
		}
	}
}

func (m *Menu) runReportsMenu() error {
	for {
		fmt.Fprintln(m.out)
		fmt.Fprintln(m.out, "=== Отчёты ===")
		fmt.Fprintln(m.out, "1. Загрузка ресурсов")
		fmt.Fprintln(m.out, "0. Назад")
		fmt.Fprint(m.out, "Выбор: ")

		choice, err := m.readLine()
		if err != nil {
			return err
		}

		switch choice {
		case "1":
			if err := m.showUtilizationReport(); err != nil {
				m.printError(err.Error())
			}
		case "0":
			return nil
		default:
			m.printError("Неверный выбор. Введите цифру из меню.")
		}
	}
}

func (m *Menu) listUsers() {
	users := m.services.Auth.ListUsers()
	fmt.Fprintln(m.out)
	if len(users) == 0 {
		fmt.Fprintln(m.out, "Пользователей пока нет.")
		return
	}

	for _, user := range users {
		fmt.Fprintf(m.out, "#%d | %s | %s | %s\n", user.ID, user.FullName, user.Email, user.Role)
	}
}

func (m *Menu) createUser() error {
	fmt.Fprintln(m.out)
	fullName, err := m.prompt("ФИО: ")
	if err != nil {
		return err
	}
	email, err := m.prompt("Email: ")
	if err != nil {
		return err
	}
	password, err := m.prompt("Пароль: ")
	if err != nil {
		return err
	}
	role, err := m.promptRole()
	if err != nil {
		return err
	}

	user, _, err := m.services.Auth.Register(fullName, email, password, role)
	if err != nil {
		return err
	}

	fmt.Fprintf(m.out, "Пользователь создан: #%d %s (%s)\n", user.ID, user.Email, user.Role)
	return nil
}

func (m *Menu) listResources(onlyActive bool) {
	resources := m.services.Resource.List("", onlyActive)
	fmt.Fprintln(m.out)
	if len(resources) == 0 {
		fmt.Fprintln(m.out, "Ресурсов не найдено.")
		return
	}

	for _, resource := range resources {
		fmt.Fprintf(
			m.out,
			"#%d | %s | %s | %s | capacity=%d | active=%t\n",
			resource.ID,
			resource.Name,
			resource.Type,
			resource.Location,
			resource.Capacity,
			resource.IsActive,
		)
	}
}

func (m *Menu) createResource() error {
	fmt.Fprintln(m.out)
	name, err := m.prompt("Название: ")
	if err != nil {
		return err
	}
	resourceType, err := m.promptResourceType()
	if err != nil {
		return err
	}
	location, err := m.prompt("Локация: ")
	if err != nil {
		return err
	}
	capacity, err := m.promptInt("Вместимость (для workspace можно 0): ")
	if err != nil {
		return err
	}
	description, err := m.prompt("Описание: ")
	if err != nil {
		return err
	}

	resource, err := m.services.Resource.Create(name, resourceType, location, capacity, description)
	if err != nil {
		return err
	}

	fmt.Fprintf(m.out, "Ресурс создан: #%d %s\n", resource.ID, resource.Name)
	return nil
}

func (m *Menu) disableResource() error {
	fmt.Fprintln(m.out)
	id, err := m.promptInt64("ID ресурса для отключения: ")
	if err != nil {
		return err
	}

	resource, err := m.services.Resource.Disable(id)
	if err != nil {
		return err
	}

	fmt.Fprintf(m.out, "Ресурс отключён: #%d %s\n", resource.ID, resource.Name)
	return nil
}

func (m *Menu) listBookings() {
	bookings := m.services.Booking.ListAll()
	fmt.Fprintln(m.out)
	if len(bookings) == 0 {
		fmt.Fprintln(m.out, "Бронирований не найдено.")
		return
	}

	for _, booking := range bookings {
		fmt.Fprintf(
			m.out,
			"#%d | user=%d | resource=%d | %s - %s | %s | %s\n",
			booking.ID,
			booking.UserID,
			booking.ResourceID,
			booking.StartTime.Format(menuTimeLayout),
			booking.EndTime.Format(menuTimeLayout),
			booking.Status,
			booking.Purpose,
		)
	}
}

func (m *Menu) createBooking() error {
	fmt.Fprintln(m.out)
	userID, err := m.promptInt64("ID пользователя: ")
	if err != nil {
		return err
	}
	resourceID, err := m.promptInt64("ID ресурса: ")
	if err != nil {
		return err
	}
	start, err := m.promptTime("Начало (YYYY-MM-DD HH:MM): ")
	if err != nil {
		return err
	}
	end, err := m.promptTime("Конец (YYYY-MM-DD HH:MM): ")
	if err != nil {
		return err
	}
	purpose, err := m.prompt("Назначение: ")
	if err != nil {
		return err
	}

	booking, err := m.services.Booking.Create(userID, resourceID, start, end, purpose)
	if err != nil {
		return err
	}

	fmt.Fprintf(m.out, "Бронирование создано: #%d\n", booking.ID)
	return nil
}

func (m *Menu) cancelBooking() error {
	fmt.Fprintln(m.out)
	bookingID, err := m.promptInt64("ID бронирования: ")
	if err != nil {
		return err
	}

	admin, err := m.services.Auth.GetUserByEmail(m.defaultAdmin.Email)
	if err != nil {
		return err
	}

	booking, err := m.services.Booking.Cancel(admin, bookingID)
	if err != nil {
		return err
	}

	fmt.Fprintf(m.out, "Бронирование отменено: #%d\n", booking.ID)
	return nil
}

func (m *Menu) showUtilizationReport() error {
	fmt.Fprintln(m.out)
	start, err := m.promptTime("Период с (YYYY-MM-DD HH:MM): ")
	if err != nil {
		return err
	}
	end, err := m.promptTime("Период по (YYYY-MM-DD HH:MM): ")
	if err != nil {
		return err
	}

	report, err := m.services.Booking.UtilizationReport(start, end)
	if err != nil {
		return err
	}

	fmt.Fprintln(m.out)
	fmt.Fprintln(m.out, "Загрузка по ресурсам:")
	for _, item := range report.Items {
		fmt.Fprintf(
			m.out,
			"#%d | %s | %s | booked=%d min | utilization=%.2f%%\n",
			item.ResourceID,
			item.ResourceName,
			item.ResourceType,
			item.BookedMinutes,
			item.Utilization,
		)
	}

	if len(report.Stats.HourLoads) > 0 {
		fmt.Fprintln(m.out)
		fmt.Fprintln(m.out, "Пиковые часы:")
		for _, item := range report.Stats.HourLoads {
			if item.BookedMinutes == 0 {
				continue
			}
			fmt.Fprintf(m.out, "%02d:00 | booked=%d min | share=%.2f%%\n", item.Hour, item.BookedMinutes, item.SharePercent)
		}
	}

	return nil
}

func (m *Menu) showLogs() error {
	fmt.Fprintln(m.out)
	fmt.Fprintln(m.out, "=== Логи ===")
	lines := m.logs.Logs()
	if len(lines) == 0 {
		fmt.Fprintln(m.out, "Логи пока пусты.")
	} else {
		start := 0
		if len(lines) > 20 {
			start = len(lines) - 20
		}
		for _, line := range lines[start:] {
			fmt.Fprintln(m.out, line)
		}
	}

	fmt.Fprint(m.out, "Нажмите Enter, чтобы вернуться...")
	_, err := m.readLine()
	return err
}

func (m *Menu) prompt(label string) (string, error) {
	fmt.Fprint(m.out, label)
	return m.readLine()
}

func (m *Menu) promptInt(label string) (int, error) {
	value, err := m.prompt(label)
	if err != nil {
		return 0, err
	}

	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("ожидалось целое число")
	}

	return parsed, nil
}

func (m *Menu) promptInt64(label string) (int64, error) {
	value, err := m.prompt(label)
	if err != nil {
		return 0, err
	}

	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("ожидалось целое число")
	}

	return parsed, nil
}

func (m *Menu) promptTime(label string) (time.Time, error) {
	value, err := m.prompt(label)
	if err != nil {
		return time.Time{}, err
	}

	parsed, err := time.ParseInLocation(menuTimeLayout, strings.TrimSpace(value), time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("ожидался формат %s", menuTimeLayout)
	}

	return parsed.UTC(), nil
}

func (m *Menu) promptRole() (domain.Role, error) {
	fmt.Fprintln(m.out, "Роль: 1 = admin, 2 = employee")
	choice, err := m.prompt("Выбор роли: ")
	if err != nil {
		return "", err
	}

	switch choice {
	case "1":
		return domain.RoleAdmin, nil
	case "2":
		return domain.RoleEmployee, nil
	default:
		return "", fmt.Errorf("неверная роль")
	}
}

func (m *Menu) promptResourceType() (domain.ResourceType, error) {
	fmt.Fprintln(m.out, "Тип ресурса: 1 = meeting_room, 2 = workspace")
	choice, err := m.prompt("Выбор типа: ")
	if err != nil {
		return "", err
	}

	switch choice {
	case "1":
		return domain.ResourceMeetingRoom, nil
	case "2":
		return domain.ResourceWorkspace, nil
	default:
		return "", fmt.Errorf("неверный тип ресурса")
	}
}

func (m *Menu) readLine() (string, error) {
	line, err := m.reader.ReadString('\n')
	if err != nil {
		if err == io.EOF && strings.TrimSpace(line) == "" {
			return "", io.EOF
		}
	}
	return strings.TrimSpace(line), err
}

func (m *Menu) printError(message string) {
	fmt.Fprintf(m.out, "Ошибка: %s\n", message)
}
