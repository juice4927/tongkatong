package updater

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadFileResumesPartialDownload(t *testing.T) {
	payload := []byte("hello world")
	partial := []byte("hello ")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Range"); got != "bytes=6-" {
			t.Fatalf("expected range resume header, got %q", got)
		}
		w.Header().Set("Content-Range", "bytes 6-10/11")
		w.Header().Set("Content-Length", "5")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload[len(partial):])
	}))
	defer server.Close()

	dir := t.TempDir()
	destPath := filepath.Join(dir, "update.zip")
	if err := os.WriteFile(destPath+".part", partial, 0644); err != nil {
		t.Fatal(err)
	}

	hash, err := DownloadFile(server.URL, destPath, nil)
	if err != nil {
		t.Fatalf("DownloadFile returned error: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("downloaded content = %q, want %q", got, payload)
	}

	wantHash := fmt.Sprintf("%x", sha256.Sum256(payload))
	if hash != wantHash {
		t.Fatalf("hash = %s, want %s", hash, wantHash)
	}
}
