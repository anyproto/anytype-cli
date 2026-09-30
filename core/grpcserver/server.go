//go:build !nogrpcserver
// +build !nogrpcserver

package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/anytype-heart/core/api"
	"github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"google.golang.org/grpc"

	"github.com/anyproto/anytype-heart/core"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/grpcprocess"
)

var log = logging.Logger("anytype-heart")

const grpcWebStartedMessagePrefix = "gRPC Web proxy started at: "

type Server struct {
	mw           *core.Middleware
	grpcServer   *grpc.Server
	webServer    *http.Server
	grpcListener net.Listener
	webListener  net.Listener
}

func NewServer() *Server {
	return &Server{}
}

func (s *Server) Start(grpcAddr, grpcWebAddr string) error {
	app.StartWarningAfter = time.Second * 5

	applyLogLevel()

	metrics.Service.InitWithKeys(metrics.DefaultInHouseKey)

	log.Info("Starting anytype-heart...")
	s.mw = core.New()
	s.mw.SetEventSender(event.NewGrpcSender())

	var err error
	s.grpcListener, err = net.Listen("tcp", grpcAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", grpcAddr, err)
	}

	s.webListener, err = net.Listen("tcp", grpcWebAddr)
	if err != nil {
		s.grpcListener.Close()
		return fmt.Errorf("failed to listen on %s: %w", grpcWebAddr, err)
	}

	var unaryInterceptors []grpc.UnaryServerInterceptor

	if metrics.Enabled {
		unaryInterceptors = append(unaryInterceptors, grpc_prometheus.UnaryServerInterceptor)
	}

	unaryInterceptors = append(unaryInterceptors, metrics.UnaryTraceInterceptor)
	unaryInterceptors = append(unaryInterceptors, func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		resp, err = s.mw.Authorize(ctx, req, info, handler)
		if err != nil {
			log.Errorf("authorize: %s", err)
		}
		return
	})

	if os.Getenv("ANYTYPE_GRPC_NO_DEBUG_TIMEOUT") != "1" {
		unaryInterceptors = append(unaryInterceptors, metrics.LongMethodsInterceptor)
	}

	unaryInterceptors = append(unaryInterceptors, grpcprocess.ProcessInfoInterceptor(
		"/anytype.ClientCommands/AccountLocalLinkNewChallenge",
	))
	unaryInterceptors = append(unaryInterceptors, originInterceptor())

	s.grpcServer = grpc.NewServer(
		grpc.MaxRecvMsgSize(20*1024*1024),
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(unaryInterceptors...)),
	)

	service.RegisterClientCommandsServer(s.grpcServer, s.mw)

	if metrics.Enabled {
		grpc_prometheus.EnableHandlingTimeHistogram()
	}

	originPolicy := newOriginPolicy(os.Getenv(envAllowedOrigins), os.Getenv(envAllowedHosts))
	withWebsockets := websocketsEnabled()
	webrpc := grpcweb.WrapServer(s.grpcServer, wrapOptions(originPolicy, withWebsockets)...)

	s.webServer = &http.Server{
		Handler:           newProxyHandler(webrpc, originPolicy, withWebsockets),
		ReadHeaderTimeout: 30 * time.Second,
	}

	go func() {
		log.Infof("Starting gRPC server on %s", s.grpcListener.Addr())
		if err := s.grpcServer.Serve(s.grpcListener); err != nil {
			log.Errorf("gRPC server error: %v", err)
		}
	}()

	go func() {
		fmt.Printf("%s%s\n", grpcWebStartedMessagePrefix, s.webListener.Addr())
		if err := s.webServer.Serve(s.webListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("gRPC-Web server error: %v", err)
		}
	}()

	api.SetMiddlewareParams(s.mw)

	return nil
}

func (s *Server) Stop() error {
	log.Info("Shutting down servers...")

	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}

	if s.webServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.webServer.Shutdown(ctx); err != nil {
			log.Errorf("HTTP server shutdown error: %v", err)
		}
	}

	if s.mw != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.mw.AppShutdown(ctx, &pb.RpcAppShutdownRequest{})
	}

	log.Info("Servers stopped")
	return nil
}

// defaultLogLevel applies when ANYTYPE_LOG_LEVEL is unset.
const defaultLogLevel = "ERROR"

// applyLogLevel applies ANYTYPE_LOG_LEVEL to heart's loggers right away.
// heart applies it itself only when an account logs in (InitialSetParameters),
// so without this everything logged before login uses the logger's built-in
// DEBUG default.
func applyLogLevel() {
	if os.Getenv("ANYTYPE_LOG_LEVEL") == "" {
		os.Setenv("ANYTYPE_LOG_LEVEL", defaultLogLevel)
	}
	logging.SetLogLevels(os.Getenv("ANYTYPE_LOG_LEVEL"))
}
