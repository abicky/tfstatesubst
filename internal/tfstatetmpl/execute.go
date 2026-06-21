// Package tfstatetmpl executes the deliberately small tfstatesubst template
// language. It uses Go's parser, then rejects every construct outside a
// direct call of the form {{ tfstate "reference" }}.
package tfstatetmpl

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/template"
	"text/template/parse"

	"github.com/fujiwara/tfstate-lookup/tfstate"
)

// State is the portion of a Terraform state needed by the renderer.
type State interface {
	Lookup(string) (*tfstate.Object, error)
}

// Loader loads state only after the input has passed syntax validation and
// only when the input actually contains a tfstate action.
type Loader func() (State, error)

type config struct {
	allow     func(string) error
	allowNull bool
}

type resolverError struct {
	err error
}

func newResolverError(err error) resolverError {
	return resolverError{err: err}
}

func (e resolverError) Error() string {
	return e.err.Error()
}

// Option configures template execution.
type Option func(*config)

// WithAllowFunc checks every normalized tfstate reference before state is
// loaded or queried.
func WithAllowFunc(allow func(string) error) Option {
	return func(config *config) {
		config.allow = allow
	}
}

// WithAllowNull permits tfstate references whose value is null.
func WithAllowNull(allow bool) Option {
	return func(config *config) {
		config.allowNull = allow
	}
}

// Execute validates and renders input to out.
func Execute(input []byte, out io.Writer, load Loader, options ...Option) error {
	cfg := config{}
	for _, option := range options {
		option(&cfg)
	}

	var state State
	resolver := func(reference string) (string, error) {
		// Convert "'" to `"` since references into Terraform state only include double quotes.
		reference = strings.ReplaceAll(reference, "'", `"`)
		if cfg.allow != nil {
			if err := cfg.allow(reference); err != nil {
				return "", newResolverError(err)
			}
		}

		if state == nil {
			loaded, err := load()
			if err != nil {
				return "", newResolverError(err)
			}
			if loaded == nil {
				return "", newResolverError(errors.New("failed to load tfstate: loader returned no state"))
			}
			state = loaded
		}

		object, err := state.Lookup(reference)
		if err != nil {
			return "", newResolverError(fmt.Errorf("failed to lookup %q: %w", reference, err))
		}
		if object == nil || (object.Value == nil && !cfg.allowNull) {
			return "", newResolverError(fmt.Errorf("tfstate reference %q is null or was not found", reference))
		}
		return object.String(), nil
	}

	tmpl, err := template.New("stdin").Funcs(template.FuncMap{"tfstate": resolver}).Parse(string(input))
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	if err := validateTemplate(tmpl); err != nil {
		return err
	}

	var result bytes.Buffer
	if err := tmpl.Execute(&result, nil); err != nil {
		var rerr resolverError
		if errors.As(err, &rerr) {
			return rerr
		}
		return fmt.Errorf("failed to execute template: %w", err)
	}

	if _, err := result.WriteTo(out); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	return nil
}

func validateTemplate(tmpl *template.Template) error {
	if len(tmpl.Templates()) != 1 {
		return errors.New("restricted template: template definitions are not allowed")
	}
	return validateNode(tmpl.Tree.Root)
}

func validateNode(node parse.Node) error {
	switch node := node.(type) {
	case *parse.ListNode:
		for _, child := range node.Nodes {
			if err := validateNode(child); err != nil {
				return err
			}
		}
		return nil
	case *parse.TextNode:
		return nil
	case *parse.ActionNode:
		return validateAction(node)
	default:
		return fmt.Errorf("restricted template: node type %v is not allowed", node.Type())
	}
}

func validateAction(action *parse.ActionNode) error {
	pipe := action.Pipe
	if len(pipe.Decl) != 0 || pipe.IsAssign || len(pipe.Cmds) != 1 {
		return fmt.Errorf("restricted template: pipelines and variables are not allowed")
	}

	command := pipe.Cmds[0]
	if len(command.Args) != 2 {
		return fmt.Errorf("restricted template: tfstate requires exactly one string argument")
	}
	identifier, ok := command.Args[0].(*parse.IdentifierNode)
	if !ok || identifier.Ident != "tfstate" {
		return fmt.Errorf("restricted template: only the tfstate function is allowed")
	}
	if _, ok := command.Args[1].(*parse.StringNode); !ok {
		return fmt.Errorf("restricted template: tfstate requires a literal string argument")
	}
	return nil
}
