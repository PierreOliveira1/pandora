package cli

import (
	"errors"
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/PierreOliveira1/pandora/internal/config"
)

func newHTTP(d deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "http",
		Short: "Manage the URLs and headers of HTTP services",
	}
	cmd.AddCommand(newHTTPURL(d), newHTTPHeader(d), newHTTPShow(d))
	return cmd
}

func newHTTPURL(d deps) *cobra.Command {
	var remove bool

	cmd := &cobra.Command{
		Use:         "url <service> <name> <url> | url <service> <name> --remove",
		Short:       "Set or remove a base URL of an HTTP service",
		Annotations: map[string]string{syncAnnotation: "true"},
		Args:        cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			if remove {
				if len(args) != 2 {
					return errors.New("use pandora http url <service> <name> --remove")
				}
				if err := d.registry.RemoveServiceURL(args[0], args[1]); err != nil {
					return err
				}
				fmt.Fprintf(d.stdout, "Removed URL %s from %s.\n", args[1], args[0])
				return nil
			}
			if len(args) != 3 {
				return errors.New("use pandora http url <service> <name> <url>")
			}
			if err := d.registry.SetServiceURL(args[0], args[1], args[2]); err != nil {
				return err
			}
			fmt.Fprintf(d.stdout, "URL %s of %s: %s\n", args[1], args[0], args[2])
			return nil
		},
	}
	cmd.Flags().BoolVar(&remove, "remove", false, "remove the URL")
	return cmd
}

func newHTTPHeader(d deps) *cobra.Command {
	var remove bool

	cmd := &cobra.Command{
		Use:   "header <service> <name> <template> | header <service> <name> --remove",
		Short: "Set or remove a service header",
		Long:  "Set or remove a header sent with every request of an HTTP service.\n\nThe template can only use {{env.NAME}}, for example:\n\n  pandora http header instagram Authorization \"OAuth {{env.META_TOKEN}}\"\n\nNAME is a value of the account: --env and pandora env for plain values, --secret-env for secrets. The config file stores only the template, never a secret.",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			if remove {
				if len(args) != 2 {
					return errors.New("use pandora http header <service> <name> --remove")
				}
				if err := d.registry.RemoveServiceHeader(args[0], args[1]); err != nil {
					return err
				}
				fmt.Fprintf(d.stdout, "Removed header %s from %s.\n", args[1], args[0])
				return nil
			}
			if len(args) != 3 {
				return errors.New("use pandora http header <service> <name> <template>")
			}
			if err := d.registry.SetServiceHeader(args[0], args[1], args[2]); err != nil {
				return err
			}
			fmt.Fprintf(d.stdout, "Header %s of %s set.\n", args[1], args[0])
			return nil
		},
	}
	cmd.Flags().BoolVar(&remove, "remove", false, "remove the header")
	return cmd
}

func newHTTPShow(d deps) *cobra.Command {
	return &cobra.Command{
		Use:   "show <service>",
		Short: "Show the URLs, headers and HTTP tools of a service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			info, err := d.registry.GetHTTPService(args[0])
			if err != nil {
				return err
			}
			writer := tabwriter.NewWriter(d.stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(writer, "URLs:")
			for _, name := range sortedStringKeys(info.URLs) {
				fmt.Fprintf(writer, "%s\t%s\n", name, info.URLs[name])
			}
			if len(info.URLs) == 0 {
				fmt.Fprintln(writer, "none")
			}
			fmt.Fprintln(writer, "Headers:")
			for _, name := range sortedStringKeys(info.Headers) {
				fmt.Fprintf(writer, "%s\t%s\n", name, info.Headers[name])
			}
			if len(info.Headers) == 0 {
				fmt.Fprintln(writer, "none")
			}
			fmt.Fprintln(writer, "HTTP tools:")
			tools := make([]config.CommandTool, 0, len(info.Tools))
			for _, tool := range info.Tools {
				if tool.HTTP == nil {
					continue
				}
				tools = append(tools, tool)
			}
			sort.Slice(tools, func(i, j int) bool {
				return tools[i].Name < tools[j].Name
			})
			for _, tool := range tools {
				urlName := tool.HTTP.URL
				if urlName == "" {
					urlName = singleURLName(info.URLs)
				}
				fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", tool.Name, tool.HTTP.Method, urlName, tool.HTTP.Path)
			}
			if len(tools) == 0 {
				fmt.Fprintln(writer, "none")
			}
			return writer.Flush()
		},
	}
}

func sortedStringKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for name := range values {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	return keys
}

func singleURLName(urls map[string]string) string {
	if len(urls) == 1 {
		for name := range urls {
			return name
		}
	}
	return "default"
}
