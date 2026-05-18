package netease

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/guohuiyuan/music-lib/model"
)

func TestNormalizePlaylistInputAcceptsID(t *testing.T) {
	got, err := NormalizePlaylistInput("123456")
	if err != nil {
		t.Fatalf("NormalizePlaylistInput returned error: %v", err)
	}
	want := "https://music.163.com/#/playlist?id=123456"
	if got != want {
		t.Fatalf("NormalizePlaylistInput = %q, want %q", got, want)
	}
}

func TestNormalizePlaylistInputRejectsEmpty(t *testing.T) {
	if _, err := NormalizePlaylistInput("   "); err == nil {
		t.Fatal("NormalizePlaylistInput returned nil error for empty input")
	}
}

func TestPlaylistDownloadDirUsesPlaylistName(t *testing.T) {
	root := t.TempDir()
	playlist := &model.Playlist{Name: `我的/歌单:*?`, ID: "42"}

	got := PlaylistDownloadDir(root, playlist)
	want := filepath.Join(root, "我的_歌单___")
	if got != want {
		t.Fatalf("PlaylistDownloadDir = %q, want %q", got, want)
	}
}

func TestSongFilenameUsesSongAndArtistWithoutIndex(t *testing.T) {
	song := model.Song{Name: `歌/名`, Artist: `歌:手`, Ext: "flac"}

	got := SongFilename(7, song)
	want := "歌_名 - 歌_手.flac"
	if got != want {
		t.Fatalf("SongFilename = %q, want %q", got, want)
	}
}

func TestResolveSongPathSkipsExistingFile(t *testing.T) {
	dir := t.TempDir()
	song := model.Song{Name: "Song", Artist: "Artist", Ext: "mp3"}
	path := filepath.Join(dir, "Song - Artist.mp3")
	if err := os.WriteFile(path, []byte("exists"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, exists := ResolveSongPath(dir, 1, song)
	if got != path {
		t.Fatalf("ResolveSongPath path = %q, want %q", got, path)
	}
	if !exists {
		t.Fatal("ResolveSongPath exists = false, want true")
	}
}

func TestResolveDownloadURLFallsBackFromLosslessToMP3(t *testing.T) {
	service := New()
	service.fallbacks = nil
	var levels []string
	service.neteaseURLResolver = func(cookie string, song *model.Song) (string, error) {
		levels = append(levels, song.Extra["netease_level"])
		if song.Extra["netease_level"] == "lossless" {
			return "", errors.New("lossless unavailable")
		}
		return "https://example.com/song.mp3", nil
	}

	url, song, source, fromFallback, err := service.resolveDownloadURL(nil, model.Song{Name: "Song", Artist: "Artist"}, "", "lossless")
	if err != nil {
		t.Fatalf("resolveDownloadURL returned error: %v", err)
	}
	if url != "https://example.com/song.mp3" {
		t.Fatalf("url = %q, want mp3 url", url)
	}
	if source != "netease-mp3" {
		t.Fatalf("source = %q, want netease-mp3", source)
	}
	if fromFallback {
		t.Fatal("fromFallback = true, want false")
	}
	if song.Ext != "mp3" {
		t.Fatalf("song.Ext = %q, want mp3", song.Ext)
	}
	if len(levels) != 2 || levels[0] != "lossless" || levels[1] != "standard" {
		t.Fatalf("levels = %#v, want lossless then standard", levels)
	}
}
