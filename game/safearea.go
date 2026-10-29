package game

import "sync"

// The safe area is how many screen pixels along each edge are covered by
// system bars or display cutouts (a phone's camera notch), so UI should avoid
// them. The Android side reports it from the UI thread, hence the lock.
var safeArea struct {
	sync.Mutex
	left, top, right, bottom float64
}

// SetSafeArea records the screen insets, in pixels, that UI should stay clear of.
func SetSafeArea(left, top, right, bottom float64) {
	safeArea.Lock()
	defer safeArea.Unlock()
	safeArea.left, safeArea.top, safeArea.right, safeArea.bottom = left, top, right, bottom
}

func currentSafeArea() (left, top, right, bottom float64) {
	safeArea.Lock()
	defer safeArea.Unlock()
	return safeArea.left, safeArea.top, safeArea.right, safeArea.bottom
}
