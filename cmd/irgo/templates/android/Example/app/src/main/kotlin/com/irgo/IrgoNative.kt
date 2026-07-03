package com.irgo

import android.os.Handler
import android.os.Looper
import mobile.Mobile as Irgo
import mobile.NativeInvoker
import org.json.JSONArray
import org.json.JSONObject
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.atomic.AtomicBoolean

/**
 * A native capability plugin. Plugins claim a method namespace: a plugin
 * with namespace "haptics" is offered every method of the form "haptics.*"
 * (e.g. "haptics.impact").
 *
 * Plugins are invoked on the main thread. [handle] must either:
 *  - return false when the method is not recognized (the dispatcher then
 *    tries the next plugin, or reports "not supported"), or
 *  - return true and eventually invoke completion exactly once with the
 *    method's result. The result value is JSON-encoded for the caller
 *    (JSONObject/JSONArray/String/Number/Boolean/Map/Collection or null).
 */
interface IrgoPlugin {
    /** Method namespace this plugin claims, e.g. "haptics" for "haptics.*". */
    val namespace: String

    /**
     * Handle a method call. Return false if the method is not recognized;
     * return true and call [completion] (exactly once) otherwise.
     */
    fun handle(method: String, params: JSONObject, completion: (Result<Any?>) -> Unit): Boolean
}

/**
 * Registry and dispatcher for native capability plugins.
 *
 * Calls arrive from two directions with identical semantics:
 *  - JavaScript: `irgo.native(method, params)` via [IrgoJSInterface.nativeInvoke]
 *  - Go: `native.Call(...)` via the invoker installed by [installGoInvoker]
 *
 * When no plugin claims a method, the dispatcher reports failure with the
 * exact payload "IRGO_NOT_SUPPORTED" so both callers fall back to
 * Go-registered handlers.
 */
object IrgoNative {

    /**
     * Sentinel payload meaning "no plugin implements this method". Both the
     * JS bridge and the Go native package compare against this exact string.
     */
    const val NOT_SUPPORTED = "IRGO_NOT_SUPPORTED"

    private val plugins = CopyOnWriteArrayList<IrgoPlugin>()
    private val mainHandler = Handler(Looper.getMainLooper())

    /** Register a plugin. Later registrations are consulted after earlier ones. */
    fun register(plugin: IrgoPlugin) {
        plugins.add(plugin)
    }

    /** Remove a previously registered plugin. */
    fun unregister(plugin: IrgoPlugin) {
        plugins.remove(plugin)
    }

    /**
     * Dispatch a method call to the plugin registry.
     *
     * Plugin handlers run on the main thread. The completion receives:
     *  - ok=true with a JSON-encoded result payload on success
     *  - ok=false with [NOT_SUPPORTED] when no plugin claims the method
     *  - ok=false with a JSON-encoded error-message string on failure
     *
     * The completion may be invoked on any thread.
     */
    fun dispatch(method: String, paramsJSON: String?, completion: (ok: Boolean, payloadJSON: String) -> Unit) {
        val namespace = method.substringBefore('.', missingDelimiterValue = method)
        val params = try {
            if (paramsJSON.isNullOrEmpty()) JSONObject() else JSONObject(paramsJSON)
        } catch (e: Exception) {
            JSONObject()
        }

        mainHandler.post {
            val done = AtomicBoolean(false)
            val complete: (Result<Any?>) -> Unit = { result ->
                if (done.compareAndSet(false, true)) {
                    result.fold(
                        onSuccess = { value -> completion(true, encodeJson(value)) },
                        onFailure = { error ->
                            completion(false, JSONObject.quote(error.message ?: "native call failed"))
                        }
                    )
                }
            }

            try {
                for (plugin in plugins) {
                    if (plugin.namespace != namespace) continue
                    if (plugin.handle(method, params, complete)) {
                        return@post
                    }
                }
                if (done.compareAndSet(false, true)) {
                    completion(false, NOT_SUPPORTED)
                }
            } catch (e: Exception) {
                if (done.compareAndSet(false, true)) {
                    completion(false, JSONObject.quote(e.message ?: "native call failed"))
                }
            }
        }
    }

    /**
     * Install the gomobile [NativeInvoker] so Go handler code can call native
     * capabilities via native.Call. Results are delivered back to Go with
     * Irgo.nativeResult using the same call ID.
     */
    fun installGoInvoker() {
        Irgo.setNativeInvoker(object : NativeInvoker {
            override fun invoke(callID: String?, method: String?, paramsJSON: String?) {
                if (callID == null) return
                if (method.isNullOrEmpty()) {
                    Irgo.nativeResult(callID, false, NOT_SUPPORTED)
                    return
                }
                dispatch(method, paramsJSON) { ok, payloadJSON ->
                    Irgo.nativeResult(callID, ok, payloadJSON)
                }
            }
        })
    }

    /** Encode a plugin result value as JSON text. */
    private fun encodeJson(value: Any?): String = when (value) {
        null, JSONObject.NULL -> "null"
        is JSONObject -> value.toString()
        is JSONArray -> value.toString()
        is String -> JSONObject.quote(value)
        is Boolean -> value.toString()
        is Number -> value.toString()
        is Map<*, *> -> {
            val obj = JSONObject()
            value.forEach { (k, v) ->
                if (k != null) obj.put(k.toString(), v ?: JSONObject.NULL)
            }
            obj.toString()
        }
        is Collection<*> -> {
            val arr = JSONArray()
            value.forEach { arr.put(it ?: JSONObject.NULL) }
            arr.toString()
        }
        else -> JSONObject.quote(value.toString())
    }
}
