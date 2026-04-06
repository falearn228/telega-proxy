# TG Fyne Proxy

Новый Go-проект с локальным MTProto-прокси Telegram. Архитектура взята по мотивам `tg-ws-proxy`, но вместо Python/CTk здесь отдельное Go-ядро, desktop GUI на `Fyne` и production Android-native слой на `Kotlin`.

## Что уже есть

- GUI на `Fyne` для запуска и остановки прокси, редактирования конфига, `tg://proxy` ссылки, QR-кода, import/export конфига и просмотра логов.
- Android-native app с `ForegroundService`, notification channel, boot autostart и прямым управлением тем же Go runtime через `gomobile bind`.
- Локальный MTProto-only прокси на `host:port` из настроек.
- Разбор MTProto obfuscated handshake и определение `DC`/`media`.
- Прямой `TCP` upstream до Telegram DC с корректным re-encrypt bridge.
- Конфигурация в пользовательском каталоге:
  - Linux: `~/.config/TgFyneProxy/config.json`
  - Windows: `%AppData%\\TgFyneProxy\\config.json`
  - Android: каталог, который возвращает `os.UserConfigDir()`

## Архитектура

```text
cmd/tg-fyne-proxy        -> точка входа GUI
internal/ui              -> окно, форма настроек, status/log panels
internal/core            -> controller и orchestration
internal/core/proxy      -> MTProto parsing, crypto, local proxy service
internal/config          -> load/save/defaults/validation
mobilebridge             -> bindable Go API для Android-native слоя
android-app              -> Kotlin app, foreground service, boot receiver
```

Android-specific files:

```text
cmd/tg-fyne-proxy/AndroidManifest.xml -> custom manifest override for fyne mobile packaging
mobilebridge/bridge.go                -> exported bridge for gomobile bind
android-app/...                       -> native Android application
docs/android-foreground-service.md    -> Android-native foreground-service notes
```

## Что отличается от `tg-ws-proxy`

- Новый код полностью на Go.
- GUI на `Fyne`, а не tray-first Python UI.
- Поддерживаются два транспорта: `telegram-wss` и `tcp-direct` fallback.
- При включенном `Connect via Telegram WebSocket` приложение пытается идти в `wss://kws<dc>.web.telegram.org/apiws`, а при неудаче откатывается на прямой TCP.
- Есть базовый preconnected `WS pool` для ускорения подключения следующих клиентских сессий.
- Для проблемных DC добавлены `cooldown` и `blacklist`: если WS постоянно редиректит или падает, приложение временно перестает долбить этот DC по WS и уходит в TCP fallback.
- Для Android/mobile добавлен отдельный layout на `AppTabs`, сохранение активной вкладки и lifecycle hooks через `Fyne`.
- Для production Android добавлен отдельный native app path: `ForegroundService`, `WakeLock`, `BOOT_COMPLETED`, notification actions и Kotlin control UI.

## Запуск

```bash
go run ./cmd/tg-fyne-proxy
```

## Сборка

### Linux / Windows

```bash
go build ./cmd/tg-fyne-proxy
GOOS=windows GOARCH=amd64 go build ./cmd/tg-fyne-proxy
```

Или через packaging helper:

```bash
make linux
make windows
```

### Android

В репозитории теперь есть два Android пути:

1. `Fyne` packaging, если нужен именно existing cross-platform UI.
2. Native Android app, если нужен надежный foreground service.

#### Android через Fyne

Для Android у `Fyne` нужен Android SDK/NDK и утилита `fyne`.

```bash
go install fyne.io/tools/cmd/fyne@latest
make android-apk
```

Для packaging metadata добавлен [`FyneApp.toml`](/home/falearn/projects/tg-fyne-proxy/FyneApp.toml), а типовые команды лежат в [`Makefile`](/home/falearn/projects/tg-fyne-proxy/Makefile).
Кастомный Android manifest лежит в [`AndroidManifest.xml`](/home/falearn/projects/tg-fyne-proxy/cmd/tg-fyne-proxy/AndroidManifest.xml) и будет подхвачен mobile build pipeline `Fyne`.

#### Production Android-native app

Этот путь использует `gomobile bind` и каталог [`android-app`](/home/falearn/projects/tg-fyne-proxy/android-app).

```bash
gomobile bind -target=android -javapkg com.falearn.tgfyneproxy.go \
  -o android-app/app/libs/tgfyneproxy-go.aar ./mobilebridge

cd android-app
gradle assembleDebug
```

То же самое через `Makefile`:

```bash
make android-go-aar
make android-native-debug
make android-native-release
make android-native-bundle
```

Android app включает:

- `ProxyForegroundService` с `startForeground(...)`
- `NotificationChannel` и action `Stop`
- `WakeLock`
- `BOOT_COMPLETED` и `MY_PACKAGE_REPLACED` receiver для `autostart`
- native UI для редактирования конфига и управления прокси
- import/export JSON конфига
- QR для `tg://proxy` ссылки
- export QR в PNG и share flow
- diagnostics по `WS blacklist/cooldown`, pool и последнему upstream mode
- per-DC diagnostics cards в native Android UI

Release signing:

- пример локального файла: [`keystore.properties.example`](/home/falearn/projects/tg-fyne-proxy/android-app/keystore.properties.example)
- также поддерживаются env vars: `ANDROID_SIGNING_STORE_FILE`, `ANDROID_SIGNING_STORE_PASSWORD`, `ANDROID_SIGNING_KEY_ALIAS`, `ANDROID_SIGNING_KEY_PASSWORD`
- release CI scaffold лежит в [`android-native.yml`](/home/falearn/projects/tg-fyne-proxy/.github/workflows/android-native.yml)
  Для CI предусмотрены secrets: `ANDROID_SIGNING_STORE_FILE_BASE64`, `ANDROID_SIGNING_STORE_PASSWORD`, `ANDROID_SIGNING_KEY_ALIAS`, `ANDROID_SIGNING_KEY_PASSWORD`
- release pipeline собирает и `APK`, и `AAB`

## Ограничение Android / Tradeoff

`Fyne` Android build по-прежнему не равен foreground-service варианту. Для надежной фоновой работы нужно собирать native Android app из [`android-app`](/home/falearn/projects/tg-fyne-proxy/android-app), а не только `fyne package -os android`.
Детали лежат в [`android-foreground-service.md`](/home/falearn/projects/tg-fyne-proxy/docs/android-foreground-service.md).

## Следующий этап

1. Добавить richer Android diagnostics: отдельно показать `WS blacklist/cooldown` и текущий upstream mode.
2. Подтянуть export/import и QR в native Android UI.
3. Добавить release signing flow и CI для `gomobile bind + gradle assembleRelease`.
