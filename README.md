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

1. Подготовьте `.env` на основе шаблона:

```bash
cp .env.example .env
```

Перед публикацией обязательно замените значения `POSTGRES_PASSWORD`, `APP_JWT_SECRET` и `APP_ADMIN_PASSWORD` на свои.

2. Поднимите внутренний стек PostgreSQL, Redis и приложения:

```bash
docker compose up -d
```

Или короткой командой из корня проекта:

```bash
./booking up
```

2. Проверьте состояние контейнеров:

```bash
docker compose ps
```

Приложение слушает `:8080` только внутри docker-сети. Для публикации в интернет используйте отдельный `nginx`, который проксирует запросы на `app:8080`.

Для просмотра логов приложения:

```bash
docker compose logs -f app
```

Или:

```bash
./booking logs
```

Для локального запуска без Docker всё ещё можно использовать:

```bash
go run .
```

## Команда booking

В корне проекта есть короткая обёртка для Docker-команд:

```bash
./booking up
./booking restart
./booking down
./booking logs
```

Переменные окружения по умолчанию:

- `APP_ADDRESS=:8080`
- `APP_DATABASE_URL=postgres://postgres:<password>@postgres:5432/diplom?sslmode=disable`
- `APP_REDIS_ENABLED=true`
- `APP_REDIS_ADDR=redis:6379`
- `APP_REDIS_PASSWORD=`
- `APP_REDIS_DB=0`

В прод-конфигурации внешние порты `8080`, `5432` и `6379` не публикуются. Сервисы доступны только внутри docker-сети.

Предсозданный администратор задаётся через `.env`:

- email: `admin@corp.local`
- password: значение `APP_ADMIN_PASSWORD`

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
