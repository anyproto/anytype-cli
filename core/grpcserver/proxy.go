//go:build !nogrpcserver

package grpcserver

import (
	"context"
	"net/http"
	"os"

	"github.com/anyproto/anytype-heart/util/localorigin"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// Mirrors anytype-heart's cmd/grpcserver/proxy.go, which the CLI cannot import.

// envAllowedOrigins adds comma-separated exact origins to the gRPC-Web allowlist.
const envAllowedOrigins = "ANYTYPE_GRPCWEB_ALLOWED_ORIGINS"

// envAllowedHosts adds comma-separated Host header values to the allowlist, for
// servers bound to a routable interface and reached by name.
const envAllowedHosts = "ANYTYPE_GRPCWEB_ALLOWED_HOSTS"

// envEnableWebsockets re-enables the grpc-websockets transport, which is still
// origin-checked when on.
const envEnableWebsockets = "ANYTYPE_GRPCWEB_ENABLE_WEBSOCKETS"

// newOriginPolicy builds the allowlist guarding the gRPC-Web proxy: loopback
// origins, the desktop app's file:// renderer and the Webclipper extension,
// plus whatever the operator configured.
func newOriginPolicy(allowedOrigins, allowedHosts string) *localorigin.Policy {
	return localorigin.New(allowedOrigins,
		localorigin.AllowFileOrigin(),
		localorigin.AllowHosts(allowedHosts),
		localorigin.AllowWebclipperExtension(),
	)
}

// websocketsEnabled reports whether the grpc-websockets transport is on. It is
// off by default: WebSocket handshakes skip the CORS preflight, so it would let
// any site reach the RPC surface directly.
func websocketsEnabled() bool {
	return os.Getenv(envEnableWebsockets) == "1"
}

func wrapOptions(policy *localorigin.Policy, withWebsockets bool) []grpcweb.Option {
	opts := []grpcweb.Option{grpcweb.WithOriginFunc(policy.AllowOrigin)}
	if withWebsockets {
		opts = append(opts,
			grpcweb.WithWebsockets(true),
			grpcweb.WithWebsocketOriginFunc(policy.AllowRequest),
		)
	}
	return opts
}

// newProxyHandler serves gRPC-Web only to callers the policy trusts. grpcweb's
// CORS layer does not reject a disallowed origin on a non-preflight request (it
// only omits the Access-Control-* headers and dispatches anyway), so the
// request is rejected here before it reaches the handler.
func newProxyHandler(webrpc *grpcweb.WrappedGrpcServer, policy *localorigin.Policy, withWebsockets bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		isWebsocket := webrpc.IsGrpcWebSocketRequest(r)
		if isWebsocket && !withWebsockets {
			http.Error(w, "grpc-websockets transport is disabled", http.StatusForbidden)
			return
		}
		if !webrpc.IsGrpcWebRequest(r) && !webrpc.IsAcceptableGrpcCorsRequest(r) && !isWebsocket {
			return
		}
		if !policy.AllowRequest(r) {
			log.Warnf("rejected grpc-web request from untrusted origin %q (host %q)", r.Header.Get("Origin"), r.Host)
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		webrpc.ServeHTTP(w, r)
	}
}

// originInterceptor carries the Origin forwarded by the gRPC-Web proxy into the
// request context, so localorigin.OriginFromContext works on this transport.
func originInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			ctx = localorigin.WithOrigin(ctx, localorigin.OriginFromMetadata(md))
		}
		return handler(ctx, req)
	}
}
