package automation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"

	"github.com/anyproto/anytype-cli/core/credentials"
	"github.com/anyproto/anytype-cli/core/localstate"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var errMissing = os.ErrNotExist
var operationPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{7,127}$`)

type operation struct {
	SchemaVersion int                        `json:"schema_version"`
	Id            string                     `json:"id"`
	Binding       string                     `json:"binding"`
	Intent        string                     `json:"intent"`
	Command       string                     `json:"command"`
	AccountId     string                     `json:"account_id,omitempty"`
	Settings      string                     `json:"settings,omitempty"`
	Stage         string                     `json:"stage"`
	ResourceId    string                     `json:"resource_id,omitempty"`
	ResolvedGrant *model.AccountAuthAppGrant `json:"resolved_grant,omitempty"`
}

func (e *Engine) opPath(id string) string { return filepath.Join(e.StateDir, "operations", id+".json") }
func (e *Engine) saveOp(op *operation, stage string) error {
	op.Stage = stage
	if localstate.WriteJSON(e.opPath(op.Id), op) != nil {
		return problem("journal_write_failed", "Cannot persist operation progress; resume the same operation ID.", 2)
	}
	return nil
}
func (e *Engine) operation(r Request) (*operation, bool, error) {
	if !operationPattern.MatchString(r.OperationId) {
		return nil, false, problem("operation_id_required", "Provide an operation ID of 8–128 letters, digits, hyphens or underscores.", 2)
	}
	// Secret values and output preferences deliberately do not enter the journal.
	intent, _ := json.Marshal(struct {
		Command, Expected, Name, Network, Address, Space, Credential string
		Grant                                                        any
	}{r.Command, r.ExpectedAccount, r.Name, r.NetworkConfig, r.ListenAddress, r.SpaceId, r.CredentialId, r.Grant})
	digest := sha256.Sum256(intent)
	op := &operation{SchemaVersion: 1, Id: r.OperationId, Binding: e.binding(), Intent: hex.EncodeToString(digest[:]), Command: r.Command, Stage: "prepared"}
	previous := new(operation)
	err := localstate.ReadJSON(e.opPath(op.Id), previous)
	if err == nil {
		if previous.SchemaVersion != 1 || previous.Binding != op.Binding || previous.Intent != op.Intent {
			return nil, false, problem("operation_conflict", "This operation ID belongs to a different instance or request. Restore the original request.", 2)
		}
		return previous, true, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, false, problem("journal_unreadable", "Cannot safely read the operation record.", 2)
	}
	return op, false, nil
}

type PrivateSecrets = credentials.Store

var NewSecrets = credentials.New
