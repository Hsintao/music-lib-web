package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guohuiyuan/music-lib/model"
	"music-lib-web/internal/config"
	"music-lib-web/internal/jobs"
)

type fakeMusicService struct{}

func (fakeMusicService) ParsePlaylist(ctx context.Context, link string) (*model.Playlist, []model.Song, error) {
	if strings.TrimSpace(link) == "" {
		return nil, nil, ErrBadRequest("playlist link is required")
	}
	return &model.Playlist{ID: "42", Name: "测试歌单", TrackCount: 1}, []model.Song{{ID: "1", Name: "歌曲", Artist: "歌手"}}, nil
}

type fakeJobDownloader struct{}

func (fakeJobDownloader) DownloadSong(ctx context.Context, playlist *model.Playlist, song model.Song, index int, downloadRoot string) (string, error) {
	return "/tmp/song.mp3", nil
}

func TestParsePlaylistRejectsEmptyLink(t *testing.T) {
	api := New(config.Default(), fakeMusicService{}, jobs.NewStore(fakeJobDownloader{}, 1))
	req := httptest.NewRequest(http.MethodPost, "/api/playlists/parse", bytes.NewBufferString(`{"link":" "}`))
	rec := httptest.NewRecorder()

	api.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateJobAndGetStatus(t *testing.T) {
	api := New(config.Default(), fakeMusicService{}, jobs.NewStore(fakeJobDownloader{}, 1))
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
	api := New(config.Default(), fakeMusicService{}, jobs.NewStore(fakeJobDownloader{}, 1))
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
