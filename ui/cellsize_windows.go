//go:build windows

package ui

// terminalCellAspect has no window-size ioctl to consult on Windows, so the
// caller keeps its default.
func terminalCellAspect() float64 { return 0 }
