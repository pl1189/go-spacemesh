package mesh

import (
	"context"

	"github.com/spacemeshos/go-spacemesh/common/types"
)

//go:generate mockgen -typed -package=mocks -destination=./mocks/mocks.go -source=./interface.go

type conservativeState interface {
	UpdateCache(context.Context, types.LayerID, types.BlockID, []types.TransactionWithResult, []types.Transaction) error
	RevertCache(types.LayerID) error
	LinkTXsWithProposal(types.LayerID, types.ProposalID, []types.TransactionID) error
	LinkTXsWithBlock(types.LayerID, types.BlockID, []types.TransactionID) error
}

type vmState interface {
	GetStateRoot() (types.Hash32, error)
	Revert(types.LayerID) error
	Apply(
		types.LayerID,
		[]types.Transaction,
		[]types.CoinbaseReward,
	) ([]types.Transaction, []types.TransactionWithResult, error)
}

type layerClock interface {
	CurrentLayer() types.LayerID
}
