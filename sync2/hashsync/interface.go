package hashsync

import (
	"context"
	"io"

	"github.com/spacemeshos/go-spacemesh/common/types"
	"github.com/spacemeshos/go-spacemesh/p2p"
	"github.com/spacemeshos/go-spacemesh/p2p/server"
)

//go:generate mockgen -typed -package=hashsync -destination=./mocks_test.go -source=./interface.go

// Iterator points to in item in ItemStore
type Iterator interface {
	// Equal returns true if this iterator is equal to another Iterator
	Equal(other Iterator) bool
	// Key returns the key corresponding to iterator position. It returns
	// nil if the ItemStore is empty
	Key() Ordered
	// Next advances the iterator
	Next()
}

type RangeInfo struct {
	Fingerprint any
	Count       int
	Start, End  Iterator
}

// ItemStore represents the data store that can be synced against a remote peer
type ItemStore interface {
	// Add adds a key to the store
	Add(ctx context.Context, k Ordered) error
	// GetRangeInfo returns RangeInfo for the item range in the tree.
	// If count >= 0, at most count items are returned, and RangeInfo
	// is returned for the corresponding subrange of the requested range.
	// If both x and y is nil, the whole set of items is used.
	// If only x or only y is nil, GetRangeInfo panics
	GetRangeInfo(preceding Iterator, x, y Ordered, count int) RangeInfo
	// Min returns the iterator pointing at the minimum element
	// in the store. If the store is empty, it returns nil
	Min() Iterator
	// Max returns the iterator pointing at the maximum element
	// in the store. If the store is empty, it returns nil
	Max() Iterator
	// Copy makes a shallow copy of the ItemStore
	Copy() ItemStore
	// Has returns true if the specified key is present in ItemStore
	Has(k Ordered) bool
}

type Requester interface {
	Run(context.Context) error
	StreamRequest(context.Context, p2p.Peer, []byte, server.StreamRequestCallback, ...string) error
}

type SyncBase interface {
	Count() int
	Derive(p p2p.Peer) Syncer
	Probe(ctx context.Context, p p2p.Peer) (ProbeResult, error)
	Wait() error
}

type Syncer interface {
	Peer() p2p.Peer
	Sync(ctx context.Context, x, y *types.Hash32) error
	Serve(ctx context.Context, req []byte, stream io.ReadWriter) error
}

type PairwiseSyncer interface {
	Probe(ctx context.Context, peer p2p.Peer, is ItemStore, x, y *types.Hash32) (ProbeResult, error)
	SyncStore(ctx context.Context, peer p2p.Peer, is ItemStore, x, y *types.Hash32) error
	Serve(ctx context.Context, req []byte, stream io.ReadWriter, is ItemStore) error
}

type syncRunner interface {
	splitSync(ctx context.Context, syncPeers []p2p.Peer) error
	fullSync(ctx context.Context, syncPeers []p2p.Peer) error
}
