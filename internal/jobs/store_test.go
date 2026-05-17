package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/guohuiyuan/music-lib/model"
)

type fakeDownloader struct {
	fail        map[string]bool
	lastRoot    string
	lastCookie  string
	lastQuality string
	block       chan struct{}
	started     chan struct{}
}

func (f *fakeDownloader) DownloadSong(ctx context.Context, playlist *model.Playlist, song model.Song, index int, downloadRoot string, cookie string, quality string) (DownloadResult, error) {
	f.lastRoot = downloadRoot
	f.lastCookie = cookie
	f.lastQuality = quality
	if f.started != nil {
		select {
		case f.started <- struct{}{}:
		default:
		}
	}
	if f.block != nil {
		select {
		case <-ctx.Done():
			return DownloadResult{}, ctx.Err()
		case <-f.block:
		}
	}
	if f.fail[song.ID] {
		return DownloadResult{}, errors.New("network failed")
	}
	return DownloadResult{FilePath: "/tmp/" + song.ID + ".mp3", Source: "netease"}, nil
}

func TestJobCompletesWithErrorsAndRetriesFailures(t *testing.T) {
	downloader := &fakeDownloader{fail: map[string]bool{"2": true}}
	store := NewStore(downloader, 2)
	playlist := &model.Playlist{ID: "p1", Name: "列表"}
	songs := []model.Song{
		{ID: "1", Name: "A", Artist: "AA"},
		{ID: "2", Name: "B", Artist: "BB"},
	}

	job := store.Create(playlist, songs, "/tmp/music", "", "mp3")
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
	job := store.Create(playlist, []model.Song{{ID: "1", Name: "A", Artist: "AA"}}, "/custom/root", "", "mp3")

	store.Run(context.Background(), job.ID)

	if downloader.lastRoot != "/custom/root" {
		t.Fatalf("download root = %q, want %q", downloader.lastRoot, "/custom/root")
	}
}

func TestListReturnsJobsNewestFirstWithoutCookie(t *testing.T) {
	downloader := &fakeDownloader{fail: map[string]bool{}}
	store := NewStore(downloader, 1)
	playlist := &model.Playlist{ID: "p1", Name: "列表"}
	first := store.Create(playlist, []model.Song{{ID: "1", Name: "A", Artist: "AA"}}, "/tmp/music", "MUSIC_U=secret", "mp3")
	time.Sleep(time.Nanosecond)
	second := store.Create(playlist, []model.Song{{ID: "2", Name: "B", Artist: "BB"}}, "/tmp/music", "", "mp3")

	got := store.List()
	if len(got) != 2 {
		t.Fatalf("List length = %d, want 2", len(got))
	}
	if got[0].ID != second.ID || got[1].ID != first.ID {
		t.Fatalf("List order = %s, %s; want newest first", got[0].ID, got[1].ID)
	}
	if got[1].Cookie != "" {
		t.Fatal("job list exposed cookie")
	}
}

func TestJobPassesCookieToDownloaderWithoutExposingIt(t *testing.T) {
	downloader := &fakeDownloader{fail: map[string]bool{}}
	store := NewStore(downloader, 1)
	playlist := &model.Playlist{ID: "p1", Name: "列表"}
	job := store.Create(playlist, []model.Song{{ID: "1", Name: "A", Artist: "AA"}}, "/tmp/music", "MUSIC_U=secret", "mp3")

	store.Run(context.Background(), job.ID)

	if downloader.lastCookie != "MUSIC_U=secret" {
		t.Fatalf("cookie = %q, want cookie passed to downloader", downloader.lastCookie)
	}
	got, _ := store.Get(job.ID)
	if got.Cookie != "" {
		t.Fatal("job snapshot exposed cookie")
	}
}

func TestJobPassesQualityToDownloader(t *testing.T) {
	downloader := &fakeDownloader{fail: map[string]bool{}}
	store := NewStore(downloader, 1)
	playlist := &model.Playlist{ID: "p1", Name: "列表"}
	job := store.Create(playlist, []model.Song{{ID: "1", Name: "A", Artist: "AA"}}, "/tmp/music", "", "lossless")

	store.Run(context.Background(), job.ID)

	if downloader.lastQuality != "lossless" {
		t.Fatalf("quality = %q, want lossless", downloader.lastQuality)
	}
	got, _ := store.Get(job.ID)
	if got.Quality != "lossless" {
		t.Fatalf("job quality = %q, want lossless", got.Quality)
	}
}

func TestCancelRunningJob(t *testing.T) {
	downloader := &fakeDownloader{
		fail:    map[string]bool{},
		block:   make(chan struct{}),
		started: make(chan struct{}, 1),
	}
	store := NewStore(downloader, 1)
	playlist := &model.Playlist{ID: "p1", Name: "列表"}
	job := store.Create(playlist, []model.Song{{ID: "1", Name: "A", Artist: "AA"}}, "/tmp/music", "", "mp3")

	done := make(chan struct{})
	go func() {
		store.Run(context.Background(), job.ID)
		close(done)
	}()
	select {
	case <-downloader.started:
	case <-time.After(time.Second):
		t.Fatal("download did not start")
	}
	if err := store.Cancel(job.ID); err != nil {
		t.Fatalf("Cancel returned error: %v", err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("run did not finish after cancel")
	}

	got, _ := store.Get(job.ID)
	if got.Status != StatusCanceled {
		t.Fatalf("Status = %q, want %q", got.Status, StatusCanceled)
	}
}
