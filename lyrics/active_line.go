package lyrics

import "time"

// ActiveLineIndex returns the index of the line that should read as "playing
// now" at pos: the last line whose Start has already passed. It returns -1
// when pos is before the first line, or when there are no lines — a lead-in
// with nothing to highlight yet.
//
// lines must be ordered by Start ascending, which is how both Fetch and
// ParseEmbedded return them; the scan stops at the first line in the future.
func ActiveLineIndex(lines []Line, pos time.Duration) int {
	active := -1
	for i, line := range lines {
		if line.Start > pos {
			break
		}
		active = i
	}
	return active
}
