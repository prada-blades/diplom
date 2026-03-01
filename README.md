# diplom

Учебный backend для ВКР: серверная часть системы бронирования общих корпоративных ресурсов предприятия.

## Что уже реализовано

- регистрация и вход пользователя;
- JWT-подобная аутентификация без внешних зависимостей;
- PostgreSQL-хранилище с SQL-миграцией при старте;
- роли `employee` и `admin`;
- справочник ресурсов (`meeting_room`, `workspace`);
- создание и отмена бронирований;
- поиск доступных ресурсов по временному интервалу;
- административный просмотр всех броней;
- отчёт по загрузке ресурсов;
- unit-тесты для правил бронирования и временных конфликтов;
- OpenAPI-контракт в `openapi.yaml`.

## Быстрый запуск

1. Поднимите PostgreSQL:

```bash
docker compose up -d
```

2. Запустите сервер:

```bash
go run .
```

Сервер слушает `:8080`.

Переменные окружения по умолчанию:

- `APP_ADDRESS=:8080`
- `APP_DATABASE_URL=postgres://postgres:postgres@localhost:5432/diplom?sslmode=disable`
- `APP_JWT_SECRET=development-secret`

Предсозданный администратор по умолчанию:

- email: `admin@corp.local`
- password: `admin123`

## Тесты

```bash
go test ./...
```

## Контракты и схемы

- Markdown API-документация: `API.md`
- OpenAPI-спецификация: `openapi.yaml`
- SQL-миграция: `internal/repository/postgres/migrations/001_init.sql`

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
