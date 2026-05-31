// Command testserver is a tiny stdlib-only HTTP server that exercises every
// feature of Yon: the four methods, query/header echo, Basic and Bearer auth,
// redirects, arbitrary status codes, a >256 KB body (display truncation), and a
// slow endpoint (cancel/timeout). Run it and load testserver.yon in Yon.
//
//	go run ./testserver        # listens on :7878
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const addr = ":7878"

// Demo credentials the bundled testserver.yon collection uses.
const (
	demoBearer   = "yon-demo-token"
	demoUser     = "alice"
	demoPassword = "secret"
)

func main() {
	log.Printf("testserver listening on http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, newMux()))
}

// newMux builds the routing table. Extracted from main so tests can mount it on
// an httptest server.
func newMux() *http.ServeMux {
	mux := http.NewServeMux()

	for _, p := range []string{"/get", "/post", "/put", "/delete", "/anything", "/headers"} {
		mux.HandleFunc(p, echo)
	}
	mux.HandleFunc("/basic-auth/{user}/{pass}", basicAuth)
	mux.HandleFunc("/bearer", bearerAuth)
	mux.HandleFunc("/status/{code}", status)
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/get", http.StatusFound)
	})
	mux.HandleFunc("/large", large)
	mux.HandleFunc("/slow", slow)
	mux.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, sampleJSON())
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"service": "yon-testserver",
			"endpoints": []string{
				"/get", "/post", "/put", "/delete", "/headers",
				"/basic-auth/{user}/{pass}", "/bearer", "/status/{code}",
				"/redirect", "/large", "/slow?seconds=N", "/json",
			},
			"credentials": map[string]string{
				"bearer": demoBearer, "basicUser": demoUser, "basicPass": demoPassword,
			},
		})
	})
	return mux
}

// echo reflects the request back as JSON.
func echo(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	writeJSON(w, http.StatusOK, map[string]any{
		"method":  r.Method,
		"path":    r.URL.Path,
		"query":   flatten(r.URL.Query()),
		"headers": flatten(r.Header),
		"body":    string(body),
	})
}

func basicAuth(w http.ResponseWriter, r *http.Request) {
	user := r.PathValue("user")
	pass := r.PathValue("pass")
	u, p, ok := r.BasicAuth()
	if !ok || u != user || p != pass {
		w.Header().Set("WWW-Authenticate", `Basic realm="yon"`)
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"authenticated": false,
			"hint":          fmt.Sprintf("send Basic auth %s / %s", user, pass),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "user": u})
}

func bearerAuth(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	token := strings.TrimPrefix(auth, "Bearer ")
	if token != demoBearer || !strings.HasPrefix(auth, "Bearer ") {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"authenticated": false,
			"hint":          "send Bearer token " + demoBearer,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "token": token})
}

func status(w http.ResponseWriter, r *http.Request) {
	code, err := strconv.Atoi(r.PathValue("code"))
	if err != nil || code < 100 || code > 599 {
		code = http.StatusBadRequest
	}
	writeJSON(w, code, map[string]any{"status": code, "text": http.StatusText(code)})
}

func large(w http.ResponseWriter, r *http.Request) {
	items := make([]map[string]any, 4000)
	for i := range items {
		items[i] = map[string]any{
			"id":    i,
			"name":  fmt.Sprintf("item-%05d", i),
			"value": i * 7,
			"note":  "lorem ipsum dolor sit amet consectetur adipiscing elit",
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(items), "items": items})
}

func slow(w http.ResponseWriter, r *http.Request) {
	secs := 5
	if v := r.URL.Query().Get("seconds"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			secs = n
		}
	}
	select {
	case <-time.After(time.Duration(secs) * time.Second):
		writeJSON(w, http.StatusOK, map[string]any{"sleptSeconds": secs})
	case <-r.Context().Done():
		// Client cancelled (Yon's Cancel button or timeout) — just stop.
	}
}

// flatten collapses a header/query multimap to single values (first wins).
func flatten(m map[string][]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out
}

func sampleJSON() map[string]any {
	return map[string]any{
		"id":      42,
		"name":    "Yon",
		"active":  true,
		"score":   9.75,
		"tags":    []string{"http", "offline", "fast"},
		"nested":  map[string]any{"a": 1, "b": []int{1, 2, 3}, "c": nil},
		"message": "Throw a request. Catch a response.",
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Yon-Testserver", "1")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
