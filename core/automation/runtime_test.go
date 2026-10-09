package automation

import (
	"context"
	"os"
	"testing"

	"github.com/anyproto/anytype-cli/core/localstate"
	"github.com/anyproto/anytype-cli/core/runtimeinfo"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type versionRPC struct {
	service.ClientCommandsClient
	nonce string
}

func (v *versionRPC) AppGetVersion(_ context.Context, _ *pb.RpcAppGetVersionRequest, opts ...grpc.CallOption) (*pb.RpcAppGetVersionResponse, error) {
	for _, opt := range opts {
		if h, ok := opt.(grpc.HeaderCallOption); ok {
			*h.HeaderAddr = metadata.Pairs(runtimeinfo.Header, v.nonce)
		}
	}
	return &pb.RpcAppGetVersionResponse{Version: "test"}, nil
}
func TestRuntimeObservationMustMatchLiveNonceAndBinding(t *testing.T) {
	for _, kind := range []string{"valid", "missing", "stale", "wrong_config", "wrong_data", "wrong_pid", "old_service"} {
		t.Run(kind, func(t *testing.T) {
			e, _, _ := fixture(t, false)
			e.Live = nil
			v := &versionRPC{nonce: "nonce-current"}
			e.Client = v
			s := runtimeinfo.Snapshot{SchemaVersion: 1, Nonce: v.nonce, PID: 123, ConfigPath: e.ConfigPath, DataPath: e.DataPath}
			switch kind {
			case "stale":
				s.Nonce = "prior-process"
			case "wrong_config":
				s.ConfigPath += "wrong"
			case "wrong_data":
				s.DataPath += "wrong"
			case "wrong_pid":
				s.PID = 0
			case "old_service":
				v.nonce = ""
			}
			if kind != "missing" {
				if err := localstate.WriteJSON(e.RuntimePath, s); err != nil {
					t.Fatal(err)
				}
			}
			_, err := e.live(context.Background())
			if kind == "valid" && err != nil {
				t.Fatal(err)
			}
			if kind != "valid" && err == nil {
				t.Fatal("invalid runtime accepted")
			}
			if _, statErr := os.Stat(e.StateDir); !os.IsNotExist(statErr) {
				t.Fatal("inspection created state")
			}
		})
	}
}

func TestInspectionMakesUnverifiedAPIBindingActionable(t *testing.T) {
	for _, observation := range []string{"unverified_event_order", "rpc_outcome_unknown", "stop_outcome_unknown", "concurrent_lifecycle"} {
		e, rpc, _ := fixture(t, true)
		rpc.snapshot.APIObservation = observation
		rpc.snapshot.APIAddress, rpc.snapshot.APIBound = "", false
		out := run(e, Request{Command: "inspect"})
		if out.ExitCode() != 0 {
			t.Fatal(out)
		}
		result := out.Result.(map[string]any)
		api := result["api"].(map[string]any)
		mcp := result["mcp"].(map[string]any)
		if api["origin"] != "" || api["bound"] != false || api["ready"] != false || api["binding_observation"] != observation || api["next_action"] == "" || mcp["url"] != "" || mcp["ready"] != false {
			t.Fatal("unverified binding exposed endpoints or lacked next action", result)
		}
	}
}

func TestInspectionExplainsUncertainAccountLifecycle(t *testing.T) {
	for _, observation := range []string{"stop_outcome_unknown", "concurrent_lifecycle", "account_start_outcome_unknown"} {
		e, rpc, _ := fixture(t, true)
		rpc.snapshot.AccountTransition = true
		rpc.snapshot.APIObservation = observation
		rpc.snapshot.APIAddress, rpc.snapshot.APIBound = "", false
		out := run(e, Request{Command: "inspect"})
		assertCode(t, out, "account_transition")
		result := out.Result.(map[string]any)
		api := result["api"].(map[string]any)
		if api["origin"] != "" || api["bound"] != false || api["binding_observation"] != observation || api["next_action"] == "" || result["account"].(Account).Authenticated {
			t.Fatal(out)
		}
	}
}
