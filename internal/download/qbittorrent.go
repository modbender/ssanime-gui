package download

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// qbittorrentBackend talks to a user-run qBittorrent over its WebUI API (v2).
// It connects per download_clients row (host/port/username/password) and
// implements add / progress / remove against the live client.
//
// First cut is functional-but-minimal: it adds the magnet, then polls
// /torrents/info by hash for progress and completion. Like the embedded backend
// it leaves data on disk for the encoder and only deletes when asked.
type qbittorrentBackend struct {
	base   string
	user   string
	pass   string
	root   string
	client *http.Client

	mu       sync.Mutex
	loggedIn bool
}

func newQBittorrentBackend(_ context.Context, cfg Config) (Backend, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("cookiejar: %w", err)
	}
	host := cfg.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := cfg.Port
	if port == 0 {
		port = 8080
	}
	tr := cfg.HTTPTransport
	return &qbittorrentBackend{
		base: fmt.Sprintf("http://%s:%d", host, port),
		user: cfg.Username,
		pass: cfg.Password,
		root: cfg.DownloadRoot,
		client: &http.Client{
			Jar:       jar,
			Timeout:   30 * time.Second,
			Transport: tr,
		},
	}, nil
}

func (b *qbittorrentBackend) Kind() string { return KindQBittorrent }

// login authenticates and stores the SID cookie in the jar. Idempotent.
func (b *qbittorrentBackend) login(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.loggedIn {
		return nil
	}
	form := url.Values{"username": {b.user}, "password": {b.pass}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		b.base+"/api/v2/auth/login", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", b.base)
	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("qbittorrent login: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("qbittorrent login: status %d", resp.StatusCode)
	}
	b.loggedIn = true
	return nil
}

func (b *qbittorrentBackend) Add(ctx context.Context, req Request) (Handle, error) {
	if err := b.login(ctx); err != nil {
		return nil, err
	}
	hash, err := infoHashFor(req)
	if err != nil {
		return nil, err
	}
	src := req.Magnet
	if src == "" && req.InfoHash != "" {
		src = "magnet:?xt=urn:btih:" + req.InfoHash
	}
	if src == "" {
		src = req.TorrentURL
	}
	if src == "" {
		return nil, fmt.Errorf("qbittorrent add: no magnet/infohash/url in request")
	}

	form := url.Values{
		"urls":     {src},
		"savepath": {filepath.Join(b.root, hash)},
	}
	if err := b.post(ctx, "/api/v2/torrents/add", form); err != nil {
		return nil, fmt.Errorf("qbittorrent add: %w", err)
	}

	h := &externalHandle{
		backend: b,
		hash:    strings.ToLower(hash),
		done:    make(chan struct{}),
		poll:    b.poll,
		remove:  b.remove,
	}
	go h.watch(ctx)
	return h, nil
}

// post sends an authenticated form POST, retrying login once on 403.
func (b *qbittorrentBackend) post(ctx context.Context, path string, form url.Values) error {
	do := func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.base+path,
			strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Referer", b.base)
		return b.client.Do(req)
	}
	resp, err := do()
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusForbidden {
		resp.Body.Close()
		b.mu.Lock()
		b.loggedIn = false
		b.mu.Unlock()
		if err := b.login(ctx); err != nil {
			return err
		}
		if resp, err = do(); err != nil {
			return err
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d for %s", resp.StatusCode, path)
	}
	return nil
}

// poll fetches one torrent's progress by hash via /torrents/info.
func (b *qbittorrentBackend) poll(ctx context.Context, hash string) (Progress, error) {
	if err := b.login(ctx); err != nil {
		return Progress{}, err
	}
	u := b.base + "/api/v2/torrents/info?hashes=" + url.QueryEscape(hash)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Progress{}, err
	}
	req.Header.Set("Referer", b.base)
	resp, err := b.client.Do(req)
	if err != nil {
		return Progress{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Progress{}, fmt.Errorf("qbittorrent info: status %d", resp.StatusCode)
	}
	var rows []struct {
		Hash      string  `json:"hash"`
		Size      int64   `json:"size"`
		Completed int64   `json:"completed"`
		Progress  float64 `json:"progress"`
		Dlspeed   int64   `json:"dlspeed"`
		NumSeeds  int     `json:"num_seeds"`
		NumLeechs int     `json:"num_leechs"`
		State     string  `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return Progress{}, err
	}
	if len(rows) == 0 {
		return Progress{}, nil // not visible yet
	}
	r := rows[0]
	return Progress{
		BytesDone:  r.Completed,
		BytesTotal: r.Size,
		Peers:      r.NumSeeds + r.NumLeechs,
		SpeedBps:   r.Dlspeed,
		Done:       r.Progress >= 1.0 || isQbDoneState(r.State),
	}, nil
}

func isQbDoneState(s string) bool {
	switch s {
	case "uploading", "stalledUP", "pausedUP", "queuedUP", "forcedUP", "checkingUP":
		return true
	default:
		return false
	}
}

// remove deletes the torrent from qBittorrent. deleteFiles maps to qB's
// deleteFiles flag so Phase 4 (deleteData=false) keeps the file for the encoder.
func (b *qbittorrentBackend) remove(ctx context.Context, hash string, deleteFiles bool) error {
	if err := b.login(ctx); err != nil {
		return err
	}
	form := url.Values{
		"hashes":      {hash},
		"deleteFiles": {strconv.FormatBool(deleteFiles)},
	}
	return b.post(ctx, "/api/v2/torrents/delete", form)
}

func (b *qbittorrentBackend) Close() error { return nil }
