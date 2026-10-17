package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/aitakemyjob/memtree/internal/config"
)

// errNoService means no memtree is listening, so the hook should work on the
// database directly.
var errNoService = errors.New("memtree is not running")

// readAll reads at most limit bytes from r (hook payloads are bounded).
func readAll(r io.Reader, limit int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, limit))
}

// servicePort is the port a running memtree listens on.
func servicePort() int {
	if p, err := config.DefaultPath(); err == nil {
		if c, err := config.Open(p); err == nil {
			return c.Get().MCPPort
		}
	}
	return config.DefaultMCPPort
}

// hookViaService sends the event to a running memtree so the UI sees it
// live. Only a failure to connect falls back to direct mode: once a request
// is on the wire it must not be repeated, or captures would double.
func hookViaService(harness, event string, payload []byte) (string, error) {
	base := fmt.Sprintf("127.0.0.1:%d", servicePort())
	conn, err := net.DialTimeout("tcp", base, 250*time.Millisecond)
	if err != nil {
		return "", errNoService
	}
	conn.Close()
	body, _ := json.Marshal(map[string]any{
		"method": "hooks.handle",
		"args":   map[string]any{"harness": harness, "event": event, "payload": json.RawMessage(orEmptyJSON(payload))},
	})
	req, _ := http.NewRequest("POST", "http://"+base+"/api/invoke", bytes.NewReader(body))
	req.Header.Set("X-Memtree", "1")
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 1200 * time.Millisecond}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Result struct{ Output string } `json:"result"`
		Error  string                  `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Error != "" {
		return "", errors.New(out.Error)
	}
	return out.Result.Output, nil
}

// hookDirect handles the event against the database in this process. SQLite
// in WAL mode lets it coexist with a running memtree.
func hookDirect(harness, event string, payload []byte) (string, error) {
	a, err := bootstrap(0, false)
	if err != nil {
		return "", err
	}
	defer a.close()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	return a.hooks.Handle(ctx, harness, event, json.RawMessage(orEmptyJSON(payload)))
}

// orEmptyJSON turns an empty payload into "{}" so it still decodes.
func orEmptyJSON(b []byte) []byte {
	if len(bytes.TrimSpace(b)) == 0 {
		return []byte("{}")
	}
	return b
}

// logHookError appends to hook-errors.log beside the config file, trimming it
// when it grows past 256 KB. Logging must never itself fail the hook.
func logHookError(harness, event string, err error) {
	p, perr := config.DefaultPath()
	if perr != nil {
		return
	}
	path := filepath.Join(filepath.Dir(p), "hook-errors.log")
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if st, serr := os.Stat(path); serr == nil && st.Size() > 256<<10 {
		_ = os.Remove(path)
	}
	f, ferr := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if ferr != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s %s: %v\n", time.Now().Format(time.RFC3339), harness, event, err)
}
