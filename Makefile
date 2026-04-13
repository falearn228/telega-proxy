APP_NAME=tg-fyne-proxy
MAIN=./cmd/tg-fyne-proxy
ANDROID_JAVA_PKG=com.falearn.tgfyneproxy.go
ANDROID_GO_AAR=./android-app/app/libs/tgfyneproxy-go.aar
ANDROID_API=26

.PHONY: build run tidy fmt linux windows android-apk android-aab android-go-aar android-native-debug android-native-release android-native-bundle

build:
	go build $(MAIN)

run:
	go run $(MAIN)

tidy:
	go mod tidy

fmt:
	go fmt ./...

linux:
	fyne package -os linux

windows:
	fyne package -os windows

android-apk:
	fyne package -os android

android-aab:
	fyne package -os android -release

android-go-aar:
	gomobile bind -target=android -androidapi=$(ANDROID_API) -javapkg $(ANDROID_JAVA_PKG) -o $(ANDROID_GO_AAR) ./mobilebridge

android-native-debug: android-go-aar
	cd android-app && gradle assembleDebug

android-native-release: android-go-aar
	cd android-app && gradle assembleRelease

android-native-bundle: android-go-aar
	cd android-app && gradle bundleRelease
