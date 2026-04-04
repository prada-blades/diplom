# diplom

Учебный backend для ВКР: серверная часть системы бронирования общих корпоративных ресурсов предприятия.

## Что уже реализовано

- регистрация и вход пользователя;
- JWT-аутентификация через библиотеку `github.com/golang-jwt/jwt/v5`;
- PostgreSQL-хранилище с SQL-миграцией при старте;
- Redis-кэш для read-only endpoint'ов с graceful fallback;
- роли `employee` и `admin`;
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

1. Поднимите PostgreSQL, Redis и сервер:

```bash
docker compose up -d postgres redis app
```

2. Для просмотра логов сервера используйте:

```bash
docker compose logs -f app
```

3. Когда нужна админка, запускайте её отдельной Docker-командой:

```bash
docker compose run --rm app admin
```

Сервер слушает `:8080` и живёт в отдельном контейнере, поэтому не зависит от открытого терминала.
Выход из админки не влияет на контейнер `app`.

## Команда booking

Для коротких команд есть обёртка `booking` в корне проекта.

Из корня проекта:

```bash
./booking up
./booking admin
./booking logs
./booking ps
```

Чтобы запускать `booking` из любой директории:

```bash
make install-booking
export PATH="$HOME/.local/bin:$PATH"
```

После этого можно использовать:

```bash
booking up
booking admin
booking logs
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

## CLI администратора

Главное меню содержит разделы:

- пользователи;
- ресурсы;
- бронирования;
- отчёты.

Через CLI можно:

- просматривать и создавать пользователей;
- просматривать, создавать и отключать ресурсы;
- просматривать, создавать и отменять бронирования;
- смотреть отчёт по загрузке ресурсов за период.

Серверные логи смотрятся вне админки:

- `docker compose logs -f app`
- `docker logs -f diplom-app`

## Режимы запуска

Поддерживаются два режима:

- `go run .` или `go run . server` запускает только HTTP API;
- `go run . admin` запускает только консольную админку.

В Docker-сценарии обычно напрямую используется не `go run`, а:

- `docker compose up -d postgres redis app` для сервера;
- `docker compose run --rm app admin` для админки.

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
