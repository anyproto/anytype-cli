package shell

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"
)

// newTestRoot builds root → group → leaf, where leaf records the flag values it
// ran with.
func newTestRoot(got *[]string, gotBool *bool) *cobra.Command {
	root := &cobra.Command{Use: "root", SilenceErrors: true, SilenceUsage: true}
	group := &cobra.Command{Use: "group"}
	var spaces []string
	var readOnly bool
	leaf := &cobra.Command{
		Use: "leaf",
		RunE: func(cmd *cobra.Command, args []string) error {
			*got = append([]string(nil), spaces...)
			*gotBool = readOnly
			return nil
		},
	}
	leaf.Flags().StringArrayVar(&spaces, "space", nil, "")
	leaf.Flags().BoolVar(&readOnly, "read-only", false, "")
	group.AddCommand(leaf)
	root.AddCommand(group)
	return root
}

func TestExecuteLineDoesNotCarryFlagsBetweenCommands(t *testing.T) {
	var got []string
	var gotBool bool
	root := newTestRoot(&got, &gotBool)

	if err := executeLine(root, "group leaf --space A --read-only"); err != nil {
		t.Fatalf("first command: %v", err)
	}
	if err := executeLine(root, "group leaf --space B"); err != nil {
		t.Fatalf("second command: %v", err)
	}

	if !reflect.DeepEqual(got, []string{"B"}) {
		t.Errorf("second command saw --space %v, want [B]", got)
	}
	if gotBool {
		t.Error("second command saw --read-only from the first command")
	}
}

func TestExecuteLineWithoutFlagsSeesDefaults(t *testing.T) {
	var got []string
	var gotBool bool
	root := newTestRoot(&got, &gotBool)

	if err := executeLine(root, "group leaf --space A --read-only"); err != nil {
		t.Fatalf("first command: %v", err)
	}
	if err := executeLine(root, "group leaf"); err != nil {
		t.Fatalf("second command: %v", err)
	}

	if len(got) != 0 || gotBool {
		t.Errorf("second command saw --space %v --read-only=%v, want defaults", got, gotBool)
	}
}

func TestSplitLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    []string
		wantErr bool
	}{
		{"plain words", "auth apikey list", []string{"auth", "apikey", "list"}, false},
		{"repeated spaces", "space   list", []string{"space", "list"}, false},
		{"double quotes keep spaces", `auth apikey create bot --space "My Team"`, []string{"auth", "apikey", "create", "bot", "--space", "My Team"}, false},
		{"single quotes keep spaces", `create 'my bot'`, []string{"create", "my bot"}, false},
		{"quotes are removed", `--space "Personal"`, []string{"--space", "Personal"}, false},
		{"escaped quote inside double quotes", `create "say \"hi\""`, []string{"create", `say "hi"`}, false},
		{"other backslashes are kept", `--path "C:\dir"`, []string{"--path", `C:\dir`}, false},
		{"quotes join with adjacent text", `--name=my" "bot`, []string{"--name=my bot"}, false},
		{"empty quoted argument", `create ""`, []string{"create", ""}, false},
		{"tabs separate", "space\tlist", []string{"space", "list"}, false},
		{"unterminated quote", `create "my bot`, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := splitLine(tt.line)
			if (err != nil) != tt.wantErr {
				t.Fatalf("splitLine(%q) error = %v, wantErr %v", tt.line, err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitLine(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}
