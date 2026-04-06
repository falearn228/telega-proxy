package com.falearn.tgfyneproxy.android

import android.content.Context
import com.falearn.tgfyneproxy.go.mobilebridge.Mobilebridge
import org.json.JSONObject
import java.io.File

internal object GoProxyBridge {
    private var initialized = false

    @Synchronized
    fun initialize(context: Context) {
        if (initialized) {
            return
        }
        val dir = File(context.filesDir, AndroidConstants.bridgeStorageDir)
        dir.mkdirs()
        Mobilebridge.configureStorageDir(dir.absolutePath)
        initialized = true
    }

    fun loadConfigJson(context: Context): String {
        initialize(context)
        return Mobilebridge.loadConfigJSON()
    }

    fun saveConfigJson(context: Context, raw: String) {
        initialize(context)
        Mobilebridge.saveConfigJSON(raw)
    }

    fun validateConfigJson(raw: String) {
        Mobilebridge.validateConfigJSON(raw)
    }

    fun startProxy(context: Context) {
        initialize(context)
        Mobilebridge.startProxy()
    }

    fun stopProxy(context: Context) {
        initialize(context)
        Mobilebridge.stopProxy()
    }

    fun isRunning(context: Context): Boolean {
        initialize(context)
        return Mobilebridge.isRunning()
    }

    fun snapshot(context: Context): JSONObject {
        initialize(context)
        return JSONObject(Mobilebridge.snapshotJSON())
    }

    fun generateSecret(): String = Mobilebridge.generateSecret()
}
