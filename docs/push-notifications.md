# Push-уведомления (FCM)

Система push-уведомлений реализована через [Firebase Cloud Messaging (FCM)](https://firebase.google.com/docs/cloud-messaging). Бэкенд использует Firebase Admin Go SDK для отправки уведомлений на устройства пользователей.

## Как это работает

```
Flutter App
  ├─ Получает FCM-токен устройства
  └─ POST /me/fcm-token  ──►  Go Backend
                                 ├─ Хранит токен в fcm_tokens (PostgreSQL)
                                 └─ При событии → FCM → устройство
```

Уведомления отправляются асинхронно (горутина) и не блокируют HTTP-ответ. Если Firebase не настроен — приложение работает без уведомлений.

---

## Настройка бэкенда

### 1. Получить service account key

Firebase-проект создаётся и настраивается на стороне Flutter-разработчика. Бэкенду нужен только серверный ключ:

1. Открыть [Firebase Console](https://console.firebase.google.com) → нужный проект
2. **Project Settings** → вкладка **Service Accounts**
3. Нажать **Generate new private key** → скачать JSON-файл
4. Передать файл бэкенд-разработчику (не коммитить в git)

### 2. Задать переменную окружения

```env
APP_FIREBASE_CREDENTIALS=./firebase-service-account.json
```

Добавить в `.env` (для Docker) или передать через окружение. Если переменная не задана — приложение стартует без уведомлений, все остальные функции работают в штатном режиме.

### 3. Добавить файл в .gitignore

```gitignore
firebase-service-account.json
```

---

## API для Flutter-клиента

Все эндпоинты требуют авторизации: `Authorization: Bearer <jwt>`.

### Зарегистрировать FCM-токен

Вызывать после логина и при каждом обновлении токена (`onTokenRefresh`).

```
POST /me/fcm-token
Content-Type: application/json

{
  "token": "<fcm-device-token>"
}
```

**Ответы:**

| Код | Описание |
|-----|----------|
| `200 OK` | Токен сохранён |
| `400 Bad Request` | Отсутствует поле `token` |
| `401 Unauthorized` | Невалидный или отсутствующий JWT |

---

### Удалить FCM-токен

Вызывать при логауте пользователя, чтобы прекратить получение уведомлений.

```
DELETE /me/fcm-token
Content-Type: application/json

{
  "token": "<fcm-device-token>"
}
```

**Ответы:**

| Код | Описание |
|-----|----------|
| `200 OK` | Токен удалён |
| `401 Unauthorized` | Невалидный или отсутствующий JWT |

---

## Сценарии уведомлений

Бэкенд отправляет push автоматически при следующих событиях:

| Событие | Кому | `notification.title` | `notification.body` |
|---------|------|----------------------|---------------------|
| Бронирование создано | Автор брони | `Бронирование создано` | `«Переговорка А» · 12.05 10:00–11:00` |
| Бронирование отменено | Автор брони | `Бронирование отменено` | `«Переговорка А» · 12.05 10:00–11:00` |
| Ресурс деактивирован | Все, у кого есть активные будущие брони на этот ресурс | `Ресурс недоступен` | `«Переговорка А» деактивирован, ваша бронь отменена` |

### Поля `data` в FCM-сообщении

| Ключ | Значение | Пример |
|------|----------|--------|
| `event` | Тип события | `booking_created`, `booking_cancelled`, `resource_disabled` |
| `booking_id` | ID брони (строка) | `"42"` — только для событий бронирования |
| `resource_id` | ID ресурса (строка) | `"7"` — только для `resource_disabled` |

---

## Интеграция Flutter

### Зависимости (`pubspec.yaml`)

```yaml
dependencies:
  firebase_core: ^3.0.0
  firebase_messaging: ^15.0.0
```

### Инициализация (`main.dart`)

```dart
void main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await Firebase.initializeApp(options: DefaultFirebaseOptions.currentPlatform);
  FirebaseMessaging.onBackgroundMessage(_backgroundHandler);
  runApp(const MyApp());
}

@pragma('vm:entry-point')
Future<void> _backgroundHandler(RemoteMessage message) async {
  // обработка уведомления в фоне
}
```

### Запрос разрешений (iOS)

```dart
await FirebaseMessaging.instance.requestPermission(
  alert: true,
  badge: true,
  sound: true,
);
```

### Получение токена и регистрация на бэкенде

Вызывать после успешного логина:

```dart
final token = await FirebaseMessaging.instance.getToken();
if (token != null) {
  await apiClient.post('/me/fcm-token', {'token': token});
}

// Обновление токена при ротации
FirebaseMessaging.instance.onTokenRefresh.listen((newToken) {
  apiClient.post('/me/fcm-token', {'token': newToken});
});
```

### Удаление токена при логауте

```dart
final token = await FirebaseMessaging.instance.getToken();
if (token != null) {
  await apiClient.delete('/me/fcm-token', {'token': token});
}
await FirebaseMessaging.instance.deleteToken();
```

### Обработка входящих уведомлений

```dart
// Foreground (приложение открыто)
FirebaseMessaging.onMessage.listen((RemoteMessage message) {
  final event = message.data['event'];
  // показать локальное уведомление или обновить UI
});

// Background tap (пользователь тапнул по уведомлению)
FirebaseMessaging.onMessageOpenedApp.listen((RemoteMessage message) {
  final bookingId = message.data['booking_id'];
  // навигация к нужному экрану
});
```

### Чеклист

- [ ] `google-services.json` / `GoogleService-Info.plist` от **того же** Firebase-проекта, что и бэкенд
- [ ] Запросить разрешения на уведомления (iOS)
- [ ] После логина — `POST /me/fcm-token`
- [ ] Подписаться на `onTokenRefresh` — переотправлять токен
- [ ] При логауте — `DELETE /me/fcm-token`
- [ ] Обработать `onMessage` (foreground) и `onMessageOpenedApp` (background tap)

---

## Схема базы данных

```sql
CREATE TABLE fcm_tokens (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token      TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (token)
);
```

- Один токен может принадлежать только одному пользователю. При повторной регистрации того же токена (`ON CONFLICT`) `user_id` обновляется — это корректно при смене аккаунта на устройстве.
- При удалении пользователя его токены удаляются каскадно.
- Невалидные токены (ответ FCM `registration-token-not-registered`) автоматически удаляются при следующей отправке.
