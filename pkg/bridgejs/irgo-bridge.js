/**
 * Irgo Bridge - Virtual HTTP and WebSocket for Datastar
 *
 * This script intercepts HTTP requests and WebSocket connections from Datastar
 * and routes them through the native bridge (iOS/Android) to the Go framework.
 *
 * On desktop/web, it falls back to real network requests.
 */

(function () {
  "use strict";

  // ========================================
  // PLATFORM DETECTION
  // ========================================

  const isIOS = !!(
    window.webkit &&
    window.webkit.messageHandlers &&
    window.webkit.messageHandlers.irgo
  );
  const isAndroid = typeof window.Irgo !== "undefined";
  const isNative = isIOS || isAndroid;

  // Store original constructors for fallback
  const NativeWebSocket = window.WebSocket;
  const NativeFetch = window.fetch;
  const NativeXHR = window.XMLHttpRequest;

  // ========================================
  // SECURITY - PER-LAUNCH SECRET
  // ========================================

  // Get the per-launch secret injected by the desktop app
  function getSecret() {
    return window.__IRGO_SECRET__ || "";
  }

  // Add secret header to request headers
  function addSecretHeader(headers) {
    const secret = getSecret();
    if (secret) {
      headers["X-Irgo-Secret"] = secret;
    }
    return headers;
  }

  // Add secret to WebSocket URL as query parameter
  // (WebSocket API doesn't support custom headers on connect)
  function addSecretToWsUrl(url) {
    const secret = getSecret();
    if (!secret) {
      return url;
    }
    // Don't add if already present
    if (url.includes("secret=")) {
      return url;
    }
    const separator = url.includes("?") ? "&" : "?";
    return `${url}${separator}secret=${encodeURIComponent(secret)}`;
  }

  // ========================================
  // NATIVE BRIDGE INTERFACE
  // ========================================

  const NativeBridge = {
    // Buffered HTTP request (Android legacy path only). On iOS all HTTP goes
    // through the irgo:// WKURLSchemeHandler, which streams natively — there
    // is no postMessage HTTP path, so reject instead of hanging a promise.
    async httpRequest(method, url, headers, body) {
      if (isAndroid) {
        // Async callback pattern: the @JavascriptInterface method returns
        // immediately; Go processes the request on a worker thread and calls
        // back via window._irgo_http_response / _irgo_http_error.
        return new Promise((resolve, reject) => {
          const requestId = generateUUID();
          pendingHttpRequests.set(requestId, { resolve, reject });

          window.Irgo.httpRequest(
            requestId,
            method,
            url,
            JSON.stringify(headers),
            body || "",
          );
        });
      }
      throw new Error(
        "NativeBridge.httpRequest is Android-only; use fetch() (iOS routes it via the irgo:// scheme)",
      );
    },

    // WebSocket connect
    async wsConnect(url) {
      if (isIOS) {
        return new Promise((resolve, reject) => {
          const requestId = generateUUID();
          pendingWsConnects.set(requestId, { resolve, reject });

          window.webkit.messageHandlers.irgo.postMessage({
            type: "ws_connect",
            requestId,
            url,
          });
        });
      } else if (isAndroid) {
        return window.Irgo.wsConnect(url);
      }
      throw new Error("Native bridge not available");
    },

    // WebSocket send
    wsSend(sessionId, data) {
      if (isIOS) {
        window.webkit.messageHandlers.irgo.postMessage({
          type: "ws_send",
          sessionId,
          data,
        });
      } else if (isAndroid) {
        window.Irgo.wsSend(sessionId, data);
      }
    },

    // Cancel an in-flight streaming HTTP request
    httpRequestCancel(requestId) {
      if (isAndroid && typeof window.Irgo.httpRequestCancel === "function") {
        window.Irgo.httpRequestCancel(requestId);
      }
    },

    // WebSocket close
    wsClose(sessionId, code, reason) {
      if (isIOS) {
        window.webkit.messageHandlers.irgo.postMessage({
          type: "ws_close",
          sessionId,
          code: code || 1000,
          reason: reason || "",
        });
      } else if (isAndroid) {
        window.Irgo.wsClose(sessionId, code || 1000, reason || "");
      }
    },
  };

  // Pending request registries
  const pendingHttpRequests = new Map();
  const pendingWsConnects = new Map();
  const pendingStreams = new Map();
  const pendingNativeCalls = new Map();

  // Streaming HTTP support (progressive SSE) requires the httpRequestStream
  // native method; older shells fall back to buffered requests.
  const hasAndroidStreaming =
    isAndroid && typeof window.Irgo.httpRequestStream === "function";

  // ========================================
  // VIRTUAL WEBSOCKET IMPLEMENTATION
  // ========================================

  class VirtualWebSocket {
    static CONNECTING = 0;
    static OPEN = 1;
    static CLOSING = 2;
    static CLOSED = 3;

    constructor(url, protocols) {
      this.url = normalizeWebSocketUrl(url);
      this.protocols = protocols;
      this.readyState = VirtualWebSocket.CONNECTING;
      this.sessionId = null;
      this.bufferedAmount = 0;
      this.extensions = "";
      this.protocol = "";
      this.binaryType = "blob";

      // Event handlers
      this.onopen = null;
      this.onmessage = null;
      this.onclose = null;
      this.onerror = null;

      // Event listeners
      this._listeners = {
        open: [],
        message: [],
        close: [],
        error: [],
      };

      // Connect
      this._connect();
    }

    async _connect() {
      try {
        if (isNative) {
          this.sessionId = await NativeBridge.wsConnect(this.url);
          VirtualWebSocket._sessions.set(this.sessionId, this);

          this.readyState = VirtualWebSocket.OPEN;
          this._dispatchEvent("open", { target: this });
        } else {
          // Desktop/web: use real WebSocket with secret in URL
          const secureUrl = addSecretToWsUrl(this.url);
          this._native = new NativeWebSocket(secureUrl, this.protocols);
          this._native.binaryType = this.binaryType;

          this._native.onopen = (e) => {
            this.readyState = VirtualWebSocket.OPEN;
            this._dispatchEvent("open", e);
          };
          this._native.onmessage = (e) => {
            this._dispatchEvent("message", e);
          };
          this._native.onclose = (e) => {
            this.readyState = VirtualWebSocket.CLOSED;
            this._dispatchEvent("close", e);
          };
          this._native.onerror = (e) => {
            this._dispatchEvent("error", e);
          };
        }
      } catch (error) {
        this.readyState = VirtualWebSocket.CLOSED;
        this._dispatchEvent("error", { target: this, error });
      }
    }

    send(data) {
      if (this.readyState !== VirtualWebSocket.OPEN) {
        throw new DOMException("WebSocket is not open", "InvalidStateError");
      }

      if (this._native) {
        this._native.send(data);
      } else {
        NativeBridge.wsSend(
          this.sessionId,
          typeof data === "string" ? data : JSON.stringify(data),
        );
      }
    }

    close(code = 1000, reason = "") {
      if (
        this.readyState === VirtualWebSocket.CLOSING ||
        this.readyState === VirtualWebSocket.CLOSED
      ) {
        return;
      }

      this.readyState = VirtualWebSocket.CLOSING;

      if (this._native) {
        this._native.close(code, reason);
      } else {
        NativeBridge.wsClose(this.sessionId, code, reason);
        VirtualWebSocket._sessions.delete(this.sessionId);
        this.readyState = VirtualWebSocket.CLOSED;
        this._dispatchEvent("close", {
          code,
          reason,
          wasClean: true,
          target: this,
        });
      }
    }

    addEventListener(type, listener) {
      if (this._listeners[type]) {
        this._listeners[type].push(listener);
      }
    }

    removeEventListener(type, listener) {
      if (this._listeners[type]) {
        const idx = this._listeners[type].indexOf(listener);
        if (idx !== -1) {
          this._listeners[type].splice(idx, 1);
        }
      }
    }

    _dispatchEvent(type, event) {
      // Call direct handler
      const handler = this["on" + type];
      if (handler) {
        handler.call(this, event);
      }

      // Call registered listeners
      if (this._listeners[type]) {
        for (const listener of this._listeners[type]) {
          listener.call(this, event);
        }
      }
    }
  }

  // Session registry for native callbacks
  VirtualWebSocket._sessions = new Map();

  // ========================================
  // NATIVE CALLBACK HANDLERS
  // ========================================

  // Called by native code when a buffered HTTP response is ready
  // (Android legacy path)
  window._irgo_http_response = function (requestId, status, headers, body) {
    const pending = pendingHttpRequests.get(requestId);
    if (pending) {
      pendingHttpRequests.delete(requestId);
      pending.resolve({
        status,
        headers: headers ? JSON.parse(headers) : {},
        body: body ? decodeBase64Utf8(body) : "",
      });
    }
  };

  // Called by native code on a buffered HTTP error (Android legacy path)
  window._irgo_http_error = function (requestId, error) {
    const pending = pendingHttpRequests.get(requestId);
    if (pending) {
      pendingHttpRequests.delete(requestId);
      pending.reject(new Error(error));
    }
  };

  // Called by native code when WebSocket connects (iOS)
  window._irgo_ws_connected = function (requestId, sessionId) {
    const pending = pendingWsConnects.get(requestId);
    if (pending) {
      pendingWsConnects.delete(requestId);
      pending.resolve(sessionId);
    }
  };

  // Called by native code on WebSocket connect error (iOS)
  window._irgo_ws_connect_error = function (requestId, error) {
    const pending = pendingWsConnects.get(requestId);
    if (pending) {
      pendingWsConnects.delete(requestId);
      pending.reject(new Error(error));
    }
  };

  // Called by native code when WebSocket message arrives
  window._irgo_ws_message = function (sessionId, data) {
    const ws = VirtualWebSocket._sessions.get(sessionId);
    if (ws) {
      ws._dispatchEvent("message", { data, target: ws });
    }
  };

  // Called by native code when WebSocket closes
  window._irgo_ws_close = function (sessionId, code, reason) {
    const ws = VirtualWebSocket._sessions.get(sessionId);
    if (ws) {
      ws.readyState = VirtualWebSocket.CLOSED;
      ws._dispatchEvent("close", { code, reason, wasClean: true, target: ws });
      VirtualWebSocket._sessions.delete(sessionId);
    }
  };

  // Called by native code on WebSocket error
  window._irgo_ws_error = function (sessionId, error) {
    const ws = VirtualWebSocket._sessions.get(sessionId);
    if (ws) {
      ws._dispatchEvent("error", { error, target: ws });
    }
  };

  // ========================================
  // UTILITY FUNCTIONS
  // ========================================

  function generateUUID() {
    return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(
      /[xy]/g,
      function (c) {
        const r = (Math.random() * 16) | 0;
        const v = c === "x" ? r : (r & 0x3) | 0x8;
        return v.toString(16);
      },
    );
  }

  // Decode a base64 string into raw bytes.
  function base64ToBytes(b64) {
    const binary = atob(b64);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) {
      bytes[i] = binary.charCodeAt(i);
    }
    return bytes;
  }

  // Decode a base64 string (produced from raw UTF-8 bytes by the native side)
  // back into a JS string. Plain atob() yields a Latin-1 "binary string" which
  // corrupts multi-byte UTF-8 (e.g. curly quotes, emoji), so we re-decode.
  function decodeBase64Utf8(b64) {
    return new TextDecoder("utf-8").decode(base64ToBytes(b64));
  }

  // Normalize fetch() headers (Headers | array | object | undefined) to a
  // plain string→string object the native bridge can JSON-encode.
  function headersToObject(h) {
    const out = {};
    if (!h) return out;
    if (typeof Headers !== "undefined" && h instanceof Headers) {
      h.forEach(function (value, key) {
        out[key] = value;
      });
    } else if (Array.isArray(h)) {
      for (const pair of h) {
        out[pair[0]] = pair[1];
      }
    } else {
      for (const key in h) {
        if (Object.prototype.hasOwnProperty.call(h, key)) {
          out[key] = h[key];
        }
      }
    }
    return out;
  }

  // Convert a fetch() body into a string for the native bridge.
  async function bodyToString(body) {
    if (body == null) return "";
    if (typeof body === "string") return body;
    if (typeof URLSearchParams !== "undefined" && body instanceof URLSearchParams) {
      return body.toString();
    }
    if (typeof Blob !== "undefined" && body instanceof Blob) {
      return await body.text();
    }
    if (body instanceof ArrayBuffer) {
      return new TextDecoder("utf-8").decode(body);
    }
    if (ArrayBuffer.isView(body)) {
      return new TextDecoder("utf-8").decode(body);
    }
    try {
      return String(body);
    } catch (e) {
      return "";
    }
  }

  // Resolve a fetch() URL to an app-relative path (e.g. "/todos?x=1") when the
  // request targets the native app, or return null for external http(s) URLs
  // that should use the real network.
  function toAppPath(rawUrl) {
    let u;
    try {
      u = new URL(rawUrl, window.location.href);
    } catch (e) {
      // Unparseable - assume app-relative
      return rawUrl.startsWith("/") ? rawUrl : "/" + rawUrl;
    }
    if (u.protocol === "irgo:") {
      return u.pathname + u.search;
    }
    if (u.origin === window.location.origin) {
      return u.pathname + u.search;
    }
    // External http(s):// request - not handled by the bridge
    return null;
  }

  function normalizeWebSocketUrl(url) {
    // Already a WebSocket URL
    if (url.startsWith("ws://") || url.startsWith("wss://")) {
      return url;
    }

    // Convert http(s):// to ws(s)://
    if (url.startsWith("http://")) {
      return "ws://" + url.slice(7);
    }
    if (url.startsWith("https://")) {
      return "wss://" + url.slice(8);
    }

    // Relative URL - use irgo:// scheme for native
    if (isNative) {
      return "irgo://ws" + (url.startsWith("/") ? "" : "/") + url;
    }

    // Desktop: build absolute ws(s):// URL
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const host = window.location.host;

    if (url.startsWith("//")) {
      return protocol + url;
    }

    if (url.startsWith("/")) {
      return protocol + "//" + host + url;
    }

    // Relative path
    const basePath = window.location.pathname.substring(
      0,
      window.location.pathname.lastIndexOf("/") + 1,
    );
    return protocol + "//" + host + basePath + url;
  }

  // ========================================
  // DESKTOP SECURITY - INJECT SECRET INTO ALL REQUESTS
  // ========================================

  // Patch fetch to add secret header (Datastar uses fetch for all requests)
  if (!isNative && getSecret()) {
    window.fetch = function (input, init) {
      init = init || {};
      init.headers = init.headers || {};

      // Handle Headers object or plain object
      if (init.headers instanceof Headers) {
        if (!init.headers.has("X-Irgo-Secret")) {
          init.headers.set("X-Irgo-Secret", getSecret());
        }
      } else {
        if (!init.headers["X-Irgo-Secret"]) {
          init.headers["X-Irgo-Secret"] = getSecret();
        }
      }

      return NativeFetch.call(window, input, init);
    };
  }

  // Patch XMLHttpRequest to add secret header (for legacy code)
  if (!isNative && getSecret()) {
    const XHROpen = NativeXHR.prototype.open;
    const XHRSend = NativeXHR.prototype.send;

    NativeXHR.prototype.open = function (method, url, async, user, password) {
      this._irgoUrl = url;
      return XHROpen.apply(this, arguments);
    };

    NativeXHR.prototype.send = function (body) {
      const secret = getSecret();
      if (secret && this._irgoUrl) {
        // Only add for same-origin requests
        try {
          const url = new URL(this._irgoUrl, window.location.href);
          if (url.origin === window.location.origin) {
            this.setRequestHeader("X-Irgo-Secret", secret);
          }
        } catch (e) {
          // Relative URL - same origin
          this.setRequestHeader("X-Irgo-Secret", secret);
        }
      }
      return XHRSend.apply(this, arguments);
    };
  }

  // ========================================
  // STREAMING HTTP (progressive SSE)
  // ========================================

  // Native chunk callbacks. The native side calls these as Go flushes the
  // response, letting Datastar process SSE events while the handler is still
  // running - true streaming instead of one buffered blob at the end.

  // Build a fetch Headers object from the bridge's headers wire format
  // (values may be strings or arrays for multi-value headers).
  function toFetchHeaders(headersObjOrJSON) {
    const responseHeaders = new Headers();
    let parsed = headersObjOrJSON || {};
    if (typeof parsed === "string") {
      try {
        parsed = JSON.parse(parsed);
      } catch (e) {
        parsed = {};
      }
    }
    for (const key in parsed) {
      if (!Object.prototype.hasOwnProperty.call(parsed, key)) continue;
      const value = parsed[key];
      try {
        if (Array.isArray(value)) {
          for (const v of value) responseHeaders.append(key, v);
        } else {
          responseHeaders.set(key, value);
        }
      } catch (e) {
        // Skip headers the Headers API forbids setting.
      }
    }
    return responseHeaders;
  }

  // 204/205/304 must have a null body per the Fetch spec.
  function isNullBodyStatus(status) {
    return status === 204 || status === 205 || status === 304;
  }

  window._irgo_stream_response = function (requestId, status, headersJSON) {
    const stream = pendingStreams.get(requestId);
    if (!stream) return;

    const responseHeaders = toFetchHeaders(headersJSON);

    const st = status || 200;
    const nullBody = isNullBodyStatus(st);

    let bodyStream = null;
    if (!nullBody) {
      bodyStream = new ReadableStream({
        start(controller) {
          stream.controller = controller;
        },
        cancel() {
          stream.cancelled = true;
          pendingStreams.delete(requestId);
          NativeBridge.httpRequestCancel(requestId);
        },
      });
    }

    stream.resolve(new Response(bodyStream, { status: st, headers: responseHeaders }));
    if (nullBody) {
      pendingStreams.delete(requestId);
    }
  };

  window._irgo_stream_chunk = function (requestId, base64Chunk) {
    const stream = pendingStreams.get(requestId);
    if (!stream || stream.cancelled || !stream.controller) return;
    try {
      stream.controller.enqueue(base64ToBytes(base64Chunk));
    } catch (e) {
      // Stream already closed/errored - stop delivery.
      stream.cancelled = true;
      pendingStreams.delete(requestId);
      NativeBridge.httpRequestCancel(requestId);
    }
  };

  window._irgo_stream_complete = function (requestId, errorMessage) {
    const stream = pendingStreams.get(requestId);
    if (!stream) return;
    pendingStreams.delete(requestId);

    if (errorMessage) {
      if (stream.controller && !stream.cancelled) {
        try {
          stream.controller.error(new Error(errorMessage));
        } catch (e) {
          /* already closed */
        }
      } else {
        stream.reject(new Error(errorMessage));
      }
      return;
    }
    if (stream.controller && !stream.cancelled) {
      try {
        stream.controller.close();
      } catch (e) {
        /* already closed */
      }
    }
  };

  // Issue a streaming request through the native bridge, resolving to a
  // fetch Response whose body is a live ReadableStream.
  function nativeStreamingFetch(method, path, headers, bodyString, signal) {
    return new Promise(function (resolve, reject) {
      const requestId = generateUUID();
      pendingStreams.set(requestId, {
        resolve,
        reject,
        controller: null,
        cancelled: false,
      });

      if (signal) {
        if (signal.aborted) {
          pendingStreams.delete(requestId);
          reject(makeAbortError());
          return;
        }
        signal.addEventListener("abort", function () {
          const stream = pendingStreams.get(requestId);
          if (!stream) return;
          stream.cancelled = true;
          pendingStreams.delete(requestId);
          NativeBridge.httpRequestCancel(requestId);
          if (stream.controller) {
            try {
              stream.controller.error(makeAbortError());
            } catch (e) {
              /* already closed */
            }
          } else {
            stream.reject(makeAbortError());
          }
        });
      }

      window.Irgo.httpRequestStream(
        requestId,
        method,
        path,
        JSON.stringify(headers),
        bodyString || "",
      );
    });
  }

  function makeAbortError() {
    try {
      return new DOMException("The operation was aborted.", "AbortError");
    } catch (e) {
      const err = new Error("The operation was aborted.");
      err.name = "AbortError";
      return err;
    }
  }

  // ========================================
  // NATIVE CAPABILITIES - irgo.native()
  // ========================================

  // Result callback shared by iOS and Android plugin registries.
  window._irgo_native_result = function (requestId, ok, payloadJSON) {
    const pending = pendingNativeCalls.get(requestId);
    if (!pending) return;
    pendingNativeCalls.delete(requestId);

    let payload = null;
    if (payloadJSON) {
      try {
        payload = JSON.parse(payloadJSON);
      } catch (e) {
        payload = payloadJSON;
      }
    }

    if (ok) {
      pending.resolve(payload);
    } else if (payloadJSON === "IRGO_NOT_SUPPORTED") {
      // No native plugin claimed the method - fall back to the Go-side
      // registry via the /_irgo/native endpoint (served through the
      // virtual bridge on mobile, real HTTP on web/desktop).
      httpNativeCall(pending.method, pending.params).then(
        pending.resolve,
        pending.reject,
      );
    } else {
      pending.reject(new Error(String(payload || "native call failed")));
    }
  };

  function httpNativeCall(method, params) {
    return window
      .fetch("/_irgo/native", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ method, params: params || {} }),
      })
      .then(function (res) {
        return res.json().then(function (data) {
          if (data && data.ok) return data.result;
          const message =
            data && data.error ? data.error : "native call failed";
          const err = new Error(message);
          if (res.status === 501) err.notSupported = true;
          throw err;
        });
      });
  }

  // Invoke a native capability: nativeCall('haptics.impact', {style: 'light'})
  // Resolution: platform plugin first, then Go-registered fallback handlers.
  // Returns a promise for the method's result (or null).
  function nativeCall(method, params) {
    if (typeof method !== "string" || !method) {
      return Promise.reject(new Error("irgo.native: method name required"));
    }

    // The __IRGO_IOS_NATIVE__ flag is injected by shells that implement the
    // {type:"native"} message. Without the guard, an older shell would drop
    // the message and the promise would hang forever.
    if (isIOS && window.__IRGO_IOS_NATIVE__) {
      return new Promise(function (resolve, reject) {
        const requestId = generateUUID();
        pendingNativeCalls.set(requestId, { resolve, reject, method, params });
        window.webkit.messageHandlers.irgo.postMessage({
          type: "native",
          requestId,
          method,
          params: JSON.stringify(params || {}),
        });
      });
    }

    if (isAndroid && typeof window.Irgo.nativeInvoke === "function") {
      return new Promise(function (resolve, reject) {
        const requestId = generateUUID();
        pendingNativeCalls.set(requestId, { resolve, reject, method, params });
        window.Irgo.nativeInvoke(requestId, method, JSON.stringify(params || {}));
      });
    }

    // Web/desktop (or an older shell): Go-side handlers only.
    return httpNativeCall(method, params);
  }

  // ========================================
  // ANDROID FETCH INTERCEPTION
  // ========================================

  // On Android, shouldInterceptRequest cannot see fetch() request bodies and
  // cannot stream SSE responses, so we route Datastar's fetch() through the
  // @JavascriptInterface bridge instead. (iOS uses its WKURLSchemeHandler with
  // the irgo:// base URL, so its fetch() is left untouched here.)
  if (isAndroid) {
    window.fetch = async function (input, init) {
      init = init || {};

      let rawUrl;
      let method;
      let headers;
      let body;
      let signal = init.signal;

      if (typeof Request !== "undefined" && input instanceof Request) {
        rawUrl = input.url;
        method = (init.method || input.method || "GET").toUpperCase();
        headers = headersToObject(init.headers || input.headers);
        if (!signal) signal = input.signal;
        if (init.body != null) {
          body = init.body;
        } else if (method !== "GET" && method !== "HEAD") {
          // Body lives on the Request object - read a clone so the original
          // stays usable for the fallback path.
          try {
            body = await input.clone().text();
          } catch (e) {
            body = null;
          }
        } else {
          body = null;
        }
      } else {
        rawUrl = typeof input === "string" ? input : String(input);
        method = (init.method || "GET").toUpperCase();
        headers = headersToObject(init.headers);
        body = init.body != null ? init.body : null;
      }

      const path = toAppPath(rawUrl);
      if (path === null) {
        // External request - use the real network fetch.
        return NativeFetch.call(window, input, init);
      }

      // FormData needs multipart encoding with a boundary; Request does
      // both (String(formData) would send the literal "[object FormData]").
      if (typeof FormData !== "undefined" && body instanceof FormData) {
        const encoded = new Request(rawUrl, { method: "POST", body });
        headers["Content-Type"] = encoded.headers.get("content-type");
        body = await encoded.text();
      }

      const bodyString = await bodyToString(body);

      // Preferred: streaming path (progressive SSE).
      if (hasAndroidStreaming) {
        return nativeStreamingFetch(method, path, headers, bodyString, signal);
      }

      // Legacy shells: buffered request/response. SSE cannot stream through
      // this path — a long-lived handler's response only arrives when the
      // handler returns, so warn loudly instead of hanging silently.
      if ((headers["Accept"] || headers["accept"] || "").includes("text/event-stream")) {
        console.warn(
          "[irgo] This Android shell has no httpRequestStream - SSE responses " +
            "will be buffered (long-lived handlers will hang). Update " +
            "IrgoJSInterface.kt to the current irgo shell to enable streaming.",
        );
      }
      const res = await NativeBridge.httpRequest(method, path, headers, bodyString);

      const status = res.status || 200;
      return new Response(isNullBodyStatus(status) ? null : res.body, {
        status,
        headers: toFetchHeaders(res.headers),
      });
    };
  }

  // ========================================
  // GLOBAL EXPORTS
  // ========================================

  // Replace WebSocket on native platforms
  if (isNative) {
    window.WebSocket = VirtualWebSocket;
  }

  // Export irgo namespace
  window.irgo = {
    isNative,
    isIOS,
    isAndroid,
    VirtualWebSocket,
    NativeBridge,

    // Invoke a native capability, e.g.:
    //   irgo.native('haptics.impact', {style: 'light'})
    //   irgo.native('clipboard.write', {text: 'hello'})
    // Resolves with the method's result; rejects if unsupported everywhere.
    native: nativeCall,

    // Navigate programmatically
    navigate: function (path) {
      window.location.href = path;
    },

    // Get WebSocket sessions
    getSessions: function () {
      return Array.from(VirtualWebSocket._sessions.keys());
    },
  };

  console.log(
    "[irgo] Bridge loaded, platform:",
    isNative ? (isIOS ? "iOS" : "Android") : "web",
  );
})();
