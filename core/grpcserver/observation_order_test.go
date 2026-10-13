package grpcserver

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/anyproto/anytype-cli/core/localstate"
	"github.com/anyproto/anytype-cli/core/runtimeinfo"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"google.golang.org/grpc"
)

type observationFixture struct {
	file        string
	r           *runtimeinfo.Recorder
	interceptor grpc.UnaryServerInterceptor
	t           *testing.T
}

func newObservationFixture(t *testing.T) *observationFixture {
	t.Helper()
	file := filepath.Join(t.TempDir(), "runtime.json")
	r, err := runtimeinfo.New(file, "config", "data")
	if err != nil {
		t.Fatal(err)
	}
	return &observationFixture{file: file, r: r, interceptor: observationInterceptor(r), t: t}
}

func (f *observationFixture) snapshot() runtimeinfo.Snapshot {
	f.t.Helper()
	var snapshot runtimeinfo.Snapshot
	if err := localstate.ReadJSON(f.file, &snapshot); err != nil {
		f.t.Fatal(err)
	}
	return snapshot
}

func (f *observationFixture) event(address string, success bool) {
	f.t.Helper()
	stream := &observationStream{recorder: f.r}
	if err := stream.Send(&pb.Event{Messages: []*pb.EventMessage{{Value: &pb.EventMessageValueOfAccountJsonApiStatus{AccountJsonApiStatus: &pb.EventAccountJsonApiStatus{ListenAddr: address, Success: success}}}}}); err != nil {
		f.t.Fatal(err)
	}
}

func (f *observationFixture) rebind(address string, response *pb.RpcAccountChangeJsonApiAddrResponse, rpcErr error) {
	f.t.Helper()
	_, _ = f.interceptor(context.Background(), &pb.RpcAccountChangeJsonApiAddrRequest{ListenAddr: address}, &grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/AccountChangeJsonApiAddr"}, func(context.Context, any) (any, error) { return response, rpcErr })
}

func (f *observationFixture) selectAccount(address string, during func()) {
	f.t.Helper()
	_, err := f.interceptor(context.Background(), &pb.RpcAccountSelectRequest{Id: "account-a", JsonApiListenAddr: address}, &grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/AccountSelect"}, func(context.Context, any) (any, error) {
		if during != nil {
			during()
		}
		return &pb.RpcAccountSelectResponse{Account: &model.Account{Id: "account-a"}}, nil
	})
	if err != nil {
		f.t.Fatal(err)
	}
}

func bindResponse(address string, success bool) *pb.RpcAccountChangeJsonApiAddrResponse {
	return &pb.RpcAccountChangeJsonApiAddrResponse{Status: &pb.EventAccountJsonApiStatus{ListenAddr: address, Success: success}}
}

func TestNewerAPIOutcomesRejectAllDelayedStatusEvents(t *testing.T) {
	for _, outcome := range []string{"disable", "new_address_success", "same_address_success", "new_address_failure", "same_address_failure", "protobuf_error", "transport_error"} {
		t.Run(outcome, func(t *testing.T) {
			f := newObservationFixture(t)
			f.r.Account(&model.Account{Id: "account-a"})
			f.rebind("127.0.0.1:4500", bindResponse("127.0.0.1:4500", true), nil)
			address := "127.0.0.1:4600"
			response := bindResponse(address, true)
			var rpcErr error
			switch outcome {
			case "disable":
				address, response = "", &pb.RpcAccountChangeJsonApiAddrResponse{}
			case "same_address_success":
				address = "127.0.0.1:4500"
				response = bindResponse(address, true)
			case "new_address_failure":
				response = bindResponse(address, false)
			case "same_address_failure":
				address = "127.0.0.1:4500"
				response = bindResponse(address, false)
			case "protobuf_error":
				response = &pb.RpcAccountChangeJsonApiAddrResponse{Error: &pb.RpcAccountChangeJsonApiAddrResponseError{Code: pb.RpcAccountChangeJsonApiAddrResponseErrorCode(101)}}
			case "transport_error":
				response = nil
				rpcErr = errors.New("unknown outcome")
			}
			f.rebind(address, response, rpcErr)
			before := f.snapshot()
			if outcome == "disable" && (before.APIBound || before.APIAddress != "") {
				t.Fatal(before)
			}
			if outcome == "protobuf_error" || outcome == "transport_error" {
				if before.APIBound || before.APIAddress != "" || before.APIObservation != "rpc_outcome_unknown" {
					t.Fatal(before)
				}
			}
			// Both a stale success and a stale failure must lose to the newer
			// RPC result, including when their address equals the newer result.
			for _, oldAddress := range []string{"127.0.0.1:4500", "127.0.0.1:4600"} {
				for _, success := range []bool{true, false} {
					f.event(oldAddress, success)
					if after := f.snapshot(); after != before {
						t.Fatal("delayed event replaced newer RPC evidence", before, after)
					}
				}
			}
			f.rebind("127.0.0.1:4700", bindResponse("127.0.0.1:4700", true), nil)
			f.event("127.0.0.1:4500", false)
			if after := f.snapshot(); !after.APIBound || after.APIAddress != "127.0.0.1:4700" {
				t.Fatal("explicit enable not observed", after)
			}
		})
	}
}

func TestFirstAccountStartupEventBeforeAndAfterResponse(t *testing.T) {
	for _, beforeResponse := range []bool{true, false} {
		f := newObservationFixture(t)
		var during func()
		if beforeResponse {
			during = func() { f.event("127.0.0.1:4500", true) }
		}
		f.selectAccount("127.0.0.1:4500", during)
		if !beforeResponse {
			f.event("127.0.0.1:4500", true)
		}
		if s := f.snapshot(); !s.APIBound || s.APIAddress != "127.0.0.1:4500" || s.AccountId != "account-a" || s.AccountTransition {
			t.Fatal(s)
		}
	}
}

func TestStartupAfterStopRejectsStaleEventsAndReportsAmbiguity(t *testing.T) {
	for _, sameAddress := range []bool{false, true} {
		f := newObservationFixture(t)
		f.selectAccount("127.0.0.1:4500", nil)
		f.event("127.0.0.1:4500", true)
		f.r.Stopped()
		f.event("127.0.0.1:4500", true)
		if s := f.snapshot(); s.APIBound || s.APIAddress != "" {
			t.Fatal("stopped listener resurrected", s)
		}
		address := "127.0.0.1:4600"
		if sameAddress {
			address = "127.0.0.1:4500"
		}
		f.selectAccount(address, nil)
		f.event("127.0.0.1:4500", true)
		if s := f.snapshot(); s.APIBound || s.APIAddress != "" {
			t.Fatal("old address attributed to new startup", s)
		}
		f.event(address, true)
		if s := f.snapshot(); sameAddress {
			if s.APIBound || s.APIObservation != "unverified_event_order" {
				t.Fatal("same-address event given invented freshness", s)
			}
		} else if !s.APIBound || s.APIAddress != address {
			t.Fatal("unambiguous later startup not observed", s)
		}
		// A fresh server process has no delayed events from the previous sender.
		var err error
		f.r, err = runtimeinfo.New(f.file, "config", "data")
		if err != nil {
			t.Fatal(err)
		}
		f.interceptor = observationInterceptor(f.r)
		f.selectAccount("127.0.0.1:4500", nil)
		f.event("127.0.0.1:4500", true)
		if s := f.snapshot(); !s.APIBound {
			t.Fatal("fresh-process startup blocked", s)
		}
	}
}

func TestSameAccountRecoveryRetainsDisableAgainstOldEvents(t *testing.T) {
	f := newObservationFixture(t)
	f.r.Account(&model.Account{Id: "account-a"})
	f.rebind("127.0.0.1:4500", bindResponse("127.0.0.1:4500", true), nil)
	f.rebind("", &pb.RpcAccountChangeJsonApiAddrResponse{}, nil)
	f.r.WalletTransition()
	f.selectAccount("127.0.0.1:4500", nil)
	f.event("127.0.0.1:4500", true)
	if s := f.snapshot(); s.APIBound || s.APIAddress != "" || s.APIObservation != "rpc_response" {
		t.Fatal("recovery lost authoritative disable", s)
	}
}

func TestConcurrentRebindHandlersAreNotQueuedAndRemainUnverified(t *testing.T) {
	f := newObservationFixture(t)
	f.r.Account(&model.Account{Id: "account-a"})
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_, _ = f.interceptor(context.Background(), &pb.RpcAccountChangeJsonApiAddrRequest{ListenAddr: "127.0.0.1:4500"}, &grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/AccountChangeJsonApiAddr"}, func(context.Context, any) (any, error) {
			close(entered)
			<-release
			return bindResponse("127.0.0.1:4500", true), nil
		})
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	called := false
	_, err := f.interceptor(ctx, &pb.RpcAccountChangeJsonApiAddrRequest{}, &grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/AccountChangeJsonApiAddr"}, func(context.Context, any) (any, error) {
		called = true
		return &pb.RpcAccountChangeJsonApiAddrResponse{}, nil
	})
	close(release)
	<-finished
	if err != nil || !called {
		t.Fatal("observer prevented native handler entry", called, err)
	}
	if s := f.snapshot(); s.APIBound || s.APIAddress != "" || s.AccountId != "account-a" || s.AccountTransition || s.APIObservation != "concurrent_lifecycle" {
		t.Fatal("overlap advertised unproven native effect order", s)
	}
	f.rebind("127.0.0.1:4600", bindResponse("127.0.0.1:4600", true), nil)
	before := f.snapshot()
	if !before.APIBound || before.AccountTransition {
		t.Fatal("independent rebind did not restore evidence", before)
	}
	canceled, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	_, err = f.interceptor(canceled, &pb.RpcAccountStopRequest{}, &grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/AccountStop"}, func(context.Context, any) (any, error) {
		t.Fatal("already canceled request reached handler")
		return nil, nil
	})
	if s := f.snapshot(); !errors.Is(err, context.Canceled) || s != before {
		t.Fatal(s)
	}
}

func TestStopAndShutdownCancelInFlightStartWithoutWaitingForItsHandler(t *testing.T) {
	for _, method := range []string{"AccountStop", "AppShutdown"} {
		t.Run(method, func(t *testing.T) {
			f := newObservationFixture(t)
			startEntered := make(chan struct{})
			stopEntered := make(chan struct{})
			releaseOldResponse := make(chan struct{})
			startDone := make(chan struct{})
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			go func() {
				defer close(startDone)
				_, _ = f.interceptor(ctx, &pb.RpcAccountSelectRequest{Id: "account-a", JsonApiListenAddr: "127.0.0.1:4500"}, &grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/AccountSelect"}, func(context.Context, any) (any, error) {
					close(startEntered)
					// Heart's recovery can only finish after stop enters and cancels it.
					select {
					case <-stopEntered:
					case <-ctx.Done():
						return nil, ctx.Err()
					}
					<-releaseOldResponse
					return &pb.RpcAccountSelectResponse{Account: &model.Account{Id: "account-a"}}, nil
				})
			}()
			<-startEntered
			var request any = &pb.RpcAccountStopRequest{}
			var response any = &pb.RpcAccountStopResponse{}
			if method == "AppShutdown" {
				request, response = &pb.RpcAppShutdownRequest{}, &pb.RpcAppShutdownResponse{}
			}
			_, err := f.interceptor(ctx, request, &grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/" + method}, func(context.Context, any) (any, error) {
				close(stopEntered)
				return response, nil
			})
			close(releaseOldResponse)
			<-startDone
			if err != nil {
				t.Fatal("cancellation could not reach Heart while start was blocked", err)
			}
			select {
			case <-stopEntered:
			default:
				t.Fatal("cancellation handler never entered")
			}
			f.event("127.0.0.1:4500", true)
			if s := f.snapshot(); s.AccountId != "" || !s.AccountTransition || s.APIBound || s.APIObservation != "concurrent_lifecycle" {
				t.Fatal("overlapping start/cancellation claimed a known outcome", s)
			}
			// A later independent stop can establish a known stopped outcome.
			_, err = f.interceptor(context.Background(), request, &grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/" + method}, func(context.Context, any) (any, error) { return response, nil })
			if s := f.snapshot(); err != nil || s.AccountTransition || s.APIBound || s.APIObservation != "stopped" {
				t.Fatal(s, err)
			}
		})
	}
}

func TestNewAccountStartsSupersedeBlockedRecoveryWithoutObserverGate(t *testing.T) {
	for _, method := range []string{"AccountSelect", "AccountCreate"} {
		t.Run(method, func(t *testing.T) {
			f := newObservationFixture(t)
			firstEntered := make(chan struct{})
			newerEntered := make(chan struct{})
			firstDone := make(chan struct{})
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			go func() {
				defer close(firstDone)
				_, _ = f.interceptor(ctx, &pb.RpcAccountSelectRequest{Id: "account-a", JsonApiListenAddr: "127.0.0.1:4500"}, &grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/AccountSelect"}, func(context.Context, any) (any, error) {
					close(firstEntered)
					select {
					case <-newerEntered:
					case <-ctx.Done():
						return nil, ctx.Err()
					}
					return nil, context.Canceled
				})
			}()
			<-firstEntered
			var request any = &pb.RpcAccountSelectRequest{Id: "account-b", JsonApiListenAddr: "127.0.0.1:4600"}
			var response any = &pb.RpcAccountSelectResponse{Account: &model.Account{Id: "account-b"}}
			if method == "AccountCreate" {
				request = &pb.RpcAccountCreateRequest{Name: "New account", JsonApiListenAddr: "127.0.0.1:4600"}
				response = &pb.RpcAccountCreateResponse{Account: &model.Account{Id: "account-b"}}
			}
			called := false
			_, err := f.interceptor(ctx, request, &grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/" + method}, func(context.Context, any) (any, error) {
				called = true
				close(newerEntered)
				<-firstDone
				return response, nil
			})
			<-firstDone
			if err != nil || !called {
				t.Fatal("new start could not reach Heart to supersede recovery", err)
			}
			f.event("127.0.0.1:4600", true)
			if s := f.snapshot(); s.APIBound || !s.AccountTransition || s.APIObservation != "concurrent_lifecycle" {
				t.Fatal("overlap outcome was guessed", s)
			}
			f.selectAccount("127.0.0.1:4700", nil)
			f.event("127.0.0.1:4700", true)
			if s := f.snapshot(); !s.APIBound || s.AccountTransition || s.AccountId != "account-a" {
				t.Fatal("independent account observation did not recover", s)
			}
		})
	}
}

func TestFailedStopAndShutdownDiscardBindingBeforeHandler(t *testing.T) {
	for _, method := range []string{"AccountStop", "AppShutdown"} {
		for _, transportFailure := range []bool{false, true} {
			f := newObservationFixture(t)
			f.r.Account(&model.Account{Id: "account-a"})
			f.rebind("127.0.0.1:4500", bindResponse("127.0.0.1:4500", true), nil)
			var request any = &pb.RpcAccountStopRequest{}
			var response any = &pb.RpcAccountStopResponse{Error: &pb.RpcAccountStopResponseError{Code: pb.RpcAccountStopResponseError_FAILED_TO_STOP_NODE}}
			if method == "AppShutdown" {
				request = &pb.RpcAppShutdownRequest{}
				response = &pb.RpcAppShutdownResponse{Error: &pb.RpcAppShutdownResponseError{Code: pb.RpcAppShutdownResponseErrorCode(101)}}
			}
			_, _ = f.interceptor(context.Background(), request, &grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/" + method}, func(context.Context, any) (any, error) {
				if s := f.snapshot(); s.APIBound || !s.AccountTransition || s.APIObservation != "stop_pending" {
					t.Fatal("stop entry retained binding", s)
				}
				if transportFailure {
					return nil, errors.New("close outcome lost")
				}
				return response, nil
			})
			if s := f.snapshot(); s.APIBound || !s.AccountTransition || s.APIObservation != "stop_outcome_unknown" {
				t.Fatal("failed stop claimed a known state", s)
			}
			f.selectAccount("127.0.0.1:4500", func() { f.event("127.0.0.1:4500", false) })
			if s := f.snapshot(); s.APIBound || s.APIAddress != "" || s.APIObservation != "unverified_event_order" {
				t.Fatal("same-account restart revived pre-stop listener", s)
			}
		}
	}
}

func TestWildcardStartupMatchingAndHistoryShareAlias(t *testing.T) {
	for _, addresses := range [][2]string{{"0.0.0.0:4500", "[::]:4500"}, {"[::]:4500", "0.0.0.0:4500"}} {
		f := newObservationFixture(t)
		f.selectAccount(addresses[0], nil)
		f.event(addresses[1], true)
		if s := f.snapshot(); !s.APIBound || s.APIAddress != addresses[1] {
			t.Fatal("fresh wildcard event was not matched", s)
		}
		f.r.Stopped()
		f.selectAccount(addresses[1], nil)
		f.event(addresses[1], true)  // prior startup's delayed success
		f.event(addresses[1], false) // current bind failure cannot identify its generation
		if s := f.snapshot(); s.APIBound || s.APIAddress != "" || s.APIObservation != "unverified_event_order" {
			t.Fatal("wildcard spelling invented event freshness", s)
		}
	}
}

func TestDifferentUnicastAddressesKeepDistinctStartupEvidence(t *testing.T) {
	f := newObservationFixture(t)
	f.selectAccount("127.0.0.1:4500", nil)
	f.event("[::1]:4500", true)
	if s := f.snapshot(); s.APIBound {
		t.Fatal("unicast address mismatch accepted", s)
	}
	f.event("127.0.0.1:4500", true)
	f.r.Stopped()
	f.selectAccount("[::1]:4500", nil)
	f.event("127.0.0.1:4500", true)
	if s := f.snapshot(); s.APIBound {
		t.Fatal("old unicast event attributed to different endpoint", s)
	}
	f.event("[::1]:4500", true)
	if s := f.snapshot(); !s.APIBound || s.APIAddress != "[::1]:4500" {
		t.Fatal("unicast history incorrectly conflated", s)
	}
}

func TestInterruptedStartupCannotReuseEarlierBindingEvent(t *testing.T) {
	f := newObservationFixture(t)
	_, _ = f.interceptor(context.Background(), &pb.RpcAccountSelectRequest{Id: "account-a", JsonApiListenAddr: "127.0.0.1:4500"}, &grpc.UnaryServerInfo{FullMethod: "/anytype.ClientCommands/AccountSelect"}, func(context.Context, any) (any, error) {
		f.event("127.0.0.1:4500", true)
		return nil, errors.New("account startup failed after API bind")
	})
	f.selectAccount("127.0.0.1:4500", nil)
	f.event("127.0.0.1:4500", true)
	if s := f.snapshot(); s.APIBound || s.APIAddress != "" || s.APIObservation != "unverified_event_order" {
		t.Fatal(s)
	}
}
