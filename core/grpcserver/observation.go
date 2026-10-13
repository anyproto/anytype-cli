package grpcserver

import (
	"context"
	"path"

	"github.com/anyproto/anytype-cli/core/runtimeinfo"
	"github.com/anyproto/anytype-heart/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// observationStream is an internal event sink registered with the existing
// GrpcSender. It never retains messages, credentials, or account metadata keys.
type observationStream struct {
	grpc.ServerStream
	recorder *runtimeinfo.Recorder
}

func (s *observationStream) Context() context.Context { return context.Background() }
func (s *observationStream) Send(e *pb.Event) error {
	for _, m := range e.Messages {
		if v := m.GetAccountJsonApiStatus(); v != nil {
			s.recorder.APIEvent(v.ListenAddr, v.Success)
		}
	}
	return nil
}

func observationInterceptor(r *runtimeinfo.Recorder) grpc.UnaryServerInterceptor {
	order := &observationOrder{}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		method := path.Base(info.FullMethod)
		switch method {
		case "WalletCreate", "WalletRecover", "AccountCreate", "AccountSelect", "AccountStop", "AppShutdown", "AccountChangeJsonApiAddr", "AccountMove", "AccountChangeNetworkConfigAndRestart":
		default:
			resp, err := handler(ctx, req)
			if method == "AppGetVersion" {
				_ = grpc.SetHeader(ctx, metadata.Pairs(runtimeinfo.Header, r.Nonce()))
			}
			return resp, err
		}
		beginErr := order.begin(ctx, func(overlap bool) {
			switch method {
			case "WalletCreate", "WalletRecover":
				r.WalletTransition()
			case "AccountSelect":
				r.Transition()
			case "AccountCreate":
				r.AccountSelection("")
			case "AccountStop", "AppShutdown", "AccountMove", "AccountChangeNetworkConfigAndRestart":
				r.Stopping()
			}
			switch v := req.(type) {
			case *pb.RpcWalletCreateRequest:
				r.DataPath(v.RootPath)
			case *pb.RpcWalletRecoverRequest:
				r.DataPath(v.RootPath)
			case *pb.RpcAccountSelectRequest:
				r.DataPath(v.RootPath)
				r.AccountSelection(v.Id)
				r.APIStarting(v.JsonApiListenAddr)
			case *pb.RpcAccountCreateRequest:
				r.APIStarting(v.JsonApiListenAddr)
			case *pb.RpcAccountChangeJsonApiAddrRequest:
				r.APIChanging(v.ListenAddr)
			}
			if overlap {
				r.LifecycleUncertain()
			}
		})
		if beginErr != nil {
			return nil, beginErr
		}
		var resp any
		var err error
		// Always release the coordinator, including if Heart panics. Only a
		// normally returned response can contribute an observation below.
		returned := false
		defer func() {
			order.finish(func() {
				if returned {
					observeResponse(r, method, req, resp, err)
				}
			})
		}()
		resp, err = handler(ctx, req)
		returned = true
		return resp, err
	}
}

func observeResponse(r *runtimeinfo.Recorder, method string, req, resp any, err error) {
	apiOutcomeObserved := false
	stopObserved := false
	accountObserved := false
	if err == nil {
		switch v := resp.(type) {
		case *pb.RpcAccountCreateResponse:
			if v != nil && v.GetError().GetCode() == pb.RpcAccountCreateResponseError_NULL && v.Account != nil && v.Account.Id != "" {
				r.Account(v.Account)
				accountObserved = true
			}
		case *pb.RpcAccountSelectResponse:
			if v != nil && v.GetError().GetCode() == pb.RpcAccountSelectResponseError_NULL && v.Account != nil && v.Account.Id != "" {
				r.Account(v.Account)
				accountObserved = true
			}
		case *pb.RpcAccountStopResponse:
			if v != nil && v.GetError().GetCode() == pb.RpcAccountStopResponseError_NULL {
				r.Stopped()
				stopObserved = true
			}
		case *pb.RpcAppShutdownResponse:
			if v != nil && v.GetError().GetCode() == pb.RpcAppShutdownResponseError_NULL {
				r.Stopped()
				stopObserved = true
			}
		case *pb.RpcAccountMoveResponse:
			if v != nil && v.GetError().GetCode() == pb.RpcAccountMoveResponseError_NULL {
				r.Stopped()
				stopObserved = true
			}
		case *pb.RpcAccountChangeJsonApiAddrResponse:
			if v != nil && v.GetError().GetCode() == pb.RpcAccountChangeJsonApiAddrResponseError_NULL && v.Status != nil {
				r.API(v.Status.ListenAddr, v.Status.Success)
				apiOutcomeObserved = true
			} else if request, ok := req.(*pb.RpcAccountChangeJsonApiAddrRequest); ok && request.ListenAddr == "" && v != nil && v.GetError().GetCode() == pb.RpcAccountChangeJsonApiAddrResponseError_NULL {
				// Heart returns no status/event when a successful empty-address
				// reassignment disables the API listener.
				r.API("", false)
				apiOutcomeObserved = true
			}
		}
	}
	if method == "AccountChangeJsonApiAddr" && !apiOutcomeObserved {
		r.APIChangeUncertain()
	}
	if (method == "AccountStop" || method == "AppShutdown" || method == "AccountMove") && !stopObserved {
		r.StopUncertain()
	}
	// Network restart returns neither an account nor an authoritative API bind
	// result. Even success cannot prove listener continuity. A later selection
	// can establish account identity; an explicit rebind establishes the API.
	if (method == "AccountCreate" || method == "AccountSelect" || method == "AccountChangeNetworkConfigAndRestart") && !accountObserved {
		r.AccountStartUncertain()
	}
}
