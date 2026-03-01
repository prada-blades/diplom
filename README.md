# diplom

Учебный backend для ВКР: серверная часть системы бронирования общих корпоративных ресурсов предприятия.

## Что уже реализовано

- регистрация и вход пользователя;
- JWT-подобная аутентификация без внешних зависимостей;
- роли `employee` и `admin`;
- справочник ресурсов (`meeting_room`, `workspace`);
- создание и отмена бронирований;
- поиск доступных ресурсов по временному интервалу;
- административный просмотр всех броней;
- отчёт по загрузке ресурсов.

## Быстрый запуск

```bash
go run .
```

Сервер слушает `:8080`.

Предсозданный администратор по умолчанию:

- email: `admin@corp.local`
- password: `admin123`

## Основные endpoint'ы

- `GET /health`
- `POST /auth/register`
- `POST /auth/login`
- `GET /me`
- `GET /resources`
- `POST /resources` (admin)
- `PUT /resources/{id}` (admin)
- `DELETE /resources/{id}` (admin)
- `GET /availability?start=...&end=...`
- `GET /bookings/my`
- `POST /bookings`
- `DELETE /bookings/{id}`
- `GET /admin/bookings` (admin)
- `GET /admin/reports/utilization?start=...&end=...` (admin)
