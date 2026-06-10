package osutil

import "errors"

// ErrNoGUISelection indicates a GUI selection helper is unavailable.
// Exported so callers can detect this state.
var ErrNoGUISelection = errors.New("no GUI selection helper available")
