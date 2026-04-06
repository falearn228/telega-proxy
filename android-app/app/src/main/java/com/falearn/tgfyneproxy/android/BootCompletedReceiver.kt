package com.falearn.tgfyneproxy.android

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent

class BootCompletedReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val prefs = context.getSharedPreferences(AndroidConstants.prefFile, Context.MODE_PRIVATE)
        if (!prefs.getBoolean(AndroidConstants.prefAutostart, false)) {
            return
        }
        ProxyStarter.start(context)
    }
}
