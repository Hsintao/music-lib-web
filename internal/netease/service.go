package netease

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/guohuiyuan/music-lib/model"
	libnetease "github.com/guohuiyuan/music-lib/netease"
	"github.com/guohuiyuan/music-lib/utils"
)

var numericID = regexp.MustCompile(`^\d+$`)

type Service struct {
	Client *http.Client
}

func New() *Service {
	return &Service{
		Client: &http.Client{Timeout: 2 * time.Minute},
	}
}

func NormalizePlaylistInput(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("playlist link is required")
	}
	if numericID.MatchString(input) {
		return "https://music.163.com/#/playlist?id=" + input, nil
	}
	return input, nil
}

func (s *Service) ParsePlaylist(ctx context.Context, link string, cookie string) (*model.Playlist, []model.Song, error) {
	normalized, err := NormalizePlaylistInput(link)
	if err != nil {
		return nil, nil, err
	}
	client := libnetease.New(strings.TrimSpace(cookie))
	type result struct {
		playlist *model.Playlist
		songs    []model.Song
		err      error
	}
	ch := make(chan result, 1)
	go func() {
		playlist, songs, err := client.ParsePlaylist(normalized)
		ch <- result{playlist: playlist, songs: songs, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case res := <-ch:
		return res.playlist, res.songs, res.err
	}
}

func PlaylistDownloadDir(root string, playlist *model.Playlist) string {
	name := "unknown"
	if playlist != nil && strings.TrimSpace(playlist.Name) != "" {
		name = playlist.Name
	}
	return filepath.Join(root, sanitizeFilename(name))
}

func SongFilename(index int, song model.Song) string {
	ext := strings.Trim(strings.TrimSpace(song.Ext), ".")
	if ext == "" {
		ext = "mp3"
	}
	name := sanitizeFilename(song.Name)
	artist := sanitizeFilename(song.Artist)
	return fmt.Sprintf("%s - %s.%s", name, artist, ext)
}

func ResolveSongPath(dir string, index int, song model.Song) (string, bool) {
	path := filepath.Join(dir, SongFilename(index, song))
	if _, err := os.Stat(path); err == nil {
		return path, true
	}
	return path, false
}

func (s *Service) DownloadSong(ctx context.Context, playlist *model.Playlist, song model.Song, index int, downloadRoot string, cookie string, quality string) (string, error) {
	dir := PlaylistDownloadDir(downloadRoot, playlist)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if path, exists := ResolveSongPath(dir, index, song); exists {
		return path, nil
	}
	applyQuality(&song, quality)

	downloadURL, err := libnetease.New(strings.TrimSpace(cookie)).GetDownloadURL(&song)
	if err != nil {
		return "", friendlyDownloadError(err)
	}
	if strings.TrimSpace(downloadURL) == "" {
		return "", errors.New("未获取到下载地址，可能是 VIP 或版权限制")
	}

	if song.Ext == "" {
		song.Ext = extFromURL(downloadURL)
	}
	path, exists := ResolveSongPath(dir, index, song)
	if exists {
		return path, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("下载 HTTP 状态异常: %d", resp.StatusCode)
	}

	tmp := path + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(out, resp.Body)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return "", closeErr
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return path, nil
}

func applyQuality(song *model.Song, quality string) {
	if song.Extra == nil {
		song.Extra = map[string]string{}
	}
	switch quality {
	case "lossless":
		song.Extra["netease_level"] = "lossless"
	default:
		song.Extra["netease_level"] = "standard"
		if song.Ext == "" {
			song.Ext = "mp3"
		}
	}
}

func sanitizeFilename(name string) string {
	return utils.SanitizeFilename(strings.TrimSpace(name))
}

func friendlyDownloadError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "vip") || strings.Contains(msg, "copyright") || strings.Contains(msg, "download url not found") {
		return errors.New("未获取到下载地址，可能是 VIP 或版权限制")
	}
	return err
}

func extFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err == nil {
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(parsed.Path)), ".")
		switch ext {
		case "mp3", "flac", "m4a":
			return ext
		}
	}
	return "mp3"
}
