package main

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"strings"

	"music-lib-web/internal/config"
	"music-lib-web/internal/jobs"
	"music-lib-web/internal/netease"
	"music-lib-web/internal/server"
)

func main() {
	cfg, err := config.ParseFlags(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}

	music := netease.New()
	store := jobs.NewStore(music, cfg.Concurrency)
	api := server.New(cfg, music, store)

	mux := http.NewServeMux()
	mux.Handle("/api/", api)
	mux.Handle("/", staticHandler())

	fmt.Printf("music-lib-web listening on http://%s\n", cfg.Addr)
	fmt.Printf("downloads will be saved under %s\n", cfg.DownloadDir)
	if err := http.ListenAndServe(cfg.Addr, mux); err != nil {
		log.Fatal(err)
	}
}

func staticHandler() http.Handler {
	sub := os.DirFS("web")
	files := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean(r.URL.Path)
		if clean == "." || clean == "/" {
			r.URL.Path = "/"
			files.ServeHTTP(w, r)
			return
		}
		name := strings.TrimPrefix(clean, "/")
		if _, err := fs.Stat(sub, name); err != nil {
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
}
