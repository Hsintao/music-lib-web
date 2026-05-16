package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/guohuiyuan/music-lib/model"
	"music-lib-web/internal/config"
	"music-lib-web/internal/jobs"
)

const Disclaimer = "本工具仅用于学习和技术研究。请遵守法律法规，不要商用；下载的资源请按上游项目提示及时删除。music-lib 使用 AGPL-3.0 许可证。"

type MusicService interface {
	ParsePlaylist(ctx context.Context, link string) (*model.Playlist, []model.Song, error)
}

type API struct {
	cfg   config.Config
	music MusicService
	jobs  *jobs.Store
	mux   *http.ServeMux
}

type badRequest string

func ErrBadRequest(message string) error {
	return badRequest(message)
}

func (e badRequest) Error() string {
	return string(e)
}

func New(cfg config.Config, music MusicService, store *jobs.Store) *API {
	api := &API{cfg: cfg, music: music, jobs: store, mux: http.NewServeMux()}
	api.routes()
	return api
}

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mux.ServeHTTP(w, r)
}

func (a *API) routes() {
	a.mux.HandleFunc("GET /api/config", a.handleConfig)
	a.mux.HandleFunc("POST /api/playlists/parse", a.handleParsePlaylist)
	a.mux.HandleFunc("POST /api/jobs", a.handleCreateJob)
	a.mux.HandleFunc("GET /api/jobs/{id}", a.handleGetJob)
	a.mux.HandleFunc("POST /api/jobs/{id}/retry", a.handleRetryJob)
}

func (a *API) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"addr":         a.cfg.Addr,
		"download_dir": a.cfg.DownloadDir,
		"concurrency":  a.cfg.Concurrency,
		"disclaimer":   Disclaimer,
	})
}

func (a *API) handleParsePlaylist(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Link string `json:"link"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	playlist, songs, err := a.music.ParsePlaylist(r.Context(), req.Link)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"playlist": playlist, "songs": songs})
}

func (a *API) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlaylistLink string `json:"playlist_link"`
		DownloadDir  string `json:"download_dir"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	playlist, songs, err := a.music.ParsePlaylist(r.Context(), req.PlaylistLink)
	if err != nil {
		writeError(w, statusForError(err), err)
		return
	}
	downloadDir := strings.TrimSpace(req.DownloadDir)
	if downloadDir == "" {
		downloadDir = a.cfg.DownloadDir
	}
	job := a.jobs.Create(playlist, songs, downloadDir)
	go a.jobs.Run(context.Background(), job.ID)
	writeJSON(w, http.StatusAccepted, job)
}

func (a *API) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, ok := a.jobs.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("job %s not found", id))
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (a *API) handleRetryJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := a.jobs.RetryFailures(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	job, _ := a.jobs.Get(id)
	writeJSON(w, http.StatusAccepted, job)
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}
	return nil
}

func statusForError(err error) int {
	if err == nil {
		return http.StatusOK
	}
	var br badRequest
	if errors.As(err, &br) {
		return http.StatusBadRequest
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "required") || strings.Contains(msg, "invalid") {
		return http.StatusBadRequest
	}
	return http.StatusBadGateway
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
