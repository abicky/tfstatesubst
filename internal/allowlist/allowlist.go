// Package allowlist restricts which Terraform state references may be
// resolved by a template.
package allowlist

import (
	"fmt"
	"regexp"
)

// List contains the regular expression that tfstate references must match.
type List struct {
	pattern *regexp.Regexp
}

// New constructs an allow-list. An empty expression permits every tfstate
// reference. The expression uses Go regular expression syntax.
func New(expression string) (List, error) {
	if expression == "" {
		return List{}, nil
	}
	pattern, err := regexp.Compile(expression)
	if err != nil {
		return List{}, fmt.Errorf("failed to compile allow reference expression %q: %w", expression, err)
	}
	return List{pattern: pattern}, nil
}

// Check returns an error unless reference matches the configured pattern.
func (l List) Check(reference string) error {
	if l.pattern == nil || l.pattern.MatchString(reference) {
		return nil
	}
	return fmt.Errorf(
		"tfstate reference %q does not match allow reference expression %q",
		reference,
		l.pattern.String(),
	)
}
