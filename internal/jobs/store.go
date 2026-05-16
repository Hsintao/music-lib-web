package jobs

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/guohuiyuan/music-lib/model"
)

type Status string

const (
	StatusQueued              Status = "queued"
	StatusRunning             Status = "running"
	StatusCompleted           Status = "completed"
	StatusCompletedWithErrors Status = "completed_with_errors"
	StatusFailed              Status = "failed"
	StatusCanceled            Status = "canceled"
)

type SongStatus string

const (
	SongQueued  SongStatus = "queued"
	SongRunning SongStatus = "running"
	SongSuccess SongStatus = "success"
	SongFailed  SongStatus = "failed"
	SongSkipped SongStatus = "skipped"
)

type Downloader interface {
	DownloadSong(ctx context.Context, playlist *model.Playlist, song model.Song, index int, downloadRoot string, cookie string, quality string) (string, error)
}

type SongResult struct {
	SongID   string     `json:"song_id"`
	Name     string     `json:"name"`
	Artist   string     `json:"artist"`
	Status   SongStatus `json:"status"`
	FilePath string     `json:"file_path,omitempty"`
	Error    string     `json:"error,omitempty"`
}

type Job struct {
	ID           string         `json:"id"`
	Status       Status         `json:"status"`
	Playlist     model.Playlist `json:"playlist"`
	Total        int            `json:"total"`
	SuccessCount int            `json:"success_count"`
	FailureCount int            `json:"failure_count"`
	CurrentSong  string         `json:"current_song,omitempty"`
	DownloadDir  string         `json:"download_dir"`
	Quality      string         `json:"quality"`
	Results      []SongResult   `json:"results"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`

	songs  []model.Song
	Cookie string `json:"-"`
	cancel context.CancelFunc
}

type Store struct {
	mu          sync.RWMutex
	jobs        map[string]*Job
	downloader  Downloader
	concurrency int
}

func NewStore(downloader Downloader, concurrency int) *Store {
	if concurrency < 1 {
		concurrency = 1
	}
	return &Store{
		jobs:        map[string]*Job{},
		downloader:  downloader,
		concurrency: concurrency,
	}
}

func (s *Store) Create(playlist *model.Playlist, songs []model.Song, downloadDir string, cookie string, quality string) *Job {
	now := time.Now()
	quality = NormalizeQuality(quality)
	id := fmt.Sprintf("%d", now.UnixNano())
	results := make([]SongResult, len(songs))
	for i, song := range songs {
		results[i] = SongResult{
			SongID: song.ID,
			Name:   song.Name,
			Artist: song.Artist,
			Status: SongQueued,
		}
	}
	job := &Job{
		ID:          id,
		Status:      StatusQueued,
		Total:       len(songs),
		DownloadDir: downloadDir,
		Quality:     quality,
		Results:     results,
		CreatedAt:   now,
		UpdatedAt:   now,
		songs:       append([]model.Song(nil), songs...),
		Cookie:      cookie,
	}
	if playlist != nil {
		job.Playlist = *playlist
	}

	s.mu.Lock()
	s.jobs[id] = job
	s.mu.Unlock()
	return cloneJob(job)
}

func (s *Store) Get(id string) (*Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	if !ok {
		return nil, false
	}
	return cloneJob(job), true
}

func (s *Store) Run(ctx context.Context, id string) {
	s.run(ctx, id, false)
}

func (s *Store) RetryFailures(ctx context.Context, id string) error {
	s.mu.RLock()
	_, ok := s.jobs[id]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("job %s not found", id)
	}
	s.run(ctx, id, true)
	return nil
}

func (s *Store) Cancel(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return fmt.Errorf("job %s not found", id)
	}
	if job.cancel != nil {
		job.cancel()
	}
	if job.Status == StatusQueued || job.Status == StatusRunning {
		job.Status = StatusCanceled
		job.CurrentSong = ""
		job.UpdatedAt = time.Now()
	}
	return nil
}

func (s *Store) run(ctx context.Context, id string, retryOnly bool) {
	s.mu.Lock()
	job, ok := s.jobs[id]
	if !ok {
		s.mu.Unlock()
		return
	}
	job.Status = StatusRunning
	job.CurrentSong = ""
	runCtx, cancel := context.WithCancel(ctx)
	job.cancel = cancel
	job.UpdatedAt = time.Now()
	indexes := s.pendingIndexes(job, retryOnly)
	for _, idx := range indexes {
		job.Results[idx].Status = SongQueued
		job.Results[idx].Error = ""
		job.Results[idx].FilePath = ""
	}
	s.recount(job)
	s.mu.Unlock()

	sem := make(chan struct{}, s.concurrency)
	var wg sync.WaitGroup
	for _, idx := range indexes {
		if runCtx.Err() != nil {
			break
		}
		idx := idx
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			s.runOne(runCtx, id, idx)
		}()
	}
	wg.Wait()

	s.mu.Lock()
	defer s.mu.Unlock()
	job = s.jobs[id]
	job.cancel = nil
	s.recount(job)
	job.CurrentSong = ""
	if runCtx.Err() != nil {
		job.Status = StatusCanceled
	} else if job.FailureCount > 0 {
		job.Status = StatusCompletedWithErrors
	} else {
		job.Status = StatusCompleted
	}
	job.UpdatedAt = time.Now()
}

func (s *Store) runOne(ctx context.Context, id string, idx int) {
	s.mu.Lock()
	job := s.jobs[id]
	song := job.songs[idx]
	job.Results[idx].Status = SongRunning
	job.CurrentSong = song.Name
	job.UpdatedAt = time.Now()
	playlist := job.Playlist
	downloadDir := job.DownloadDir
	cookie := job.Cookie
	quality := job.Quality
	s.mu.Unlock()

	filePath, err := s.downloader.DownloadSong(ctx, &playlist, song, idx+1, downloadDir, cookie, quality)

	s.mu.Lock()
	defer s.mu.Unlock()
	job = s.jobs[id]
	if err != nil {
		job.Results[idx].Status = SongFailed
		job.Results[idx].Error = err.Error()
	} else {
		job.Results[idx].Status = SongSuccess
		job.Results[idx].FilePath = filePath
	}
	s.recount(job)
	job.UpdatedAt = time.Now()
}

func NormalizeQuality(quality string) string {
	switch quality {
	case "lossless":
		return "lossless"
	default:
		return "mp3"
	}
}

func (s *Store) pendingIndexes(job *Job, retryOnly bool) []int {
	indexes := make([]int, 0, len(job.songs))
	for i := range job.songs {
		if retryOnly && job.Results[i].Status != SongFailed {
			continue
		}
		if !retryOnly && job.Results[i].Status == SongSuccess {
			continue
		}
		indexes = append(indexes, i)
	}
	return indexes
}

func (s *Store) recount(job *Job) {
	job.SuccessCount = 0
	job.FailureCount = 0
	for _, result := range job.Results {
		switch result.Status {
		case SongSuccess, SongSkipped:
			job.SuccessCount++
		case SongFailed:
			job.FailureCount++
		}
	}
}

func cloneJob(job *Job) *Job {
	if job == nil {
		return nil
	}
	cp := *job
	cp.Results = append([]SongResult(nil), job.Results...)
	cp.songs = append([]model.Song(nil), job.songs...)
	cp.Cookie = ""
	cp.cancel = nil
	return &cp
}
