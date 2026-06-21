package tfstatetmpl_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/abicky/tfstatesubst/internal/tfstatetmpl"
	"github.com/fujiwara/tfstate-lookup/tfstate"
)

type fakeState map[string]any

func (s fakeState) Lookup(reference string) (*tfstate.Object, error) {
	value, ok := s[reference]
	if !ok {
		return &tfstate.Object{}, nil
	}
	return &tfstate.Object{Value: value}, nil
}

func TestExecute(t *testing.T) {
	t.Parallel()

	state := fakeState{
		"data.aws_iam_role.lambda.arn": "arn:aws:iam::123456789012:role/lambda",
		`aws_subnet.lambda["az-a"].id`: "subnet-1234",
		"aws_instance.web.ids":         []any{"i-1", "i-2"},
		"aws_instance.web.optional":    nil,
	}

	tests := []struct {
		name            string
		input           string
		state           tfstatetmpl.State
		wantOutput      string
		wantLoads       int
		wantErrContains string
		options         []tfstatetmpl.Option
	}{
		{
			name: "With expected tfstate references",
			input: strings.Join([]string{
				"role: {{ tfstate `data.aws_iam_role.lambda.arn` }}",
				"subnet: {{ tfstate \"aws_subnet.lambda['az-a'].id\" }}",
				"instances: '{{ tfstate `aws_instance.web.ids` }}'",
				"",
			}, "\n"),
			state: state,
			wantOutput: strings.Join([]string{
				"role: arn:aws:iam::123456789012:role/lambda",
				"subnet: subnet-1234",
				`instances: '["i-1","i-2"]'`,
				"",
			}, "\n"),
			wantLoads: 1,
		},
		{
			name:       "Without an action",
			input:      "kind: ConfigMap",
			wantOutput: "kind: ConfigMap",
			wantLoads:  0,
		},
		{
			name:            "With missing reference",
			input:           "prefix {{ tfstate `missing` }}",
			state:           fakeState{},
			wantOutput:      "",
			wantLoads:       1,
			wantErrContains: "is null or was not found",
		},
		{
			name:       "With allowed null reference",
			input:      "optional: {{ tfstate `aws_instance.web.optional` }}",
			state:      state,
			wantOutput: "optional: null",
			wantLoads:  1,
			options:    []tfstatetmpl.Option{tfstatetmpl.WithAllowNull(true)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			loads := 0
			err := tfstatetmpl.Execute([]byte(tt.input), &output, func() (tfstatetmpl.State, error) {
				loads++
				return tt.state, nil
			}, tt.options...)

			if tt.wantErrContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Errorf("got error %v, want it to contain %q", err, tt.wantErrContains)
				}
			} else if err != nil {
				t.Fatal(err)
			}

			if loads != tt.wantLoads {
				t.Errorf("loaded state %d times, want %d", loads, tt.wantLoads)
			}

			if output.String() != tt.wantOutput {
				t.Fatalf("output:\n%s\nwant:\n%s", output.String(), tt.wantOutput)
			}
		})
	}
}

func TestExecuteRejectsUnrestrictedTemplates(t *testing.T) {
	t.Parallel()

	state := fakeState{}

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "field",
			input: "{{ .value }}",
		},
		{
			name:  "if",
			input: "{{ if tfstate `x` }}yes{{ end }}",
		},
		{
			name:  "range",
			input: "{{ range tfstate `x` }}{{ end }}",
		},
		{
			name:  "variable",
			input: "{{ $x := tfstate `x` }}",
		},
		{
			name:  "pipeline",
			input: "{{ tfstate `x` | tfstate }}",
		},
		{
			name:  "number argument",
			input: "{{ tfstate 1 }}",
		},
		{
			name:  "too many args",
			input: "{{ tfstate `x` `y` }}",
		},
		{
			name:  "definition",
			input: "{{ define `other` }}text{{ end }}",
		},
		{
			name:  "unknown function",
			input: "{{ printf `x` }}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			err := tfstatetmpl.Execute([]byte(tt.input), &output, func() (tfstatetmpl.State, error) {
				return state, nil
			})
			if err == nil {
				t.Fatal("want an error")
			}
			if output.Len() != 0 {
				t.Errorf("wrote partial output %q", output.String())
			}
		})
	}
}
