package mobile

import "sync"

// LifecycleHandler lets Go app code react to app lifecycle transitions.
// Register with SetLifecycleHandler; native shells drive the transitions.
type LifecycleHandler interface {
	// OnBackground is called when the app moves to the background.
	// Pause timers, flush state, stop non-essential work here.
	OnBackground()

	// OnForeground is called when the app returns to the foreground.
	OnForeground()
}

var (
	lifecycleMu      sync.RWMutex
	lifecycleHandler LifecycleHandler
)

// SetLifecycleHandler registers a Go-side lifecycle handler.
// Called from Go app code (not gomobile-bindable by design).
func SetLifecycleHandler(h LifecycleHandler) {
	lifecycleMu.Lock()
	defer lifecycleMu.Unlock()
	lifecycleHandler = h
}

// OnBackground is called by native code when the app enters the background
// (iOS: sceneDidEnterBackground, Android: onPause/onStop). The bridge
// persists its state (cookie jar) and notifies the app's handler.
func OnBackground() {
	bridgeMu.RLock()
	b := globalBridge
	bridgeMu.RUnlock()
	if b != nil && b.jar != nil {
		b.jar.save()
	}

	lifecycleMu.RLock()
	h := lifecycleHandler
	lifecycleMu.RUnlock()
	if h != nil {
		h.OnBackground()
	}
}

// OnForeground is called by native code when the app returns to the
// foreground (iOS: sceneWillEnterForeground, Android: onResume).
func OnForeground() {
	lifecycleMu.RLock()
	h := lifecycleHandler
	lifecycleMu.RUnlock()
	if h != nil {
		h.OnForeground()
	}
}
