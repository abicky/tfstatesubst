package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/abicky/tfstatesubst/internal/allowlist"
	"github.com/abicky/tfstatesubst/internal/tfstatetmpl"
	"github.com/fujiwara/tfstate-lookup/tfstate"
	"github.com/spf13/cobra"
)

const (
	toolName       = "tfstatesubst"
	defaultVersion = "dev"
	defaultTimeout = 30 * time.Second
)

// These variables are overwritten by -ldflags in release builds.
var (
	version  = defaultVersion
	revision string
)

type rootOptions struct {
	stateLocation string
	timeout       time.Duration
}

type renderOptions struct {
	allowReferenceExpression string
	allowNull                bool
}

var (
	rootOpts rootOptions
)

func Execute() {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	allowNull, allowNullErr := defaultAllowNull()
	renderOpts := &renderOptions{allowNull: allowNull}

	cmd := &cobra.Command{
		Use:   toolName,
		Short: "Substitute Terraform state values into restricted Go templates",
		Long: `tfstatesubst reads a restricted Go template from standard input, replaces calls
to the tfstate function with values from Terraform state, and writes the result
to standard output.`,
		Example: "  echo '{{ tfstate `data.aws_iam_role.lambda.arn` }}' | tfstatesubst\n\n" +
			"  tfstatesubst --state .terraform/terraform.tfstate < input.yaml\n\n" +
			"  TFSTATESUBST_TFSTATE=s3://example/terraform.tfstate tfstatesubst < input.yaml",
		Version:       version,
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if allowNullErr != nil && !cmd.Flags().Changed("allow-null") {
				return allowNullErr
			}
			if f, ok := cmd.InOrStdin().(*os.File); ok && f == os.Stdin {
				// cf. https://stackoverflow.com/a/26567513
				stat, _ := f.Stat()
				if (stat.Mode() & os.ModeCharDevice) != 0 {
					return errors.New("data from stdin is required")
				}
			}

			return runRender(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), renderOpts)
		},
	}

	if version == defaultVersion {
		if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			version = strings.TrimPrefix(bi.Main.Version, "v")
			cmd.Version = version
		}
	}
	if revision != "" {
		cmd.SetVersionTemplate(fmt.Sprintf(
			`{{with .Name}}{{printf "%%s " .}}{{end}}{{printf "version %%s" .Version}} (revision %s)
`, revision))
	}

	cmd.PersistentFlags().StringVarP(
		&rootOpts.stateLocation,
		"state",
		"s",
		defaultStateLocation(),
		"Terraform state file path or URL",
	)
	cmd.PersistentFlags().DurationVar(
		&rootOpts.timeout,
		"timeout",
		defaultTimeout,
		"timeout for reading Terraform state",
	)
	cmd.Flags().StringVar(
		&renderOpts.allowReferenceExpression,
		"allow-reference-expression",
		os.Getenv("TFSTATESUBST_ALLOW_REFERENCE_EXPRESSION"),
		"regular expression matching tfstate references to allow",
	)
	cmd.Flags().BoolVar(
		&renderOpts.allowNull,
		"allow-null",
		allowNull,
		"render null tfstate values instead of returning an error",
	)

	return cmd
}

func defaultStateLocation() string {
	if location := os.Getenv("TFSTATESUBST_TFSTATE"); location != "" {
		return location
	}
	return "terraform.tfstate"
}

func defaultAllowNull() (bool, error) {
	value := os.Getenv("TFSTATESUBST_ALLOW_NULL")
	if value == "" {
		return false, nil
	}
	allow, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("parse TFSTATESUBST_ALLOW_NULL: %w", err)
	}
	return allow, nil
}

func runRender(ctx context.Context, in io.Reader, out io.Writer, opts *renderOptions) error {
	allowed, err := allowlist.New(opts.allowReferenceExpression)
	if err != nil {
		return err
	}

	input, err := io.ReadAll(in)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, rootOpts.timeout)
	defer cancel()

	return tfstatetmpl.Execute(input, out, func() (tfstatetmpl.State, error) {
		state, err := tfstate.ReadURL(ctx, rootOpts.stateLocation)
		if err != nil {
			return nil, fmt.Errorf("read tfstate %q: %w", rootOpts.stateLocation, err)
		}
		return state, nil
	},
		tfstatetmpl.WithAllowFunc(allowed.Check),
		tfstatetmpl.WithAllowNull(opts.allowNull),
	)
}
