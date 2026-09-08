package spotify

import (
	"slices"
	"testing"

	playlist4pb "github.com/devgianlu/go-librespot/proto/spotify/playlist4"
	"google.golang.org/protobuf/proto"
)

func rootlistItem(uri string) *playlist4pb.Item {
	return &playlist4pb.Item{Uri: proto.String(uri)}
}

func TestParseRootlistItemsFlat(t *testing.T) {
	items := []*playlist4pb.Item{
		rootlistItem("spotify:playlist:aaa"),
		rootlistItem("spotify:playlist:bbb"),
	}

	info := parseRootlistItems(items)

	if len(info.folderOf) != 0 {
		t.Fatalf("expected no folders, got %v", info.folderOf)
	}
	if len(info.children) != 0 {
		t.Fatalf("expected no folder children, got %v", info.children)
	}
	if info.order["aaa"] != 0 || info.order["bbb"] != 1 {
		t.Fatalf("unexpected order: %v", info.order)
	}
}

func TestParseRootlistItemsSingleFolder(t *testing.T) {
	items := []*playlist4pb.Item{
		rootlistItem("spotify:playlist:root1"),
		rootlistItem("spotify:start-group:1a2b3c:Chill%20Vibes"),
		rootlistItem("spotify:playlist:inside1"),
		rootlistItem("spotify:playlist:inside2"),
		rootlistItem("spotify:end-group:1a2b3c"),
		rootlistItem("spotify:playlist:root2"),
	}

	info := parseRootlistItems(items)

	if _, ok := info.folderOf["root1"]; ok {
		t.Errorf("root1 should not be in a folder")
	}
	if _, ok := info.folderOf["root2"]; ok {
		t.Errorf("root2 should not be in a folder")
	}
	if got := info.folderOf["inside1"]; got != "Chill Vibes" {
		t.Errorf("inside1 folder = %q, want %q", got, "Chill Vibes")
	}
	if got := info.folderOf["inside2"]; got != "Chill Vibes" {
		t.Errorf("inside2 folder = %q, want %q", got, "Chill Vibes")
	}

	wantOrder := map[string]int{"root1": 0, "inside1": 1, "inside2": 2, "root2": 3}
	for id, want := range wantOrder {
		if info.order[id] != want {
			t.Errorf("order[%q] = %d, want %d", id, info.order[id], want)
		}
	}

	if got := info.children["Chill Vibes"]; !slices.Equal(got, []string{"inside1", "inside2"}) {
		t.Errorf("children[Chill Vibes] = %v, want [inside1 inside2]", got)
	}
	if !slices.Equal(info.paths, []string{"Chill Vibes"}) {
		t.Errorf("paths = %v, want [Chill Vibes]", info.paths)
	}
}

func TestParseRootlistItemsNestedFolders(t *testing.T) {
	items := []*playlist4pb.Item{
		rootlistItem("spotify:start-group:outer:Study"),
		rootlistItem("spotify:playlist:direct"),
		rootlistItem("spotify:start-group:inner:Focus"),
		rootlistItem("spotify:playlist:nested"),
		rootlistItem("spotify:end-group:inner"),
		rootlistItem("spotify:playlist:direct2"),
		rootlistItem("spotify:end-group:outer"),
	}

	info := parseRootlistItems(items)

	if got := info.folderOf["direct"]; got != "Study" {
		t.Errorf("direct folder = %q, want %q", got, "Study")
	}
	if got := info.folderOf["nested"]; got != "Study / Focus" {
		t.Errorf("nested folder = %q, want %q", got, "Study / Focus")
	}
	if got := info.folderOf["direct2"]; got != "Study" {
		t.Errorf("direct2 folder = %q, want %q", got, "Study")
	}

	// The outer folder's "play all" must include the nested subfolder's
	// tracks too, in rootlist order.
	if got := info.children["Study"]; !slices.Equal(got, []string{"direct", "nested", "direct2"}) {
		t.Errorf("children[Study] = %v, want [direct nested direct2]", got)
	}
	if got := info.children["Study / Focus"]; !slices.Equal(got, []string{"nested"}) {
		t.Errorf("children[Study / Focus] = %v, want [nested]", got)
	}
	if !slices.Equal(info.paths, []string{"Study", "Study / Focus"}) {
		t.Errorf("paths = %v, want [Study, Study / Focus]", info.paths)
	}
}

func TestParseRootlistItemsUnbalancedEndGroupIgnored(t *testing.T) {
	items := []*playlist4pb.Item{
		rootlistItem("spotify:end-group:stray"),
		rootlistItem("spotify:playlist:solo"),
	}

	info := parseRootlistItems(items)

	if _, ok := info.folderOf["solo"]; ok {
		t.Errorf("solo should not be in a folder after a stray end-group")
	}
	if info.order["solo"] != 0 {
		t.Errorf("order[solo] = %d, want 0", info.order["solo"])
	}
}

func TestParseRootlistItemsUnknownKindSkipped(t *testing.T) {
	items := []*playlist4pb.Item{
		rootlistItem("spotify:playlist:one"),
		rootlistItem("spotify:some-future-kind:xyz"),
		rootlistItem("spotify:playlist:two"),
	}

	info := parseRootlistItems(items)

	if info.order["one"] != 0 || info.order["two"] != 1 {
		t.Errorf("unknown item kind should not consume a position: %v", info.order)
	}
}

func TestFolderLeafName(t *testing.T) {
	cases := map[string]string{
		"Study":         "Study",
		"Study / Focus": "Focus",
		"A / B / C":     "C",
	}
	for path, want := range cases {
		if got := folderLeafName(path); got != want {
			t.Errorf("folderLeafName(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestIsSpotifyFolderID(t *testing.T) {
	path, ok := isSpotifyFolderID(spotifyFolderIDPrefix + "Chill / Study")
	if !ok || path != "Chill / Study" {
		t.Errorf("isSpotifyFolderID = %q, %v; want %q, true", path, ok, "Chill / Study")
	}
	if _, ok := isSpotifyFolderID("37i9dQZF1DXcBWIGoYBM5M"); ok {
		t.Errorf("a real playlist ID should not be treated as a folder ID")
	}
}
