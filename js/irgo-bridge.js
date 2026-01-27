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
    // HTTP request handler
    async httpRequest(method, url, headers, body) {
      if (isIOS) {
        return new Promise((resolve, reject) => {
          const requestId = generateUUID();
          pendingHttpRequests.set(requestId, { resolve, reject });

          window.webkit.messageHandlers.irgo.postMessage({
            type: "http",
            requestId,
            method,
            url,
            headers: JSON.stringify(headers),
            body: body ? btoa(body) : null,
          });
        });
      } else if (isAndroid) {
        const response = window.Irgo.handleRequest(
          method,
          url,
          JSON.stringify(headers),
          body || "",
        );
        return JSON.parse(response);
      }
      throw new Error("Native bridge not available");
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

  // Called by native code when HTTP response is ready (iOS)
  window._irgo_http_response = function (requestId, status, headers, body) {
    const pending = pendingHttpRequests.get(requestId);
    if (pending) {
      pendingHttpRequests.delete(requestId);
      pending.resolve({
        status,
        headers: JSON.parse(headers),
        body: body ? atob(body) : "",
      });
    }
  };

  // Called by native code on HTTP error (iOS)
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
