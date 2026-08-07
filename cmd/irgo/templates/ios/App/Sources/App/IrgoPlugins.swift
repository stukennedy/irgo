import Foundation
import UIKit
import SafariServices
import UserNotifications
import Security

/// Error raised by built-in plugins.
public struct IrgoPluginError: LocalizedError {
    public let message: String

    public init(_ message: String) {
        self.message = message
    }

    public var errorDescription: String? { message }
}

public extension IrgoNative {
    /// Register the built-in plugins (device, haptics, clipboard, share,
    /// browser, storage, notifications). Called by IrgoWebViewController
    /// during setup; safe to call more than once.
    func registerBuiltins() {
        register(IrgoDevicePlugin())
        register(IrgoHapticsPlugin())
        register(IrgoClipboardPlugin())
        register(IrgoSharePlugin())
        register(IrgoBrowserPlugin())
        register(IrgoStoragePlugin())
        register(IrgoNotificationsPlugin())
    }
}

/// Finds the view controller to present system UI (share sheets, Safari)
/// from. Main-thread only.
enum IrgoPresentation {
    static func topViewController() -> UIViewController? {
        let keyWindow = UIApplication.shared.connectedScenes
            .compactMap { $0 as? UIWindowScene }
            .flatMap { $0.windows }
            .first { $0.isKeyWindow }
        var top = keyWindow?.rootViewController
        while let presented = top?.presentedViewController {
            top = presented
        }
        return top
    }
}

// MARK: - device

/// device.info -> {platform, model, osVersion, appVersion, bundleId}
public final class IrgoDevicePlugin: IrgoPlugin {
    public let namespace = "device"

    public init() {}

    public func handle(method: String, params: [String: Any], completion: @escaping (Result<Any?, Error>) -> Void) -> Bool {
        switch method {
        case "device.info":
            let appVersion = Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String
            completion(.success([
                "platform": "ios",
                "model": UIDevice.current.model,
                "osVersion": UIDevice.current.systemVersion,
                "appVersion": appVersion ?? "",
                "bundleId": Bundle.main.bundleIdentifier ?? "",
            ]))
            return true
        default:
            return false
        }
    }
}

// MARK: - haptics

/// haptics.impact {style}, haptics.notification {type}, haptics.selection
public final class IrgoHapticsPlugin: IrgoPlugin {
    public let namespace = "haptics"

    public init() {}

    public func handle(method: String, params: [String: Any], completion: @escaping (Result<Any?, Error>) -> Void) -> Bool {
        switch method {
        case "haptics.impact":
            let style: UIImpactFeedbackGenerator.FeedbackStyle
            switch params["style"] as? String ?? "medium" {
            case "light":
                style = .light
            case "heavy":
                style = .heavy
            default:
                style = .medium
            }
            let generator = UIImpactFeedbackGenerator(style: style)
            generator.prepare()
            generator.impactOccurred()
            completion(.success(nil))
            return true
        case "haptics.notification":
            let type: UINotificationFeedbackGenerator.FeedbackType
            switch params["type"] as? String ?? "success" {
            case "warning":
                type = .warning
            case "error":
                type = .error
            default:
                type = .success
            }
            let generator = UINotificationFeedbackGenerator()
            generator.prepare()
            generator.notificationOccurred(type)
            completion(.success(nil))
            return true
        case "haptics.selection":
            let generator = UISelectionFeedbackGenerator()
            generator.prepare()
            generator.selectionChanged()
            completion(.success(nil))
            return true
        default:
            return false
        }
    }
}

// MARK: - clipboard

/// clipboard.write {text}, clipboard.read -> {text}
public final class IrgoClipboardPlugin: IrgoPlugin {
    public let namespace = "clipboard"

    public init() {}

    public func handle(method: String, params: [String: Any], completion: @escaping (Result<Any?, Error>) -> Void) -> Bool {
        switch method {
        case "clipboard.write":
            guard let text = params["text"] as? String else {
                completion(.failure(IrgoPluginError("clipboard.write requires a text parameter")))
                return true
            }
            UIPasteboard.general.string = text
            completion(.success(nil))
            return true
        case "clipboard.read":
            completion(.success(["text": UIPasteboard.general.string ?? ""]))
            return true
        default:
            return false
        }
    }
}

// MARK: - share

/// share.text {text, title?} -> {completed}
public final class IrgoSharePlugin: IrgoPlugin {
    public let namespace = "share"

    public init() {}

    public func handle(method: String, params: [String: Any], completion: @escaping (Result<Any?, Error>) -> Void) -> Bool {
        guard method == "share.text" else { return false }

        guard let text = params["text"] as? String else {
            completion(.failure(IrgoPluginError("share.text requires a text parameter")))
            return true
        }
        guard let presenter = IrgoPresentation.topViewController() else {
            completion(.failure(IrgoPluginError("No view controller available to present from")))
            return true
        }

        let controller = UIActivityViewController(activityItems: [text], applicationActivities: nil)
        if let title = params["title"] as? String, !title.isEmpty {
            controller.setValue(title, forKey: "subject")
        }

        // iPad requires a popover anchor; center on the presenting view.
        if let popover = controller.popoverPresentationController {
            let sourceView: UIView = presenter.view
            popover.sourceView = sourceView
            popover.sourceRect = CGRect(x: sourceView.bounds.midX, y: sourceView.bounds.midY, width: 0, height: 0)
            popover.permittedArrowDirections = []
        }

        controller.completionWithItemsHandler = { _, completed, _, error in
            if let error = error {
                completion(.failure(error))
            } else {
                completion(.success(["completed": completed]))
            }
        }

        presenter.present(controller, animated: true)
        return true
    }
}

// MARK: - browser

/// browser.open {url}
public final class IrgoBrowserPlugin: IrgoPlugin {
    public let namespace = "browser"

    public init() {}

    public func handle(method: String, params: [String: Any], completion: @escaping (Result<Any?, Error>) -> Void) -> Bool {
        guard method == "browser.open" else { return false }

        guard let urlString = params["url"] as? String, let url = URL(string: urlString) else {
            completion(.failure(IrgoPluginError("browser.open requires a valid url parameter")))
            return true
        }

        let scheme = url.scheme?.lowercased() ?? ""
        if (scheme == "http" || scheme == "https"),
           let presenter = IrgoPresentation.topViewController() {
            let safari = SFSafariViewController(url: url)
            presenter.present(safari, animated: true) {
                completion(.success(nil))
            }
        } else {
            UIApplication.shared.open(url, options: [:]) { opened in
                if opened {
                    completion(.success(nil))
                } else {
                    completion(.failure(IrgoPluginError("Unable to open URL")))
                }
            }
        }
        return true
    }
}

// MARK: - storage

/// storage.get {key} -> {value: string|null}, storage.set {key, value},
/// storage.remove {key}. Backed by the Keychain so values survive app
/// reinstalls and are encrypted at rest.
public final class IrgoStoragePlugin: IrgoPlugin {
    public let namespace = "storage"

    private static let service = "irgo.storage"

    public init() {}

    public func handle(method: String, params: [String: Any], completion: @escaping (Result<Any?, Error>) -> Void) -> Bool {
        switch method {
        case "storage.get":
            guard let key = params["key"] as? String, !key.isEmpty else {
                completion(.failure(IrgoPluginError("storage.get requires a key parameter")))
                return true
            }
            if let value = IrgoStoragePlugin.read(key: key) {
                completion(.success(["value": value]))
            } else {
                completion(.success(["value": NSNull()]))
            }
            return true
        case "storage.set":
            guard let key = params["key"] as? String, !key.isEmpty else {
                completion(.failure(IrgoPluginError("storage.set requires a key parameter")))
                return true
            }
            guard let value = params["value"] as? String else {
                completion(.failure(IrgoPluginError("storage.set requires a string value parameter")))
                return true
            }
            if IrgoStoragePlugin.write(key: key, value: value) {
                completion(.success(nil))
            } else {
                completion(.failure(IrgoPluginError("Keychain write failed")))
            }
            return true
        case "storage.remove":
            guard let key = params["key"] as? String, !key.isEmpty else {
                completion(.failure(IrgoPluginError("storage.remove requires a key parameter")))
                return true
            }
            IrgoStoragePlugin.remove(key: key)
            completion(.success(nil))
            return true
        default:
            return false
        }
    }

    // MARK: Keychain

    private static func baseQuery(key: String) -> [String: Any] {
        return [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: key,
        ]
    }

    private static func read(key: String) -> String? {
        var query = baseQuery(key: key)
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne

        var result: AnyObject?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        guard status == errSecSuccess, let data = result as? Data else {
            return nil
        }
        return String(data: data, encoding: .utf8)
    }

    private static func write(key: String, value: String) -> Bool {
        let data = Data(value.utf8)

        // Update in place when the item exists, otherwise add it.
        let updateStatus = SecItemUpdate(
            baseQuery(key: key) as CFDictionary,
            [kSecValueData as String: data] as CFDictionary
        )
        if updateStatus == errSecSuccess {
            return true
        }
        guard updateStatus == errSecItemNotFound else {
            return false
        }

        var query = baseQuery(key: key)
        query[kSecValueData as String] = data
        query[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlock
        return SecItemAdd(query as CFDictionary, nil) == errSecSuccess
    }

    private static func remove(key: String) {
        _ = SecItemDelete(baseQuery(key: key) as CFDictionary)
    }
}

// MARK: - notifications

/// notifications.requestPermission -> {granted},
/// notifications.show {title, body, id?, delaySeconds?} -> {id}
public final class IrgoNotificationsPlugin: IrgoPlugin {
    public let namespace = "notifications"

    public init() {}

    public func handle(method: String, params: [String: Any], completion: @escaping (Result<Any?, Error>) -> Void) -> Bool {
        switch method {
        case "notifications.requestPermission":
            UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .badge, .sound]) { granted, _ in
                completion(.success(["granted": granted]))
            }
            return true
        case "notifications.show":
            let title = params["title"] as? String ?? ""
            let body = params["body"] as? String ?? ""
            guard !title.isEmpty || !body.isEmpty else {
                completion(.failure(IrgoPluginError("notifications.show requires a title or body")))
                return true
            }

            let content = UNMutableNotificationContent()
            content.title = title
            content.body = body
            content.sound = .default

            let identifier = (params["id"] as? String).flatMap { $0.isEmpty ? nil : $0 } ?? UUID().uuidString

            var trigger: UNNotificationTrigger?
            if let delay = IrgoNotificationsPlugin.doubleValue(params["delaySeconds"]), delay > 0 {
                trigger = UNTimeIntervalNotificationTrigger(timeInterval: delay, repeats: false)
            }

            let request = UNNotificationRequest(identifier: identifier, content: content, trigger: trigger)
            UNUserNotificationCenter.current().add(request) { error in
                if let error = error {
                    completion(.failure(error))
                } else {
                    completion(.success(["id": identifier]))
                }
            }
            return true
        default:
            return false
        }
    }

    private static func doubleValue(_ value: Any?) -> Double? {
        if let number = value as? NSNumber {
            return number.doubleValue
        }
        if let string = value as? String {
            return Double(string)
        }
        return nil
    }
}
