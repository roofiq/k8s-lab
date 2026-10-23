package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"
)

const (
	targetURL = "https://example.com"
	interval  = 30 * time.Second
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	client := &http.Client{Timeout: 10 * time.Second}

	for {
		checkEndpoint(client)
		time.Sleep(interval)
	}
}

func checkEndpoint(client *http.Client) {
	start := time.Now()
	resp, err := client.Get(targetURL)
	if err != nil {
		slog.Error("request failed", "error", err)
		return
	}
	defer resp.Body.Close()

	slog.Info("endpoint checked",
		"status", resp.StatusCode,
		"duration_ms", time.Since(start).Milliseconds(),
	)
}
