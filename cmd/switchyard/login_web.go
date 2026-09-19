package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	DefaultFrontendURL = "https://switchyard-frontend.vercel.app"
	DefaultGatewayURL  = "http://localhost:8080"
)

type callbackPayload struct {
	Key     string `json:"key"`
	Project string `json:"project"`
	State   string `json:"state"`
}

// StartWebAuthFlow starts an interactive browser-based OAuth authentication flow.
// It spins up a local loopback server, opens the Switchyard Web Frontend in the user's
// browser, and waits for the credentials callback.
func StartWebAuthFlow(frontendURL, gatewayURL string) (*Credentials, error) {
	if frontendURL == "" {
		frontendURL = DefaultFrontendURL
	}
	frontendURL = strings.TrimRight(frontendURL, "/#")

	if gatewayURL == "" {
		gatewayURL = DefaultGatewayURL
	}
	gatewayURL = strings.TrimRight(gatewayURL, "/")

	// 1. Find an available loopback port between 45454 and 45464
	var listener net.Listener
	var port int
	var err error
	for p := 45454; p <= 45464; p++ {
		listener, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			port = p
			break
		}
	}
	if listener == nil {
		return nil, fmt.Errorf("could not bind loopback port for authentication (tried 45454-45464): %w", err)
	}
	defer listener.Close()

	// 2. Generate a 32-byte cryptographically secure random nonce
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, fmt.Errorf("failed to generate secure state nonce: %w", err)
	}
	stateNonce := hex.EncodeToString(stateBytes)

	// 3. Prepare result channel and server
	type authResult struct {
		creds *Credentials
		err   error
	}
	resultCh := make(chan authResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		// Enable CORS for frontend requests
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		var receivedKey, receivedProject, receivedState string

		if r.Method == http.MethodPost {
			var payload callbackPayload
			if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
				receivedKey = strings.TrimSpace(payload.Key)
				receivedProject = strings.TrimSpace(payload.Project)
				receivedState = strings.TrimSpace(payload.State)
			}
		}

		if receivedKey == "" {
			// Try query parameters (GET or fallback)
			q := r.URL.Query()
			receivedKey = strings.TrimSpace(q.Get("key"))
			receivedProject = strings.TrimSpace(q.Get("project"))
			receivedState = strings.TrimSpace(q.Get("state"))
		}

		// Nonce validation
		if receivedState == "" || receivedState != stateNonce {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":"invalid_state","message":"State nonce mismatch"}`)
			return
		}

		if receivedKey == "" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":"missing_key","message":"API key missing"}`)
			return
		}

		creds := &Credentials{
			APIKey:    receivedKey,
			ProjectID: receivedProject,
			BaseURL:   gatewayURL,
		}

		if err := saveCredentials(creds); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, `{"error":"save_failed","message":"Failed to write ~/.switchyard/credentials.json"}`)
			resultCh <- authResult{err: fmt.Errorf("failed to save credentials: %w", err)}
			return
		}

		// Success response
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>Switchyard CLI Authorized</title>
  <style>
    body { background: #090d16; color: #f3f4f6; font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; display: flex; align-items: center; justify-content: center; height: 100vh; margin: 0; }
    .card { background: #0f1420; border: 1px solid rgba(255,255,255,0.1); border-radius: 12px; padding: 32px; max-width: 420px; text-align: center; box-shadow: 0 10px 40px -10px rgba(0,0,0,0.8); }
    h1 { color: #10b981; font-size: 18px; margin-bottom: 8px; }
    p { color: #9ca3af; font-size: 13px; line-height: 1.5; margin-bottom: 24px; }
    .btn { background: rgba(255,255,255,0.08); color: #e5e7eb; border: 1px solid rgba(255,255,255,0.1); padding: 8px 16px; border-radius: 6px; font-size: 12px; cursor: pointer; text-decoration: none; }
  </style>
</head>
<body>
  <div class="card">
    <h1>[SUCCESS] CLI Authorized Successfully</h1>
    <p>Your local terminal is now connected to Switchyard.<br>You can safely close this window and return to your terminal.</p>
    <a class="btn" href="javascript:window.close()">Close Window</a>
  </div>
</body>
</html>`)

		resultCh <- authResult{creds: creds}
	})

	server := &http.Server{
		Handler: mux,
	}

	go func() {
		_ = server.Serve(listener)
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	// 4. Construct Auth URL and open user's browser
	authURL := fmt.Sprintf("%s/?port=%d&state=%s&gateway=%s#/cli-auth",
		frontendURL, port, stateNonce, url.QueryEscape(gatewayURL))

	fmt.Println()
	fmt.Println(bold(cyan("  Opening your browser to authenticate with Switchyard...")))
	fmt.Println(dim("  If your browser does not open automatically, visit:"))
	fmt.Printf("    %s\n\n", yellow(authURL))
	fmt.Println(dim("  Waiting for authentication... (Press Ctrl+C to cancel)"))

	if err := openBrowser(authURL); err != nil {
		fmt.Printf("  %s %v\n", dim("(Could not launch browser automatically:"), err)
	}

	// 5. Handle cancellation via Ctrl+C or timeout
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case res := <-resultCh:
		if res.err != nil {
			return nil, res.err
		}
		fmt.Println()
		fmt.Println(bold(green("  [SUCCESS] Authenticated successfully!")))
		fmt.Println(dim("  Credentials saved to ~/.switchyard/credentials.json"))
		if res.creds.ProjectID != "" {
			fmt.Printf("  Project: %s\n", cyan(res.creds.ProjectID))
		}
		fmt.Println()
		return res.creds, nil

	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("authentication timed out after 5 minutes")

	case <-sigCh:
		fmt.Println()
		return nil, fmt.Errorf("authentication cancelled by user")
	}
}

// openBrowser launches the system default web browser.
func openBrowser(targetURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", targetURL)
	case "darwin":
		cmd = exec.Command("open", targetURL)
	default:
		cmd = exec.Command("xdg-open", targetURL)
	}
	return cmd.Start()
}
