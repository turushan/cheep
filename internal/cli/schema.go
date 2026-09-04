package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/turushan/nccli/internal/output"
)

type commandSchema struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Flags       []flagSchema `json:"flags,omitempty"`
}

type flagSchema struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description"`
}

type schemaData struct {
	Version      string          `json:"version"`
	Experimental bool            `json:"experimental"`
	Commands     []commandSchema `json:"commands"`
}

func newSchemaCommand(root *cobra.Command, state *state) *cobra.Command {
	return &cobra.Command{
		Use:   "schema",
		Short: "Print the experimental machine-readable command inventory",
		Args: func(_ *cobra.Command, args []string) error {
			return requireNoArgs(args)
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			data := buildSchema(root)
			return output.Printer{Stdout: state.stdout, Stderr: state.stderr, JSON: state.json}.
				Data("schema", data, func(w io.Writer) error {
					for _, command := range data.Commands {
						if _, err := fmt.Fprintln(w, command.Name); err != nil {
							return err
						}
					}
					return nil
				})
		},
	}
}

func buildSchema(root *cobra.Command) schemaData {
	commands := make([]commandSchema, 0)
	var walk func(*cobra.Command)
	walk = func(command *cobra.Command) {
		if command != root && !command.Hidden && !isCompletionCommand(command) {
			item := commandSchema{
				Name:        schemaCommandName(root, command),
				Description: command.Short,
			}
			command.NonInheritedFlags().VisitAll(func(flag *pflag.Flag) {
				item.Flags = append(item.Flags, flagSchema{
					Name:        flag.Name,
					Type:        flag.Value.Type(),
					Default:     flag.DefValue,
					Description: flag.Usage,
				})
			})
			sort.Slice(item.Flags, func(i, j int) bool { return item.Flags[i].Name < item.Flags[j].Name })
			commands = append(commands, item)
		}
		for _, child := range command.Commands() {
			walk(child)
		}
	}
	walk(root)
	sort.Slice(commands, func(i, j int) bool { return commands[i].Name < commands[j].Name })
	return schemaData{Version: "experimental-v0", Experimental: true, Commands: commands}
}

func schemaCommandName(root *cobra.Command, command *cobra.Command) string {
	name := strings.TrimSpace(strings.TrimPrefix(command.CommandPath(), root.CommandPath()))
	return strings.ReplaceAll(name, " ", ".")
}

func isCompletionCommand(command *cobra.Command) bool {
	for current := command; current != nil; current = current.Parent() {
		if current.Name() == "completion" {
			return true
		}
	}
	return false
}
