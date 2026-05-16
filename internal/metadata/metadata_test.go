package metadata

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	id3v2 "github.com/bogem/id3v2/v2"
	"github.com/go-flac/flacpicture"
	"github.com/go-flac/flacvorbis"
	flac "github.com/go-flac/go-flac"
)

func TestEmbedMP3WritesTitleLyricsAndCover(t *testing.T) {
	path := filepath.Join(t.TempDir(), "song.mp3")
	if err := os.WriteFile(path, append([]byte{0xff, 0xfb, 0x90, 0x64}, make([]byte, 100)...), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := Embed(path, Tags{
		Title:      "Song",
		Artist:     "Artist",
		Album:      "Album",
		Lyrics:     "[00:00.00] lyric",
		CoverData:  tinyPNG(t),
		CoverMIME:  "image/png",
		TrackIndex: 3,
	})
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}

	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		t.Fatalf("Open tag: %v", err)
	}
	defer tag.Close()
	if tag.Title() != "Song" || tag.Artist() != "Artist" || tag.Album() != "Album" {
		t.Fatalf("tag fields = %q/%q/%q", tag.Title(), tag.Artist(), tag.Album())
	}
	if len(tag.GetFrames("USLT")) == 0 {
		t.Fatal("missing lyrics frame")
	}
	if len(tag.GetFrames("APIC")) == 0 {
		t.Fatal("missing picture frame")
	}
}

func TestEmbedFLACWritesVorbisLyricsAndPicture(t *testing.T) {
	path := filepath.Join(t.TempDir(), "song.flac")
	if err := os.WriteFile(path, minimalFLAC(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := Embed(path, Tags{
		Title:     "Song",
		Artist:    "Artist",
		Album:     "Album",
		Lyrics:    "[00:00.00] lyric",
		CoverData: tinyPNG(t),
		CoverMIME: "image/png",
	})
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}

	file, err := flac.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	var foundVorbis, foundPicture bool
	for _, block := range file.Meta {
		switch block.Type {
		case flac.VorbisComment:
			foundVorbis = true
			vc, err := flacvorbis.ParseFromMetaDataBlock(*block)
			if err != nil {
				t.Fatalf("Parse vorbis: %v", err)
			}
			values, err := vc.Get("LYRICS")
			if err != nil {
				t.Fatalf("Get lyrics: %v", err)
			}
			if len(values) != 1 || values[0] != "[00:00.00] lyric" {
				t.Fatalf("lyrics = %#v", values)
			}
		case flac.Picture:
			foundPicture = true
			if _, err := flacpicture.ParseFromMetaDataBlock(*block); err != nil {
				t.Fatalf("Parse picture: %v", err)
			}
		}
	}
	if !foundVorbis || !foundPicture {
		t.Fatalf("found vorbis=%v picture=%v", foundVorbis, foundPicture)
	}
}

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png Encode: %v", err)
	}
	return buf.Bytes()
}

func minimalFLAC() []byte {
	streamInfo := make([]byte, 34)
	buf := bytes.NewBufferString("fLaC")
	buf.Write((&flac.MetaDataBlock{Type: flac.StreamInfo, Data: streamInfo}).Marshal(true))
	buf.Write([]byte{0xff, 0xf8, 0, 0})
	return buf.Bytes()
}
