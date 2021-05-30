package repl

import (
	"encoding/hex"
	"fmt"
	"io"
	"strconv"

	apitypes "github.com/spacemeshos/api/release/go/spacemesh/v1"

	gosmtypes "github.com/spacemeshos/go-spacemesh/common/types"

	"github.com/spacemeshos/CLIWallet/log"
	"github.com/spacemeshos/go-spacemesh/common/util"
)

// number of bytes in 1 GiB
const GIB uint64 = 1_262_485_504

func (r *repl) printSmeshingStatus() {
	res, err := r.client.SmeshingStatus()
	if err != nil {
		log.Error("failed to get proof of space status: %v", err)
		return
	}

	switch res.Status {
	case apitypes.SmeshingStatusResponse_SMESHING_STATUS_IDLE:
		fmt.Println(printPrefix, "Proof of space data was not created.")
	case apitypes.SmeshingStatusResponse_SMESHING_STATUS_CREATING_POST_DATA:
		fmt.Println(printPrefix, "Proof of space data creation is in progress.")
	case apitypes.SmeshingStatusResponse_SMESHING_STATUS_ACTIVE:
		fmt.Println(printPrefix, "Proof of space data was created and is used for smeshing.")
	default:
		fmt.Println("printPrefix", "Unexpected api result.")
	}
}

/// setupPos start an interactive proof of space data creation process
func (r *repl) setupPos() {
	cfg, err := r.client.Config()
	if err != nil {
		log.Error("failed get proof of space config: %v", err)
		return
	}

	addrStr := inputNotBlank(enterRewardsAddress)
	addr := gosmtypes.HexToAddress(addrStr)
	dataDir := inputNotBlank(posDataDirMsg)

	unitSize := uint64(cfg.BitsPerLabel) * cfg.LabelsPerUnit / 8
	unitSizeInGiB := float32(unitSize) / float32(GIB)
	numUnitsStr := inputNotBlank(fmt.Sprintf(posSizeMsg, unitSizeInGiB, cfg.MinNumUnits, cfg.MaxNumUnits))
	numUnits, err := strconv.ParseUint(numUnitsStr, 10, 64)
	if err != nil {
		log.Error("invalid input: %v", err)
		return
	}

	// TODO: validate numUnits against min/max

	// TODO: validate provider id is valid by enum the providers here....

	providerIdStr := inputNotBlank(posProviderMsg)
	providerId, err := strconv.ParseUint(providerIdStr, 10, 32)
	if err != nil {
		log.Error("failed to parse your input: %v", err)
		return
	}

	// request summary information
	fmt.Println(printPrefix, "Proof of space setup request summary")
	fmt.Println("Directory path (relative to node or absolute):", dataDir)
	fmt.Println("Number of units:", numUnits)
	fmt.Println("Size (GiB):", unitSizeInGiB*float32(numUnits))
	fmt.Println("Compute provider id:", providerId)
	fmt.Println("Bits per label:", cfg.BitsPerLabel)
	fmt.Println("Labels per unit:", cfg.LabelsPerUnit)
	fmt.Println("Number of files:", 1)

	req := &apitypes.StartSmeshingRequest{}
	req.Coinbase = &apitypes.AccountId{Address: addr.Bytes()}
	req.Opts = &apitypes.PostInitOpts{
		DataDir:           dataDir,
		NumUnits:          uint32(numUnits),
		NumFiles:          1,
		ComputeProviderId: uint32(providerId),
		Throttle:          false,
	}

	resp, err := r.client.StartSmeshing(req)
	if err != nil {
		log.Error("failed to set up proof of space due to an error: %v", err)
		return
	}

	if resp.Code != 0 {
		log.Error("failed to set up proof of space. Node response code: %d", resp.Code)
		return
	}

	fmt.Println(printPrefix, "Proof of space setup has started and your node will be smeshing as soon as it is complete. Please add the following to your node's config file so it will continue smeshing after you restart it")
	fmt.Println(printPrefix, "todo: Json to add to node config file here")
}

func (r *repl) printPostDataCreationProgress() {
	cfg, err := r.client.Config()
	if err != nil {
		log.Error("failed to query for smeshing config: %v", err)
		return
	}

	stream, err := r.client.PostDataCreationProgressStream()
	if err != nil {
		log.Error("failed to get post data creation stream: %v", err)
		return
	}

	var initial bool
	for {
		e, err := stream.Recv()
		if err == io.EOF {
			log.Info("api server closed the server-side stream")
			return
		} else if err != nil {
			log.Error("error reading from post data creation stream: %v", err)
			return
		}

		numLabels := uint64(e.Status.SessionOpts.NumUnits) * cfg.LabelsPerUnit
		numLabelsWrittenPct := uint64(float64(e.Status.NumLabelsWritten) / float64(numLabels) * 100)

		if initial == false {
			fmt.Printf("session options: %+v\n", e.Status.SessionOpts)
			fmt.Printf("config: %+v\n", cfg)
			fmt.Printf("num labels target: %+v\n", numLabels)
			initial = true
		}

		fmt.Printf("num labels written: %d (%d%%)\n",
			e.Status.NumLabelsWritten, numLabelsWrittenPct)
	}
}

func (r *repl) stopSmeshing() {
	deleteData := yesOrNoQuestion(confirmDeleteDataMsg) == "y"
	resp, err := r.client.StopSmeshing(deleteData)

	if err != nil {
		log.Error("failed to stop smeshing: %v", err)
		return
	}

	if resp.Code != 0 {
		log.Error("failed to stop smeshing. Response status: %d", resp.Code)
		return
	}

	fmt.Println(printPrefix, "Smeshing stopped. Don't forget to remove smeshing related data from your node's config file or startup flags so it won't start smeshing after you restart it")

}

var computeApiClassName = map[int32]string{
	0: "Unspecified",
	1: "CPU",
	2: "CUDA",
	3: "VULKAN",
}

/// setupProofOfSpace prints the available proof of space compute providers
func (r *repl) printPosProviders() {

	providers, err := r.client.GetPostComputeProviders(false)
	if err != nil {
		log.Error("failed to get compute providers: %v", err)
		return
	}

	if len(providers) == 0 {
		fmt.Println(printPrefix, "No supported compute providers found")
		return
	}

	fmt.Println(printPrefix, "Supported providers on your system:")

	for i, p := range providers {
		if i != 0 {
			fmt.Println("-----")
		}
		fmt.Println("Provider id:", p.GetId())
		fmt.Println("Model:", p.GetModel())
		fmt.Println("Compute api:", computeApiClassName[int32(p.GetComputeApi())])
		fmt.Println("Performance:", p.GetPerformance())
	}
}

func (r *repl) print() {
	providers, err := r.client.GetPostComputeProviders(false)
	if err != nil {
		log.Error("failed to get compute providers: %v", err)
		return
	}

	if len(providers) == 0 {
		fmt.Println(printPrefix, "No supported compute providers found")
		return
	}

	fmt.Println(printPrefix, "Supported providers on your system:")

	for i, p := range providers {
		if i != 0 {
			fmt.Println("-----")
		}
		fmt.Println("Provider id:", p.GetId())
		fmt.Println("Model:", p.GetModel())
		fmt.Println("Compute api:", computeApiClassName[int32(p.GetComputeApi())])
		fmt.Println("Performance:", p.GetPerformance())
	}
}

func (r *repl) printSmesherId() {
	if resp, err := r.client.GetSmesherId(); err != nil {
		log.Error("failed to get smesher id: %v", err)
	} else {
		fmt.Println(printPrefix, "Smesher id:", "0x"+hex.EncodeToString(resp))
	}
}

func (r *repl) printRewardsAddress() {
	if resp, err := r.client.GetRewardsAddress(); err != nil {
		log.Error("failed to get rewards address: %v", err)
	} else {
		fmt.Println(printPrefix, "Rewards address is:", resp.String())
	}
}

// setRewardsAddress sets the smesher's reward address to a user provider address
func (r *repl) setRewardsAddress() {
	addrStr := inputNotBlank(enterAddressMsg)
	addr := gosmtypes.HexToAddress(addrStr)

	resp, err := r.client.SetRewardsAddress(addr)

	if err != nil {
		log.Error("failed to set rewards address: %v", err)
		return
	}

	if resp.Code == 0 {
		fmt.Println(printPrefix, "Rewards address set to:", addr.String())
	} else {
		// todo: what are the possible non-zero status codes here?
		fmt.Println(printPrefix, fmt.Sprintf("Response status code: %d", resp.Code))
	}
}

////////// The following methods use the global state service and not the smesher service

// printSmesherRewards prints all rewards awarded to a smesher identified by an id
func (r *repl) printSmesherRewards() {

	smesherIdStr := inputNotBlank(smesherIdMsg)
	smesherId := util.FromHex(smesherIdStr)

	// todo: request offset and total from user
	rewards, total, err := r.client.SmesherRewards(smesherId, 0, 0)
	if err != nil {
		log.Error("failed to get rewards: %v", err)
		return
	}

	fmt.Println(printPrefix, fmt.Sprintf("Total rewards: %d", total))
	for i, r := range rewards {
		if i != 0 {
			fmt.Println("-----")
		}
		printReward(r)
	}
}

// printSmesherRewards prints all rewards awarded to a smesher identified by an id
func (r *repl) printCurrentSmesherRewards() {
	if smesherId, err := r.client.GetSmesherId(); err != nil {
		log.Error("failed to get smesher id: %v", err)
	} else {

		fmt.Println(printPrefix, "Smesher id:", "0x"+hex.EncodeToString(smesherId))

		// todo: request offset and total from user
		rewards, total, err := r.client.SmesherRewards(smesherId, 0, 10000)
		if err != nil {
			log.Error("failed to get rewards: %v", err)
			return
		}

		fmt.Println(printPrefix, fmt.Sprintf("Total rewards: %d", total))
		for i, r := range rewards {
			if i != 0 {
				fmt.Println("-----")
			}
			printReward(r)
		}
	}
}
