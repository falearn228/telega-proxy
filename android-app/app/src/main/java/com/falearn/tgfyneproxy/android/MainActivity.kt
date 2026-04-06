package com.falearn.tgfyneproxy.android

import android.Manifest
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Bitmap
import android.graphics.Color
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.view.ViewGroup
import android.widget.Button
import android.widget.CheckBox
import android.widget.EditText
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.TextView
import androidx.activity.ComponentActivity
import androidx.activity.result.contract.ActivityResultContracts
import androidx.core.content.ContextCompat
import androidx.core.content.FileProvider
import androidx.core.view.setPadding
import com.google.android.material.card.MaterialCardView
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import com.google.android.material.snackbar.Snackbar
import com.google.zxing.BarcodeFormat
import com.google.zxing.qrcode.QRCodeWriter
import org.json.JSONObject
import java.io.File
import java.io.FileOutputStream
import java.io.IOException
import java.util.TreeMap

class MainActivity : ComponentActivity() {
    private data class DcDiagEntry(
        var state: String = "-",
        var until: String = "-",
        var idle: Int = 0,
        var refilling: Boolean = false,
    )

    private lateinit var rootView: android.view.View
    private lateinit var hostInput: EditText
    private lateinit var portInput: EditText
    private lateinit var secretInput: EditText
    private lateinit var dcMapInput: EditText
    private lateinit var bufferInput: EditText
    private lateinit var poolInput: EditText
    private lateinit var timeoutInput: EditText
    private lateinit var statusView: TextView
    private lateinit var linkView: TextView
    private lateinit var statsView: TextView
    private lateinit var diagView: TextView
    private lateinit var dcDiagContainer: LinearLayout
    private lateinit var logsView: TextView
    private lateinit var wsCheck: CheckBox
    private lateinit var verboseCheck: CheckBox
    private lateinit var autostartCheck: CheckBox

    private val handler = Handler(Looper.getMainLooper())
    private val refreshTask = object : Runnable {
        override fun run() {
            refreshSnapshot()
            handler.postDelayed(this, 1_500)
        }
    }

    private val notificationPermissionLauncher = registerForActivityResult(
        ActivityResultContracts.RequestPermission(),
    ) {}

    private val importConfigLauncher = registerForActivityResult(
        ActivityResultContracts.OpenDocument(),
    ) { uri ->
        if (uri == null) {
            return@registerForActivityResult
        }
        runAction {
            contentResolver.openInputStream(uri).use { input ->
                val raw = input?.bufferedReader()?.readText() ?: throw IOException("Empty file")
                GoProxyBridge.saveConfigJson(this, raw)
                loadConfigIntoForm()
                refreshSnapshot()
                showMessage(getString(R.string.imported))
            }
        }
    }

    private val exportConfigLauncher = registerForActivityResult(
        ActivityResultContracts.CreateDocument("application/json"),
    ) { uri ->
        if (uri == null) {
            return@registerForActivityResult
        }
        runAction {
            val raw = buildConfigJson().toString(2)
            contentResolver.openOutputStream(uri, "wt").use { output ->
                output?.writer()?.use { writer ->
                    writer.write(raw)
                } ?: throw IOException("Cannot open export target")
            }
            showMessage(getString(R.string.exported))
        }
    }

    private val exportQrLauncher = registerForActivityResult(
        ActivityResultContracts.CreateDocument("image/png"),
    ) { uri ->
        if (uri == null) {
            return@registerForActivityResult
        }
        runAction {
            val link = linkView.text.toString().trim()
            require(link.isNotBlank()) { getString(R.string.no_link) }
            val bitmap = buildQrBitmap(link)
            contentResolver.openOutputStream(uri, "w").use { output ->
                requireNotNull(output) { "Cannot open export target" }
                check(bitmap.compress(Bitmap.CompressFormat.PNG, 100, output)) { "Failed to encode PNG" }
            }
            showMessage(getString(R.string.qr_exported))
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)
        bindViews()
        requestNotificationPermissionIfNeeded()
        bindActions()
        loadConfigIntoForm()
        refreshSnapshot()
    }

    override fun onStart() {
        super.onStart()
        handler.post(refreshTask)
    }

    override fun onStop() {
        handler.removeCallbacks(refreshTask)
        super.onStop()
    }

    private fun bindViews() {
        rootView = findViewById(R.id.root)
        hostInput = findViewById(R.id.hostInput)
        portInput = findViewById(R.id.portInput)
        secretInput = findViewById(R.id.secretInput)
        dcMapInput = findViewById(R.id.dcMapInput)
        bufferInput = findViewById(R.id.bufferInput)
        poolInput = findViewById(R.id.poolInput)
        timeoutInput = findViewById(R.id.timeoutInput)
        statusView = findViewById(R.id.statusView)
        linkView = findViewById(R.id.linkView)
        statsView = findViewById(R.id.statsView)
        diagView = findViewById(R.id.diagView)
        dcDiagContainer = findViewById(R.id.dcDiagContainer)
        logsView = findViewById(R.id.logsView)
        wsCheck = findViewById(R.id.wsCheck)
        verboseCheck = findViewById(R.id.verboseCheck)
        autostartCheck = findViewById(R.id.autostartCheck)

        val prefs = getSharedPreferences(AndroidConstants.prefFile, Context.MODE_PRIVATE)
        autostartCheck.isChecked = prefs.getBoolean(AndroidConstants.prefAutostart, false)
    }

    private fun bindActions() {
        findViewById<Button>(R.id.saveButton).setOnClickListener {
            runAction {
                GoProxyBridge.saveConfigJson(this, buildConfigJson().toString())
                saveAutostartPreference()
                showMessage(getString(R.string.saved))
                refreshSnapshot()
            }
        }
        findViewById<Button>(R.id.secretButton).setOnClickListener {
            secretInput.setText(GoProxyBridge.generateSecret())
        }
        findViewById<Button>(R.id.startButton).setOnClickListener {
            runAction {
                GoProxyBridge.saveConfigJson(this, buildConfigJson().toString())
                saveAutostartPreference()
                ProxyStarter.start(this)
                refreshSnapshot()
            }
        }
        findViewById<Button>(R.id.stopButton).setOnClickListener {
            runAction {
                ProxyStarter.stop(this)
                refreshSnapshot()
            }
        }
        findViewById<Button>(R.id.copyLinkButton).setOnClickListener {
            copyToClipboard(linkView.text.toString())
        }
        findViewById<Button>(R.id.importButton).setOnClickListener {
            importConfigLauncher.launch(arrayOf("application/json", "text/plain"))
        }
        findViewById<Button>(R.id.exportButton).setOnClickListener {
            exportConfigLauncher.launch("tg-fyne-proxy-config.json")
        }
        findViewById<Button>(R.id.qrButton).setOnClickListener {
            showQrDialog(linkView.text.toString().trim())
        }
        findViewById<Button>(R.id.exportQrButton).setOnClickListener {
            exportQrLauncher.launch("tg-fyne-proxy-qr.png")
        }
        findViewById<Button>(R.id.shareLinkButton).setOnClickListener {
            shareLink(linkView.text.toString().trim())
        }
        findViewById<Button>(R.id.exportQrButton).setOnLongClickListener {
            shareQrPng(linkView.text.toString().trim())
            true
        }
        findViewById<Button>(R.id.openTelegramButton).setOnClickListener {
            val link = linkView.text.toString().trim()
            if (link.isEmpty()) {
                showMessage(getString(R.string.no_link))
                return@setOnClickListener
            }
            startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(link)))
        }
    }

    private fun loadConfigIntoForm() {
        runAction {
            val json = JSONObject(GoProxyBridge.loadConfigJson(this))
            hostInput.setText(json.optString("host", "127.0.0.1"))
            portInput.setText(json.optInt("port", 1443).toString())
            secretInput.setText(json.optString("secret"))
            dcMapInput.setText(formatDcMap(json.optJSONObject("dc_map") ?: JSONObject()))
            bufferInput.setText(json.optInt("buf_kb", 256).toString())
            poolInput.setText(json.optInt("pool_size", 4).toString())
            timeoutInput.setText(json.optInt("connect_timeout_sec", 10).toString())
            wsCheck.isChecked = json.optBoolean("connect_via_ws", false)
            verboseCheck.isChecked = json.optBoolean("verbose", false)
        }
    }

    private fun buildConfigJson(): JSONObject {
        val config = JSONObject()
        config.put("host", hostInput.text.toString().trim())
        config.put("port", portInput.text.toString().trim().toInt())
        config.put("secret", secretInput.text.toString().trim())
        config.put("dc_map", parseDcMap(dcMapInput.text.toString()))
        config.put("verbose", verboseCheck.isChecked)
        config.put("buf_kb", bufferInput.text.toString().trim().toInt())
        config.put("pool_size", poolInput.text.toString().trim().toInt())
        config.put("connect_via_ws", wsCheck.isChecked)
        config.put("prefer_ipv6", false)
        config.put("connect_timeout_sec", timeoutInput.text.toString().trim().toInt())
        GoProxyBridge.validateConfigJson(config.toString())
        return config
    }

    private fun refreshSnapshot() {
        runAction {
            val snapshot = GoProxyBridge.snapshot(this)
            val running = snapshot.optBoolean("running", false)
            val listenAddr = snapshot.optString("listen_addr")
            val lastError = snapshot.optString("last_error")
            val status = buildString {
                append(if (running) getString(R.string.status_running) else getString(R.string.status_stopped))
                if (listenAddr.isNotBlank()) {
                    append(": ")
                    append(listenAddr)
                }
                if (lastError.isNotBlank()) {
                    append("\n")
                    append(getString(R.string.last_error, lastError))
                }
            }
            statusView.text = status
            linkView.text = snapshot.optString("tg_link")
            statsView.text = formatStats(snapshot.optJSONObject("stats") ?: JSONObject())
            diagView.text = formatDiagnostics(snapshot.optJSONObject("diag") ?: JSONObject())
            renderPerDcDiagnostics(snapshot.optJSONObject("diag") ?: JSONObject())
            logsView.text = snapshot.optString("logs")
        }
    }

    private fun formatStats(stats: JSONObject): String = getString(
        R.string.stats_template,
        stats.optLong("ConnectionsTotal", 0),
        stats.optLong("ConnectionsActive", 0),
        stats.optLong("ConnectionsErrored", 0),
        stats.optLong("ConnectionsWS", 0),
        stats.optLong("ConnectionsTCP", 0),
        stats.optLong("WSErrors", 0),
        stats.optLong("BytesUp", 0),
        stats.optLong("BytesDown", 0),
    )

    private fun formatDcMap(obj: JSONObject): String {
        val keys = obj.keys().asSequence().toList().sortedBy { it.toIntOrNull() ?: 0 }
        return keys.joinToString("\n") { key -> "$key:${obj.optString(key)}" }
    }

    private fun parseDcMap(raw: String): JSONObject {
        val obj = JSONObject()
        raw.lineSequence()
            .map { it.trim() }
            .filter { it.isNotEmpty() }
            .forEach { line ->
                val idx = line.indexOf(':')
                require(idx > 0) { getString(R.string.invalid_dc_map_line, line) }
                val dc = line.substring(0, idx).trim().toInt()
                val value = line.substring(idx + 1).trim()
                obj.put(dc.toString(), value)
            }
        return obj
    }

    private fun formatDiagnostics(diag: JSONObject): String {
        val parts = mutableListOf<String>()
        parts += getString(R.string.diag_last_mode, diag.optString("last_upstream_mode", getString(R.string.diag_none)))

        val wsState = diag.optJSONArray("ws_state")
        parts += getString(R.string.diag_ws_state)
        if (wsState == null || wsState.length() == 0) {
            parts += "  ${getString(R.string.diag_none)}"
        } else {
            for (i in 0 until wsState.length()) {
                val item = wsState.optJSONObject(i) ?: continue
                parts += "  " + getString(
                    R.string.diag_ws_state_item,
                    item.optInt("dc", 0),
                    item.optBoolean("is_media", false).toString(),
                    item.optString("state", getString(R.string.diag_none)),
                    item.optString("cooldown_until", "-"),
                )
            }
        }

        val wsPool = diag.optJSONArray("ws_pool")
        parts += getString(R.string.diag_ws_pool)
        if (wsPool == null || wsPool.length() == 0) {
            parts += "  ${getString(R.string.diag_none)}"
        } else {
            for (i in 0 until wsPool.length()) {
                val item = wsPool.optJSONObject(i) ?: continue
                parts += "  " + getString(
                    R.string.diag_ws_pool_item,
                    item.optInt("dc", 0),
                    item.optBoolean("is_media", false).toString(),
                    item.optInt("idle", 0),
                    item.optBoolean("refilling", false).toString(),
                )
            }
        }
        return parts.joinToString("\n")
    }

    private fun renderPerDcDiagnostics(diag: JSONObject) {
        dcDiagContainer.removeAllViews()
        val grouped = TreeMap<String, DcDiagEntry>()
        val wsState = diag.optJSONArray("ws_state")
        for (i in 0 until (wsState?.length() ?: 0)) {
            val item = wsState?.optJSONObject(i) ?: continue
            val key = dcKey(item.optInt("dc", 0), item.optBoolean("is_media", false))
            val entry = grouped.getOrPut(key) { DcDiagEntry() }
            entry.state = item.optString("state", "-")
            entry.until = item.optString("cooldown_until", "-")
        }
        val wsPool = diag.optJSONArray("ws_pool")
        for (i in 0 until (wsPool?.length() ?: 0)) {
            val item = wsPool?.optJSONObject(i) ?: continue
            val key = dcKey(item.optInt("dc", 0), item.optBoolean("is_media", false))
            val entry = grouped.getOrPut(key) { DcDiagEntry() }
            entry.idle = item.optInt("idle", 0)
            entry.refilling = item.optBoolean("refilling", false)
        }
        if (grouped.isEmpty()) {
            dcDiagContainer.addView(TextView(this).apply { text = getString(R.string.diag_none) })
            return
        }

        grouped.entries.forEach { (key, entry) ->
            val parts = key.split("|")
            dcDiagContainer.addView(buildDcCard(parts[0].toInt(), parts[1], entry))
        }
    }

    private fun buildDcCard(dc: Int, media: String, entry: DcDiagEntry): MaterialCardView {
        val density = resources.displayMetrics.density
        val container = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding((16 * density).toInt())
        }

        val title = TextView(this).apply {
            text = getString(R.string.diag_dc_group, dc, media)
            textSize = 16f
        }
        container.addView(title)
        container.addView(TextView(this).apply {
            text = getString(R.string.diag_card_state, entry.state)
        })
        container.addView(TextView(this).apply {
            text = getString(R.string.diag_card_until, entry.until)
        })
        container.addView(TextView(this).apply {
            text = getString(R.string.diag_card_pool, entry.idle, entry.refilling.toString())
        })

        return MaterialCardView(this).apply {
            radius = 16f * density
            cardElevation = 2f * density
            useCompatPadding = true
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.WRAP_CONTENT,
            ).apply {
                bottomMargin = (10 * density).toInt()
            }
            addView(container)
        }
    }

    private fun dcKey(dc: Int, isMedia: Boolean): String = "$dc|${isMedia}"

    private fun saveAutostartPreference() {
        getSharedPreferences(AndroidConstants.prefFile, Context.MODE_PRIVATE)
            .edit()
            .putBoolean(AndroidConstants.prefAutostart, autostartCheck.isChecked)
            .apply()
    }

    private fun copyToClipboard(text: String) {
        val clipboard = getSystemService(CLIPBOARD_SERVICE) as android.content.ClipboardManager
        clipboard.setPrimaryClip(android.content.ClipData.newPlainText("tg://proxy", text))
        showMessage(getString(R.string.copied))
    }

    private fun requestNotificationPermissionIfNeeded() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            if (ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
                notificationPermissionLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
            }
        }
    }

    private fun showQrDialog(link: String) {
        if (link.isBlank()) {
            showMessage(getString(R.string.no_link))
            return
        }
        runAction {
            val bitmap = buildQrBitmap(link)
            val content = LinearLayout(this).apply {
                orientation = LinearLayout.VERTICAL
                setPadding((16 * resources.displayMetrics.density).toInt())
            }
            val hint = TextView(this).apply {
                text = getString(R.string.qr_hint)
            }
            val image = ImageView(this).apply {
                setImageBitmap(bitmap)
                adjustViewBounds = true
                layoutParams = LinearLayout.LayoutParams(
                    ViewGroup.LayoutParams.MATCH_PARENT,
                    (280 * resources.displayMetrics.density).toInt(),
                )
            }
            val text = TextView(this).apply {
                setTextIsSelectable(true)
                this.text = link
            }
            content.addView(hint)
            content.addView(image)
            content.addView(text)
            MaterialAlertDialogBuilder(this)
                .setTitle(R.string.qr_title)
                .setView(content)
                .setPositiveButton(R.string.close, null)
                .show()
        }
    }

    private fun buildQrBitmap(link: String): Bitmap {
        val size = 768
        val matrix = QRCodeWriter().encode(link, BarcodeFormat.QR_CODE, size, size)
        val bitmap = Bitmap.createBitmap(size, size, Bitmap.Config.ARGB_8888)
        for (x in 0 until size) {
            for (y in 0 until size) {
                bitmap.setPixel(x, y, if (matrix[x, y]) Color.BLACK else Color.WHITE)
            }
        }
        return bitmap
    }

    private fun shareLink(link: String) {
        if (link.isBlank()) {
            showMessage(getString(R.string.no_link))
            return
        }
        val intent = Intent(Intent.ACTION_SEND).apply {
            type = "text/plain"
            putExtra(Intent.EXTRA_TEXT, link)
        }
        startActivity(Intent.createChooser(intent, getString(R.string.share_chooser)))
    }

    private fun shareQrPng(link: String) {
        if (link.isBlank()) {
            showMessage(getString(R.string.no_link))
            return
        }
        runAction {
            val bitmap = buildQrBitmap(link)
            val shareFile = File(File(cacheDir, AndroidConstants.qrCacheDir).apply { mkdirs() }, AndroidConstants.qrShareFileName)
            FileOutputStream(shareFile).use { output ->
                check(bitmap.compress(Bitmap.CompressFormat.PNG, 100, output)) { "Failed to encode PNG" }
            }
            val uri = FileProvider.getUriForFile(this, "${BuildConfig.APPLICATION_ID}.fileprovider", shareFile)
            val intent = Intent(Intent.ACTION_SEND).apply {
                type = "image/png"
                putExtra(Intent.EXTRA_STREAM, uri)
                putExtra(Intent.EXTRA_TEXT, link)
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            }
            startActivity(Intent.createChooser(intent, getString(R.string.share_qr_chooser)))
        }
    }

    private fun runAction(block: () -> Unit) {
        runCatching(block).onFailure {
            showMessage(it.message ?: it.javaClass.simpleName)
        }
    }

    private fun showMessage(message: String) {
        Snackbar.make(rootView, message, Snackbar.LENGTH_LONG).show()
    }
}
