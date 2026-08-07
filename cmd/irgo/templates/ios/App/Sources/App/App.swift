import UIKit

/// UIKit entry point hosting the Irgo WebView. Used by the SwiftPM /
/// xtool build (`irgo app run ios` on Linux). The Xcode project under
/// ios/Example uses an AppDelegate + SceneDelegate pair instead.
///
/// Deliberately UIKit-only (no SwiftUI): SwiftUI apps built against recent
/// SDKs pull in SwiftUICore, which does not exist on devices running
/// iOS < 18 and crashes at launch when cross-linked with ld64.lld.
@main
class AppDelegate: UIResponder, UIApplicationDelegate {
    var window: UIWindow?

    func application(
        _ application: UIApplication,
        didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?
    ) -> Bool {
        let window = UIWindow(frame: UIScreen.main.bounds)
        window.rootViewController = IrgoWebViewController()
        window.makeKeyAndVisible()
        self.window = window
        return true
    }
}
