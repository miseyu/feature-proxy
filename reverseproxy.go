package proxy

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

type proxyAction string

const (
	proxyAdd    = proxyAction("Add")
	proxyRemove = proxyAction("Remove")
)

var proxyHandlerLifetime = 30 * time.Second

type proxyControl struct {
	Action    proxyAction
	Subdomain string
	IPAddress string
	Port      int
}

type ReverseProxy struct {
	mu                sync.RWMutex
	cfg               *Config
	domains           []string
	domainMap         map[string]proxyHandlers
	accessCounterUnit time.Duration
}

func NewReverseProxy(cfg *Config) *ReverseProxy {
	return &ReverseProxy{
		cfg:       cfg,
		domainMap: make(map[string]proxyHandlers),
	}
}

func (r *ReverseProxy) ServeHTTPWithPort(w http.ResponseWriter, req *http.Request, port int) {
	subdomain := strings.ToLower(strings.Split(req.Host, ".")[0])

	if handler := r.FindHandler(subdomain, port); handler != nil {
		slog.Debug(f("proxy handler found for subdomain %s", subdomain))
		handler.ServeHTTP(w, req)
	} else {
		slog.Debug(f("proxy handler not found for subdomain %s", subdomain))
		http.NotFound(w, req)
	}
}

func (r *ReverseProxy) Exists(subdomain string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.domainMap[subdomain]
	if exists {
		return true
	}
	for _, name := range r.domains {
		if m, _ := path.Match(name, subdomain); m {
			return true
		}
	}
	return false
}

func (r *ReverseProxy) Subdomains() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ds := make([]string, len(r.domains))
	copy(ds, r.domains)
	return ds
}

func (r *ReverseProxy) FindHandler(subdomain string, port int) http.Handler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	slog.Debug(f("FindHandler for %s:%d", subdomain, port))

	proxyHandlers, ok := r.domainMap[subdomain]
	if !ok {
		for _, name := range r.domains {
			if m, _ := path.Match(name, subdomain); m {
				proxyHandlers = r.domainMap[name]
				break
			}
		}
		if proxyHandlers == nil {
			return nil
		}
	}

	handler, ok := proxyHandlers.Handler(port)
	if !ok {
		return nil
	}
	return handler
}

type proxyHandler struct {
	handler http.Handler
	timer   *time.Timer
}

func newProxyHandler(h http.Handler) *proxyHandler {
	return &proxyHandler{
		handler: h,
		timer:   time.NewTimer(proxyHandlerLifetime),
	}
}

func (h *proxyHandler) alive() bool {
	select {
	case <-h.timer.C:
		return false
	default:
		return true
	}
}

func (h *proxyHandler) extend() {
	h.timer.Reset(proxyHandlerLifetime) // extend lifetime
}

type proxyHandlers map[int]map[string]*proxyHandler

func (ph proxyHandlers) Handler(port int) (http.Handler, bool) {
	handlers := ph[port]
	if len(handlers) == 0 {
		return nil, false
	}
	for ipaddress, handler := range ph[port] {
		if handler.alive() {
			// return first (randomized by Go's map)
			return handler.handler, true
		} else {
			slog.Info(f("proxy handler to %s is dead", ipaddress))
			delete(ph[port], ipaddress)
		}
	}
	return nil, false
}

func (ph proxyHandlers) add(port int, ipaddress string, h http.Handler) {
	if ph[port] == nil {
		ph[port] = make(map[string]*proxyHandler)
	}
	slog.Info(f("new proxy handler to %s", ipaddress))
	ph[port][ipaddress] = newProxyHandler(h)
}

type Transport struct {
	Transport              http.RoundTripper
	Timeout                time.Duration
	Subdomain              string
	AuthCookieValidateFunc func(*http.Cookie) error
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	slog.Debug(f("subdomain %s %s roundtrip", t.Subdomain, req.URL))
	// OPTIONS request is not authenticated because it is preflighted.
	if t.AuthCookieValidateFunc != nil && req.Method != http.MethodOptions {
		cookie, err := req.Cookie(AuthCookieName)
		if err != nil || cookie == nil {
			slog.Warn(f("subdomain %s %s roundtrip failed: %s", t.Subdomain, req.URL, err))
			return newForbiddenResponse(), nil
		}
		if err := t.AuthCookieValidateFunc(cookie); err != nil {
			slog.Warn(f("subdomain %s %s roundtrip failed: %s", t.Subdomain, req.URL, err))
			return newForbiddenResponse(), nil
		}
	}
	if t.Timeout == 0 {
		return t.Transport.RoundTrip(req)
	}
	ctx, cancel := context.WithTimeout(req.Context(), t.Timeout)
	defer cancel()
	resp, err := t.Transport.RoundTrip(req.WithContext(ctx))
	if err == nil {
		return resp, nil
	}
	slog.Warn(f("subdomain %s %s roundtrip failed: %s", t.Subdomain, req.URL, err))

	// timeout
	if ctx.Err() == context.DeadlineExceeded {
		return newTimeoutResponse(t.Subdomain, req.URL.String()), nil
	}
	return resp, err
}

func newTimeoutResponse(subdomain string, u string) *http.Response {
	resp := new(http.Response)
	resp.StatusCode = http.StatusGatewayTimeout
	msg := fmt.Sprintf("%s upstream timeout: %s", subdomain, u)
	resp.Body = io.NopCloser(strings.NewReader(msg))
	return resp
}

func newForbiddenResponse() *http.Response {
	resp := new(http.Response)
	resp.StatusCode = http.StatusForbidden
	resp.Body = io.NopCloser(strings.NewReader("Forbidden"))
	return resp
}
