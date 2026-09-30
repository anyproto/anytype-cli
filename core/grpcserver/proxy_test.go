//go:build !nogrpcserver

package grpcserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/anyproto/anytype-heart/util/localorigin"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const proxyHost = "127.0.0.1:31011"

const probedMethod = "/anytype.ClientCommands/AccountLocalLinkNewChallenge"

// newTestProxy wires the real grpcweb wrapper, with the options production
// uses, in front of a sentinel that records whether the RPC was dispatched.
func newTestProxy(t *testing.T, policy *localorigin.Policy, withWebsockets bool) (http.HandlerFunc, *atomic.Bool) {
	t.Helper()

	var dispatched atomic.Bool
	sentinel := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dispatched.Store(true)
		w.WriteHeader(http.StatusOK)
	})

	// WrapServer registers the real gRPC methods; WrapHandler does not, so
	// register the probed endpoint to keep the CORS preflight check faithful.
	opts := append(wrapOptions(policy, withWebsockets), grpcweb.WithEndpointsFunc(func() []string {
		return []string{probedMethod}
	}))
	webrpc := grpcweb.WrapHandler(sentinel, opts...)
	return newProxyHandler(webrpc, policy, withWebsockets), &dispatched
}

func newGrpcWebRequest(host, origin string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, probedMethod, strings.NewReader(""))
	r.Host = host
	r.Header.Set("Content-Type", "application/grpc-web+proto")
	r.Header.Set("X-Grpc-Web", "1")
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	return r
}

func newWebsocketRequest(host, origin string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, probedMethod, nil)
	r.Host = host
	r.Header.Set("Connection", "Upgrade")
	r.Header.Set("Upgrade", "websocket")
	r.Header.Set("Sec-Websocket-Protocol", "grpc-websockets")
	r.Header.Set("Sec-Websocket-Version", "13")
	r.Header.Set("Sec-Websocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	return r
}

func TestProxyHandlerRejectsUntrustedOriginBeforeDispatch(t *testing.T) {
	tests := []struct {
		name   string
		host   string
		origin string
	}{
		{"malicious site", proxyHost, "https://evil.com"},
		{"malicious site on the proxy port", proxyHost, "https://evil.com:31011"},
		{"sandboxed iframe or data url", proxyHost, "null"},
		{"non-loopback lan origin", proxyHost, "http://192.168.1.5:3030"},
		{"dns rebinding", "evil.com:31011", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, dispatched := newTestProxy(t, newOriginPolicy("", ""), false)
			w := httptest.NewRecorder()

			handler(w, newGrpcWebRequest(tt.host, tt.origin))

			if w.Code != http.StatusForbidden {
				t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
			}
			if dispatched.Load() {
				t.Error("the rpc must not reach the handler")
			}
		})
	}
}

func TestProxyHandlerAllowsTrustedCallers(t *testing.T) {
	tests := []struct {
		name    string
		origin  string
		headers map[string]string
	}{
		{"packaged electron renderer", "", map[string]string{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "cors"}},
		{"native client", "", nil},
		{"web build on a loopback origin", "http://127.0.0.1:3030", nil},
		{"electron dev renderer", "http://localhost:8080", nil},
		{"webclipper extension", "chrome-extension://jbnammhjiplhpjfncnlejjjejghimdkf", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, dispatched := newTestProxy(t, newOriginPolicy("", ""), false)
			w := httptest.NewRecorder()
			req := newGrpcWebRequest(proxyHost, tt.origin)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			handler(w, req)

			if w.Code == http.StatusForbidden {
				t.Errorf("status = %d, want not forbidden", w.Code)
			}
			if !dispatched.Load() {
				t.Error("the rpc should reach the handler")
			}
		})
	}
}

func TestProxyHandlerHonorsConfiguredAllowlists(t *testing.T) {
	t.Run("an allowed host is accepted", func(t *testing.T) {
		handler, dispatched := newTestProxy(t, newOriginPolicy("", "anytype.example.com"), false)
		w := httptest.NewRecorder()

		handler(w, newGrpcWebRequest("anytype.example.com:31011", ""))

		if w.Code == http.StatusForbidden || !dispatched.Load() {
			t.Errorf("status = %d, dispatched = %v; want the allowed host to pass", w.Code, dispatched.Load())
		}
	})

	t.Run("an allowed origin is accepted", func(t *testing.T) {
		handler, dispatched := newTestProxy(t, newOriginPolicy("http://192.168.1.5:3030", ""), false)
		w := httptest.NewRecorder()

		handler(w, newGrpcWebRequest(proxyHost, "http://192.168.1.5:3030"))

		if w.Code == http.StatusForbidden || !dispatched.Load() {
			t.Errorf("status = %d, dispatched = %v; want the allowed origin to pass", w.Code, dispatched.Load())
		}
	})
}

func TestProxyHandlerWebsockets(t *testing.T) {
	t.Run("disabled by default, whatever the origin", func(t *testing.T) {
		for _, origin := range []string{"https://evil.com", "null", "file://", "http://127.0.0.1:3030", ""} {
			handler, dispatched := newTestProxy(t, newOriginPolicy("", ""), false)
			w := httptest.NewRecorder()

			handler(w, newWebsocketRequest(proxyHost, origin))

			if w.Code != http.StatusForbidden {
				t.Errorf("origin %q: status = %d, want %d", origin, w.Code, http.StatusForbidden)
			}
			if dispatched.Load() {
				t.Errorf("origin %q: the rpc must not reach the handler", origin)
			}
		}
	})

	t.Run("when enabled, an untrusted origin is still refused", func(t *testing.T) {
		for _, origin := range []string{"https://evil.com", "null", "http://192.168.1.5:3030"} {
			handler, dispatched := newTestProxy(t, newOriginPolicy("", ""), true)
			w := httptest.NewRecorder()

			handler(w, newWebsocketRequest(proxyHost, origin))

			if w.Code != http.StatusForbidden {
				t.Errorf("origin %q: status = %d, want %d", origin, w.Code, http.StatusForbidden)
			}
			if dispatched.Load() {
				t.Errorf("origin %q: the rpc must not reach the handler", origin)
			}
		}
	})
}

func TestWebsocketsEnabledReadsEnv(t *testing.T) {
	t.Setenv(envEnableWebsockets, "")
	if websocketsEnabled() {
		t.Error("websockets must be off by default")
	}
	t.Setenv(envEnableWebsockets, "1")
	if !websocketsEnabled() {
		t.Errorf("websockets must be on when %s=1", envEnableWebsockets)
	}
}

func TestOriginInterceptorCarriesOriginIntoContext(t *testing.T) {
	tests := []struct {
		name string
		md   metadata.MD
		want string
	}{
		{"webclipper extension origin", metadata.Pairs("origin", "chrome-extension://jbnammhjiplhpjfncnlejjjejghimdkf"), "chrome-extension://jbnammhjiplhpjfncnlejjjejghimdkf"},
		{"loopback page origin", metadata.Pairs("origin", "http://localhost:3000"), "http://localhost:3000"},
		{"native caller sends none", metadata.MD{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			handler := func(ctx context.Context, _ interface{}) (interface{}, error) {
				got = localorigin.OriginFromContext(ctx)
				return nil, nil
			}
			ctx := metadata.NewIncomingContext(context.Background(), tt.md)

			_, err := originInterceptor()(ctx, nil, &grpc.UnaryServerInfo{FullMethod: probedMethod}, handler)

			if err != nil {
				t.Fatalf("interceptor returned error: %v", err)
			}
			if got != tt.want {
				t.Errorf("origin = %q, want %q", got, tt.want)
			}
		})
	}
}
