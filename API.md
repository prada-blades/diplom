# API Documentation

Документация по HTTP API серверной части системы бронирования общих корпоративных ресурсов.

## Общая информация

- Базовый адрес по умолчанию: `http://localhost:8080`
- Формат данных: `application/json`
- Время во всех запросах и ответах: `RFC3339`
- Авторизация: `Authorization: Bearer <token>`

## Роли

- `employee`: обычный пользователь
- `admin`: администратор системы

## Формат ошибок

При ошибке сервер возвращает JSON:

```json
{
  "error": {
    "code": "error_code",
    "message": "human readable message"
  }
}
```

## Предсозданный администратор

По умолчанию после запуска создаётся администратор:

- `email`: `admin@corp.local`
- `password`: `admin123`

## 1. Health Check

### `GET /health`

Проверка доступности сервиса.

Пример ответа:

```json
{
  "status": "ok",
  "time": "2026-03-01T12:00:00Z"
}
```

## 2. Аутентификация

### `POST /auth/register`

Регистрация нового пользователя с ролью `employee`.

Тело запроса:

```json
{
  "full_name": "Ivan Petrov",
  "email": "ivan@example.com",
  "password": "secret123"
}
```

Успешный ответ: `201 Created`

```json
{
  "user": {
    "id": 2,
    "full_name": "Ivan Petrov",
    "email": "ivan@example.com",
    "role": "employee",
    "created_at": "2026-03-01T12:00:00Z"
  },
  "token": "jwt-token"
}
```

Ошибки:

- `400 register_failed`: некорректные данные или email уже существует
- `400 invalid_json`: неверный JSON

### `POST /auth/login`

Вход пользователя.

Тело запроса:

```json
{
  "email": "ivan@example.com",
  "password": "secret123"
}
```

Успешный ответ: `200 OK`

```json
{
  "user": {
    "id": 2,
    "full_name": "Ivan Petrov",
    "email": "ivan@example.com",
    "role": "employee",
    "created_at": "2026-03-01T12:00:00Z"
  },
  "token": "jwt-token"
}
```

Ошибки:

- `401 login_failed`: неверные учётные данные
- `400 invalid_json`: неверный JSON

### `GET /me`

Возвращает текущего авторизованного пользователя.

Требует авторизацию: `employee`, `admin`

Пример ответа:

```json
{
  "id": 2,
  "full_name": "Ivan Petrov",
  "email": "ivan@example.com",
  "role": "employee",
  "created_at": "2026-03-01T12:00:00Z"
}
```

Ошибки:

- `401 unauthorized`: отсутствует или недействителен токен

## 3. Ресурсы

### Структура ресурса

```json
{
  "id": 1,
  "name": "Room A",
  "type": "meeting_room",
  "location": "Office 3, Floor 2",
  "capacity": 8,
  "description": "Main meeting room",
  "is_active": true,
  "created_at": "2026-03-01T12:00:00Z",
  "updated_at": "2026-03-01T12:00:00Z"
}
```

`type` может быть:

- `meeting_room`
- `workspace`

### `GET /resources`

Возвращает список ресурсов.

Параметры запроса:

- `type` (optional): фильтр по типу ресурса
- `include_inactive=true` (optional): включить неактивные ресурсы

Пример:

`GET /resources?type=meeting_room`

Успешный ответ: `200 OK`

```json
{
  "items": [
    {
      "id": 1,
      "name": "Room A",
      "type": "meeting_room",
      "location": "Office 3, Floor 2",
      "capacity": 8,
      "description": "Main meeting room",
      "is_active": true,
      "created_at": "2026-03-01T12:00:00Z",
      "updated_at": "2026-03-01T12:00:00Z"
    }
  ]
}
```

Авторизация не требуется.

### `GET /resources/{id}`

Возвращает ресурс по идентификатору.

Успешный ответ: `200 OK`

Ошибки:

- `400 invalid_resource_id`: неверный ID
- `404 not_found`: ресурс не найден

Авторизация не требуется.

### `POST /resources`

Создаёт новый ресурс.

Требует авторизацию: `admin`

Тело запроса:

```json
{
  "name": "Room A",
  "type": "meeting_room",
  "location": "Office 3, Floor 2",
  "capacity": 8,
  "description": "Main meeting room"
}
```

Для `workspace` поле `capacity` может быть `0`.

Успешный ответ: `201 Created`

Ошибки:

- `401 unauthorized`: нет токена
- `403 forbidden`: недостаточно прав
- `400 resource_create_failed`: невалидные поля
- `400 invalid_json`: неверный JSON

### `PUT /resources/{id}`

Обновляет ресурс.

Требует авторизацию: `admin`

Тело запроса:

```json
{
  "name": "Room A Updated",
  "type": "meeting_room",
  "location": "Office 3, Floor 2",
  "capacity": 10,
  "description": "Updated description",
  "is_active": true
}
```

Успешный ответ: `200 OK`

Ошибки:

- `400 invalid_resource_id`: неверный ID
- `401 unauthorized`: нет токена
- `403 forbidden`: недостаточно прав
- `400 resource_update_failed`: невалидные поля или ошибка обновления

### `DELETE /resources/{id}`

Мягко деактивирует ресурс (`is_active = false`).

Требует авторизацию: `admin`

Успешный ответ: `200 OK`

Ошибки:

- `400 invalid_resource_id`: неверный ID
- `401 unauthorized`: нет токена
- `403 forbidden`: недостаточно прав
- `404 not_found`: ресурс не найден

## 4. Бронирование

### Структура бронирования

```json
{
  "id": 1,
  "resource_id": 1,
  "user_id": 2,
  "start_time": "2026-03-02T09:00:00Z",
  "end_time": "2026-03-02T10:00:00Z",
  "status": "active",
  "purpose": "Team sync",
  "created_at": "2026-03-01T12:00:00Z"
}
```

`status` может быть:

- `active`
- `cancelled`

### `POST /bookings`

Создаёт бронирование.

Требует авторизацию: `employee`, `admin`

Тело запроса:

```json
{
  "resource_id": 1,
  "start_time": "2026-03-02T09:00:00Z",
  "end_time": "2026-03-02T10:00:00Z",
  "purpose": "Team sync"
}
```

Успешный ответ: `201 Created`

Ошибки:

- `401 unauthorized`: нет токена
- `400 booking_create_failed`: ресурс не найден, ресурс неактивен, пересечение по времени, бронирование в прошлом, неверный интервал
- `400 invalid_json`: неверный JSON
- `400 invalid_start_time`: `start_time` не в формате `RFC3339`
- `400 invalid_end_time`: `end_time` не в формате `RFC3339`

### `GET /bookings/my`

Возвращает список бронирований текущего пользователя.

Требует авторизацию: `employee`, `admin`

Успешный ответ: `200 OK`

```json
{
  "items": [
    {
      "id": 1,
      "resource_id": 1,
      "user_id": 2,
      "start_time": "2026-03-02T09:00:00Z",
      "end_time": "2026-03-02T10:00:00Z",
      "status": "active",
      "purpose": "Team sync",
      "created_at": "2026-03-01T12:00:00Z"
    }
  ]
}
```

Ошибки:

- `401 unauthorized`: нет токена

### `DELETE /bookings/{id}`

Отменяет бронирование.

Требует авторизацию: `employee`, `admin`

Правила доступа:

- пользователь может отменить только свою бронь;
- администратор может отменить любую бронь.

Успешный ответ: `200 OK`

Пример ответа:

```json
{
  "id": 1,
  "resource_id": 1,
  "user_id": 2,
  "start_time": "2026-03-02T09:00:00Z",
  "end_time": "2026-03-02T10:00:00Z",
  "status": "cancelled",
  "purpose": "Team sync",
  "created_at": "2026-03-01T12:00:00Z",
  "cancelled_at": "2026-03-01T12:30:00Z"
}
```

Ошибки:

- `400 invalid_booking_id`: неверный ID
- `401 unauthorized`: нет токена
- `403 booking_cancel_failed`: нет прав на отмену
- `404 booking_cancel_failed`: бронь не найдена

## 5. Доступность ресурсов

### `GET /availability`

Возвращает список свободных ресурсов в заданный интервал.

Требует авторизацию: `employee`, `admin`

Параметры запроса:

- `start` (required): начало интервала, `RFC3339`
- `end` (required): конец интервала, `RFC3339`
- `type` (optional): `meeting_room` или `workspace`

Пример:

`GET /availability?start=2026-03-02T09:00:00Z&end=2026-03-02T10:00:00Z&type=meeting_room`

Успешный ответ: `200 OK`

```json
{
  "items": [
    {
      "id": 2,
      "name": "Room B",
      "type": "meeting_room",
      "location": "Office 3, Floor 2",
      "capacity": 6,
      "description": "Small room",
      "is_active": true,
      "created_at": "2026-03-01T12:00:00Z",
      "updated_at": "2026-03-01T12:00:00Z"
    }
  ]
}
```

Ошибки:

- `401 unauthorized`: нет токена
- `400 invalid_period`: отсутствует `start/end` или неверный формат
- `400 availability_failed`: невалидный интервал

## 6. Административные эндпоинты

### `GET /admin/bookings`

Возвращает все бронирования в системе.

Требует авторизацию: `admin`

Успешный ответ: `200 OK`

```json
{
  "items": [
    {
      "id": 1,
      "resource_id": 1,
      "user_id": 2,
      "start_time": "2026-03-02T09:00:00Z",
      "end_time": "2026-03-02T10:00:00Z",
      "status": "active",
      "purpose": "Team sync",
      "created_at": "2026-03-01T12:00:00Z"
    }
  ]
}
```

Ошибки:

- `401 unauthorized`: нет токена
- `403 forbidden`: недостаточно прав

### `GET /admin/reports/utilization`

Возвращает отчёт по загрузке ресурсов за указанный период.

Требует авторизацию: `admin`

Параметры запроса:

- `start` (required): начало периода, `RFC3339`
- `end` (required): конец периода, `RFC3339`

Пример:

`GET /admin/reports/utilization?start=2026-03-02T00:00:00Z&end=2026-03-03T00:00:00Z`

Успешный ответ: `200 OK`

```json
{
  "items": [
    {
      "resource_id": 1,
      "resource_name": "Room A",
      "resource_type": "meeting_room",
      "booked_minutes": 60,
      "utilization_percent": 4.1666666667
    }
  ]
}
```

Ошибки:

- `401 unauthorized`: нет токена
- `403 forbidden`: недостаточно прав
- `400 invalid_period`: отсутствует `start/end` или неверный формат
- `400 report_failed`: невалидный интервал

## 7. Пример сценария использования

1. Администратор входит через `POST /auth/login`.
2. Администратор создаёт ресурсы через `POST /resources`.
3. Пользователь регистрируется через `POST /auth/register`.
4. Пользователь получает свой профиль через `GET /me`.
5. Пользователь запрашивает свободные ресурсы через `GET /availability`.
6. Пользователь создаёт бронь через `POST /bookings`.
7. Пользователь просматривает свои брони через `GET /bookings/my`.
8. Администратор контролирует общую загрузку через `GET /admin/bookings` и `GET /admin/reports/utilization`.

## 8. Замечания по текущей реализации

- Текущая версия использует `PostgreSQL`; начальная SQL-миграция выполняется при старте сервера автоматически.
- Токен реализован без внешних библиотек, в формате JWT-подобной строки, совместимой с текущим сервером.
- Формальный контракт для клиента также зафиксирован в `openapi.yaml`.
- Для production-версии рекомендуется заменить самописный JWT-механизм на полноценную библиотеку и добавить refresh-токены.
