package allowlist_test

import (
	"strings"
	"testing"

	"github.com/abicky/tfstatesubst/internal/allowlist"
)

func TestCheckUnconfiguredAllowsAllReferences(t *testing.T) {
	t.Parallel()

	list, err := allowlist.New("")
	if err != nil {
		t.Fatal(err)
	}
	for _, reference := range []string{"aws_vpc.main.id", "data.aws_ami.ubuntu.id", "output.endpoint"} {
		if err := list.Check(reference); err != nil {
			t.Errorf("Check(%q): %v", reference, err)
		}
	}
}

func TestCheck(t *testing.T) {
	t.Parallel()

	list, err := allowlist.New(`^(?:module\.[^.]+\.)*(?:aws_(?:vpc|subnet)\.main|data\.aws_ami\.ubuntu)\b`)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		reference string
		wantErr   string
	}{
		{"aws_vpc.main.id", ""},
		{`module.network.aws_subnet.main["public"].id`, ""},
		{"data.aws_ami.ubuntu.id", ""},
		{"aws_instance.main.id", "does not match"},
		{"aws_vpc.other.id", "does not match"},
		{"data.aws_ami.other.id", "does not match"},
		{"output.endpoint", "does not match"},
	}
	for _, tt := range tests {
		t.Run(tt.reference, func(t *testing.T) {
			t.Parallel()
			err := list.Check(tt.reference)
			if tt.wantErr == "" && err != nil {
				t.Fatal(err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Errorf("got error %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestNewRejectsInvalidExpression(t *testing.T) {
	t.Parallel()

	_, err := allowlist.New("[")
	if err == nil || !strings.Contains(err.Error(), "failed to compile allow reference expression") {
		t.Errorf("got unexpected error %v", err)
	}
}
