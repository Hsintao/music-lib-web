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

	"github.com/guohuiyuan/music-lib/bilibili"
	"github.com/guohuiyuan/music-lib/fivesing"
	"github.com/guohuiyuan/music-lib/jamendo"
	"github.com/guohuiyuan/music-lib/joox"
	"github.com/guohuiyuan/music-lib/kugou"
	"github.com/guohuiyuan/music-lib/kuwo"
	"github.com/guohuiyuan/music-lib/migu"
	"github.com/guohuiyuan/music-lib/model"
	libnetease "github.com/guohuiyuan/music-lib/netease"
	"github.com/guohuiyuan/music-lib/qianqian"
	"github.com/guohuiyuan/music-lib/qq"
	"github.com/guohuiyuan/music-lib/soda"
	"github.com/guohuiyuan/music-lib/utils"
	"music-lib-web/internal/jobs"
	"music-lib-web/internal/metadata"
)

var numericID = regexp.MustCompile(`^\d+$`)

type Service struct {
	Client             *http.Client
	fallbacks          []fallbackSource
	neteaseURLResolver func(cookie string, song *model.Song) (string, error)
}

type fallbackSource struct {
	Name           string
	Search         func(keyword string) ([]model.Song, error)
	GetDownloadURL func(song *model.Song) (string, error)
}

func New() *Service {
	return &Service{
		Client:             &http.Client{Timeout: 2 * time.Minute},
		fallbacks:          defaultFallbackSources(),
		neteaseURLResolver: resolveNeteaseURL,
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

func (s *Service) DownloadSong(ctx context.Context, playlist *model.Playlist, song model.Song, index int, downloadRoot string, cookie string, quality string) (jobs.DownloadResult, error) {
	dir := PlaylistDownloadDir(downloadRoot, playlist)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return jobs.DownloadResult{}, err
	}
	if path, exists := ResolveSongPath(dir, index, song); exists {
		return jobs.DownloadResult{FilePath: path, Source: "cache"}, nil
	}
	applyQuality(&song, quality)

	downloadURL, chosenSong, source, fromFallback, err := s.resolveDownloadURL(ctx, song, cookie, quality)
	if err != nil {
		return jobs.DownloadResult{}, err
	}
	song = chosenSong
	if song.Ext == "" {
		song.Ext = extFromURL(downloadURL)
	}

	path, exists := ResolveSongPath(dir, index, song)
	if exists {
		return jobs.DownloadResult{FilePath: path, Source: "cache"}, nil
	}

	if err := s.downloadToPath(ctx, downloadURL, path); err != nil {
		if fromFallback {
			return jobs.DownloadResult{}, err
		}
		// Primary source failed at transfer stage, attempt cross-source URL fallback.
		altURL, altSong, altSource, _, altErr := s.resolveFallbackURL(ctx, song)
		if altErr != nil {
			return jobs.DownloadResult{}, err
		}
		song = altSong
		source = altSource
		if song.Ext == "" {
			song.Ext = extFromURL(altURL)
		}
		path = filepath.Join(dir, SongFilename(index, song))
		if _, exists := ResolveSongPath(dir, index, song); exists {
			return jobs.DownloadResult{FilePath: path, Source: "cache"}, nil
		}
		if err := s.downloadToPath(ctx, altURL, path); err != nil {
			return jobs.DownloadResult{}, err
		}
	}

	_ = s.embedMetadata(ctx, path, song, index, cookie)
	return jobs.DownloadResult{FilePath: path, Source: source}, nil
}

func (s *Service) resolveDownloadURL(ctx context.Context, song model.Song, cookie string, quality string) (string, model.Song, string, bool, error) {
	url, err := s.neteaseURLResolver(strings.TrimSpace(cookie), &song)
	if err == nil && strings.TrimSpace(url) != "" {
		return url, song, "netease", false, nil
	}
	return s.resolveFallbackURL(ctx, song)
}

func (s *Service) resolveFallbackURL(ctx context.Context, song model.Song) (string, model.Song, string, bool, error) {
	keyword := strings.TrimSpace(song.Name + " " + song.Artist)
	if keyword == "" {
		return "", song, "", false, errors.New("未获取到下载地址，可能是 VIP 或版权限制")
	}
	for _, src := range s.fallbacks {
		candidates, err := callSearchWithTimeout(ctx, src.Search, keyword, 8*time.Second)
		if err != nil || len(candidates) == 0 {
			continue
		}
		for _, candidate := range rankCandidates(song, candidates) {
			url, urlErr := callURLWithTimeout(ctx, src.GetDownloadURL, &candidate, 8*time.Second)
			if urlErr != nil || strings.TrimSpace(url) == "" {
				continue
			}
			if candidate.Ext == "" {
				candidate.Ext = extFromURL(url)
			}
			return url, candidate, src.Name, true, nil
		}
	}
	return "", song, "", false, errors.New("未获取到下载地址，且自动换源失败")
}

func (s *Service) downloadToPath(ctx context.Context, downloadURL string, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("下载 HTTP 状态异常: %d", resp.StatusCode)
	}

	tmp := path + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, resp.Body)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (s *Service) embedMetadata(ctx context.Context, path string, song model.Song, index int, cookie string) error {
	client := libnetease.New(strings.TrimSpace(cookie))
	lyrics, _ := client.GetLyrics(&song)
	coverData, coverMIME, _ := s.fetchCover(ctx, song.Cover)
	return metadata.Embed(path, metadata.Tags{
		Title:      song.Name,
		Artist:     song.Artist,
		Album:      song.Album,
		Lyrics:     lyrics,
		CoverData:  coverData,
		CoverMIME:  coverMIME,
		TrackIndex: index,
	})
}

func (s *Service) fetchCover(ctx context.Context, coverURL string) ([]byte, string, error) {
	coverURL = strings.TrimSpace(coverURL)
	if coverURL == "" {
		return nil, "", nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, coverURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("cover HTTP status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 15*1024*1024))
	if err != nil {
		return nil, "", err
	}
	mime := normalizeImageMIME(resp.Header.Get("Content-Type"), data)
	if mime == "" {
		return nil, "", fmt.Errorf("unsupported cover content type %q", resp.Header.Get("Content-Type"))
	}
	return data, mime, nil
}

func normalizeImageMIME(contentType string, data []byte) string {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch contentType {
	case "image/jpeg", "image/png":
		return contentType
	}
	detected := http.DetectContentType(data)
	switch detected {
	case "image/jpeg", "image/png":
		return detected
	default:
		return ""
	}
}

func resolveNeteaseURL(cookie string, song *model.Song) (string, error) {
	return libnetease.New(strings.TrimSpace(cookie)).GetDownloadURL(song)
}

func defaultFallbackSources() []fallbackSource {
	return []fallbackSource{
		{Name: "qq", Search: qq.Search, GetDownloadURL: qq.GetDownloadURL},
		{Name: "kugou", Search: kugou.Search, GetDownloadURL: kugou.GetDownloadURL},
		{Name: "kuwo", Search: kuwo.Search, GetDownloadURL: kuwo.GetDownloadURL},
		{Name: "migu", Search: migu.Search, GetDownloadURL: migu.GetDownloadURL},
		{Name: "qianqian", Search: qianqian.Search, GetDownloadURL: qianqian.GetDownloadURL},
		{Name: "soda", Search: soda.Search, GetDownloadURL: soda.GetDownloadURL},
		{Name: "jamendo", Search: jamendo.Search, GetDownloadURL: jamendo.GetDownloadURL},
		{Name: "joox", Search: joox.Search, GetDownloadURL: joox.GetDownloadURL},
		{Name: "bilibili", Search: bilibili.Search, GetDownloadURL: bilibili.GetDownloadURL},
		{Name: "fivesing", Search: fivesing.Search, GetDownloadURL: fivesing.GetDownloadURL},
	}
}

func callSearchWithTimeout(ctx context.Context, fn func(string) ([]model.Song, error), keyword string, timeout time.Duration) ([]model.Song, error) {
	type result struct {
		songs []model.Song
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		songs, err := fn(keyword)
		ch <- result{songs: songs, err: err}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, context.DeadlineExceeded
	case res := <-ch:
		return res.songs, res.err
	}
}

func callURLWithTimeout(ctx context.Context, fn func(*model.Song) (string, error), song *model.Song, timeout time.Duration) (string, error) {
	type result struct {
		url string
		err error
	}
	ch := make(chan result, 1)
	cp := *song
	go func() {
		url, err := fn(&cp)
		ch <- result{url: url, err: err}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-timer.C:
		return "", context.DeadlineExceeded
	case res := <-ch:
		return res.url, res.err
	}
}

func rankCandidates(target model.Song, candidates []model.Song) []model.Song {
	best := make([]model.Song, 0, len(candidates))
	rest := make([]model.Song, 0, len(candidates))
	targetName := normalizeToken(target.Name)
	targetArtist := normalizeToken(target.Artist)
	for _, song := range candidates {
		name := normalizeToken(song.Name)
		artist := normalizeToken(song.Artist)
		if name == targetName && (targetArtist == "" || strings.Contains(artist, targetArtist) || strings.Contains(targetArtist, artist)) {
			best = append(best, song)
		} else {
			rest = append(rest, song)
		}
	}
	return append(best, rest...)
}

func normalizeToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "", "、", "", ",", "", "/", "", "-", "", "_", "", "·", "", ".", "", "(", "", ")", "")
	return replacer.Replace(value)
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
