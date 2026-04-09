# diplom

Учебный backend для ВКР: серверная часть системы бронирования общих корпоративных ресурсов предприятия с role-based доступом для `employee` и `admin`.

## Что уже реализовано

- регистрация и вход пользователя;
- JWT-аутентификация через библиотеку `github.com/golang-jwt/jwt/v5`;
- PostgreSQL-хранилище с SQL-миграцией при старте;
- Redis-кэш для read-only endpoint'ов с graceful fallback;
- роли `employee` и `admin` с разграничением прав на уровне сервера;
- справочник ресурсов (`meeting_room`, `workspace`);
- создание и отмена бронирований;
- поиск доступных ресурсов по временному интервалу;
- подбор оптимальных вариантов бронирования переговорной по времени, вместимости и исторической загрузке;
- административный просмотр всех броней;
- отчёт по загрузке ресурсов;
- статистика загрузки по ресурсам, часам и дням недели;
- unit-тесты для правил бронирования и временных конфликтов;
- OpenAPI-контракт в `openapi.yaml`.

## Быстрый запуск

1. Поднимите PostgreSQL, Redis и приложение:

```bash
docker compose up -d
```

2. Проверьте состояние контейнеров:

```bash
docker compose ps
```

Сервер слушает `:8080`.

Для просмотра логов приложения:

```bash
docker compose logs -f app
```

Для локального запуска без Docker всё ещё можно использовать:

```bash
go run .
```

Переменные окружения по умолчанию:

- `APP_ADDRESS=:8080`
- `APP_DATABASE_URL=postgres://postgres:postgres@localhost:5432/diplom?sslmode=disable`
- `APP_JWT_SECRET=development-secret`
- `APP_REDIS_ENABLED=false`
- `APP_REDIS_ADDR=localhost:6379`
- `APP_REDIS_PASSWORD=`
- `APP_REDIS_DB=0`

Предсозданный администратор по умолчанию:

- email: `admin@corp.local`
- password: `admin123`

Один и тот же веб-клиент может работать с обеими ролями.
Сервер сам определяет права по JWT и возвращает `403 forbidden` для административных операций, недоступных пользователю.

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
- `POST /recommendations/schedule`
- `GET /bookings/my`
- `POST /bookings`
- `DELETE /bookings/{id}`
- `GET /admin/bookings` (admin)
- `GET /admin/reports/utilization?start=...&end=...` (admin)
