package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/guohuiyuan/music-lib/model"
)

type fakeDownloader struct {
	fail       map[string]bool
	lastRoot   string
	lastCookie string
}

func (f *fakeDownloader) DownloadSong(ctx context.Context, playlist *model.Playlist, song model.Song, index int, downloadRoot string, cookie string) (string, error) {
	f.lastRoot = downloadRoot
	f.lastCookie = cookie
	if f.fail[song.ID] {
		return "", errors.New("network failed")
	}
	return "/tmp/" + song.ID + ".mp3", nil
}

func TestJobCompletesWithErrorsAndRetriesFailures(t *testing.T) {
	downloader := &fakeDownloader{fail: map[string]bool{"2": true}}
	store := NewStore(downloader, 2)
	playlist := &model.Playlist{ID: "p1", Name: "列表"}
	songs := []model.Song{
		{ID: "1", Name: "A", Artist: "AA"},
		{ID: "2", Name: "B", Artist: "BB"},
	}

	job := store.Create(playlist, songs, "/tmp/music", "")
	store.Run(context.Background(), job.ID)

	got, ok := store.Get(job.ID)
	if !ok {
		t.Fatal("job not found")
	}
	if got.Status != StatusCompletedWithErrors {
		t.Fatalf("Status = %q, want %q", got.Status, StatusCompletedWithErrors)
	}
	if got.SuccessCount != 1 || got.FailureCount != 1 {
		t.Fatalf("counts = success %d failure %d, want 1/1", got.SuccessCount, got.FailureCount)
	}

	downloader.fail["2"] = false
	if err := store.RetryFailures(context.Background(), job.ID); err != nil {
		t.Fatalf("RetryFailures returned error: %v", err)
	}
	got, _ = store.Get(job.ID)
	if got.Status != StatusCompleted {
		t.Fatalf("Status after retry = %q, want %q", got.Status, StatusCompleted)
	}
	if got.SuccessCount != 2 || got.FailureCount != 0 {
		t.Fatalf("counts after retry = success %d failure %d, want 2/0", got.SuccessCount, got.FailureCount)
	}
}

func TestJobPassesCustomDownloadRootToDownloader(t *testing.T) {
	downloader := &fakeDownloader{fail: map[string]bool{}}
	store := NewStore(downloader, 1)
	playlist := &model.Playlist{ID: "p1", Name: "列表"}
	job := store.Create(playlist, []model.Song{{ID: "1", Name: "A", Artist: "AA"}}, "/custom/root", "")

	store.Run(context.Background(), job.ID)

	if downloader.lastRoot != "/custom/root" {
		t.Fatalf("download root = %q, want %q", downloader.lastRoot, "/custom/root")
	}
}

func TestJobPassesCookieToDownloaderWithoutExposingIt(t *testing.T) {
	downloader := &fakeDownloader{fail: map[string]bool{}}
	store := NewStore(downloader, 1)
	playlist := &model.Playlist{ID: "p1", Name: "列表"}
	job := store.Create(playlist, []model.Song{{ID: "1", Name: "A", Artist: "AA"}}, "/tmp/music", "MUSIC_U=secret")

	store.Run(context.Background(), job.ID)

	if downloader.lastCookie != "MUSIC_U=secret" {
		t.Fatalf("cookie = %q, want cookie passed to downloader", downloader.lastCookie)
	}
	got, _ := store.Get(job.ID)
	if got.Cookie != "" {
		t.Fatal("job snapshot exposed cookie")
	}
}
