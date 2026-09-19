package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebAuth_LoopbackCallback(t *testing.T) {
	// Isolate credentials file to temporary directory
	tempDir := t.TempDir()
	origHome := os.Getenv("USERPROFILE")
	if origHome == "" {
		origHome = os.Getenv("HOME")
	}
	os.Setenv("USERPROFILE", tempDir)
	os.Setenv("HOME", tempDir)
	defer func() {
		os.Setenv("USERPROFILE", origHome)
		os.Setenv("HOME", origHome)
	}()

	// Find an open port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port

	stateNonce := "test-nonce-12345"
	doneCh := make(chan *Credentials, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		var payload callbackPayload
		_ = json.NewDecoder(r.Body).Decode(&payload)

		if payload.State != stateNonce {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		creds := &Credentials{
			APIKey:    payload.Key,
			ProjectID: payload.Project,
			BaseURL:   "http://localhost:8080",
		}
		_ = saveCredentials(creds)
		w.WriteHeader(http.StatusOK)
		doneCh <- creds
	})

	server := &http.Server{Handler: mux}
	go func() {
		_ = server.Serve(listener)
	}()
	defer server.Close()

	client := &http.Client{Timeout: 3 * time.Second}

	t.Run("rejects invalid state nonce", func(t *testing.T) {
		badBody, _ := json.Marshal(map[string]string{
			"key":     "ngk_live_sample",
			"project": "prj_sample",
			"state":   "wrong-nonce",
		})
		resp, err := client.Post(fmt.Sprintf("http://127.0.0.1:%d/callback", port), "application/json", bytes.NewReader(badBody))
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("accepts valid callback and saves credentials", func(t *testing.T) {
		validBody, _ := json.Marshal(map[string]string{
			"key":     "ngk_live_validkey123",
			"project": "prj_valid123",
			"state":   stateNonce,
		})
		resp, err := client.Post(fmt.Sprintf("http://127.0.0.1:%d/callback", port), "application/json", bytes.NewReader(validBody))
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		select {
		case received := <-doneCh:
			assert.Equal(t, "ngk_live_validkey123", received.APIKey)
			assert.Equal(t, "prj_valid123", received.ProjectID)

			// Verify file on disk
			savedCreds, err := loadCredentials()
			require.NoError(t, err)
			assert.Equal(t, "ngk_live_validkey123", savedCreds.APIKey)
			assert.Equal(t, "prj_valid123", savedCreds.ProjectID)
			assert.FileExists(t, filepath.Join(tempDir, ".switchyard", "credentials.json"))

		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for credentials callback")
		}
	})
}
