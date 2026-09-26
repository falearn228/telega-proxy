# TG Fyne Proxy

Новый Go-проект с локальным MTProto-прокси Telegram. Архитектура взята по мотивам `tg-ws-proxy`, но вместо Python/CTk здесь отдельное Go-ядро, desktop GUI на `Fyne` и production Android-native слой на `Kotlin`.

## Что уже есть

- GUI на `Fyne` для запуска и остановки прокси, редактирования конфига, `tg://proxy` ссылки, QR-кода, import/export конфига и просмотра логов.
- Android-native app с `ForegroundService`, notification channel, boot autostart и прямым управлением тем же Go runtime через `gomobile bind`.
- Локальный MTProto-only прокси на `host:port` из настроек.
- Разбор MTProto obfuscated handshake и определение `DC`/`media`.
- Прямой `TCP` upstream до Telegram DC с корректным re-encrypt bridge.
- Конфигурация хранится локально:
  - Linux: `~/.config/TgFyneProxy/config.json`
  - Windows: `%AppData%\\TgFyneProxy\\config.json`
  - Android через `Fyne`: каталог, который возвращает `os.UserConfigDir()`
  - Android-native app: private app storage в `filesDir/tg-fyne-proxy`

## Архитектура

```text
cmd/tg-fyne-proxy        -> точка входа GUI
cmd/tg-proxy-service     -> консольная точка входа без CGO
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

### Консольный сервис без CGO

```bash
make run-service
# или собрать отдельный бинарник:
make build-service
./bin/tg-proxy-service
```

Сервис использует `~/.config/TgFyneProxy/config.json`, общий с desktop GUI. При первом запуске создаёт конфиг со случайным секретом и правами `0600`; параметры и секрет можно взять из этого файла. Слушает `host:port` из конфига (по умолчанию `127.0.0.1:1443`) и останавливается по `Ctrl+C` или `SIGTERM`. Для запуска не нужны Fyne, Wayland, X11 и CGO.

Для исходящих подключений к Telegram сервис использует `HTTPS_PROXY` (или `https_proxy`) и учитывает `NO_PROXY`. Поддерживается HTTP-прокси с CONNECT, в том числе URL с Basic-авторизацией. Если в конфиге включён `use_relay`, адрес `relay_addr` имеет приоритет над переменными окружения. `ALL_PROXY` и SOCKS-прокси этим путём не поддерживаются. Входящий локальный порт через прокси окружения не маршрутизируется.

### Фоновый запуск на Linux

Закрой запущенный в терминале экземпляр, затем выполни:

```bash
make install-user-service
```

Команда собирает бинарник без CGO, устанавливает его в `~/.local/bin`, включает и запускает пользовательский `systemd` сервис. Если в текущем окружении задан `HTTPS_PROXY`, он вместе с `NO_PROXY` сохраняется в `~/.config/tg-proxy-service/proxy.env` с правами `0600`; существующий файл при повторной установке не перезаписывается. После изменения конфига или proxy.env перезапусти сервис: `systemctl --user restart tg-proxy-service`.

```bash
make status-user-service # состояние
make logs-user-service   # логи
make stop-user-service   # остановка
```

Сервис запускается при входе пользователя в систему. Для работы после выхода из пользовательской сессии отдельно включи lingering через `loginctl enable-linger "$USER"`.

## Сборка

### Linux / Windows

```bash
go build ./cmd/tg-fyne-proxy
GOOS=windows GOARCH=amd64 go build ./cmd/tg-fyne-proxy
```

Windows GUI package через `Fyne` из Linux/WSL требует `fyne` CLI, `Icon.png` в `cmd/tg-fyne-proxy` и MinGW cross-compiler:

```bash
go install fyne.io/tools/cmd/fyne@latest
sudo apt-get install -y gcc-mingw-w64-x86-64
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc \
  fyne package -os windows --src ./cmd/tg-fyne-proxy --name tg-fyne-proxy
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
make android-aab
```

Для packaging metadata добавлен [`FyneApp.toml`](/home/falearn/projects/tg-fyne-proxy/FyneApp.toml), а типовые команды лежат в [`Makefile`](/home/falearn/projects/tg-fyne-proxy/Makefile).
Кастомный Android manifest лежит в [`AndroidManifest.xml`](/home/falearn/projects/tg-fyne-proxy/cmd/tg-fyne-proxy/AndroidManifest.xml) и будет подхвачен mobile build pipeline `Fyne`.

#### Production Android-native app

Этот путь использует `gomobile bind` и каталог [`android-app`](/home/falearn/projects/tg-fyne-proxy/android-app).

```bash
gomobile bind -target=android -androidapi=26 -javapkg com.falearn.tgfyneproxy.go \
  -o android-app/app/libs/tgfyneproxy-go.aar ./mobilebridge

cd android-app
./gradlew assembleDebug
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
- runtime permission flow для `POST_NOTIFICATIONS`
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

## Что важно понимать по Android

`Fyne`-сборка и native Android app используют один и тот же Go proxy core, но это два разных runtime path.

- `fyne package -os android` удобно для быстрого mobile build существующего UI, но он не заменяет отдельный foreground service.
- Каталог [`android-app`](/home/falearn/projects/tg-fyne-proxy/android-app) нужен для сценария, где важна более надежная фоновая работа, автозапуск после boot/update и постоянная notification.
- Native Android path хранит конфиг в app-private storage через bridge, поэтому его storage semantics отличаются от desktop/`Fyne` path.
