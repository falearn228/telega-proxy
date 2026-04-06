package com.falearn.tgfyneproxy.android

import android.app.Application

class ProxyApplication : Application() {
    override fun onCreate() {
        super.onCreate()
        GoProxyBridge.initialize(this)
    }
}
