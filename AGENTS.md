# Repository Guidelines

## Project Structure & Module Organization

This is a Go project for a local Telegram MTProto proxy with desktop and Android front ends.

- `cmd/tg-fyne-proxy/`: desktop Fyne application entry point and app icon.
- `internal/config/`: config loading, validation, import/export, Telegram link generation.
- `internal/core/`: controller layer that owns runtime config and proxy lifecycle.
- `internal/core/proxy/`: MTProto parsing, relay support, WebSocket transport, TCP bridge, and proxy service tests.
- `internal/platform/`: OS-specific helpers such as Windows autostart.
- `internal/ui/`: Fyne desktop/mobile UI.
- `mobilebridge/`: exported Go bridge used by `gomobile bind`.
- `android-app/`: native Android Kotlin app, Gradle config, resources, and foreground service.
- `docs/`: design and Android foreground-service notes.

## Build, Test, and Development Commands

- `make run`: run the desktop app locally with `go run ./cmd/tg-fyne-proxy`.
- `make build`: build the desktop Go binary.
- `make fmt`: run `go fmt ./...`.
- `go test ./...`: run all Go tests. On Linux CI this may require Fyne desktop CGO dependencies.
- `make android-go-aar`: build `android-app/app/libs/tgfyneproxy-go.aar` using `gomobile bind -androidapi=26`.
- `make android-native-debug`: build the Go AAR, then run Gradle `assembleDebug`.
- `make windows`: package the Fyne Windows build. Requires Fyne CLI and a working cross-compiler when run from Linux.

## Coding Style & Naming Conventions

Use standard Go formatting with `gofmt`. Keep package names short and lowercase. Prefer clear exported names for public bridge APIs, for example `StartProxy`, and unexported helpers for package-local behavior. Keep Android Kotlin code in package `com.falearn.tgfyneproxy.android`; generated Go bindings use `com.falearn.tgfyneproxy.go`.

## Testing Guidelines

Go tests use the standard `testing` package and live next to implementation files as `*_test.go`. Name tests by behavior, for example `TestDialUpstreamUsesHTTPConnectRelay`. Add focused tests for config validation, protocol parsing, relay behavior, and platform helpers. For Android-native CI, prefer testing non-desktop packages to avoid requiring GL/X11 headers.

## Commit & Pull Request Guidelines

Recent commits use short messages such as `#notask: fix autostart`. Keep commits concise and action-oriented. Pull requests should include a short problem statement, implementation summary, test results, and platform notes when behavior differs on Windows, Linux, or Android. Include screenshots or screen recordings for UI changes and mention any required SDK, NDK, signing, or `gomobile` setup changes.

## Security & Configuration Tips

Do not commit real signing secrets or private keystores. Use GitHub secrets for Android release signing. Local proxy secrets live in config JSON; avoid pasting production secrets into issues or logs.
