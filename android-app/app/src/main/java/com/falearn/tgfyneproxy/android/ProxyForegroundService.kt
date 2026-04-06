package com.falearn.tgfyneproxy.android

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import android.os.PowerManager
import androidx.core.app.NotificationCompat
import org.json.JSONObject

class ProxyForegroundService : Service() {
    private val handler = Handler(Looper.getMainLooper())
    private var wakeLock: PowerManager.WakeLock? = null

    private val statusTicker = object : Runnable {
        override fun run() {
            updateNotification()
            handler.postDelayed(this, 5_000)
        }
    }

    override fun onCreate() {
        super.onCreate()
        createNotificationChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        return when (intent?.action ?: AndroidConstants.actionStart) {
            AndroidConstants.actionStart -> {
                startProxy()
                START_STICKY
            }
            AndroidConstants.actionStop -> {
                stopProxy()
                START_NOT_STICKY
            }
            AndroidConstants.actionRefresh -> {
                updateNotification()
                START_STICKY
            }
            else -> START_STICKY
        }
    }

    override fun onDestroy() {
        handler.removeCallbacks(statusTicker)
        releaseWakeLock()
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null

    private fun startProxy() {
        runCatching {
            GoProxyBridge.startProxy(this)
            acquireWakeLock()
            startForeground(AndroidConstants.notificationId, buildNotification(GoProxyBridge.snapshot(this)))
            handler.removeCallbacks(statusTicker)
            handler.post(statusTicker)
        }.onFailure {
            startForeground(AndroidConstants.notificationId, buildErrorNotification(it.message ?: "Unknown error"))
            stopSelf()
        }
    }

    private fun stopProxy() {
        runCatching { GoProxyBridge.stopProxy(this) }
        handler.removeCallbacks(statusTicker)
        releaseWakeLock()
        stopForeground(STOP_FOREGROUND_REMOVE)
        stopSelf()
    }

    private fun updateNotification() {
        val manager = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        val notification = runCatching {
            buildNotification(GoProxyBridge.snapshot(this))
        }.getOrElse {
            buildErrorNotification(it.message ?: "Snapshot failed")
        }
        manager.notify(AndroidConstants.notificationId, notification)
    }

    private fun buildNotification(snapshot: JSONObject): Notification {
        val running = snapshot.optBoolean("running", false)
        val listenAddr = snapshot.optString("listen_addr", "")
        val stats = snapshot.optJSONObject("stats") ?: JSONObject()
        val title = if (running) getString(R.string.notification_running) else getString(R.string.notification_stopped)
        val body = buildString {
            if (listenAddr.isNotBlank()) {
                append(listenAddr)
            }
            val total = stats.optLong("ConnectionsTotal", 0)
            val active = stats.optLong("ConnectionsActive", 0)
            if (total > 0 || active > 0) {
                if (isNotEmpty()) append(" | ")
                append(getString(R.string.notification_stats, active, total))
            }
        }.ifBlank { getString(R.string.notification_idle) }

        val mainIntent = PendingIntent.getActivity(
            this,
            10,
            Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        val stopIntent = PendingIntent.getBroadcast(
            this,
            11,
            Intent(this, ProxyNotificationReceiver::class.java).setAction(AndroidConstants.actionStop),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )

        return NotificationCompat.Builder(this, AndroidConstants.notificationChannelId)
            .setSmallIcon(R.drawable.ic_stat_proxy)
            .setContentTitle(title)
            .setContentText(body)
            .setStyle(NotificationCompat.BigTextStyle().bigText(body))
            .setOngoing(running)
            .setOnlyAlertOnce(true)
            .setContentIntent(mainIntent)
            .addAction(0, getString(R.string.stop_proxy), stopIntent)
            .build()
    }

    private fun buildErrorNotification(message: String): Notification {
        val mainIntent = PendingIntent.getActivity(
            this,
            12,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        return NotificationCompat.Builder(this, AndroidConstants.notificationChannelId)
            .setSmallIcon(R.drawable.ic_stat_proxy)
            .setContentTitle(getString(R.string.notification_error))
            .setContentText(message)
            .setStyle(NotificationCompat.BigTextStyle().bigText(message))
            .setContentIntent(mainIntent)
            .setAutoCancel(true)
            .build()
    }

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) {
            return
        }
        val manager = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        val channel = NotificationChannel(
            AndroidConstants.notificationChannelId,
            getString(R.string.notification_channel_name),
            NotificationManager.IMPORTANCE_LOW,
        ).apply {
            description = getString(R.string.notification_channel_description)
        }
        manager.createNotificationChannel(channel)
    }

    private fun acquireWakeLock() {
        if (wakeLock?.isHeld == true) {
            return
        }
        val powerManager = getSystemService(Context.POWER_SERVICE) as PowerManager
        wakeLock = powerManager.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "tgfyneproxy:proxy").apply {
            setReferenceCounted(false)
            acquire()
        }
    }

    private fun releaseWakeLock() {
        wakeLock?.let {
            if (it.isHeld) {
                it.release()
            }
        }
        wakeLock = null
    }
}
