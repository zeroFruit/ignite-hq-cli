package ignitecmd

import (
	"fmt"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"

	"github.com/ignite-hq/cli/ignite/pkg/cliquiz"
	"github.com/ignite-hq/cli/ignite/pkg/clispinner"
	"github.com/ignite-hq/cli/ignite/pkg/cosmosaccount"
	"github.com/ignite-hq/cli/ignite/pkg/cosmosutil"
	"github.com/ignite-hq/cli/ignite/services/chain"
	"github.com/ignite-hq/cli/ignite/services/network"
	"github.com/ignite-hq/cli/ignite/services/network/networkchain"
)

const (
	flagValidatorAccount         = "validator-account"
	flagValidatorWebsite         = "validator-website"
	flagValidatorDetails         = "validator-details"
	flagValidatorSecurityContact = "validator-security-contact"
	flagValidatorMoniker         = "validator-moniker"
	flagValidatorIdentity        = "validator-identity"
	flagValidatorSelfDelegation  = "validator-self-delegation"
	flagValidatorGasPrice        = "validator-gas-price"
)

// NewNetworkChainInit returns a new command to initialize a chain from a published chain ID
func NewNetworkChainInit() *cobra.Command {
	c := &cobra.Command{
		Use:   "init [launch-id]",
		Short: "Initialize a chain from a published chain ID",
		Args:  cobra.ExactArgs(1),
		RunE:  networkChainInitHandler,
	}
	c.Flags().String(flagValidatorAccount, cosmosaccount.DefaultAccount, "Account for the chain validator")
	c.Flags().String(flagValidatorWebsite, "", "Associate a website with the validator")
	c.Flags().String(flagValidatorDetails, "", "Details about the validator")
	c.Flags().String(flagValidatorSecurityContact, "", "Validator security contact email")
	c.Flags().String(flagValidatorMoniker, "", "Custom validator moniker")
	c.Flags().String(flagValidatorIdentity, "", "Validator identity signature (ex. UPort or Keybase)")
	c.Flags().String(flagValidatorSelfDelegation, "", "Validator minimum self delegation")
	c.Flags().String(flagValidatorGasPrice, "", "Validator gas price")
	c.Flags().AddFlagSet(flagNetworkFrom())
	c.Flags().AddFlagSet(flagSetHome())
	c.Flags().AddFlagSet(flagSetKeyringBackend())
	c.Flags().AddFlagSet(flagSetYes())
	return c
}

func networkChainInitHandler(cmd *cobra.Command, args []string) error {
	nb, err := newNetworkBuilder(cmd)
	if err != nil {
		return err
	}
	defer nb.Cleanup()

	// parse launch ID
	launchID, err := network.ParseID(args[0])
	if err != nil {
		return err
	}

	// check if the provided account for the validator exists.
	validatorAccount, _ := cmd.Flags().GetString(flagValidatorAccount)
	if _, err = nb.AccountRegistry.GetByName(validatorAccount); err != nil {
		return err
	}

	// if a chain has already been initialized with this launch ID, we ask for confirmation
	// before erasing the directory.
	chainHome, exist, err := networkchain.IsChainHomeExist(launchID)
	if err != nil {
		return err
	}

	if !getYes(cmd) && exist {
		prompt := promptui.Prompt{
			Label: fmt.Sprintf("The chain has already been initialized under: %s. Would you like to overwrite the home directory",
				chainHome,
			),
			IsConfirm: true,
		}
		nb.Spinner.Stop()
		if _, err := prompt.Run(); err != nil {
			fmt.Println("said no")
			return nil
		}
		nb.Spinner.Start()
	}

	n, err := nb.Network()
	if err != nil {
		return err
	}

	chainLaunch, err := n.ChainLaunch(cmd.Context(), launchID)
	if err != nil {
		return err
	}

	c, err := nb.Chain(networkchain.SourceLaunch(chainLaunch))
	if err != nil {
		return err
	}

	if err := c.Init(cmd.Context()); err != nil {
		return err
	}

	genesisPath, err := c.GenesisPath()
	if err != nil {
		return err
	}

	genesis, err := cosmosutil.ParseGenesisFromPath(genesisPath)
	if err != nil {
		return err
	}

	// ask validator information.
	v, err := askValidatorInfo(cmd, genesis.StakeDenom)
	if err != nil {
		return err
	}
	nb.Spinner.SetText("Generating your Gentx")
	nb.Spinner.Start()

	gentxPath, err := c.InitAccount(cmd.Context(), v, validatorAccount)
	if err != nil {
		return err
	}

	nb.Spinner.Stop()
	fmt.Printf("%s Gentx generated: %s\n", clispinner.Bullet, gentxPath)

	return nil
}

// askValidatorInfo prompts to the user questions to query validator information
func askValidatorInfo(cmd *cobra.Command, stakeDenom string) (chain.Validator, error) {
	var (
		account, _         = cmd.Flags().GetString(flagValidatorAccount)
		website, _         = cmd.Flags().GetString(flagValidatorWebsite)
		details, _         = cmd.Flags().GetString(flagValidatorDetails)
		securityContact, _ = cmd.Flags().GetString(flagValidatorSecurityContact)
		moniker, _         = cmd.Flags().GetString(flagValidatorMoniker)
		identity, _        = cmd.Flags().GetString(flagValidatorIdentity)
		selfDelegation, _  = cmd.Flags().GetString(flagValidatorSelfDelegation)
		gasPrice, _        = cmd.Flags().GetString(flagValidatorGasPrice)
	)
	if gasPrice == "" {
		gasPrice = "0" + stakeDenom
	}
	v := chain.Validator{
		Name:              account,
		Website:           website,
		Details:           details,
		Moniker:           moniker,
		Identity:          identity,
		SecurityContact:   securityContact,
		MinSelfDelegation: selfDelegation,
		GasPrices:         gasPrice,
	}

	questions := append([]cliquiz.Question{},
		cliquiz.NewQuestion("Staking amount",
			&v.StakingAmount,
			cliquiz.DefaultAnswer("95000000stake"),
			cliquiz.Required(),
		),
		cliquiz.NewQuestion("Commission rate",
			&v.CommissionRate,
			cliquiz.DefaultAnswer("0.10"),
			cliquiz.Required(),
		),
		cliquiz.NewQuestion("Commission max rate",
			&v.CommissionMaxRate,
			cliquiz.DefaultAnswer("0.20"),
			cliquiz.Required(),
		),
		cliquiz.NewQuestion("Commission max change rate",
			&v.CommissionMaxChangeRate,
			cliquiz.DefaultAnswer("0.01"),
			cliquiz.Required(),
		),
	)
	return v, cliquiz.Ask(questions...)
}
