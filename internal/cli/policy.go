package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/registry"
)

type policyOutput struct {
	Ref      string   `json:"ref"`
	ReadOnly bool     `json:"readonly"`
	Allow    []string `json:"allow"`
	Deny     []string `json:"deny"`
}

func newPolicy(d deps) *cobra.Command {
	var (
		readOnly bool
		writable bool
		allow    []string
		deny     []string
		remove   []string
		clear    bool
		asJSON   bool
	)
	cmd := &cobra.Command{
		Use:         "policy <service/account> [flags]",
		Short:       "Show or change the policy of an account",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{syncAnnotation: "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := registry.ParseRef(args[0])
			if err != nil {
				return err
			}
			if ref.Account == "" {
				return fmt.Errorf("account is required: use %s/<account>", ref.Service)
			}
			if readOnly && writable {
				return errors.New("use only one of --readonly or --writable")
			}
			input := registry.PolicyInput{
				Allow:      allow,
				Deny:       deny,
				RemoveRule: remove,
				Clear:      clear,
			}
			if readOnly {
				value := true
				input.ReadOnly = &value
			}
			if writable {
				value := false
				input.ReadOnly = &value
			}
			change := input.ReadOnly != nil || clear || len(allow) > 0 || len(deny) > 0 || len(remove) > 0
			if change {
				value, err := d.registry.SetPolicy(ref, input)
				if err != nil {
					return err
				}
				if asJSON {
					return writeJSON(d.stdout, policyView(ref.String(), value.ReadOnly, value.Allow, value.Deny))
				}
				printPolicy(d, ref.String(), value.ReadOnly, value.Allow, value.Deny)
				return nil
			}
			value, err := d.registry.GetPolicy(ref)
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(d.stdout, policyView(ref.String(), value.ReadOnly, value.Allow, value.Deny))
			}
			printPolicy(d, ref.String(), value.ReadOnly, value.Allow, value.Deny)
			return nil
		},
	}
	cmd.Flags().BoolVar(&readOnly, "readonly", false, "make the account read-only")
	cmd.Flags().BoolVar(&writable, "writable", false, "allow tools that change data")
	cmd.Flags().StringArrayVar(&allow, "allow", nil, "allow a tool pattern (repeatable)")
	cmd.Flags().StringArrayVar(&deny, "deny", nil, "deny a tool pattern (repeatable)")
	cmd.Flags().StringArrayVar(&remove, "remove", nil, "remove a tool pattern from allow and deny (repeatable)")
	cmd.Flags().BoolVar(&clear, "clear", false, "clear read-only, allow and deny before applying changes")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output JSON")
	return cmd
}

func policyView(ref string, readOnly bool, allow, deny []string) policyOutput {
	return policyOutput{
		Ref:      ref,
		ReadOnly: readOnly,
		Allow:    sortedRules(allow),
		Deny:     sortedRules(deny),
	}
}

func printPolicy(d deps, ref string, readOnly bool, allow, deny []string) {
	fmt.Fprintln(d.stdout, ref)
	fmt.Fprintf(d.stdout, "  read-only: %s\n", yesNo(readOnly))
	fmt.Fprintf(d.stdout, "  allow: %s\n", ruleList(allow))
	fmt.Fprintf(d.stdout, "  deny: %s\n", ruleList(deny))
}

func sortedRules(rules []string) []string {
	out := append([]string(nil), rules...)
	sort.Strings(out)
	if out == nil {
		return []string{}
	}
	return out
}

func ruleList(rules []string) string {
	if len(rules) == 0 {
		return "(none)"
	}
	return strings.Join(sortedRules(rules), ", ")
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
