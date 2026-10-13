package automation

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anyproto/anytype-cli/core"
	"github.com/anyproto/anytype-cli/core/config"
	"github.com/anyproto/anytype-cli/core/localstate"
	"github.com/anyproto/anytype-cli/core/runtimeinfo"
	heartconfig "github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type memorySecrets map[string]string

func (s memorySecrets) Get(k string) (string, string, error) {
	v, ok := s[k]
	if !ok {
		return "", "", os.ErrNotExist
	}
	return v, "mock", nil
}
func (s memorySecrets) Set(k, v string) (string, error) { s[k] = v; return "mock", nil }

type fakeService struct {
	value            ServiceStatus
	installs, starts int
}

func (s *fakeService) Status(context.Context) (ServiceStatus, error) { return s.value, nil }
func (s *fakeService) Install(_ context.Context, addr string) error {
	s.installs++
	s.value = ServiceStatus{Installed: true, BindingVerified: true, DefinitionHash: "service-hash", ListenAddress: addr}
	return nil
}
func (s *fakeService) Start(context.Context) error { s.starts++; s.value.Running = true; return nil }

type fakeRPC struct {
	service.ClientCommandsClient
	snapshot                                                                *runtimeinfo.Snapshot
	secrets                                                                 memorySecrets
	names                                                                   string
	apps                                                                    []*model.AccountAuthAppInfo
	records                                                                 []*types.Struct
	walletCalls, createCalls, selectCalls, keyCalls, joinCalls, revokeCalls int
	initialCalls, recoverCalls, sessionCalls                                int
	walletErr, createErr, keyErr, joinErr                                   error
	createCode                                                              pb.RpcAccountCreateResponseErrorCode
	badGrant                                                                bool
	afterKey                                                                func()
	afterCreate                                                             func()
}

func (f *fakeRPC) InitialSetParameters(context.Context, *pb.RpcInitialSetParametersRequest, ...grpc.CallOption) (*pb.RpcInitialSetParametersResponse, error) {
	f.initialCalls++
	return &pb.RpcInitialSetParametersResponse{}, nil
}
func (f *fakeRPC) WalletCreate(context.Context, *pb.RpcWalletCreateRequest, ...grpc.CallOption) (*pb.RpcWalletCreateResponse, error) {
	f.walletCalls++
	return &pb.RpcWalletCreateResponse{AccountKey: "account-private-fixture"}, f.walletErr
}
func (f *fakeRPC) WalletRecover(context.Context, *pb.RpcWalletRecoverRequest, ...grpc.CallOption) (*pb.RpcWalletRecoverResponse, error) {
	f.recoverCalls++
	return &pb.RpcWalletRecoverResponse{}, nil
}
func (f *fakeRPC) WalletCreateSession(context.Context, *pb.RpcWalletCreateSessionRequest, ...grpc.CallOption) (*pb.RpcWalletCreateSessionResponse, error) {
	f.sessionCalls++
	return &pb.RpcWalletCreateSessionResponse{AccountId: "account-a", Token: "session-private-fixture"}, nil
}
func (f *fakeRPC) account() *model.Account {
	f.snapshot.AccountId = "account-a"
	f.snapshot.ProfileId = "profile-a"
	f.snapshot.TechSpaceId = "tech-a"
	if f.snapshot.NetworkId == "" {
		cfg := heartconfig.New()
		nodes, err := cfg.GetNodeConfWithError()
		if err != nil {
			panic(err)
		}
		f.snapshot.NetworkId = nodes.NetworkId
	}
	f.snapshot.AccountTransition = false
	return &model.Account{Id: "account-a", Info: &model.AccountInfo{ProfileObjectId: "profile-a", TechSpaceId: "tech-a", NetworkId: f.snapshot.NetworkId}}
}

func (f *fakeRPC) settings(network, addr string) {
	if network != "" {
		f.snapshot.NetworkId, _ = config.ReadNetworkIdFromYAML(network)
	}
	f.snapshot.APIAddress, f.snapshot.APIBound = addr, addr != ""
}
func (f *fakeRPC) AccountCreate(_ context.Context, r *pb.RpcAccountCreateRequest, _ ...grpc.CallOption) (*pb.RpcAccountCreateResponse, error) {
	f.createCalls++
	if f.secrets["operation-account-create-0001"] == "" {
		panic("AccountCreate before recovery key persisted")
	}
	f.names = r.Name
	f.settings(r.NetworkCustomConfigFilePath, r.JsonApiListenAddr)
	a := f.account()
	if f.afterCreate != nil {
		f.afterCreate()
	}
	return &pb.RpcAccountCreateResponse{Account: a, Error: &pb.RpcAccountCreateResponseError{Code: f.createCode}}, f.createErr
}
func (f *fakeRPC) AccountSelect(_ context.Context, r *pb.RpcAccountSelectRequest, _ ...grpc.CallOption) (*pb.RpcAccountSelectResponse, error) {
	f.selectCalls++
	f.settings(r.NetworkCustomConfigFilePath, r.JsonApiListenAddr)
	return &pb.RpcAccountSelectResponse{Account: f.account()}, nil
}
func (f *fakeRPC) ObjectSearch(_ context.Context, r *pb.RpcObjectSearchRequest, _ ...grpc.CallOption) (*pb.RpcObjectSearchResponse, error) {
	if len(r.Filters) > 0 && r.Filters[0].RelationKey == "id" {
		return &pb.RpcObjectSearchResponse{Records: []*types.Struct{{Fields: map[string]*types.Value{"id": pbtypes.String("profile-a"), "name": pbtypes.String(f.names)}}}}, nil
	}
	return &pb.RpcObjectSearchResponse{Records: f.records}, nil
}
func (f *fakeRPC) AccountLocalLinkListApps(context.Context, *pb.RpcAccountLocalLinkListAppsRequest, ...grpc.CallOption) (*pb.RpcAccountLocalLinkListAppsResponse, error) {
	return &pb.RpcAccountLocalLinkListAppsResponse{App: f.apps}, nil
}
func (f *fakeRPC) AccountLocalLinkUpdateApp(context.Context, *pb.RpcAccountLocalLinkUpdateAppRequest, ...grpc.CallOption) (*pb.RpcAccountLocalLinkUpdateAppResponse, error) {
	return &pb.RpcAccountLocalLinkUpdateAppResponse{Error: &pb.RpcAccountLocalLinkUpdateAppResponseError{Code: pb.RpcAccountLocalLinkUpdateAppResponseError_BAD_INPUT}}, nil
}
func (f *fakeRPC) AccountLocalLinkCreateApp(_ context.Context, r *pb.RpcAccountLocalLinkCreateAppRequest, _ ...grpc.CallOption) (*pb.RpcAccountLocalLinkCreateAppResponse, error) {
	f.keyCalls++
	a := *r.App
	a.AppHash = "credential-new"
	a.AppKey = "api-private-fixture"
	if f.badGrant {
		a.Grant = nil
	}
	f.apps = append(f.apps, &a)
	if f.afterKey != nil {
		f.afterKey()
	}
	return &pb.RpcAccountLocalLinkCreateAppResponse{AppKey: a.AppKey}, f.keyErr
}
func (f *fakeRPC) AccountLocalLinkRevokeApp(_ context.Context, r *pb.RpcAccountLocalLinkRevokeAppRequest, _ ...grpc.CallOption) (*pb.RpcAccountLocalLinkRevokeAppResponse, error) {
	f.revokeCalls++
	var kept []*model.AccountAuthAppInfo
	for _, a := range f.apps {
		if a.AppHash != r.AppHash {
			kept = append(kept, a)
		}
	}
	f.apps = kept
	return &pb.RpcAccountLocalLinkRevokeAppResponse{}, nil
}
func (f *fakeRPC) SpaceInviteView(context.Context, *pb.RpcSpaceInviteViewRequest, ...grpc.CallOption) (*pb.RpcSpaceInviteViewResponse, error) {
	return &pb.RpcSpaceInviteViewResponse{SpaceId: "space-full-id", SpaceName: "shared"}, nil
}
func (f *fakeRPC) SpaceJoin(context.Context, *pb.RpcSpaceJoinRequest, ...grpc.CallOption) (*pb.RpcSpaceJoinResponse, error) {
	f.joinCalls++
	return &pb.RpcSpaceJoinResponse{}, f.joinErr
}

func fixture(t *testing.T, authenticated bool) (*Engine, *fakeRPC, *fakeService) {
	t.Helper()
	dir := t.TempDir()
	secrets := memorySecrets{}
	snap := &runtimeinfo.Snapshot{SchemaVersion: 1}
	rpc := &fakeRPC{snapshot: snap, secrets: secrets, names: "Existing Name"}
	svc := &fakeService{}
	e := &Engine{Client: rpc, ConfigPath: filepath.Join(dir, "config.json"), DataPath: filepath.Join(dir, "data"), WorkPath: filepath.Join(dir, "work"), StateDir: filepath.Join(dir, "automation"), RuntimePath: filepath.Join(dir, "runtime.json"), Secrets: secrets, Service: svc, Live: func(context.Context) (*runtimeinfo.Snapshot, error) { return snap, nil }, IdentityFromKey: func(key string) (string, error) {
		if key == "wrong" {
			return "other", nil
		}
		return "account-a", nil
	}}
	if authenticated {
		rpc.account()
		secrets["session-token"] = "session-private-fixture"
	}
	return e, rpc, svc
}
func run(e *Engine, r Request) Envelope { return e.Run(context.Background(), r) }
func assertCode(t *testing.T, out Envelope, code string) {
	t.Helper()
	if out.Error == nil || out.Error.Code != code {
		t.Fatalf("want %s, got %+v", code, out)
	}
}
func createRequest() Request {
	return Request{Command: "auth create", OperationId: "create-0001", ExpectedAccount: "none", Name: "  Агент 🌿  "}
}
func keyRequest() Request {
	return Request{Command: "auth apikey create", OperationId: "apikey-0001", ExpectedAccount: "account-a", Name: "stable app", Grant: core.GrantFlags{AllSpaces: true, ReadWrite: true}}
}
func TestNameValidation(t *testing.T) {
	for _, name := range []string{"", "  ", "a\x00b", "a\nb", strings.Repeat("x", 4097), string([]byte{255})} {
		if _, err := ValidateName(name); err == nil {
			t.Fatalf("accepted invalid name")
		}
	}
	if name, err := ValidateName("  Агент  🌿  "); err != nil || name != "Агент  🌿" {
		t.Fatal(name, err)
	}
}
func TestCreateResumeAndPrivateOutput(t *testing.T) {
	e, rpc, _ := fixture(t, false)
	r := createRequest()
	out := run(e, r)
	if out.ExitCode() != 0 || rpc.createCalls != 1 {
		t.Fatalf("create: %+v", out)
	}
	b, _ := json.Marshal(out)
	if strings.Contains(string(b), "private-fixture") {
		t.Fatal("secret leaked")
	}
	r.IncludeSecret = true
	out = run(e, r)
	if out.ExitCode() != 0 || out.Changed || rpc.createCalls != 1 || rpc.walletCalls != 1 {
		t.Fatalf("resume: %+v", out)
	}
	b, _ = json.Marshal(out)
	if !strings.Contains(string(b), "account-private-fixture") {
		t.Fatal("explicit recovery secret missing")
	}
	journal, _ := os.ReadFile(e.opPath(r.OperationId))
	if strings.Contains(string(journal), "private-fixture") {
		t.Fatal("journal contains secret")
	}
}
func TestCreateLostResponseReconcilesLiveIdentity(t *testing.T) {
	e, rpc, _ := fixture(t, false)
	rpc.createErr = status.Error(codes.Unavailable, "secret from upstream")
	r := createRequest()
	out := run(e, r)
	assertCode(t, out, "rpc_unavailable")
	rpc.createErr = nil
	out = run(e, r)
	if out.ExitCode() != 0 || rpc.createCalls != 1 {
		t.Fatalf("did not reconcile: %+v", out)
	}
}
func TestCreateUncertainNeverDuplicates(t *testing.T) {
	e, rpc, _ := fixture(t, false)
	rpc.createErr = status.Error(codes.DeadlineExceeded, "private-fixture")
	rpc.afterCreate = func() { rpc.snapshot.AccountId = "" }
	r := createRequest()
	run(e, r)
	rpc.createErr = nil
	out := run(e, r)
	assertCode(t, out, "operation_uncertain")
	if rpc.createCalls != 1 || rpc.walletCalls != 1 {
		t.Fatal("duplicate account mutation")
	}
}
func TestAccountObservedRestartSelectsOnly(t *testing.T) {
	e, rpc, _ := fixture(t, false)
	r := createRequest()
	out := run(e, r)
	if out.ExitCode() != 0 {
		t.Fatal(out)
	}
	op, _, err := e.operation(Request{Command: r.Command, OperationId: r.OperationId, ExpectedAccount: r.ExpectedAccount, Name: "Агент 🌿"})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.saveOp(op, "account_observed"); err != nil {
		t.Fatal(err)
	}
	rpc.snapshot.AccountId = ""
	out = run(e, r)
	if out.ExitCode() != 0 || rpc.createCalls != 1 || rpc.selectCalls != 1 {
		t.Fatalf("duplicate or no recovery: %+v", out)
	}
}
func TestCreationPartialErrorIsNotSuccess(t *testing.T) {
	e, rpc, _ := fixture(t, false)
	rpc.createCode = pb.RpcAccountCreateResponseErrorCode(101)
	out := run(e, createRequest())
	if out.ExitCode() == 0 {
		t.Fatal("partial RPC success claimed")
	}
	if rpc.secrets["operation-account-create-0001"] == "" {
		t.Fatal("lost recovery credential")
	}
}
func TestAccountMismatchAndOperationConflict(t *testing.T) {
	e, rpc, _ := fixture(t, true)
	out := run(e, createRequest())
	assertCode(t, out, "account_mismatch")
	if rpc.createCalls != 0 || rpc.walletCalls != 0 {
		t.Fatal("mutated existing account")
	}
	r := keyRequest()
	if out = run(e, r); out.ExitCode() != 0 {
		t.Fatal(out)
	}
	r.Name = "different"
	assertCode(t, run(e, r), "operation_conflict")
}
func TestLoginEmptyDerivesIdentityPreservesName(t *testing.T) {
	e, rpc, _ := fixture(t, false)
	r := Request{Command: "auth login", OperationId: "login-0001", ExpectedAccount: "none", Secret: "account-private-fixture"}
	out := run(e, r)
	if out.ExitCode() != 0 || rpc.names != "Existing Name" || rpc.createCalls != 0 {
		t.Fatalf("login: %+v", out)
	}
	r.Secret = ""
	out = run(e, r)
	if out.ExitCode() != 0 || rpc.selectCalls != 1 {
		t.Fatalf("login retry: %+v", out)
	}
}
func TestLoginWrongExpectedKeyStopsBeforeRPC(t *testing.T) {
	e, rpc, _ := fixture(t, false)
	r := Request{Command: "auth login", OperationId: "login-0001", ExpectedAccount: "account-a", Secret: "wrong"}
	assertCode(t, run(e, r), "account_mismatch")
	if rpc.selectCalls != 0 {
		t.Fatal("selected wrong account")
	}
}
func TestKeyRotationIdentityRecoveryAndSecrecy(t *testing.T) {
	e, rpc, _ := fixture(t, true)
	rpc.apps = []*model.AccountAuthAppInfo{{AppHash: "old-key", AppName: "stable app", AppKey: "old-private-fixture"}}
	r := keyRequest()
	out := run(e, r)
	if out.ExitCode() != 0 {
		t.Fatal(out)
	}
	b, _ := json.Marshal(out)
	if strings.Contains(string(b), "private-fixture") {
		t.Fatal("secret leaked")
	}
	r.IncludeSecret = true
	out = run(e, r)
	if out.ExitCode() != 0 || rpc.keyCalls != 1 || rpc.revokeCalls != 0 || len(rpc.apps) != 2 {
		t.Fatalf("rotation: %+v", out)
	}
	out = run(e, Request{Command: "auth apikey list"})
	b, _ = json.Marshal(out)
	if strings.Contains(string(b), "private-fixture") {
		t.Fatal("list leaked keys")
	}
}
func TestUnreceiptedKeyNotMatchedByApplicationName(t *testing.T) {
	e, rpc, _ := fixture(t, true)
	rpc.keyErr = status.Error(codes.Unavailable, "private-fixture")
	r := keyRequest()
	run(e, r)
	rpc.keyErr = nil
	assertCode(t, run(e, r), "operation_uncertain")
	if rpc.keyCalls != 1 {
		t.Fatal("duplicate key")
	}
}
func TestKeyGrantMismatchNeverReturnsSecret(t *testing.T) {
	e, rpc, _ := fixture(t, true)
	rpc.badGrant = true
	r := keyRequest()
	r.IncludeSecret = true
	out := run(e, r)
	assertCode(t, out, "grant_mismatch")
	b, _ := json.Marshal(out)
	if strings.Contains(string(b), "api-private-fixture") {
		t.Fatal("unverified key returned")
	}
}
func TestMembershipStatesAndJoinRetry(t *testing.T) {
	e, rpc, _ := fixture(t, true)
	r := Request{Command: "space join", OperationId: "join-00001", ExpectedAccount: "account-a", Secret: "https://example.test/cid#invite-secret"}
	out := run(e, r)
	if out.State != "request_sent" || out.ExitCode() != 3 {
		t.Fatal(out)
	}
	rpc.records = []*types.Struct{{Fields: map[string]*types.Value{"targetSpaceId": pbtypes.String("space-full-id"), "myParticipantStatus": pbtypes.Int64(0), "spaceAccountStatus": pbtypes.Int64(9)}}}
	r.Secret = ""
	out = run(e, r)
	if out.State != "pending_approval" || rpc.joinCalls != 1 {
		t.Fatal(out)
	}
	rpc.records[0].Fields["myParticipantStatus"] = pbtypes.Int64(3)
	out = run(e, Request{Command: "space status", SpaceId: "space-full-id"})
	assertCode(t, out, "membership_rejected")
	journal, _ := os.ReadFile(e.opPath(r.OperationId))
	if strings.Contains(string(journal), "invite-secret") || strings.Contains(string(journal), "example.test") {
		t.Fatal("invitation journal leak")
	}
}
func TestUnknownJoinOutcomeNoResubmit(t *testing.T) {
	e, rpc, _ := fixture(t, true)
	rpc.joinErr = status.Error(codes.DeadlineExceeded, "invite-secret")
	r := Request{Command: "space join", OperationId: "join-00001", ExpectedAccount: "account-a", Secret: "https://example.test/cid#invite-secret"}
	run(e, r)
	r.Secret = ""
	assertCode(t, run(e, r), "operation_uncertain")
	if rpc.joinCalls != 1 {
		t.Fatal("resubmitted join")
	}
}
func TestMutationLockAndUnsupportedInstance(t *testing.T) {
	e, rpc, _ := fixture(t, true)
	unlock, err := localstate.Lock(e.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	assertCode(t, run(e, keyRequest()), "instance_busy")
	r := keyRequest()
	r.Instance = "hermes-profile"
	assertCode(t, run(e, r), "unsupported_instance")
	if rpc.keyCalls != 0 {
		t.Fatal("mutation bypassed guard")
	}
}
func TestServiceReuseAndBindingConflict(t *testing.T) {
	e, _, svc := fixture(t, false)
	svc.value = ServiceStatus{Installed: true, BindingVerified: true, ListenAddress: config.DefaultAPIAddress, DefinitionHash: "same"}
	r := Request{Command: "service install", OperationId: "service-001"}
	out := run(e, r)
	if out.ExitCode() != 0 || out.Changed || svc.installs != 0 {
		t.Fatal(out)
	}
	r.OperationId = "service-002"
	r.ListenAddress = "127.0.0.1:5555"
	assertCode(t, run(e, r), "service_binding_conflict")
}
func TestInspectionReadOnlyAndNoReadyClaims(t *testing.T) {
	e, _, _ := fixture(t, false)
	out := run(e, Request{Command: "inspect"})
	if out.ExitCode() != 0 {
		t.Fatal(out)
	}
	if _, err := os.Stat(e.StateDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("inspection wrote state")
	}
	b, _ := json.Marshal(out)
	if strings.Contains(string(b), `"authenticated":true`) || strings.Contains(string(b), `"ready":true`) {
		t.Fatal("false readiness")
	}
}

func TestPreviouslyCompletedKeyChangedExternallyNotRevoked(t *testing.T) {
	e, rpc, _ := fixture(t, true)
	r := keyRequest()
	if out := run(e, r); out.ExitCode() != 0 {
		t.Fatal(out)
	}
	rpc.apps[0].Grant = nil
	assertCode(t, run(e, r), "grant_mismatch")
	if rpc.revokeCalls != 0 {
		t.Fatal("externally changed completed key revoked")
	}
}
func TestInvalidGrantCleanupOnlyNewCredential(t *testing.T) {
	e, rpc, _ := fixture(t, true)
	rpc.apps = []*model.AccountAuthAppInfo{{AppHash: "old", AppName: "stable app"}}
	rpc.badGrant = true
	out := run(e, keyRequest())
	assertCode(t, out, "grant_mismatch")
	if rpc.revokeCalls != 1 || len(rpc.apps) != 1 || rpc.apps[0].AppHash != "old" {
		t.Fatal("wrong cleanup identity")
	}
	out = run(e, keyRequest())
	assertCode(t, out, "grant_mismatch")
	if rpc.keyCalls != 1 {
		t.Fatal("new create on cleanup retry")
	}
}
func TestInstallBlocksReachableWrongInstance(t *testing.T) {
	e, _, svc := fixture(t, false)
	for _, code := range []string{"service_upgrade_required", "instance_mismatch", "rpc_timeout"} {
		e.Live = func(context.Context) (*runtimeinfo.Snapshot, error) { return nil, problem(code, "fixture", 2) }
		out := run(e, Request{Command: "service install", OperationId: "service-001"})
		assertCode(t, out, code)
	}
	if svc.installs != 0 {
		t.Fatal("installed over incompatible listener")
	}
}
func TestCallerDeadlineIsPreserved(t *testing.T) {
	e, _, _ := fixture(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	want, _ := ctx.Deadline()
	e.Live = func(ctx context.Context) (*runtimeinfo.Snapshot, error) {
		got, _ := ctx.Deadline()
		if !got.Equal(want) {
			t.Fatal("caller deadline shortened")
		}
		return &runtimeinfo.Snapshot{}, nil
	}
	if out := e.Run(ctx, Request{Command: "inspect"}); out.ExitCode() != 0 {
		t.Fatal(out)
	}
}

func membershipRecord(id, name string, local, account model.SpaceStatus, participant *model.ParticipantStatus) *types.Struct {
	record := &types.Struct{Fields: map[string]*types.Value{
		"targetSpaceId": pbtypes.String(id), "name": pbtypes.String(name),
		"spaceLocalStatus": pbtypes.Int64(int64(local)), "spaceAccountStatus": pbtypes.Int64(int64(account)),
	}}
	if participant != nil {
		record.Fields["myParticipantStatus"] = pbtypes.Int64(int64(*participant))
	}
	return record
}

func TestRejectedJoinOverridesStaleJoiningWithoutResubmission(t *testing.T) {
	e, rpc, _ := fixture(t, true)
	request := Request{Command: "space join", OperationId: "join-rejected", ExpectedAccount: "account-a", Secret: "https://example.test/cid#secret"}
	if out := run(e, request); out.ExitCode() != 3 {
		t.Fatal(out)
	}
	joining := model.ParticipantStatus_Joining
	rpc.records = []*types.Struct{membershipRecord("space-full-id", "Shared", model.SpaceStatus_Unknown, model.SpaceStatus_SpaceDeleted, &joining)}
	request.Secret = ""
	assertCode(t, run(e, request), "membership_rejected")
	assertCode(t, run(e, Request{Command: "space status", SpaceId: "space-full-id"}), "membership_rejected")
	if rpc.joinCalls != 1 {
		t.Fatal("rejected join resubmitted")
	}
}

func TestPersonalSpaceMembershipAndFixedGrant(t *testing.T) {
	for _, participant := range []*model.ParticipantStatus{nil, new(model.ParticipantStatus_Active)} {
		e, rpc, _ := fixture(t, true)
		rpc.records = []*types.Struct{membershipRecord("personal-space", "Personal", model.SpaceStatus_Ok, model.SpaceStatus_Unknown, participant)}
		out := run(e, Request{Command: "space status", SpaceId: "personal-space"})
		if out.ExitCode() != 0 || out.Result.(Space).State != "joined" {
			t.Fatal(out)
		}
		request := keyRequest()
		request.Grant = core.GrantFlags{Spaces: []string{"Personal"}, ReadOnly: true}
		out = run(e, request)
		if out.ExitCode() != 0 || len(rpc.apps) != 1 || !grantsEqual(rpc.apps[0].Grant, &model.AccountAuthAppGrant{SpaceIds: []string{"personal-space"}, Perm: model.AccountAuthAppGrant_Read}) {
			t.Fatal(out)
		}
	}
}

type lostReadbackRPC struct {
	*fakeRPC
	lists int
}

func (f *lostReadbackRPC) AccountLocalLinkListApps(ctx context.Context, request *pb.RpcAccountLocalLinkListAppsRequest, opts ...grpc.CallOption) (*pb.RpcAccountLocalLinkListAppsResponse, error) {
	f.lists++
	if f.lists == 2 {
		return nil, status.Error(codes.Unavailable, "lost readback")
	}
	return f.fakeRPC.AccountLocalLinkListApps(ctx, request, opts...)
}
func TestKeyRecoveryUsesDurableGrantAfterSpaceRename(t *testing.T) {
	for _, reuseName := range []bool{false, true} {
		e, rpc, _ := fixture(t, true)
		e.Client = &lostReadbackRPC{fakeRPC: rpc}
		rpc.records = []*types.Struct{membershipRecord("space-original", "Team", model.SpaceStatus_Ok, model.SpaceStatus_SpaceActive, nil)}
		request := keyRequest()
		request.Grant = core.GrantFlags{Spaces: []string{"Team"}, ReadWrite: true}
		rpc.afterKey = func() {
			var op operation
			if err := localstate.ReadJSON(e.opPath(request.OperationId), &op); err != nil || op.ResolvedGrant == nil || op.ResolvedGrant.SpaceIds[0] != "space-original" {
				t.Fatal("grant not durable before creation", op, err)
			}
		}
		assertCode(t, run(e, request), "rpc_unavailable")
		rpc.records[0].Fields["name"] = pbtypes.String("Archived Team")
		if reuseName {
			rpc.records = append(rpc.records, membershipRecord("space-new", "Team", model.SpaceStatus_Ok, model.SpaceStatus_SpaceActive, nil))
		}
		request.IncludeSecret = true
		out := run(e, request)
		if out.ExitCode() != 0 || out.Changed || rpc.keyCalls != 1 || rpc.revokeCalls != 0 || rpc.apps[0].Grant.SpaceIds[0] != "space-original" {
			t.Fatal(out, rpc.keyCalls, rpc.revokeCalls)
		}
		// Completed replay also uses the immutable grant.
		if out = run(e, request); out.ExitCode() != 0 || rpc.revokeCalls != 0 {
			t.Fatal(out)
		}
	}
}
func TestLegacyKeyReceiptWithoutGrantDoesNotMutate(t *testing.T) {
	e, rpc, _ := fixture(t, true)
	request := keyRequest()
	if out := run(e, request); out.ExitCode() != 0 {
		t.Fatal(out)
	}
	var op operation
	if err := localstate.ReadJSON(e.opPath(request.OperationId), &op); err != nil {
		t.Fatal(err)
	}
	op.ResolvedGrant = nil
	if err := e.saveOp(&op, "verified_identity"); err != nil {
		t.Fatal(err)
	}
	assertCode(t, run(e, request), "operation_uncertain")
	if rpc.keyCalls != 1 || rpc.revokeCalls != 0 {
		t.Fatal("legacy receipt mutated credential")
	}
}
func TestRelativeNetworkPathReplaysAfterPersistingAbsolutePath(t *testing.T) {
	e, rpc, _ := fixture(t, false)
	network := filepath.Join(t.TempDir(), "network.yml")
	if err := os.WriteFile(network, []byte("networkId: network-a\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cwd, network)
	if err != nil {
		t.Fatal(err)
	}
	request := createRequest()
	request.NetworkConfig = relative
	if out := run(e, request); out.ExitCode() != 0 {
		t.Fatal(out)
	}
	request.IncludeSecret = true
	if out := run(e, request); out.ExitCode() != 0 || rpc.createCalls != 1 {
		t.Fatal(out)
	}
	saved, err := e.config()
	if err != nil || saved.NetworkConfigPath != network {
		t.Fatal(saved, err)
	}
	if err := os.WriteFile(network, []byte("networkId: changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	assertCode(t, run(e, request), "settings_conflict")
}
