package automation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anyproto/anytype-cli/core"
	"github.com/anyproto/anytype-cli/core/config"
	"github.com/anyproto/anytype-cli/core/localstate"
	"github.com/anyproto/anytype-cli/core/runtimeinfo"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func New() (*Engine, func(), error) {
	conn, err := grpc.NewClient(config.GRPCDNSAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, problem("rpc_unavailable", "Cannot connect to the Anytype service.", 4)
	}
	e := &Engine{Client: service.NewClientCommandsClient(conn), ConfigPath: config.GetConfigFilePath(), DataPath: config.GetDataDir(), WorkPath: config.GetWorkDir(), StateDir: localstate.Dir(), RuntimePath: runtimeinfo.Path(), Timeout: 45 * time.Second}
	if absolute, err := filepath.Abs(e.DataPath); err == nil {
		e.DataPath = absolute
	}
	e.Secrets = NewSecrets(e.StateDir, e.ConfigPath)
	e.Service = NewNativeService()
	return e, func() { _ = conn.Close() }, nil
}

func (e *Engine) live(ctx context.Context) (*runtimeinfo.Snapshot, error) {
	if e.Live != nil {
		return e.Live(ctx)
	}
	rpcCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var header metadata.MD
	response, err := e.Client.AppGetVersion(rpcCtx, &pb.RpcAppGetVersionRequest{}, grpc.Header(&header))
	if err != nil {
		return nil, rpcError(err)
	}
	if response == nil || response.GetError().GetCode() != pb.RpcAppGetVersionResponseError_NULL {
		return nil, problem("rpc_unavailable", "The service did not answer its health probe.", 4)
	}
	nonce := header.Get(runtimeinfo.Header)
	if len(nonce) != 1 || nonce[0] == "" {
		return nil, problem("service_upgrade_required", "The running service does not provide CLI runtime observations. Explicitly upgrade/restart that service before native automation.", 1)
	}
	observation := new(runtimeinfo.Snapshot)
	if localstate.ReadJSON(e.RuntimePath, observation) != nil || observation.SchemaVersion != 1 || observation.Nonce != nonce[0] || observation.PID <= 0 || filepath.Clean(observation.ConfigPath) != filepath.Clean(e.ConfigPath) || filepath.Clean(observation.DataPath) != filepath.Clean(e.DataPath) {
		return nil, problem("instance_mismatch", "The live service does not match this CLI configuration and data binding.", 2)
	}
	return observation, nil
}

func rpcError(err error) error {
	switch status.Code(err) {
	case codes.Unauthenticated, codes.PermissionDenied:
		return problem("authentication_required", "The native session is unavailable or expired. Log in explicitly with the expected account.", 2)
	case codes.DeadlineExceeded, codes.Canceled:
		return problem("rpc_timeout", "The Anytype RPC timed out or was canceled. Resume the same operation ID for a mutation.", 4)
	case codes.Unimplemented:
		return problem("unsupported_rpc", "The running service does not implement the required RPC.", 1)
	default:
		return problem("rpc_unavailable", "The Anytype RPC did not complete. Resume the same operation ID for a mutation.", 4)
	}
}
func protoError(method, code string) error {
	if code == "NULL" {
		return nil
	}
	exit := 2
	if code == "UNKNOWN_ERROR" || code == "REQUEST_FAILED" {
		exit = 4
	}
	return problem("rpc_"+strings.ToLower(code), method+" did not complete ("+code+"). Resume the same operation ID after inspection.", exit)
}
func (e *Engine) authContext(ctx context.Context) (context.Context, string, error) {
	token, storage, err := e.Secrets.Get("session-token")
	if err != nil || token == "" {
		return nil, "", problem("authentication_required", "A native session is required. Log in with the expected account.", 2)
	}
	return metadata.AppendToOutgoingContext(ctx, "token", token), storage, nil
}

func (e *Engine) account(ctx context.Context, s *runtimeinfo.Snapshot) (Account, error) {
	a := Account{Id: s.AccountId, Observation: "live"}
	if s.AccountTransition {
		return a, problem("account_transition", "An account transition is incomplete. Resume its operation before continuing.", 2)
	}
	if s.AccountId == "" {
		return a, nil
	}
	ctx, storage, err := e.authContext(ctx)
	if err != nil {
		return a, err
	}
	a.CredentialStorage = storage
	if s.ProfileId == "" || s.TechSpaceId == "" {
		return a, problem("account_info_unavailable", "The service has not observed complete account information.", 4)
	}
	response, err := e.Client.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{SpaceId: s.TechSpaceId,
		Filters: []*model.BlockContentDataviewFilter{{RelationKey: bundle.RelationKeyId.String(), Condition: model.BlockContentDataviewFilter_Equal, Value: pbtypes.String(s.ProfileId)}},
		Keys:    []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String()}, Limit: 2})
	if err != nil {
		return a, rpcError(err)
	}
	if err = protoError("Read account profile", response.GetError().GetCode().String()); err != nil {
		return a, err
	}
	if len(response.Records) != 1 || pbtypes.GetString(response.Records[0], "id") != s.ProfileId {
		return a, problem("profile_unavailable", "The account profile is not yet readable.", 4)
	}
	a.DisplayName = pbtypes.GetString(response.Records[0], "name")
	a.Authenticated = true
	return a, nil
}
func guard(s *runtimeinfo.Snapshot, expected string, allowEmpty bool) error {
	if expected == "" {
		return problem("expected_account_required", "Provide --expect-account with the full account ID (or none for creation).", 2)
	}
	if s.AccountTransition {
		return problem("account_transition", "An account transition is incomplete; resume its operation.", 2)
	}
	if expected == "none" {
		if s.AccountId != "" {
			return problem("account_mismatch", "An account already exists; creation cannot replace it.", 2)
		}
		return nil
	}
	if s.AccountId == expected || allowEmpty && s.AccountId == "" {
		return nil
	}
	return problem("account_mismatch", "The live account differs from the expected account; migration requires a separate explicit workflow.", 2)
}

func (e *Engine) Run(ctx context.Context, r Request) Envelope {
	if r.Instance != "" && r.Instance != "default" {
		return Failure(problem("unsupported_instance", "Only --instance default is supported; Hermes profiles do not create isolated native instances.", 1))
	}
	if e.Timeout <= 0 {
		e.Timeout = 45 * time.Second
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.Timeout)
		defer cancel()
	}
	if r.Command == "capabilities" {
		return Success(capabilities(), false)
	}
	switch r.Command {
	case "inspect", "auth status", "service status":
		return e.inspect(ctx, r)
	case "space list", "space status", "auth apikey list":
		return e.read(ctx, r)
	case "auth rename":
		return Failure(problem("unsupported_operation", "Account rename is not supported by this automation contract. Reuse preserves the existing name.", 1))
	}
	unlock, err := localstate.Lock(e.StateDir)
	if err != nil {
		if errors.Is(err, localstate.ErrBusy) {
			return Failure(problem("instance_busy", "Another CLI operation owns this instance. Retry with the same operation ID.", 4))
		}
		return Failure(problem("coordination_failed", "Cannot acquire instance coordination.", 2))
	}
	defer unlock()
	if r.Command == "auth create" {
		r.Name, err = ValidateName(r.Name)
		if err != nil {
			return Failure(err)
		}
	}
	op, retry, err := e.operation(r)
	if err != nil {
		return Failure(err)
	}
	switch r.Command {
	case "service install", "service start":
		return e.serviceMutation(ctx, r, op, retry)
	case "auth create", "auth login":
		return e.authMutation(ctx, r, op, retry)
	case "space join":
		return e.join(ctx, r, op, retry)
	case "auth apikey create", "auth apikey revoke":
		return e.keyMutation(ctx, r, op, retry)
	default:
		return Failure(problem("unsupported_operation", "This command has no machine interface.", 1))
	}
}

func (e *Engine) inspect(ctx context.Context, r Request) Envelope {
	c, err := e.config()
	if err != nil {
		return Failure(err)
	}
	svc, svcErr := e.runtimeServiceStatus(ctx)
	result := map[string]any{"cli_version": core.GetVersion(), "instance": map[string]any{"id": "default", "config_path": e.ConfigPath, "data_path": e.DataPath, "work_path": e.WorkPath, "grpc_address": config.DefaultGRPCAddress, "grpc_web_address": config.DefaultGRPCWebAddress, "isolated": false}, "service": svc, "capabilities": capabilities(), "account": Account{Id: c.AccountId, Observation: "configured"}, "network": map[string]any{"id": c.NetworkId, "config_path": c.NetworkConfigPath, "observation": "configured"}, "api": map[string]any{"configured_origin": apiOrigin(config.ResolveAPIListenAddr("", false, c.ApiListenAddr)), "ready": false, "observation": "configured"}}
	result["transport"] = transportJSON(c.Settings)
	s, err := e.live(ctx)
	if err != nil {
		out := Failure(err)
		out.Result = result
		return out
	}
	svc.Health = "grpc_ready"
	svc.RuntimePID = s.PID
	if svc.Running && svc.PID > 0 && svc.PID != s.PID {
		out := Failure(problem("service_instance_mismatch", "The supervisor and native RPC endpoint belong to different processes.", 2))
		out.Result = result
		return out
	}
	result["service"] = svc
	a, aErr := e.account(ctx, s)
	result["account"] = a
	result["network"] = map[string]any{"id": s.NetworkId, "config_path": c.NetworkConfigPath, "observation": "live"}
	origin := ""
	if s.APIBound && !s.AccountTransition && s.AccountId != "" {
		origin = apiOrigin(s.APIAddress)
	}
	apiNextAction := ""
	if s.APIObservation == "unverified_event_order" || s.APIObservation == "rpc_outcome_unknown" || s.APIObservation == "stop_outcome_unknown" || s.APIObservation == "concurrent_lifecycle" || s.APIObservation == "account_start_outcome_unknown" {
		apiNextAction = "API binding is unverified. Explicitly restart the native service or rebind with AccountChangeJsonApiAddr, then inspect and probe with the integration credential."
		if s.AccountTransition {
			apiNextAction = "Account lifecycle outcome is unverified. Resume the interrupted account operation or explicitly restart the native service, then inspect before probing with the integration credential."
		}
	}
	result["api"] = map[string]any{"origin": origin, "listen_address": s.APIAddress, "bound": origin != "", "ready": false, "readiness": "requires_integration_key_probe", "observation": "live", "binding_observation": s.APIObservation, "next_action": apiNextAction}
	mcpURL := ""
	if origin != "" {
		mcpURL = origin + "/mcp/full"
	}
	result["mcp"] = map[string]any{"url": mcpURL, "authentication": "bearer", "token_source": "same_api_key", "same_grant": true, "ready": false, "readiness": "not_probed"}
	if r.ExpectedAccount != "" {
		if err = guard(s, r.ExpectedAccount, false); err != nil {
			out := Failure(err)
			out.Result = result
			return out
		}
	}
	if aErr != nil {
		out := Failure(aErr)
		out.Result = result
		return out
	}
	if svcErr != nil {
		out := Failure(svcErr)
		out.Result = result
		return out
	}
	return Success(result, false)
}

func (e *Engine) read(ctx context.Context, r Request) Envelope {
	s, err := e.live(ctx)
	if err != nil {
		return Failure(err)
	}
	if r.ExpectedAccount != "" {
		if err = guard(s, r.ExpectedAccount, false); err != nil {
			return Failure(err)
		}
	}
	if s.AccountTransition {
		return Failure(problem("account_transition", "Wait for the account transition to finish.", 4))
	}
	if s.AccountId == "" {
		return Failure(problem("authentication_required", "Log in before reading native membership or credentials.", 2))
	}
	ctx, _, err = e.authContext(ctx)
	if err != nil {
		return Failure(err)
	}
	if r.Command == "auth apikey list" {
		apps, err := e.listApps(ctx)
		if err != nil {
			return Failure(err)
		}
		result := []any{}
		for _, app := range apps {
			result = append(result, keyResult(app, ""))
		}
		return Success(map[string]any{"account_id": s.AccountId, "credentials": result}, false)
	}
	spaces, err := e.spaces(ctx, s.TechSpaceId)
	if err != nil {
		return Failure(err)
	}
	if r.Command == "space list" {
		return Success(map[string]any{"account_id": s.AccountId, "spaces": spaces}, false)
	}
	for _, space := range spaces {
		if space.Id == r.SpaceId {
			return spaceEnvelope(space, false)
		}
	}
	return Failure(problem("space_not_found", "No native membership record exists for this full space ID.", 2))
}

// saveConfig patches only fields owned by account provisioning and preserves
// unknown future settings, all existing network choices, and legacy secrets.
func (e *Engine) saveConfig(fields map[string]any) error {
	data := map[string]any{}
	if err := localstate.ReadJSON(e.ConfigPath, &data); err != nil && !errors.Is(err, os.ErrNotExist) {
		return problem("config_unreadable", "Cannot preserve the existing CLI configuration.", 2)
	}
	for k, v := range fields {
		data[k] = v
	}
	if localstate.WriteJSON(e.ConfigPath, data) != nil {
		return problem("config_write_failed", "Account state exists but configuration was not saved; resume the same operation ID.", 2)
	}
	return nil
}
