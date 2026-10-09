package automation

import (
	"context"
	"slices"

	"github.com/anyproto/anytype-cli/core"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (e *Engine) listApps(ctx context.Context) ([]*model.AccountAuthAppInfo, error) {
	r, err := e.Client.AccountLocalLinkListApps(ctx, &pb.RpcAccountLocalLinkListAppsRequest{})
	if err != nil {
		return nil, rpcError(err)
	}
	if err = protoError("List credentials", r.GetError().GetCode().String()); err != nil {
		return nil, err
	}
	return r.App, nil
}
func keyResult(app *model.AccountAuthAppInfo, key string) map[string]any {
	result := map[string]any{"credential_id": app.AppHash, "application_name": app.AppName, "scope": app.Scope.String(), "grant": grantJSON(app.Grant), "created_at": app.CreatedAt, "expires_at": app.ExpireAt}
	if key != "" {
		result["api_key"] = key
	}
	return result
}
func grantsEqual(a, b *model.AccountAuthAppGrant) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.AllSpaces != b.AllSpaces || a.Perm != b.Perm {
		return false
	}
	as, bs := slices.Clone(a.SpaceIds), slices.Clone(b.SpaceIds)
	slices.Sort(as)
	slices.Sort(bs)
	return slices.Equal(as, bs)
}
func (e *Engine) requestedGrant(ctx context.Context, r Request, tech string) (*model.AccountAuthAppGrant, error) {
	if err := core.ValidateGrantFlags(r.Grant); err != nil {
		return nil, problem("invalid_grant", err.Error(), 2)
	}
	var resolved []core.ResolvedSpace
	if !r.Grant.AllSpaces {
		native, err := e.spaces(ctx, tech)
		if err != nil {
			return nil, err
		}
		spaces := []core.SpaceListItem{}
		for _, s := range native {
			if s.State == "joined" || s.State == "syncing" {
				spaces = append(spaces, core.SpaceListItem{SpaceId: s.Id, Name: s.Name})
			}
		}
		resolved, err = core.ResolveSpaces(r.Grant.Spaces, spaces, tech)
		if err != nil {
			return nil, problem("invalid_space_grant", "A selected space is missing or its name is ambiguous. Use full IDs from space list.", 2)
		}
	}
	grant, err := core.BuildGrant(r.Grant, resolved)
	if err != nil {
		return nil, problem("invalid_grant", "An explicit space scope and permission are required.", 2)
	}
	slices.Sort(grant.SpaceIds)
	return grant, nil
}
func (e *Engine) keyMutation(ctx context.Context, r Request, op *operation, retry bool) Envelope {
	fail := func(err error) Envelope {
		out := Failure(err)
		out.Result = map[string]any{"operation_id": op.Id, "credential_id": op.ResourceId, "stage": op.Stage}
		return out
	}
	s, err := e.live(ctx)
	if err != nil {
		return fail(err)
	}
	if err = guard(s, r.ExpectedAccount, false); err != nil {
		return fail(err)
	}
	if s.AccountId == "" {
		return fail(problem("authentication_required", "Log in before managing credentials.", 2))
	}
	if op.AccountId != "" && op.AccountId != s.AccountId {
		return fail(problem("account_mismatch", "The operation belongs to another account.", 2))
	}
	op.AccountId = s.AccountId
	ctx, _, err = e.authContext(ctx)
	if err != nil {
		return fail(err)
	}
	apps, err := e.listApps(ctx)
	if err != nil {
		return fail(err)
	}
	if r.Command == "auth apikey revoke" {
		if r.CredentialId == "" {
			return fail(problem("credential_id_required", "Pass the full credential ID to revoke.", 2))
		}
		var app *model.AccountAuthAppInfo
		for _, a := range apps {
			if a.AppHash == r.CredentialId {
				app = a
				break
			}
		}
		if app == nil {
			if !retry {
				if err = e.saveOp(op, "complete"); err != nil {
					return fail(err)
				}
			}
			return Success(map[string]any{"operation_id": op.Id, "credential_id": r.CredentialId, "revoked": true}, false)
		}
		op.ResourceId = app.AppHash
		if err = e.saveOp(op, "revoke_sent"); err != nil {
			return fail(err)
		}
		response, rpcErr := e.Client.AccountLocalLinkRevokeApp(ctx, &pb.RpcAccountLocalLinkRevokeAppRequest{AppHash: app.AppHash})
		if rpcErr != nil {
			return fail(rpcError(rpcErr))
		}
		if err = protoError("Revoke credential", response.GetError().GetCode().String()); err != nil {
			return fail(err)
		}
		verified, err := e.listApps(ctx)
		if err != nil {
			return fail(err)
		}
		for _, a := range verified {
			if a.AppHash == app.AppHash {
				return fail(uncertain())
			}
		}
		if err = e.saveOp(op, "complete"); err != nil {
			return fail(err)
		}
		return Success(map[string]any{"operation_id": op.Id, "credential_id": app.AppHash, "revoked": true}, true)
	}
	if err = core.ValidateAPIKeyName(r.Name); err != nil {
		return fail(problem("invalid_application_name", "An explicit application name of 1–128 UTF-8 bytes is required.", 2))
	}
	grant := op.ResolvedGrant
	if grant == nil {
		if retry && op.Stage != "prepared" {
			// Old receipts cannot prove what a mutable space name resolved to.
			// Never recreate or revoke a key based on its current resolution.
			return fail(uncertain())
		}
		grant, err = e.requestedGrant(ctx, r, s.TechSpaceId)
		if err != nil {
			return fail(err)
		}
		op.ResolvedGrant = grant
		if err = e.saveOp(op, "prepared"); err != nil {
			return fail(err)
		}
	}
	key, _, _ := e.Secrets.Get("operation-apikey-" + op.Id)
	var app *model.AccountAuthAppInfo
	changed := false
	if retry && op.Stage != "prepared" {
		for _, a := range apps {
			if op.ResourceId != "" && a.AppHash == op.ResourceId || key != "" && a.AppKey == key {
				app = a
				break
			}
		}
		if app == nil {
			if op.Stage == "cleanup_sent" || op.Stage == "unverified_revoked" {
				if err = e.saveOp(op, "unverified_revoked"); err != nil {
					return fail(err)
				}
				return fail(problem("grant_mismatch", "The unverified newly created credential was revoked. Older credentials remain valid.", 1))
			}
			return fail(uncertain())
		}
	} else {
		// Empty AppHash is a no-op BAD_INPUT probe; old services return Unimplemented.
		probe, rpcErr := e.Client.AccountLocalLinkUpdateApp(ctx, &pb.RpcAccountLocalLinkUpdateAppRequest{})
		if rpcErr != nil {
			return fail(rpcError(rpcErr))
		}
		if probe.GetError().GetCode() != pb.RpcAccountLocalLinkUpdateAppResponseError_BAD_INPUT {
			return fail(problem("grant_support_unverified", "The running service did not prove scoped credential support.", 1))
		}
		if err = e.saveOp(op, "create_sent"); err != nil {
			return fail(err)
		}
		response, rpcErr := e.Client.AccountLocalLinkCreateApp(ctx, &pb.RpcAccountLocalLinkCreateAppRequest{App: &model.AccountAuthAppInfo{AppName: r.Name, Scope: model.AccountAuth_JsonAPI, Grant: grant}})
		if rpcErr != nil {
			return fail(rpcError(rpcErr))
		}
		if err = protoError("Create credential", response.GetError().GetCode().String()); err != nil {
			return fail(err)
		}
		key = response.AppKey
		if key == "" {
			return fail(uncertain())
		}
		if _, err = e.Secrets.Set("operation-apikey-"+op.Id, key); err != nil {
			return fail(problem("credential_storage_failed", "Credential may exist but its recovery receipt could not be saved. Do not create a duplicate.", 2))
		}
		apps, err = e.listApps(ctx)
		if err != nil {
			return fail(err)
		}
		for _, a := range apps {
			if a.AppKey == key {
				app = a
				break
			}
		}
		if app == nil {
			return fail(uncertain())
		}
		changed = true
	}
	if app.AppHash == "" {
		return fail(uncertain())
	}
	completedBefore := op.Stage == "complete"
	op.ResourceId = app.AppHash
	if !completedBefore {
		if err = e.saveOp(op, "verified_identity"); err != nil {
			return fail(err)
		}
	}
	if app.AppName != r.Name || app.Scope != model.AccountAuth_JsonAPI || !grantsEqual(app.Grant, grant) {
		if completedBefore {
			return fail(problem("grant_mismatch", "This completed operation's credential was changed externally. No token was returned and it was not revoked.", 1))
		}
		if err = e.saveOp(op, "cleanup_sent"); err != nil {
			return fail(err)
		}
		response, rpcErr := e.Client.AccountLocalLinkRevokeApp(ctx, &pb.RpcAccountLocalLinkRevokeAppRequest{AppHash: app.AppHash})
		if rpcErr != nil || response.GetError().GetCode() != pb.RpcAccountLocalLinkRevokeAppResponseError_NULL {
			return fail(problem("unverified_key_cleanup_uncertain", "The newly created credential did not match its requested grant and cleanup could not be confirmed. Resume this operation; do not distribute its secret.", 2))
		}
		remaining, listErr := e.listApps(ctx)
		if listErr != nil {
			return fail(problem("unverified_key_cleanup_uncertain", "Unverified credential revocation was sent but could not be verified. Resume this operation.", 2))
		}
		for _, a := range remaining {
			if a.AppHash == app.AppHash {
				return fail(problem("unverified_key_cleanup_uncertain", "The unverified credential remains present; resume cleanup.", 2))
			}
		}
		if err = e.saveOp(op, "unverified_revoked"); err != nil {
			return fail(err)
		}
		return fail(problem("grant_mismatch", "The newly created credential did not match its requested grant and was revoked. Older credentials remain valid.", 1))
	}
	if err = e.saveOp(op, "complete"); err != nil {
		return fail(err)
	}
	token := ""
	if r.IncludeSecret {
		token = app.AppKey
		if token == "" {
			return fail(problem("secret_unavailable", "The credential exists but its secret is unavailable for recovery.", 2))
		}
	}
	result := keyResult(app, token)
	result["operation_id"] = op.Id
	result["account_id"] = s.AccountId
	result["grant_verified"] = true
	return Success(result, changed)
}
