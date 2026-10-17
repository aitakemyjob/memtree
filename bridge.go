package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/aitakemyjob/memtree/internal/mcpserver"
)

// runMCP connects a stdio MCP client to the running memtree by relaying
// JSON-RPC to its HTTP endpoint, so the UI sees every call live. If memtree
// is not running it serves the tools directly over stdio against the same
// database, so an agent always has its memory.
func runMCP(port int, standalone bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if port == 0 {
		port = servicePort()
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	if !standalone && appRunning(base) {
		return bridge(ctx, base+"/mcp")
	}
	a, err := bootstrap(port, false)
	if err != nil {
		return err
	}
	defer a.close()
	return mcpserver.New(a.core).Run(ctx)
}

// appRunning reports whether a memtree answers /healthz at base.
func appRunning(base string) bool {
	c := http.Client{Timeout: 700 * time.Millisecond}
	resp, err := c.Get(base + "/healthz")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var h struct{ App string }
	return json.NewDecoder(resp.Body).Decode(&h) == nil && h.App == "memtree"
}

// bridge relays JSON-RPC between stdio and the app's streamable HTTP endpoint
// until either side closes.
func bridge(ctx context.Context, endpoint string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	local, err := (&mcp.StdioTransport{}).Connect(ctx)
	if err != nil {
		return err
	}
	defer local.Close()
	remote, err := (&mcp.StreamableClientTransport{Endpoint: endpoint}).Connect(ctx)
	if err != nil {
		return err
	}
	defer remote.Close()
	errc := make(chan error, 2)
	pump := func(from, to mcp.Connection) {
		for {
			msg, err := from.Read(ctx)
			if err != nil {
				errc <- err
				return
			}
			if err := to.Write(ctx, msg); err != nil {
				errc <- err
				return
			}
		}
	}
	go pump(local, remote)
	go pump(remote, local)
	select {
	case <-ctx.Done():
		return nil
	case err := <-errc:
		if ctx.Err() != nil || (err != nil && err.Error() == "EOF") {
			return nil
		}
		return err
	}
}
