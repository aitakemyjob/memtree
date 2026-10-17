package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/aitakemyjob/memtree/internal/api"
	"github.com/aitakemyjob/memtree/internal/config"
	"github.com/aitakemyjob/memtree/internal/core"
)

func start(t *testing.T, withUI bool) (*Server, *core.Service) {
	t.Helper()
	cfg, _ := config.Open(filepath.Join(t.TempDir(), "c.json"))
	svc, err := core.Open(cfg, ":memory:", "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	opts := Options{Port: 0}
	if withUI {
		opts.UI = fstest.MapFS{"index.html": {Data: []byte("<html>memtree</html>")}, "app.js": {Data: []byte("x")}}
	}
	s := New(svc, api.New(svc), opts)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Shutdown(context.Background()) })
	return s, svc
}

func do(t *testing.T, method, url, body string, hdr map[string]string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	for k, v := range hdr {
		if k == "Host" {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestHealthzAndStatus(t *testing.T) {
	s, svc := start(t, false)
	code, body := do(t, "GET", s.URL()+"/healthz", "", nil)
	if code != 200 || !strings.Contains(body, `"app":"memtree"`) {
		t.Errorf("%d %s", code, body)
	}
	if st := svc.MCPStatus(); !st.Running || !strings.HasSuffix(st.URL, "/mcp") {
		t.Errorf("status %+v", st)
	}
	if code, _ := do(t, "GET", s.URL()+"/", "", nil); code != 404 {
		t.Errorf("headless mode serves no UI, got %d", code)
	}
}

func TestInvokeRoundTripAndGuards(t *testing.T) {
	s, _ := start(t, true)
	call := func(method, args string, hdr map[string]string) (int, map[string]json.RawMessage) {
		code, body := do(t, "POST", s.URL()+"/api/invoke", `{"method":"`+method+`","args":`+args+`}`, hdr)
		var out map[string]json.RawMessage
		json.Unmarshal([]byte(body), &out)
		return code, out
	}
	ok := map[string]string{csrfHeader: "1"}
	code, out := call("memory.save", `{"summary":"ui saved note","project":"/p"}`, ok)
	if code != 200 || out["result"] == nil {
		t.Fatalf("save: %d %v", code, out)
	}
	_, out = call("memory.recall", `{"query":"ui saved","project":"/p"}`, ok)
	if !strings.Contains(string(out["result"]), "ui saved note") {
		t.Errorf("recall: %s", out["result"])
	}
	if _, out = call("nope", `{}`, ok); !strings.Contains(string(out["error"]), "unknown method") {
		t.Errorf("unknown method: %v", out)
	}
	if code, _ := call("app.state", `{}`, nil); code != 403 {
		t.Errorf("missing csrf header → %d", code)
	}
	if code, _ := call("app.state", `{}`, map[string]string{csrfHeader: "1", "Origin": "http://evil.example"}); code != 403 {
		t.Errorf("foreign origin → %d", code)
	}
	if code, _ := call("app.state", `{}`, map[string]string{csrfHeader: "1", "Host": "evil.example"}); code != 403 {
		t.Errorf("non-loopback host (DNS rebinding) → %d", code)
	}
	if code, _ := do(t, "GET", s.URL()+"/api/invoke", "", ok); code != 405 {
		t.Errorf("GET invoke → %d", code)
	}
}

func TestUIAndSPAFallback(t *testing.T) {
	s, _ := start(t, true)
	if _, body := do(t, "GET", s.URL()+"/", "", nil); !strings.Contains(body, "memtree") {
		t.Error("index not served")
	}
	if _, body := do(t, "GET", s.URL()+"/some/route", "", nil); !strings.Contains(body, "memtree") {
		t.Error("unknown route should fall back to index.html")
	}
	if _, body := do(t, "GET", s.URL()+"/app.js", "", nil); body != "x" {
		t.Error("static file")
	}
}

func TestEventStream(t *testing.T) {
	s, svc := start(t, true)
	req, _ := http.NewRequest("GET", s.URL()+"/api/events?gd=1", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got := make(chan string, 4)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				got <- string(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	<-got // ": connected"
	time.Sleep(50 * time.Millisecond)
	svc.RecordActivity(core.Activity{Tool: "recall", Summary: "ping"})
	select {
	case chunk := <-got:
		if !strings.Contains(chunk, `"type":"activity"`) || !strings.Contains(chunk, "recall") {
			t.Errorf("chunk %q", chunk)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event received")
	}
	if code, _ := do(t, "GET", s.URL()+"/api/events", "", nil); code != 403 {
		t.Errorf("event stream without gd=1 → %d", code)
	}
}

func TestBindFailureIsReported(t *testing.T) {
	s, svc := start(t, false)
	other := New(svc, api.New(svc), Options{Port: portOf(s)})
	if err := other.Start(); err == nil {
		t.Fatal("second bind on the same port should fail")
	}
	if st := svc.MCPStatus(); st.Running || st.Error == "" {
		t.Errorf("status %+v", st)
	}
}

func portOf(s *Server) int {
	var p int
	for _, c := range s.URL()[strings.LastIndex(s.URL(), ":")+1:] {
		p = p*10 + int(c-'0')
	}
	return p
}

func TestMissingFrontendExplainsItself(t *testing.T) {
	cfg, _ := config.Open(filepath.Join(t.TempDir(), "c.json"))
	svc, _ := core.Open(cfg, ":memory:", "test", nil)
	defer svc.Close()
	s := New(svc, api.New(svc), Options{UI: fstest.MapFS{".gitkeep": {}}})
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown(context.Background())
	code, body := do(t, "GET", s.URL()+"/", "", nil)
	if code != 503 || !strings.Contains(body, "no UI") {
		t.Errorf("%d %q", code, body)
	}
}
