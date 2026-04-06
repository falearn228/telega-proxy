package com.falearn.tgfyneproxy.android

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent

class ProxyNotificationReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        when (intent.action) {
            AndroidConstants.actionStart -> ProxyStarter.start(context)
            AndroidConstants.actionStop -> ProxyStarter.stop(context)
            AndroidConstants.actionRefresh -> ProxyStarter.refresh(context)
        }
    }
}
