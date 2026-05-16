package config

import "testing"

func TestDefaultsUseUncommonLocalPort(t *testing.T) {
	cfg := Default()

	if cfg.Addr != "127.0.0.1:51873" {
		t.Fatalf("Addr = %q, want %q", cfg.Addr, "127.0.0.1:51873")
	}
	if cfg.DownloadDir != "./Downloads" {
		t.Fatalf("DownloadDir = %q, want %q", cfg.DownloadDir, "./Downloads")
	}
	if cfg.Concurrency != 3 {
		t.Fatalf("Concurrency = %d, want 3", cfg.Concurrency)
	}
	if cfg.CookieFile != "./.music-lib-web-cookie" {
		t.Fatalf("CookieFile = %q, want %q", cfg.CookieFile, "./.music-lib-web-cookie")
	}
}

func TestParseFlagsOverridesDefaults(t *testing.T) {
	cfg, err := ParseFlags([]string{
		"--addr", "127.0.0.1:51991",
		"--download-dir", "/tmp/music",
		"--concurrency", "5",
		"--cookie-file", "/tmp/music-cookie",
	})
	if err != nil {
		t.Fatalf("ParseFlags returned error: %v", err)
	}

	if cfg.Addr != "127.0.0.1:51991" {
		t.Fatalf("Addr = %q, want override", cfg.Addr)
	}
	if cfg.DownloadDir != "/tmp/music" {
		t.Fatalf("DownloadDir = %q, want override", cfg.DownloadDir)
	}
	if cfg.Concurrency != 5 {
		t.Fatalf("Concurrency = %d, want 5", cfg.Concurrency)
	}
	if cfg.CookieFile != "/tmp/music-cookie" {
		t.Fatalf("CookieFile = %q, want override", cfg.CookieFile)
	}
}
