package metadata

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	id3v2 "github.com/bogem/id3v2/v2"
	"github.com/go-flac/flacpicture"
	"github.com/go-flac/flacvorbis"
	flac "github.com/go-flac/go-flac"
)

type Tags struct {
	Title      string
	Artist     string
	Album      string
	Lyrics     string
	CoverData  []byte
	CoverMIME  string
	TrackIndex int
}

func Embed(path string, tags Tags) error {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(path), ".")) {
	case "mp3":
		return embedMP3(path, tags)
	case "flac":
		return embedFLAC(path, tags)
	default:
		return nil
	}
}

func embedMP3(path string, tags Tags) error {
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		return err
	}
	defer tag.Close()

	tag.SetDefaultEncoding(id3v2.EncodingUTF8)
	tag.SetTitle(tags.Title)
	tag.SetArtist(tags.Artist)
	tag.SetAlbum(tags.Album)
	if tags.TrackIndex > 0 {
		tag.AddTextFrame("TRCK", id3v2.EncodingUTF8, strconv.Itoa(tags.TrackIndex))
	}
	if strings.TrimSpace(tags.Lyrics) != "" {
		tag.DeleteFrames("USLT")
		tag.AddUnsynchronisedLyricsFrame(id3v2.UnsynchronisedLyricsFrame{
			Encoding:          id3v2.EncodingUTF8,
			Language:          "chi",
			ContentDescriptor: "lyrics",
			Lyrics:            tags.Lyrics,
		})
	}
	if len(tags.CoverData) > 0 && strings.TrimSpace(tags.CoverMIME) != "" {
		tag.DeleteFrames("APIC")
		tag.AddAttachedPicture(id3v2.PictureFrame{
			Encoding:    id3v2.EncodingUTF8,
			MimeType:    tags.CoverMIME,
			PictureType: id3v2.PTFrontCover,
			Description: "cover",
			Picture:     tags.CoverData,
		})
	}
	return tag.Save()
}

func embedFLAC(path string, tags Tags) error {
	file, err := flac.ParseFile(path)
	if err != nil {
		return err
	}

	meta := make([]*flac.MetaDataBlock, 0, len(file.Meta)+2)
	for _, block := range file.Meta {
		if block.Type == flac.VorbisComment || block.Type == flac.Picture {
			continue
		}
		meta = append(meta, block)
	}

	vc := flacvorbis.New()
	addVorbis(vc, flacvorbis.FIELD_TITLE, tags.Title)
	addVorbis(vc, flacvorbis.FIELD_ARTIST, tags.Artist)
	addVorbis(vc, flacvorbis.FIELD_ALBUM, tags.Album)
	if tags.TrackIndex > 0 {
		addVorbis(vc, flacvorbis.FIELD_TRACKNUMBER, strconv.Itoa(tags.TrackIndex))
	}
	addVorbis(vc, "LYRICS", tags.Lyrics)
	vorbis := vc.Marshal()
	meta = append(meta, &vorbis)

	if len(tags.CoverData) > 0 && strings.TrimSpace(tags.CoverMIME) != "" {
		picture, err := flacpicture.NewFromImageData(flacpicture.PictureTypeFrontCover, "cover", tags.CoverData, tags.CoverMIME)
		if err != nil {
			return err
		}
		block := picture.Marshal()
		meta = append(meta, &block)
	}

	file.Meta = meta
	return os.WriteFile(path, file.Marshal(), 0o644)
}

func addVorbis(vc *flacvorbis.MetaDataBlockVorbisComment, key string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if err := vc.Add(key, value); err != nil {
		panic(fmt.Sprintf("invalid static vorbis field %q: %v", key, err))
	}
}
