package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/turushan/cheep/internal/exitcode"
	"github.com/turushan/cheep/internal/failure"
	"github.com/turushan/cheep/internal/provider"
)

func newAccountCommand(state *state) *cobra.Command {
	command := &cobra.Command{
		Use:   "account",
		Short: "Inspect account balances and pricing",
		Args:  noArgs,
	}
	command.AddCommand(newAccountBalanceCommand(state))
	command.AddCommand(newAccountPricingCommand(state))
	return command
}

func newAccountBalanceCommand(state *state) *cobra.Command {
	return &cobra.Command{
		Use:   "balance",
		Short: "Show the account balance",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reader, profile, err := state.reader(cmd.Context())
			if err != nil {
				return err
			}
			ctx, cancel := state.commandContext(cmd.Context())
			defer cancel()
			balance, err := reader.Balance(ctx)
			if err != nil {
				return mapProviderError(err)
			}
			return state.printer().DataWithMeta("account.balance", balance, metaFor(profile), func(w io.Writer) error {
				_, err := fmt.Fprintf(w, "Available: %s %s\nAccount: %s %s\nRequired for auto-renew: %s %s\n",
					balance.Available,
					balance.Currency,
					balance.Account,
					balance.Currency,
					balance.RequiredForAutoRenew,
					balance.Currency,
				)
				return err
			})
		},
	}
}

func newAccountPricingCommand(state *state) *cobra.Command {
	var action string
	var years int
	command := &cobra.Command{
		Use:   "pricing <tld>",
		Short: "Show an exact account price for a domain action",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			action = strings.ToLower(action)
			if !oneOf(action, "register", "renew", "transfer", "reactivate") {
				return failure.New("invalid_action", exitcode.Usage, "action must be register, renew, transfer, or reactivate")
			}
			if years < 1 || years > 10 {
				return failure.New("invalid_years", exitcode.Usage, "years must be between 1 and 10")
			}
			tld := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(args[0])), ".")
			if tld == "" || strings.Contains(tld, ".") {
				return failure.New("invalid_tld", exitcode.Usage, "TLD must be a single label such as com")
			}
			reader, profile, err := state.reader(cmd.Context())
			if err != nil {
				return err
			}
			ctx, cancel := state.commandContext(cmd.Context())
			defer cancel()
			price, err := reader.Price(ctx, provider.PriceRequest{TLD: tld, Action: action, Years: years})
			if err != nil {
				return mapProviderError(err)
			}
			return state.printer().DataWithMeta("account.pricing", price, metaFor(profile), func(w io.Writer) error {
				_, err := fmt.Fprintf(w, ".%s %s for %d year(s): %s %s\n", price.TLD, price.Action, price.Years, price.Effective, price.Currency)
				return err
			})
		},
	}
	flags := command.Flags()
	flags.StringVar(&action, "action", "register", "domain action: register, renew, transfer, or reactivate")
	flags.IntVar(&years, "years", 1, "term length in years")
	return command
}
