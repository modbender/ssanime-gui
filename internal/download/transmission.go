package download

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// transmissionBackend talks to a user-run Transmission daemon over its RPC API.
// It handles the 409 session-id handshake, adds via torrent-add, polls
// torrent-get for progress, and removes via torrent-remove. Like the others it
// keeps files on disk for the encoder unless deleteData is requested.
type transmissionBackend struct {
	endpoint string
	user     string
	pass     string
	root     string
	client   *http.Client

	mu        sync.Mutex
	sessionID string
}

func newTransmissionBackend(_ context.Context, cfg Config) (Backend, error) {
	host := cfg.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := cfg.Port
	if port == 0 {
		port = 9091
	}
	return &transmissionBackend{
		// TODO: validate host + support https when a client-management write path
		// is added (audit deferred — today only the seeder writes this row).
		endpoint: fmt.Sprintf("http://%s:%d/transmission/rpc", host, port),
		user:     cfg.Username,
		pass:     cfg.Password,
		root:     cfg.DownloadRoot,
		client:   &http.Client{Timeout: 30 * time.Second, Transport: cfg.HTTPTransport},
	}, nil
}

func (b *transmissionBackend) Kind() string { return KindTransmission }

// rpcReq is a Transmission RPC request envelope.
type rpcReq struct {
	Method    string `json:"method"`
	Arguments any    `json:"arguments,omitempty"`
}

// call performs one RPC, transparently re-doing the request once after the
// 409 X-Transmission-Session-Id handshake.
func (b *transmissionBackend) call(ctx context.Context, method string, args any, out any) error {
	body, err := json.Marshal(rpcReq{Method: method, Arguments: args})
	if err != nil {
		return err
	}
	do := func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		b.mu.Lock()
		sid := b.sessionID
		b.mu.Unlock()
		if sid != "" {
			req.Header.Set("X-Transmission-Session-Id", sid)
		}
		if b.user != "" || b.pass != "" {
			req.SetBasicAuth(b.user, b.pass)
		}
		return b.client.Do(req)
	}

	resp, err := do()
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusConflict {
		b.mu.Lock()
		b.sessionID = resp.Header.Get("X-Transmission-Session-Id")
		b.mu.Unlock()
		resp.Body.Close()
		if resp, err = do(); err != nil {
			return err
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("transmission rpc %s: status %d", method, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxAPIBytes)).Decode(out)
}

func (b *transmissionBackend) Add(ctx context.Context, req Request) (Handle, error) {
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
		return nil, fmt.Errorf("transmission add: no magnet/infohash/url in request")
	}

	args := map[string]any{
		"filename":     src,
		"download-dir": filepath.Join(b.root, hash),
	}
	var resp struct {
		Result string `json:"result"`
	}
	if err := b.call(ctx, "torrent-add", args, &resp); err != nil {
		return nil, fmt.Errorf("transmission add: %w", err)
	}
	if resp.Result != "success" {
		return nil, fmt.Errorf("transmission add: %s", resp.Result)
	}

	h := &externalHandle{
		backend:     b,
		hash:        strings.ToLower(hash),
		done:        make(chan struct{}),
		poll:        b.poll,
		remove:      b.remove,
		resolvePath: b.resolvePath,
	}
	go h.watch(ctx)
	return h, nil
}

// torrentGet fetches one torrent's fields by hash.
func (b *transmissionBackend) torrentGet(ctx context.Context, hash string, fields []string) (*transTorrent, error) {
	args := map[string]any{"ids": []string{hash}, "fields": fields}
	var resp struct {
		Arguments struct {
			Torrents []transTorrent `json:"torrents"`
		} `json:"arguments"`
	}
	if err := b.call(ctx, "torrent-get", args, &resp); err != nil {
		return nil, err
	}
	if len(resp.Arguments.Torrents) == 0 {
		return nil, nil
	}
	return &resp.Arguments.Torrents[0], nil
}

type transTorrent struct {
	HashString         string  `json:"hashString"`
	Name               string  `json:"name"`
	TotalSize          int64   `json:"totalSize"`
	SizeWhenDone       int64   `json:"sizeWhenDone"`
	HaveValid          int64   `json:"haveValid"`
	PercentDone        float64 `json:"percentDone"`
	RateDownload       int64   `json:"rateDownload"`
	PeersGettingFromUs int     `json:"peersGettingFromUs"`
	PeersSendingToUs   int     `json:"peersSendingToUs"`
	Status             int     `json:"status"`
	DownloadDir        string  `json:"downloadDir"`
	Files              []struct {
		Name           string `json:"name"`
		Length         int64  `json:"length"`
		BytesCompleted int64  `json:"bytesCompleted"`
	} `json:"files"`
}

func (b *transmissionBackend) poll(ctx context.Context, hash string) (Progress, error) {
	t, err := b.torrentGet(ctx, hash, []string{
		"hashString", "sizeWhenDone", "haveValid", "percentDone",
		"rateDownload", "peersSendingToUs", "status",
	})
	if err != nil {
		return Progress{}, err
	}
	if t == nil {
		return Progress{}, nil
	}
	return Progress{
		BytesDone:  t.HaveValid,
		BytesTotal: t.SizeWhenDone,
		Peers:      t.PeersSendingToUs,
		SpeedBps:   t.RateDownload,
		Done:       t.PercentDone >= 1.0,
	}, nil
}

// resolvePath picks the largest video file from the torrent and returns its
// absolute path + size for the encoder.
func (b *transmissionBackend) resolvePath(ctx context.Context, hash string) (string, int64, error) {
	t, err := b.torrentGet(ctx, hash, []string{"hashString", "downloadDir", "files"})
	if err != nil {
		return "", 0, err
	}
	if t == nil || len(t.Files) == 0 {
		return "", 0, fmt.Errorf("transmission: no files for %s", hash)
	}
	bestIdx, bestLen := -1, int64(-1)
	anyIdx, anyLen := 0, int64(-1)
	for i, f := range t.Files {
		if f.Length > anyLen {
			anyIdx, anyLen = i, f.Length
		}
		ext := strings.ToLower(filepath.Ext(f.Name))
		if _, ok := videoExts[ext]; ok && f.Length > bestLen {
			bestIdx, bestLen = i, f.Length
		}
	}
	idx := bestIdx
	if idx < 0 {
		idx = anyIdx
	}
	f := t.Files[idx]
	return filepath.Join(t.DownloadDir, filepath.FromSlash(f.Name)), f.Length, nil
}

func (b *transmissionBackend) remove(ctx context.Context, hash string, deleteData bool) error {
	args := map[string]any{
		"ids":               []string{hash},
		"delete-local-data": deleteData,
	}
	return b.call(ctx, "torrent-remove", args, nil)
}

func (b *transmissionBackend) Close() error { return nil }
