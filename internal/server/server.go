// Package server is the loopback HTTP listener: the MCP endpoint, a health
// check, and (in browser mode) the UI, its JSON API and its event stream.
// It only ever binds 127.0.0.1 and refuses requests whose Host is not
// loopback, which also defeats DNS-rebinding.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aitakemyjob/memtree/internal/api"
	"github.com/aitakemyjob/memtree/internal/core"
	"github.com/aitakemyjob/memtree/internal/mcpserver"
)

// Header the UI must send on API calls. Browsers cannot set arbitrary headers
// on cross-site simple requests, so requiring it blocks CSRF; EventSource
// cannot set headers, so the event stream accepts ?gd=1 instead.
const csrfHeader = "X-Memtree"

// Options configure the server.
type Options struct {
	Port int
	// UI, when set, is served at / together with /api/invoke and /api/events.
	UI fs.FS
}

// Server is the HTTP listener.
type Server struct {
	core *core.Service
	api  *api.API
	mcp  *mcpserver.Server
	opts Options
	http *http.Server
	ln   net.Listener
}

// New builds the server (not yet listening).
func New(c *core.Service, a *api.API, opts Options) *Server {
	return &Server{core: c, api: a, mcp: mcpserver.New(c), opts: opts}
}

// MCP returns the MCP server (for stdio mode and tests).
func (s *Server) MCP() *mcpserver.Server { return s.mcp }

// routes builds the handler: MCP, health, the API and, in browser mode, the UI and its event stream.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mh := s.mcp.Handler()
	mux.Handle("/mcp", mh)
	mux.Handle("/mcp/", mh)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "app": "memtree", "version": s.core.Version})
	})
	// The hook endpoint exists in every mode: hooks must reach a running
	// service whether or not the UI is served.
	mux.Handle("/api/invoke", s.guard(http.HandlerFunc(s.invoke)))
	if s.opts.UI != nil {
		mux.Handle("/api/events", s.guard(http.HandlerFunc(s.events)))
		mux.Handle("/", spa(s.opts.UI))
	}
	return localOnly(mux)
}

// localOnly rejects any request whose Host header is not a loopback address.
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.Trim(host, "[]")
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// guard requires the CSRF header (or ?gd=1 for EventSource) and a same-host
// Origin when the browser sends one.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" {
			u, err := url.Parse(o)
			if err != nil || u.Host != r.Host {
				http.Error(w, "forbidden origin", http.StatusForbidden)
				return
			}
		}
		if r.Header.Get(csrfHeader) == "" && r.URL.Query().Get("gd") != "1" {
			http.Error(w, "missing "+csrfHeader+" header", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// invoke serves POST /api/invoke {method, args} → {result} or {error}.
func (s *Server) invoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Method string          `json:"method"`
		Args   json.RawMessage `json:"args"`
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32<<20))
	if err == nil {
		err = json.Unmarshal(body, &req)
	}
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "bad request: " + err.Error()})
		return
	}
	out, err := s.api.Invoke(r.Context(), req.Method, req.Args)
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]json.RawMessage{"result": out})
}

// events streams bus events to the browser as server-sent events.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch, cancel := s.core.Bus.Subscribe()
	defer cancel()
	fmt.Fprint(w, ": connected\n\n")
	fl.Flush()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			b, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		}
	}
}

// spa serves a single-page app: unknown paths fall back to index.html.
func spa(fsys fs.FS) http.Handler {
	files := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(fsys, "index.html"); err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, noUI)
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(fsys, p); err != nil {
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
}

// Start binds the loopback port and serves in the background. A bind failure
// is recorded in the MCP status (so the UI can show it) and returned.
func (s *Server) Start() error {
	addr := fmt.Sprintf("127.0.0.1:%d", s.opts.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		s.core.SetMCPStatus(core.MCPStatus{Error: err.Error()})
		return err
	}
	s.ln = ln
	s.http = &http.Server{Handler: s.routes(), ReadHeaderTimeout: 10 * time.Second}
	s.core.SetMCPStatus(core.MCPStatus{Running: true, URL: s.URL() + "/mcp"})
	go func() {
		if err := s.http.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.core.SetMCPStatus(core.MCPStatus{Error: err.Error()})
		}
	}()
	return nil
}

// URL is the base URL the server listens on (valid after Start).
func (s *Server) URL() string {
	if s.ln == nil {
		return fmt.Sprintf("http://127.0.0.1:%d", s.opts.Port)
	}
	return "http://" + s.ln.Addr().String()
}

// Shutdown stops the listener.
func (s *Server) Shutdown(ctx context.Context) error {
	s.core.SetMCPStatus(core.MCPStatus{})
	if s.http == nil {
		return nil
	}
	return s.http.Shutdown(ctx)
}

// noUI is shown when the binary was built before the frontend was. The
// server, MCP endpoint and hooks still work; only the UI is missing.
const noUI = `<!doctype html><meta charset="utf-8"><title>memtree</title>
<body style="font:16px/1.5 system-ui;background:#04131c;color:#e7eef2;max-width:40rem;margin:4rem auto;padding:0 1rem">
<h1 style="font-family:Cambria,Georgia,serif">memtree is running, but this build has no UI</h1>
<p>The binary was built before the frontend. MCP and hooks work normally.</p>
<p>To get the UI: <code>make frontend</code>, then rebuild
(<code>make headless</code>). The UI is embedded at build time.</p></body>`
