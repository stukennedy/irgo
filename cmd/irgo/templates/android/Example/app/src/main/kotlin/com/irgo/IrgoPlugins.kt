package com.irgo

import android.Manifest
import android.app.Activity
import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.VibrationEffect
import android.os.Vibrator
import android.os.VibratorManager
import android.widget.Toast
import androidx.core.app.ActivityCompat
import androidx.core.app.NotificationCompat
import androidx.core.content.ContextCompat
import org.json.JSONObject

/**
 * Built-in native capability plugins, registered by [IrgoActivity] at
 * startup. All are reachable from JavaScript via
 * `irgo.native('namespace.method', {...})` and from Go via native.Call.
 *
 * Methods implemented:
 *  - device.info
 *  - haptics.impact / haptics.notification / haptics.selection
 *  - clipboard.write / clipboard.read
 *  - share.text
 *  - browser.open
 *  - toast.show
 *  - storage.get / storage.set / storage.remove
 *  - notifications.requestPermission / notifications.show
 */
object IrgoPlugins {

    private val registered = mutableListOf<IrgoPlugin>()
    private var notifications: NotificationsPlugin? = null

    /**
     * Register the built-in plugins for [activity]. Safe to call again on
     * activity re-creation: previously registered built-ins are replaced.
     */
    fun registerBuiltins(activity: Activity) {
        registered.forEach { IrgoNative.unregister(it) }
        registered.clear()
        notifications = null

        add(DeviceInfoPlugin(activity))
        add(HapticsPlugin(activity))
        add(ClipboardPlugin(activity))
        add(SharePlugin(activity))
        add(BrowserPlugin(activity))
        add(ToastPlugin(activity))
        add(StoragePlugin(activity))
        notifications = NotificationsPlugin(activity).also { add(it) }
    }

    /**
     * Forward permission results from the activity. Returns true if the
     * request was handled by a built-in plugin.
     */
    fun onRequestPermissionsResult(requestCode: Int, grantResults: IntArray): Boolean {
        return notifications?.onRequestPermissionsResult(requestCode, grantResults) ?: false
    }

    private fun add(plugin: IrgoPlugin) {
        IrgoNative.register(plugin)
        registered.add(plugin)
    }
}

/** device.info -> {platform, model, osVersion, appVersion, bundleId} */
class DeviceInfoPlugin(private val context: Context) : IrgoPlugin {
    override val namespace = "device"

    override fun handle(method: String, params: JSONObject, completion: (Result<Any?>) -> Unit): Boolean {
        if (method != "device.info") return false
        val appVersion = try {
            @Suppress("DEPRECATION")
            context.packageManager.getPackageInfo(context.packageName, 0).versionName ?: ""
        } catch (e: Exception) {
            ""
        }
        completion(
            Result.success(
                JSONObject()
                    .put("platform", "android")
                    .put("model", Build.MODEL ?: "")
                    .put("osVersion", Build.VERSION.RELEASE ?: "")
                    .put("appVersion", appVersion)
                    .put("bundleId", context.packageName)
            )
        )
        return true
    }
}

/**
 * haptics.impact {style: light|medium|heavy}
 * haptics.notification {type: success|warning|error}
 * haptics.selection
 *
 * Requires android.permission.VIBRATE.
 */
class HapticsPlugin(private val context: Context) : IrgoPlugin {
    override val namespace = "haptics"

    override fun handle(method: String, params: JSONObject, completion: (Result<Any?>) -> Unit): Boolean {
        when (method) {
            "haptics.impact" -> {
                val duration = when (params.optString("style", "medium")) {
                    "light" -> 10L
                    "heavy" -> 40L
                    else -> 20L
                }
                vibrate(longArrayOf(0, duration))
            }
            "haptics.notification" -> {
                val pattern = when (params.optString("type", "success")) {
                    "warning" -> longArrayOf(0, 30, 60, 30)
                    "error" -> longArrayOf(0, 40, 60, 40, 60, 40)
                    else -> longArrayOf(0, 20, 60, 20)
                }
                vibrate(pattern)
            }
            "haptics.selection" -> vibrate(longArrayOf(0, 5))
            else -> return false
        }
        completion(Result.success(null))
        return true
    }

    private fun vibrator(): Vibrator? {
        return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            val manager = context.getSystemService(Context.VIBRATOR_MANAGER_SERVICE) as? VibratorManager
            manager?.defaultVibrator
        } else {
            @Suppress("DEPRECATION")
            context.getSystemService(Context.VIBRATOR_SERVICE) as? Vibrator
        }
    }

    private fun vibrate(pattern: LongArray) {
        val vibrator = vibrator() ?: return
        try {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                vibrator.vibrate(VibrationEffect.createWaveform(pattern, -1))
            } else {
                @Suppress("DEPRECATION")
                vibrator.vibrate(pattern, -1)
            }
        } catch (e: Exception) {
            // No vibrator or permission missing - haptics are best-effort.
        }
    }
}

/** clipboard.write {text} / clipboard.read -> {text} */
class ClipboardPlugin(private val context: Context) : IrgoPlugin {
    override val namespace = "clipboard"

    override fun handle(method: String, params: JSONObject, completion: (Result<Any?>) -> Unit): Boolean {
        val manager = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        when (method) {
            "clipboard.write" -> {
                manager.setPrimaryClip(ClipData.newPlainText("irgo", params.optString("text", "")))
                completion(Result.success(null))
            }
            "clipboard.read" -> {
                val clip = manager.primaryClip
                val text = if (clip != null && clip.itemCount > 0) {
                    clip.getItemAt(0).coerceToText(context)?.toString() ?: ""
                } else {
                    ""
                }
                completion(Result.success(JSONObject().put("text", text)))
            }
            else -> return false
        }
        return true
    }
}

/** share.text {text, title?} -> system share sheet */
class SharePlugin(private val activity: Activity) : IrgoPlugin {
    override val namespace = "share"

    override fun handle(method: String, params: JSONObject, completion: (Result<Any?>) -> Unit): Boolean {
        if (method != "share.text") return false
        val text = params.optString("text", "")
        val title = params.optString("title", "")
        try {
            val send = Intent(Intent.ACTION_SEND).apply {
                type = "text/plain"
                putExtra(Intent.EXTRA_TEXT, text)
                if (title.isNotEmpty()) putExtra(Intent.EXTRA_SUBJECT, title)
            }
            activity.startActivity(Intent.createChooser(send, if (title.isEmpty()) null else title))
            completion(Result.success(null))
        } catch (e: Exception) {
            completion(Result.failure(e))
        }
        return true
    }
}

/** browser.open {url} -> external browser (http/https only) */
class BrowserPlugin(private val activity: Activity) : IrgoPlugin {
    override val namespace = "browser"

    override fun handle(method: String, params: JSONObject, completion: (Result<Any?>) -> Unit): Boolean {
        if (method != "browser.open") return false
        val url = params.optString("url", "")
        val uri = Uri.parse(url)
        val scheme = uri.scheme?.lowercase()
        if (scheme != "http" && scheme != "https") {
            completion(Result.failure(IllegalArgumentException("browser.open: only http(s) URLs are allowed")))
            return true
        }
        try {
            activity.startActivity(Intent(Intent.ACTION_VIEW, uri))
            completion(Result.success(null))
        } catch (e: Exception) {
            completion(Result.failure(e))
        }
        return true
    }
}

/** toast.show {text, long?} */
class ToastPlugin(private val context: Context) : IrgoPlugin {
    override val namespace = "toast"

    override fun handle(method: String, params: JSONObject, completion: (Result<Any?>) -> Unit): Boolean {
        if (method != "toast.show") return false
        val duration = if (params.optBoolean("long", false)) Toast.LENGTH_LONG else Toast.LENGTH_SHORT
        Toast.makeText(context, params.optString("text", ""), duration).show()
        completion(Result.success(null))
        return true
    }
}

/**
 * storage.get {key} -> {value: string|null}
 * storage.set {key, value}
 * storage.remove {key}
 *
 * Values are stored as strings in the "irgo.storage" SharedPreferences.
 */
class StoragePlugin(context: Context) : IrgoPlugin {
    override val namespace = "storage"

    private val prefs = context.getSharedPreferences("irgo.storage", Context.MODE_PRIVATE)

    override fun handle(method: String, params: JSONObject, completion: (Result<Any?>) -> Unit): Boolean {
        val key = params.optString("key", "")
        when (method) {
            "storage.get" -> {
                if (key.isEmpty()) {
                    completion(Result.failure(IllegalArgumentException("storage.get: key required")))
                    return true
                }
                val value = prefs.getString(key, null)
                completion(Result.success(JSONObject().put("value", value ?: JSONObject.NULL)))
            }
            "storage.set" -> {
                if (key.isEmpty()) {
                    completion(Result.failure(IllegalArgumentException("storage.set: key required")))
                    return true
                }
                val raw = params.opt("value")
                if (raw == null || raw == JSONObject.NULL) {
                    prefs.edit().remove(key).apply()
                } else {
                    prefs.edit().putString(key, raw.toString()).apply()
                }
                completion(Result.success(null))
            }
            "storage.remove" -> {
                if (key.isEmpty()) {
                    completion(Result.failure(IllegalArgumentException("storage.remove: key required")))
                    return true
                }
                prefs.edit().remove(key).apply()
                completion(Result.success(null))
            }
            else -> return false
        }
        return true
    }
}

/**
 * notifications.requestPermission -> {granted}
 * notifications.show {title, body, id?} -> {id}
 *
 * Uses the "irgo" notification channel (created on API 26+). On API 33+ the
 * POST_NOTIFICATIONS runtime permission is requested; the activity must
 * forward onRequestPermissionsResult to [IrgoPlugins.onRequestPermissionsResult]
 * (IrgoActivity does this automatically).
 */
class NotificationsPlugin(private val activity: Activity) : IrgoPlugin {
    override val namespace = "notifications"

    companion object {
        const val CHANNEL_ID = "irgo"
        const val PERMISSION_REQUEST_CODE = 0x1290
    }

    private var pendingPermission: ((Result<Any?>) -> Unit)? = null

    override fun handle(method: String, params: JSONObject, completion: (Result<Any?>) -> Unit): Boolean {
        when (method) {
            "notifications.requestPermission" -> requestPermission(completion)
            "notifications.show" -> show(params, completion)
            else -> return false
        }
        return true
    }

    private fun isGranted(): Boolean {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) return true
        return ContextCompat.checkSelfPermission(
            activity,
            Manifest.permission.POST_NOTIFICATIONS
        ) == PackageManager.PERMISSION_GRANTED
    }

    private fun requestPermission(completion: (Result<Any?>) -> Unit) {
        if (isGranted()) {
            completion(Result.success(JSONObject().put("granted", true)))
            return
        }
        // Only one request can be in flight; resolve a superseded one as denied.
        pendingPermission?.invoke(Result.success(JSONObject().put("granted", false)))
        pendingPermission = completion
        ActivityCompat.requestPermissions(
            activity,
            arrayOf(Manifest.permission.POST_NOTIFICATIONS),
            PERMISSION_REQUEST_CODE
        )
    }

    /** Called from the activity's onRequestPermissionsResult. */
    fun onRequestPermissionsResult(requestCode: Int, grantResults: IntArray): Boolean {
        if (requestCode != PERMISSION_REQUEST_CODE) return false
        val granted = grantResults.isNotEmpty() && grantResults[0] == PackageManager.PERMISSION_GRANTED
        pendingPermission?.invoke(Result.success(JSONObject().put("granted", granted)))
        pendingPermission = null
        return true
    }

    private fun show(params: JSONObject, completion: (Result<Any?>) -> Unit) {
        if (!isGranted()) {
            completion(Result.failure(IllegalStateException("notifications: permission not granted")))
            return
        }
        try {
            val manager = activity.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                manager.createNotificationChannel(
                    NotificationChannel(CHANNEL_ID, "Notifications", NotificationManager.IMPORTANCE_DEFAULT)
                )
            }
            val id = params.optInt("id", (System.currentTimeMillis() and 0x7FFFFFFFL).toInt())
            val notification = NotificationCompat.Builder(activity, CHANNEL_ID)
                .setSmallIcon(android.R.drawable.ic_dialog_info)
                .setContentTitle(params.optString("title", ""))
                .setContentText(params.optString("body", ""))
                .setAutoCancel(true)
                .build()
            manager.notify(id, notification)
            completion(Result.success(JSONObject().put("id", id)))
        } catch (e: Exception) {
            completion(Result.failure(e))
        }
    }
}
