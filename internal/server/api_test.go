package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guohuiyuan/music-lib/model"
	"music-lib-web/internal/config"
	"music-lib-web/internal/jobs"
)

type fakeMusicService struct {
	cookies []string
}

func (f *fakeMusicService) ParsePlaylist(ctx context.Context, link string, cookie string) (*model.Playlist, []model.Song, error) {
	f.cookies = append(f.cookies, cookie)
	if strings.TrimSpace(link) == "" {
		return nil, nil, ErrBadRequest("playlist link is required")
	}
	return &model.Playlist{ID: "42", Name: "测试歌单", TrackCount: 1}, []model.Song{{ID: "1", Name: "歌曲", Artist: "歌手"}}, nil
}

type fakeJobDownloader struct{}

func (fakeJobDownloader) DownloadSong(ctx context.Context, playlist *model.Playlist, song model.Song, index int, downloadRoot string, cookie string) (string, error) {
	return "/tmp/song.mp3", nil
}

func TestParsePlaylistRejectsEmptyLink(t *testing.T) {
	api := New(config.Default(), &fakeMusicService{}, jobs.NewStore(fakeJobDownloader{}, 1))
	req := httptest.NewRequest(http.MethodPost, "/api/playlists/parse", bytes.NewBufferString(`{"link":" "}`))
	rec := httptest.NewRecorder()

	api.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateJobAndGetStatus(t *testing.T) {
	api := New(config.Default(), &fakeMusicService{}, jobs.NewStore(fakeJobDownloader{}, 1))
	body := bytes.NewBufferString(`{"playlist_link":"https://music.163.com/#/playlist?id=42"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/jobs", body)
	rec := httptest.NewRecorder()

	api.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("Unmarshal create response: %v", err)
	}
	if created.ID == "" {
		t.Fatal("created job id is empty")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/jobs/"+created.ID, nil)
	rec = httptest.NewRecorder()
	api.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestCreateJobAcceptsCustomDownloadDir(t *testing.T) {
	api := New(config.Default(), &fakeMusicService{}, jobs.NewStore(fakeJobDownloader{}, 1))
	body := bytes.NewBufferString(`{"playlist_link":"https://music.163.com/#/playlist?id=42","download_dir":"/tmp/custom-music"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/jobs", body)
	rec := httptest.NewRecorder()

	api.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	var created struct {
		DownloadDir string `json:"download_dir"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("Unmarshal create response: %v", err)
	}
	if created.DownloadDir != "/tmp/custom-music" {
		t.Fatalf("DownloadDir = %q, want custom dir", created.DownloadDir)
	}
}

func TestCreateJobAcceptsCookieWithoutEchoingIt(t *testing.T) {
	api := New(config.Default(), &fakeMusicService{}, jobs.NewStore(fakeJobDownloader{}, 1))
	body := bytes.NewBufferString(`{"playlist_link":"https://music.163.com/#/playlist?id=42","cookie":"MUSIC_U=secret"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/jobs", body)
	rec := httptest.NewRecorder()

	api.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "MUSIC_U=secret") {
		t.Fatalf("response exposed cookie: %s", rec.Body.String())
	}
}

func TestCookieIsRememberedForLaterTasks(t *testing.T) {
	music := &fakeMusicService{}
	api := New(config.Default(), music, jobs.NewStore(fakeJobDownloader{}, 1))

	first := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewBufferString(`{"playlist_link":"https://music.163.com/#/playlist?id=42","cookie":"MUSIC_U=secret"}`))
	api.ServeHTTP(httptest.NewRecorder(), first)
	second := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewBufferString(`{"playlist_link":"https://music.163.com/#/playlist?id=43"}`))
	api.ServeHTTP(httptest.NewRecorder(), second)

	if len(music.cookies) != 2 {
		t.Fatalf("recorded cookies = %d, want 2", len(music.cookies))
	}
	if music.cookies[0] != "MUSIC_U=secret" || music.cookies[1] != "MUSIC_U=secret" {
		t.Fatalf("cookies = %#v, want remembered cookie reused", music.cookies)
	}
}

func TestCookieIsLoadedFromFileForLaterTasks(t *testing.T) {
	music := &fakeMusicService{}
	cfg := config.Default()
	cfg.CookieFile = filepath.Join(t.TempDir(), "cookie")
	if err := os.WriteFile(cfg.CookieFile, []byte("MUSIC_U=from-file\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	api := New(cfg, music, jobs.NewStore(fakeJobDownloader{}, 1))

	req := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewBufferString(`{"playlist_link":"https://music.163.com/#/playlist?id=43"}`))
	api.ServeHTTP(httptest.NewRecorder(), req)

	if len(music.cookies) != 1 || music.cookies[0] != "MUSIC_U=from-file" {
		t.Fatalf("cookies = %#v, want cookie loaded from file", music.cookies)
	}
}

func TestCookieIsWrittenToFileWhenProvided(t *testing.T) {
	cfg := config.Default()
	cfg.CookieFile = filepath.Join(t.TempDir(), "cookie")
	api := New(cfg, &fakeMusicService{}, jobs.NewStore(fakeJobDownloader{}, 1))

	req := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewBufferString(`{"playlist_link":"https://music.163.com/#/playlist?id=42","cookie":"MUSIC_U=written"}`))
	api.ServeHTTP(httptest.NewRecorder(), req)

	data, err := os.ReadFile(cfg.CookieFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "MUSIC_U=written\n" {
		t.Fatalf("cookie file = %q, want written cookie", string(data))
	}
	info, err := os.Stat(cfg.CookieFile)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("cookie file mode = %o, want 0600", info.Mode().Perm())
	}
}
