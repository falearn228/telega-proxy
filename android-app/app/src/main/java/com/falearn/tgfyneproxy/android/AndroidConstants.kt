package com.falearn.tgfyneproxy.android

internal object AndroidConstants {
    const val notificationChannelId = "proxy_service"
    const val notificationId = 1001
    const val prefFile = "proxy_prefs"
    const val prefAutostart = "autostart"
    const val bridgeStorageDir = "go-config"
    const val qrCacheDir = "qr"
    const val qrShareFileName = "tg-proxy-qr.png"

    const val actionStart = "com.falearn.tgfyneproxy.android.action.START"
    const val actionStop = "com.falearn.tgfyneproxy.android.action.STOP"
    const val actionRefresh = "com.falearn.tgfyneproxy.android.action.REFRESH"
}
