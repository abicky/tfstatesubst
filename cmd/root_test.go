package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootCommand(t *testing.T) {
	t.Setenv("TFSTATESUBST_ALLOW_REFERENCE_EXPRESSION", "")
	t.Setenv("TFSTATESUBST_ALLOW_NULL", "")

	statePath := writeState(t)
	const action = "{{ tfstate `aws_vpc.main.id` }}"
	resourceList := fmt.Sprintf(`apiVersion: config.kubernetes.io/v1
kind: ResourceList
functionConfig:
  apiVersion: example.com/v1alpha1
  kind: TfstatesubstTransformer
  metadata:
    name: tfstatesubst
items:
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: example
    data:
      vpc: '%s'
`, action)

	tests := []struct {
		name    string
		args    []string
		input   string
		want    string
		wantErr string
	}{
		{
			name:  "substitutes a state value",
			args:  []string{"--state", statePath},
			input: "role: {{ tfstate `data.aws_iam_role.lambda.arn` }}\n",
			want:  "role: arn:example:lambda\n",
		},
		{
			name:  "permits a null state value",
			args:  []string{"--state", statePath, "--allow-null"},
			input: "optional: {{ tfstate `aws_vpc.main.optional` }}\n",
			want:  "optional: null\n",
		},
		{
			name:    "rejects a null state value by default",
			args:    []string{"--state", statePath},
			input:   "optional: {{ tfstate `aws_vpc.main.optional` }}\n",
			wantErr: `tfstate reference "aws_vpc.main.optional" is null or was not found`,
		},
		{
			name:  "transforms a KRM resource list",
			args:  []string{"--state", statePath},
			input: resourceList,
			want:  strings.Replace(resourceList, action, "vpc-1234", 1),
		},
		{
			name: "permits a matching reference",
			args: []string{
				"--state", statePath,
				"--allow-reference-expression", `^data\.aws_iam_role\.lambda\.(?:arn|id)$`,
			},
			input: "{{ tfstate `data.aws_iam_role.lambda.arn` }}",
			want:  "arn:example:lambda",
		},
		{
			name: "rejects a reference before reading state",
			args: []string{
				"--state", filepath.Join(t.TempDir(), "missing.tfstate"),
				"--allow-reference-expression", `^aws_vpc\.`,
			},
			input:   "{{ tfstate `aws_instance.web[0].id` }}",
			wantErr: `tfstate reference "aws_instance.web[0].id" does not match`,
		},
		{
			name:    "rejects an invalid regular expression",
			args:    []string{"--allow-reference-expression", "["},
			input:   "unchanged",
			wantErr: "failed to compile allow reference expression",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRootCommand()
			cmd.SetArgs(tt.args)
			cmd.SetIn(strings.NewReader(tt.input))
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&output)

			err := cmd.Execute()
			if tt.wantErr == "" && err != nil {
				t.Fatal(err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Errorf("got error %v, want error containing %q", err, tt.wantErr)
			}
			if got := output.String(); got != tt.want {
				t.Errorf("got output %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRootCommandUsesAllowReferenceExpressionEnvironmentVariable(t *testing.T) {
	t.Setenv("TFSTATESUBST_ALLOW_REFERENCE_EXPRESSION", `^aws_vpc\.`)

	command := newRootCommand()
	command.SetArgs([]string{"--state", filepath.Join(t.TempDir(), "missing.tfstate")})
	command.SetIn(strings.NewReader("{{ tfstate `aws_instance.web[0].id` }}"))

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), `tfstate reference "aws_instance.web[0].id" does not match`) {
		t.Fatalf("got error %v", err)
	}
}

func TestRootCommandUsesAllowNullEnvironmentVariable(t *testing.T) {
	t.Setenv("TFSTATESUBST_ALLOW_NULL", "true")

	command := newRootCommand()
	command.SetArgs([]string{"--state", writeState(t)})
	command.SetIn(strings.NewReader("{{ tfstate `aws_vpc.main.optional` }}"))
	var output bytes.Buffer
	command.SetOut(&output)

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "null"; got != want {
		t.Errorf("got output %q, want %q", got, want)
	}
}

func TestRootCommandRejectsInvalidAllowNullEnvironmentVariable(t *testing.T) {
	t.Setenv("TFSTATESUBST_ALLOW_NULL", "invalid")

	command := newRootCommand()
	command.SetIn(strings.NewReader("unchanged"))

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "parse TFSTATESUBST_ALLOW_NULL") {
		t.Fatalf("got error %v", err)
	}
}

func writeState(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "terraform.tfstate")
	contents := `{
  "version": 4,
  "terraform_version": "1.14.0",
  "serial": 1,
  "lineage": "test",
  "outputs": {"endpoint": {"value": "example.test", "type": "string"}},
  "resources": [
    {
      "mode": "managed", "type": "aws_vpc", "name": "main", "provider": "provider[\"registry.terraform.io/hashicorp/aws\"]",
      "instances": [{"schema_version": 1, "attributes": {"id": "vpc-1234", "optional": null}}]
    },
    {
      "mode": "managed", "type": "aws_instance", "name": "web", "provider": "provider[\"registry.terraform.io/hashicorp/aws\"]",
      "instances": [
        {"index_key": 0, "schema_version": 1, "attributes": {"id": "i-1"}},
        {"index_key": 1, "schema_version": 1, "attributes": {"id": "i-2"}}
      ]
    },
    {
      "mode": "data", "type": "aws_iam_role", "name": "lambda", "provider": "provider[\"registry.terraform.io/hashicorp/aws\"]",
      "instances": [{"schema_version": 0, "attributes": {"arn": "arn:example:lambda"}}]
    }
  ]
}`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
