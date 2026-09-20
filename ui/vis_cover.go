package ui

import "time"

// The Cover visualizer: the playing track's album art, drawn as a real image
// where the terminal supports it (see cover.go), and the track's name where it
// does not — the same fallback the Lyrics mode uses.
type coverDriver struct {
	ctx VisCoverContext
}

func newCoverDriver() visModeDriver { return &coverDriver{} }

// AnalysisSpec is empty: this mode draws a picture, not audio.
func (*coverDriver) AnalysisSpec(*Visualizer) VisAnalysisSpec { return VisAnalysisSpec{} }

func (d *coverDriver) Tick(_ *Visualizer, ctx VisTickContext) {
	d.ctx = ctx.Cover
	SetCoverArt(ctx.Cover.ArtURL)
}

// A cover changes only with the track, so there is nothing to animate.
func (*coverDriver) TickInterval(*Visualizer, VisTickContext) time.Duration { return TickSlow }

func (*coverDriver) OnEnter(*Visualizer) {}
func (*coverDriver) OnLeave(*Visualizer) {}

func (d *coverDriver) Render(v *Visualizer) string {
	if out, ok := RenderCover(v.Rows, PanelWidth); ok {
		return out
	}
	return centerTwoLines(v.Rows, d.ctx.TrackTitle, d.ctx.TrackArtist)
}

// VisCoverContext is what the Cover visualizer draws from, resolved by the
// model once per tick so the driver never reaches into playlist state.
type VisCoverContext struct {
	// ArtURL is the playing track's artwork: an http(s) URL from a provider,
	// or a file:// URL for art cached from a local file's tags.
	ArtURL      string
	TrackTitle  string
	TrackArtist string
}
