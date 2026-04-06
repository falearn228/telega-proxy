# Android Foreground Service

В проекте теперь есть production-oriented Android-native слой в [`android-app/`](/home/falearn/projects/tg-fyne-proxy/android-app), а Go runtime для него экспортируется через [`mobilebridge/bridge.go`](/home/falearn/projects/tg-fyne-proxy/mobilebridge/bridge.go).

Схема:

1. `gomobile bind` собирает `AAR` из `./mobilebridge`.
2. Native Android app подтягивает этот `AAR` как локальную зависимость.
3. `ProxyForegroundService` вызывает `startForeground(...)`, держит постоянную notification и partial wake lock.
4. `MainActivity` управляет тем же Go `Controller`, что и service.
5. `BootCompletedReceiver` умеет поднимать сервис после boot/update, если включен `autostart`.

Основные файлы:

- [`android-app/app/src/main/java/com/falearn/tgfyneproxy/android/ProxyForegroundService.kt`](/home/falearn/projects/tg-fyne-proxy/android-app/app/src/main/java/com/falearn/tgfyneproxy/android/ProxyForegroundService.kt)
- [`android-app/app/src/main/java/com/falearn/tgfyneproxy/android/MainActivity.kt`](/home/falearn/projects/tg-fyne-proxy/android-app/app/src/main/java/com/falearn/tgfyneproxy/android/MainActivity.kt)
- [`android-app/app/src/main/java/com/falearn/tgfyneproxy/android/GoProxyBridge.kt`](/home/falearn/projects/tg-fyne-proxy/android-app/app/src/main/java/com/falearn/tgfyneproxy/android/GoProxyBridge.kt)
- [`android-app/app/src/main/AndroidManifest.xml`](/home/falearn/projects/tg-fyne-proxy/android-app/app/src/main/AndroidManifest.xml)

Сборка:

```bash
gomobile bind -target=android -javapkg com.falearn.tgfyneproxy.go \
  -o android-app/app/libs/tgfyneproxy-go.aar ./mobilebridge

cd android-app
gradle assembleDebug
```

Что важно понимать:

- Desktop и существующий `Fyne` GUI остаются в `cmd/tg-fyne-proxy`.
- Production Android path теперь отдельный: native Kotlin UI + foreground service + тот же Go proxy core.
- Это надежнее для background execution, чем попытка удерживать Android через один только `Fyne` activity lifecycle.
