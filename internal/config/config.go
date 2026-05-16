package config

import (
	"flag"
	"fmt"
)

type Config struct {
	Addr        string `json:"addr"`
	DownloadDir string `json:"download_dir"`
	Concurrency int    `json:"concurrency"`
	CookieFile  string `json:"cookie_file"`
}

func Default() Config {
	return Config{
		Addr:        "127.0.0.1:51873",
		DownloadDir: "./Downloads",
		Concurrency: 3,
		CookieFile:  "./.music-lib-web-cookie",
	}
}

func ParseFlags(args []string) (Config, error) {
	cfg := Default()
	fs := flag.NewFlagSet("music-lib-web", flag.ContinueOnError)
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "HTTP listen address")
	fs.StringVar(&cfg.DownloadDir, "download-dir", cfg.DownloadDir, "download root directory")
	fs.IntVar(&cfg.Concurrency, "concurrency", cfg.Concurrency, "download concurrency")
	fs.StringVar(&cfg.CookieFile, "cookie-file", cfg.CookieFile, "file used to persist Netease cookie")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if cfg.Concurrency < 1 {
		return Config{}, fmt.Errorf("concurrency must be at least 1")
	}
	return cfg, nil
}
