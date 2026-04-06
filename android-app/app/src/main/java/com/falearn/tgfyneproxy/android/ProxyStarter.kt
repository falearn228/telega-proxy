package com.falearn.tgfyneproxy.android

import android.content.Context
import android.content.Intent
import androidx.core.content.ContextCompat

internal object ProxyStarter {
    fun start(context: Context) {
        val intent = Intent(context, ProxyForegroundService::class.java).setAction(AndroidConstants.actionStart)
        ContextCompat.startForegroundService(context, intent)
    }

    fun stop(context: Context) {
        val intent = Intent(context, ProxyForegroundService::class.java).setAction(AndroidConstants.actionStop)
        context.startService(intent)
    }

    fun refresh(context: Context) {
        val intent = Intent(context, ProxyForegroundService::class.java).setAction(AndroidConstants.actionRefresh)
        context.startService(intent)
    }
}
