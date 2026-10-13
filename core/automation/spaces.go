package automation

import (
	"context"
	"net/url"
	"strings"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type Space struct {
	Id                string `json:"id"`
	Name              string `json:"name"`
	State             string `json:"state"`
	LocalStatus       string `json:"local_status"`
	AccountStatus     string `json:"account_status"`
	ParticipantStatus string `json:"participant_status,omitempty"`
	HTTPReady         bool   `json:"http_ready"`
}

func (e *Engine) spaces(ctx context.Context, tech string) ([]Space, error) {
	if tech == "" {
		return nil, problem("account_info_unavailable", "The account's native membership index is unavailable.", 4)
	}
	// Do not filter out joining/rejected spaces as the human list command does.
	r, err := e.Client.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{SpaceId: tech, Filters: []*model.BlockContentDataviewFilter{{RelationKey: bundle.RelationKeyResolvedLayout.String(), Condition: model.BlockContentDataviewFilter_Equal, Value: pbtypes.Int64(int64(model.ObjectType_spaceView))}}, Keys: []string{"targetSpaceId", "name", "spaceLocalStatus", "spaceAccountStatus", "myParticipantStatus"}})
	if err != nil {
		return nil, rpcError(err)
	}
	if err = protoError("List spaces", r.GetError().GetCode().String()); err != nil {
		return nil, err
	}
	spaces := []Space{}
	for _, v := range r.Records {
		id := pbtypes.GetString(v, "targetSpaceId")
		if id == "" {
			continue
		}
		local := model.SpaceStatus(pbtypes.GetInt64(v, "spaceLocalStatus"))
		account := model.SpaceStatus(pbtypes.GetInt64(v, "spaceAccountStatus"))
		s := Space{Id: id, Name: pbtypes.GetString(v, "name"), State: "unknown", LocalStatus: local.String(), AccountStatus: account.String()}
		participant, hasParticipant := v.Fields["myParticipantStatus"]
		if hasParticipant {
			p := model.ParticipantStatus(int32(participant.GetNumberValue()))
			s.ParticipantStatus = p.String()
			switch p {
			case model.ParticipantStatus_Declined:
				s.State = "rejected"
			case model.ParticipantStatus_Joining:
				s.State = "pending_approval"
			case model.ParticipantStatus_Removed, model.ParticipantStatus_Removing, model.ParticipantStatus_Canceled:
				s.State = "removed"
			}
		}
		// Heart's rejected join retains participant Joining while its account
		// status becomes SpaceDeleted. Terminal account state wins over it.
		switch account {
		case model.SpaceStatus_SpaceDeleted:
			if s.ParticipantStatus == model.ParticipantStatus_Joining.String() || s.State == "rejected" {
				s.State = "rejected"
			} else {
				s.State = "removed"
			}
		case model.SpaceStatus_SpaceRemoving:
			s.State = "removed"
		}
		if s.State == "unknown" {
			switch account {
			case model.SpaceStatus_SpaceJoining:
				s.State = "pending_approval"
			case model.SpaceStatus_SpaceActive:
				if local == model.SpaceStatus_Ok {
					s.State = "joined"
				} else {
					s.State = "syncing"
				}
			case model.SpaceStatus_Unknown:
				// Owned/personal spaces need not have an account-tree status.
				// A loaded local space still proves native membership.
				if local == model.SpaceStatus_Ok {
					s.State = "joined"
				}
			}
		}
		spaces = append(spaces, s)
	}
	return spaces, nil
}
func spaceEnvelope(s Space, changed bool) Envelope {
	out := Success(s, changed)
	switch s.State {
	case "pending_approval", "request_sent", "syncing":
		out.State = s.State
		out.NextAction = "Poll space status with the full space ID; do not submit the invitation again."
	case "rejected", "removed", "unknown":
		out = Failure(problem("membership_"+s.State, "Native membership is "+s.State+". Inspect the membership with its operator.", 2))
		out.Result = s
		out.Changed = changed
	}
	return out
}
func invitation(value string) (string, string, error) {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", "", problem("invalid_invitation", "Private input is not a supported invitation URL.", 2)
	}
	cid, key := "", ""
	switch u.Scheme {
	case "http", "https":
		if u.Host != "" {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			cid = parts[len(parts)-1]
			key = u.Fragment
		}
	case "anytype":
		if u.Host == "invite" || strings.HasPrefix(u.Path, "/invite") {
			cid = u.Query().Get("cid")
			key = u.Query().Get("key")
		}
	}
	if cid == "" || key == "" {
		return "", "", problem("invalid_invitation", "Private input must contain an invitation CID and key.", 2)
	}
	return cid, key, nil
}
func (e *Engine) join(ctx context.Context, r Request, op *operation, retry bool) Envelope {
	fail := func(err error) Envelope {
		out := Failure(err)
		out.Result = map[string]any{"operation_id": op.Id, "space_id": op.ResourceId, "stage": op.Stage}
		return out
	}
	s, err := e.live(ctx)
	if err != nil {
		return fail(err)
	}
	if err = guard(s, r.ExpectedAccount, false); err != nil {
		return fail(err)
	}
	if op.AccountId != "" && op.AccountId != s.AccountId {
		return fail(problem("account_mismatch", "This join operation belongs to another account.", 2))
	}
	op.AccountId = s.AccountId
	ctx, _, err = e.authContext(ctx)
	if err != nil {
		return fail(err)
	}
	cid, key := "", ""
	if r.Secret != "" {
		cid, key, err = invitation(r.Secret)
		if err != nil {
			return fail(err)
		}
		invite, rpcErr := e.Client.SpaceInviteView(ctx, &pb.RpcSpaceInviteViewRequest{InviteCid: cid, InviteFileKey: key})
		if rpcErr != nil {
			return fail(rpcError(rpcErr))
		}
		if err = protoError("View invitation", invite.GetError().GetCode().String()); err != nil {
			return fail(err)
		}
		if invite.SpaceId == "" {
			return fail(problem("invalid_invitation", "The invitation does not identify a space.", 2))
		}
		if op.ResourceId != "" && op.ResourceId != invite.SpaceId || r.SpaceId != "" && r.SpaceId != invite.SpaceId {
			return fail(problem("operation_conflict", "The invitation targets a different space from this operation.", 2))
		}
		op.ResourceId = invite.SpaceId
	}
	if op.ResourceId == "" {
		return fail(problem("private_input_required", "Provide the invitation through --invite-stdin on the first join attempt.", 2))
	}
	spaces, err := e.spaces(ctx, s.TechSpaceId)
	if err != nil {
		return fail(err)
	}
	for _, space := range spaces {
		if space.Id == op.ResourceId && space.State != "unknown" {
			if err = e.saveOp(op, "observed"); err != nil {
				return fail(err)
			}
			out := spaceEnvelope(space, false)
			out.Result = map[string]any{"operation_id": op.Id, "space": space}
			return out
		}
	}
	if retry && op.Stage != "prepared" {
		return fail(uncertain())
	}
	if key == "" {
		return fail(problem("private_input_required", "Re-enter the invitation privately; no join request was recorded.", 2))
	}
	if s.NetworkId == "" {
		return fail(problem("network_unavailable", "The live account network is unknown; refusing to guess the invitation network.", 2))
	}
	if err = e.saveOp(op, "join_sent"); err != nil {
		return fail(err)
	}
	response, rpcErr := e.Client.SpaceJoin(ctx, &pb.RpcSpaceJoinRequest{NetworkId: s.NetworkId, SpaceId: op.ResourceId, InviteCid: cid, InviteFileKey: key})
	if rpcErr != nil {
		return fail(rpcError(rpcErr))
	}
	if err = protoError("Join space", response.GetError().GetCode().String()); err != nil {
		return fail(err)
	}
	if err = e.saveOp(op, "request_sent"); err != nil {
		return fail(err)
	}
	spaces, err = e.spaces(ctx, s.TechSpaceId)
	if err == nil {
		for _, space := range spaces {
			if space.Id == op.ResourceId {
				out := spaceEnvelope(space, true)
				out.Result = map[string]any{"operation_id": op.Id, "space": space}
				return out
			}
		}
	}
	out := spaceEnvelope(Space{Id: op.ResourceId, State: "request_sent"}, true)
	out.Result = map[string]any{"operation_id": op.Id, "space": out.Result}
	return out
}
