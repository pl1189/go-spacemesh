package txs

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/exp/maps"

	"github.com/spacemeshos/go-spacemesh/common/types"
	"github.com/spacemeshos/go-spacemesh/events"
	"github.com/spacemeshos/go-spacemesh/log"
	"github.com/spacemeshos/go-spacemesh/sql"
	"github.com/spacemeshos/go-spacemesh/sql/layers"
	"github.com/spacemeshos/go-spacemesh/sql/transactions"
)

const (
	maxTXsPerAcct  = 100
	maxTXsPerNonce = 100
)

var (
	errBadNonce            = errors.New("bad nonce")
	errInsufficientBalance = errors.New("insufficient balance")
	errTooManyNonce        = errors.New("account has too many nonce pending")
	errLayerNotInOrder     = errors.New("layers not applied in order")
)

// a candidate for the mempool.
type candidate struct {
	// this is the best tx among all the txs with the same nonce
	best        *NanoTX
	postBalance uint64
}

func (s *candidate) id() types.TransactionID {
	return s.best.ID
}

func (s *candidate) layer() types.LayerID {
	return s.best.Layer
}

func (s *candidate) block() types.BlockID {
	return s.best.Block
}

func (s *candidate) nonce() uint64 {
	return s.best.Nonce
}

func (s *candidate) maxSpending() uint64 {
	return s.best.MaxSpending()
}

type accountCache struct {
	addr         types.Address
	txsByNonce   *list.List
	startNonce   uint64
	startBalance uint64
	// moreInDB is used to indicate that an account has pending txs in db that need to be
	// reconsidered for mempool after a layer is applied.
	// - there are too many nonces for an account in the mempool. the extra (higher nonce) txs are in db only.
	// - txs deemed insufficient balance by the conservative state, but can be feasible after a layer applied
	//   (that may contain incoming funds for that account)
	// - a better tx arrived (higher fee) and made higher nonce txs infeasible due to insufficient balance
	//   deemed by conservative state.
	// TODO: evict accounts that only has DB-only txs
	// https://github.com/spacemeshos/go-spacemesh/issues/3668
	moreInDB bool

	cachedTXs map[types.TransactionID]*NanoTX // shared with the cache instance
}

func (ac *accountCache) nextNonce() uint64 {
	if ac.txsByNonce.Len() == 0 {
		return ac.startNonce
	}
	return ac.txsByNonce.Back().Value.(*candidate).nonce() + 1
}

func (ac *accountCache) availBalance() uint64 {
	if ac.txsByNonce.Len() == 0 {
		return ac.startBalance
	}
	return ac.txsByNonce.Back().Value.(*candidate).postBalance
}

func (ac *accountCache) precheck(logger *zap.Logger, ntx *NanoTX) (*list.Element, *candidate, error) {
	if ac.txsByNonce.Len() >= maxTXsPerAcct {
		ac.moreInDB = true
		return nil, nil, fmt.Errorf("%w: len %d", errTooManyNonce, ac.txsByNonce.Len())
	}
	balance := ac.startBalance
	var prev *list.Element
	for e := ac.txsByNonce.Back(); e != nil; e = e.Prev() {
		cand := e.Value.(*candidate)
		if cand.nonce() > ntx.Nonce {
			continue
		}
		if cand.nonce() == ntx.Nonce {
			balance = cand.postBalance + cand.maxSpending()
		} else {
			balance = cand.postBalance
		}
		prev = e
		break
	}
	if balance < ntx.MaxSpending() {
		return nil, nil, errInsufficientBalance
	}
	return prev, &candidate{best: ntx, postBalance: balance - ntx.MaxSpending()}, nil
}

func (ac *accountCache) accept(logger *zap.Logger, ntx *NanoTX, blockSeed []byte) error {
	var (
		added, prev *list.Element
		cand        *candidate
		replaced    *NanoTX
		err         error
	)
	prev, cand, err = ac.precheck(logger, ntx)
	if err != nil {
		return err
	}

	if prev == nil { // insert at the first position
		added = ac.txsByNonce.PushFront(cand)
	} else if prevCand := prev.Value.(*candidate); prevCand.nonce() < ntx.Nonce {
		added = ac.txsByNonce.InsertAfter(cand, prev)
	} else { // existing nonce
		if !ntx.Better(prevCand.best, blockSeed) {
			return nil
		}
		added = prev
		replaced = prevCand.best
		delete(ac.cachedTXs, prevCand.best.ID)
		prevCand.best = ntx
		prevCand.postBalance = cand.postBalance
	}
	ac.cachedTXs[ntx.ID] = ntx

	if replaced != nil {
		logger.Debug("better transaction replaced for nonce",
			zap.Stringer("address", ac.addr),
			zap.Stringer("better", ntx.ID),
			zap.Stringer("replaced", replaced.ID),
			zap.Uint64("nonce", ntx.Nonce),
			zap.Uint64("max_spending", ntx.MaxSpending()),
			zap.Uint64("post_balance", cand.postBalance),
			zap.Uint64("avail_balance", ac.availBalance()),
		)
	} else {
		logger.Debug("new nonce added",
			zap.Stringer("address", ac.addr),
			zap.Stringer("tx_id", ntx.ID),
			zap.Uint64("nonce", ntx.Nonce),
			zap.Uint64("max_spending", ntx.MaxSpending()),
			zap.Uint64("post_balance", cand.postBalance),
			zap.Uint64("avail_balance", ac.availBalance()),
		)
	}

	// propagate the balance change
	next := added.Next()
	newBalance := cand.postBalance
	for next != nil {
		nextCand := next.Value.(*candidate)
		if newBalance >= nextCand.maxSpending() {
			newBalance -= nextCand.maxSpending()
			nextCand.postBalance = newBalance
			next = next.Next()
			logger.Debug("updated next balance",
				zap.Stringer("address", ac.addr),
				zap.Uint64("nonce", nextCand.nonce()),
				zap.Uint64("post_balance", nextCand.postBalance),
				zap.Uint64("avail_balance", ac.availBalance()),
			)
			continue
		}
		ac.moreInDB = true
		rm := next
		next = next.Next()
		removed := ac.txsByNonce.Remove(rm).(*candidate)
		delete(ac.cachedTXs, removed.id())
		logger.Debug("tx made infeasible by new/better transaction",
			zap.Stringer("address", ac.addr),
			zap.Stringer("tx_id", removed.id()),
			zap.Uint64("nonce", removed.nonce()),
			zap.Uint64("max_spending", ntx.MaxSpending()),
		)
	}
	return nil
}

func (ac *accountCache) addBatch(logger *zap.Logger, nonce2TXs map[uint64][]*NanoTX, blockSeed []byte) error {
	logger.Debug("account has pending txs",
		zap.Stringer("address", ac.addr),
		zap.Int("num_pending", len(nonce2TXs)),
	)
	var (
		nextNonce   = ac.nextNonce()
		balance     = ac.availBalance()
		sortedNonce = make([]uint64, 0, len(nonce2TXs))
		added       = make([]uint64, 0, len(nonce2TXs))
	)
	for nonce := range nonce2TXs {
		if nonce < nextNonce {
			continue
		}
		sortedNonce = append(sortedNonce, nonce)
	}
	sort.Slice(sortedNonce, func(i, j int) bool { return sortedNonce[i] < sortedNonce[j] })
	for _, nonce := range sortedNonce {
		best := findBest(nonce2TXs[nonce], balance, blockSeed)
		if best == nil {
			logger.Debug("no feasible transactions at nonce",
				zap.Stringer("address", ac.addr),
				zap.Uint64("nonce", nonce),
				zap.Uint64("balance", balance),
			)
			continue
		}

		logger.Debug("found best in nonce txs",
			zap.Stringer("address", ac.addr),
			zap.Stringer("tx_id", best.ID),
			zap.Uint64("nonce", nonce),
			zap.Uint64("fee", best.Fee()),
		)

		if err := ac.accept(logger, best, blockSeed); err != nil {
			if errors.Is(err, errTooManyNonce) {
				break
			}
			continue
		}
		added = append(added, nonce)
		balance = ac.availBalance()
	}

	ac.moreInDB = len(sortedNonce) > len(added)
	return nil
}

func findBest(ntxs []*NanoTX, balance uint64, blockSeed []byte) *NanoTX {
	var best *NanoTX
	for _, ntx := range ntxs {
		if balance >= ntx.MaxSpending() &&
			(best == nil || ntx.Better(best, blockSeed)) {
			best = ntx
		}
	}
	return best
}

// adding a tx to the account cache. possible outcomes:
//   - nonce is smaller than the next nonce in state: reject from cache
//   - too many txs present: reject from cache
//   - nonce already exists in the cache:
//     if it is better than the best candidate in that nonce group, swap
//   - nonce not present: add to cache.
func (ac *accountCache) add(logger *zap.Logger, tx *types.Transaction, received time.Time) error {
	if tx.Nonce < ac.startNonce {
		return errBadNonce
	}

	ntx := NewNanoTX(&types.MeshTransaction{
		Transaction: *tx,
		Received:    received,
		LayerID:     0,
		BlockID:     types.EmptyBlockID,
	})

	err := ac.accept(logger, ntx, nil)
	if err != nil {
		if errors.Is(err, errTooManyNonce) {
			mempoolTxCount.WithLabelValues(tooManyNonce).Inc()
		} else if errors.Is(err, errInsufficientBalance) {
			mempoolTxCount.WithLabelValues(balanceTooSmall).Inc()
		}
		return err
	}
	mempoolTxCount.WithLabelValues(mempool).Inc()
	return nil
}

func (ac *accountCache) addPendingFromNonce(
	logger *zap.Logger,
	db sql.StateDatabase,
	nonce uint64,
	applied types.LayerID,
) error {
	mtxs, err := transactions.GetAcctPendingFromNonce(db, ac.addr, nonce)
	if err != nil {
		return fmt.Errorf("account pending txs: %w", err)
	}

	if len(mtxs) == 0 {
		ac.moreInDB = false
		return nil
	}

	if applied != 0 {
		for _, mtx := range mtxs {
			if mtx.State == types.APPLIED {
				continue
			}
			nextLayer, nextBlock, err := getNextIncluded(db, mtx.ID, applied)
			if err != nil {
				return fmt.Errorf("get next included: %w", err)
			}
			mtx.LayerID = nextLayer
			mtx.BlockID = nextBlock
			if nextLayer != 0 {
				logger.Debug("next layer found",
					zap.Stringer("address", ac.addr),
					zap.Stringer("tx_id", mtx.ID),
					zap.Uint32("layer_id", nextLayer.Uint32()),
				)
			}
		}
	}

	byPrincipal := groupTXsByPrincipal(logger, mtxs)
	if _, ok := byPrincipal[ac.addr]; !ok {
		logger.Panic("no txs for account after grouping", zap.Stringer("address", ac.addr))
	}
	return ac.addBatch(logger, byPrincipal[ac.addr], nil)
}

// find the first nonce without a layer.
// a nonce with a valid layer indicates that it's already packed in a proposal/block.
func (ac *accountCache) getMempool(logger *zap.Logger) []*NanoTX {
	bests := make([]*NanoTX, 0, maxTXsPerAcct)
	offset := 0
	found := false
	for e := ac.txsByNonce.Front(); e != nil; e = e.Next() {
		cand := e.Value.(*candidate)
		if !found && cand.layer() == 0 {
			found = true
		} else if found && cand.layer() != 0 {
			logger.Debug("some proposals/blocks packed txs out of order",
				zap.Stringer("tx_id", cand.id()),
				zap.Uint32("layer_id", cand.layer().Uint32()),
				zap.Stringer("block_id", cand.block()),
				zap.Uint64("nonce", cand.nonce()),
			)
		}
		if found {
			bests = append(bests, cand.best)
		} else {
			offset++
		}
	}
	if len(bests) > 0 {
		logger.Debug("account in mempool",
			zap.Int("offset", offset),
			zap.Int("size", ac.txsByNonce.Len()),
			zap.Int("added", len(bests)),
			zap.Uint64("from", bests[0].Nonce),
			zap.Uint64("to", bests[len(bests)-1].Nonce),
		)
	} else {
		logger.Debug("account has no txs for mempool",
			zap.Int("offset", offset),
			zap.Int("size", ac.txsByNonce.Len()),
		)
	}
	return bests
}

// NOTE: this is the only point in time when we reconsider those previously rejected txs,
// because applying a layer changes the conservative balance in the cache.
func (ac *accountCache) resetAfterApply(
	logger *zap.Logger,
	db sql.StateDatabase,
	nextNonce, newBalance uint64,
	applied types.LayerID,
) error {
	logger.Debug("resetting to nonce",
		zap.Stringer("address", ac.addr),
		zap.Uint64("nonce", nextNonce),
	)
	for e := ac.txsByNonce.Front(); e != nil; e = e.Next() {
		delete(ac.cachedTXs, e.Value.(*candidate).id())
	}
	ac.txsByNonce = list.New()
	ac.startNonce = nextNonce
	ac.startBalance = newBalance
	return ac.addPendingFromNonce(logger, db, ac.startNonce, applied)
}

func (ac *accountCache) shouldEvict() bool {
	return ac.txsByNonce.Len() == 0 && !ac.moreInDB
}

type stateFunc func(types.Address) (uint64, uint64)

type Cache struct {
	logger *zap.Logger
	stateF stateFunc

	mu        sync.Mutex
	pending   map[types.Address]*accountCache
	cachedTXs map[types.TransactionID]*NanoTX // shared with accountCache instances
}

func NewCache(s stateFunc, logger *zap.Logger) *Cache {
	return &Cache{
		logger:    logger,
		stateF:    s,
		pending:   make(map[types.Address]*accountCache),
		cachedTXs: make(map[types.TransactionID]*NanoTX),
	}
}

func groupTXsByPrincipal(logger *zap.Logger, mtxs []*types.MeshTransaction) map[types.Address]map[uint64][]*NanoTX {
	byPrincipal := make(map[types.Address]map[uint64][]*NanoTX)
	for _, mtx := range mtxs {
		principal := mtx.Principal
		if _, ok := byPrincipal[principal]; !ok {
			byPrincipal[principal] = make(map[uint64][]*NanoTX)
		}
		if _, ok := byPrincipal[principal][mtx.Nonce]; !ok {
			byPrincipal[principal][mtx.Nonce] = make([]*NanoTX, 0, maxTXsPerNonce)
		}
		if len(byPrincipal[principal][mtx.Nonce]) < maxTXsPerNonce {
			byPrincipal[principal][mtx.Nonce] = append(byPrincipal[principal][mtx.Nonce], NewNanoTX(mtx))
		} else {
			logger.Debug("too many txs in same nonce. ignoring tx",
				zap.Stringer("tx_id", mtx.ID),
				zap.Stringer("address", principal),
				zap.Uint64("nonce", mtx.Nonce),
				zap.Uint64("fee", mtx.Fee()),
			)
		}
	}
	return byPrincipal
}

// buildFromScratch builds the cache from database.
func (c *Cache) buildFromScratch(db sql.StateDatabase) error {
	applied, err := layers.GetLastApplied(db)
	if err != nil {
		return fmt.Errorf("cache: get pending %w", err)
	}
	addresses, err := transactions.AddressesWithPendingTransactions(db)
	if err != nil {
		return fmt.Errorf("pending transactions %w", err)
	}
	var rst []*types.MeshTransaction
	for _, addr := range addresses {
		txs, err := transactions.GetAcctPendingFromNonce(db, addr.Address, addr.Nonce)
		if err != nil {
			return fmt.Errorf("get pending addr=%s nonce=%d %w", addr.Address, addr.Nonce, err)
		}
		rst = append(rst, txs...)
	}
	for _, mtx := range rst {
		if mtx.State == types.APPLIED {
			continue
		}
		nextLayer, nextBlock, err := getNextIncluded(db, mtx.ID, applied)
		if err != nil {
			return fmt.Errorf("get next included: %w", err)
		}
		mtx.LayerID = nextLayer
		mtx.BlockID = nextBlock
	}
	return c.BuildFromTXs(rst, nil)
}

// BuildFromTXs builds the cache from the provided transactions.
func (c *Cache) BuildFromTXs(rst []*types.MeshTransaction, blockSeed []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.pending = make(map[types.Address]*accountCache)
	toCleanup := make(map[types.Address]struct{})
	for _, tx := range rst {
		toCleanup[tx.Principal] = struct{}{}
	}
	defer c.cleanupAccounts(maps.Keys(toCleanup)...)

	byPrincipal := groupTXsByPrincipal(c.logger, rst)
	acctsAdded := 0
	for principal, nonce2TXs := range byPrincipal {
		c.createAcctIfNotPresent(principal)
		if err := c.pending[principal].addBatch(c.logger, nonce2TXs, blockSeed); err != nil {
			return err
		}
		if c.pending[principal].shouldEvict() {
			c.logger.Debug("account has pending txs but none feasible",
				zap.Stringer("address", principal),
				zap.Array("batch", zapcore.ArrayMarshalerFunc(func(encoder zapcore.ArrayEncoder) error {
					nonces := maps.Keys(nonce2TXs)
					slices.Sort(nonces)
					for _, nonce := range nonces {
						encoder.AppendUint64(nonce)
					}
					return nil
				})),
			)
		} else {
			acctsAdded++
		}
	}
	c.logger.Sugar().Debug("added pending tx for %d accounts", acctsAdded)
	return nil
}

func (c *Cache) createAcctIfNotPresent(addr types.Address) {
	if _, ok := c.pending[addr]; !ok {
		nextNonce, balance := c.stateF(addr)
		c.logger.Debug("created account with nonce/balance",
			zap.Stringer("address", addr),
			zap.Uint64("nonce", nextNonce),
			zap.Uint64("balance", balance),
		)
		c.pending[addr] = &accountCache{
			addr:         addr,
			startNonce:   nextNonce,
			startBalance: balance,
			txsByNonce:   list.New(),
			cachedTXs:    c.cachedTXs,
		}
	}
}

func (c *Cache) MoreInDB(addr types.Address) bool {
	acct, ok := c.pending[addr]
	if !ok {
		return false
	}
	return acct.moreInDB
}

func (c *Cache) cleanupAccounts(accounts ...types.Address) {
	for _, addr := range accounts {
		if _, ok := c.pending[addr]; ok && c.pending[addr].shouldEvict() {
			delete(c.pending, addr)
		}
	}
}

//   - errTooManyNonce: when a principal has way too many nonces, we don't want to blow up the memory. they should
//     be stored in db and retrieved after each earlier nonce is applied.
func acceptable(err error) bool {
	return err == nil || errors.Is(err, errTooManyNonce)
}

func (c *Cache) Add(ctx context.Context, db sql.StateDatabase, tx *types.Transaction, received time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	principal := tx.Principal
	c.createAcctIfNotPresent(principal)
	defer c.cleanupAccounts(principal)
	logger := c.logger.With(log.ZContext(ctx), zap.Stringer("address", principal))
	if err := c.pending[principal].add(logger, tx, received); !acceptable(err) {
		return err
	}
	mempoolTxCount.WithLabelValues(accepted).Inc()
	return transactions.Add(db, tx, received)
}

// Get gets a transaction from the cache.
func (c *Cache) Get(tid types.TransactionID) *NanoTX {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cachedTXs[tid]
}

// Has returns true if transaction exists in the cache.
func (c *Cache) Has(tid types.TransactionID) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.has(tid)
}

func (c *Cache) has(tid types.TransactionID) bool {
	return c.cachedTXs[tid] != nil
}

// LinkTXsWithProposal associates the transactions to a proposal.
func (c *Cache) LinkTXsWithProposal(
	db sql.StateDatabase,
	lid types.LayerID,
	pid types.ProposalID,
	tids []types.TransactionID,
) error {
	if len(tids) == 0 {
		return nil
	}
	if err := addToProposal(db, lid, pid, tids); err != nil {
		return fmt.Errorf("linking txs to proposal: %w", err)
	}
	c.updateLayer(lid, types.EmptyBlockID, tids)
	return nil
}

// LinkTXsWithBlock associates the transactions to a block.
func (c *Cache) LinkTXsWithBlock(
	db sql.StateDatabase,
	lid types.LayerID,
	bid types.BlockID,
	tids []types.TransactionID,
) error {
	if len(tids) == 0 {
		return nil
	}
	if err := addToBlock(db, lid, bid, tids); err != nil {
		return fmt.Errorf("add to block: %w", err)
	}
	c.updateLayer(lid, bid, tids)
	return nil
}

// updateLayer associates the transactions to a layer and optionally a block.
// A transaction is tagged with a layer when it's included in a proposal/block.
// If a transaction is included in multiple proposals/blocks in different layers,
// the lowest layer is retained.
func (c *Cache) updateLayer(lid types.LayerID, bid types.BlockID, tids []types.TransactionID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, ID := range tids {
		if _, ok := c.cachedTXs[ID]; !ok {
			// transaction is not considered best in its nonce group
			return
		}
		c.cachedTXs[ID].UpdateLayerMaybe(lid, bid)
	}
}

func (c *Cache) applyEmptyLayer(db sql.StateDatabase, lid types.LayerID) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for tid, ntx := range c.cachedTXs {
		if ntx.Layer == lid {
			nbid, nlid, err := getNextIncluded(db, tid, lid)
			if err != nil {
				return err
			}
			ntx.UpdateLayer(nbid, nlid)
		}
	}
	return nil
}

// ApplyLayer retires the applied transactions from the cache and updates the balances.
func (c *Cache) ApplyLayer(
	ctx context.Context,
	db sql.StateDatabase,
	lid types.LayerID,
	bid types.BlockID,
	results []types.TransactionWithResult,
	ineffective []types.Transaction,
) error {
	logger := c.logger.With(
		log.ZContext(ctx),
		zap.Uint32("layer_id", lid.Uint32()),
		zap.Stringer("block_id", bid),
	)
	if err := checkApplyOrder(logger, db, lid); err != nil {
		return err
	}

	if bid == types.EmptyBlockID {
		return c.applyEmptyLayer(db, lid)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	toCleanup := make(map[types.Address]struct{})
	toReset := make(map[types.Address]struct{})
	byPrincipal := make(map[types.Address]struct{})

	// commit results before reporting them
	// TODO(dshulyak) save results in vm
	if err := db.WithTx(context.Background(), func(dbtx sql.Transaction) error {
		for _, rst := range results {
			err := transactions.AddResult(dbtx, rst.ID, &rst.TransactionResult)
			if err != nil {
				return fmt.Errorf("add result tx=%s nonce=%d %w", rst.ID, rst.Nonce, err)
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("add results %w", err)
	}

	for _, rst := range results {
		byPrincipal[rst.Principal] = struct{}{}
		toCleanup[rst.Principal] = struct{}{}
		if !c.has(rst.ID) {
			RawTxCount.WithLabelValues(updated).Inc()
			if err := transactions.Add(db, &rst.Transaction, time.Now()); err != nil {
				return err
			}
		}
		if err := events.ReportResult(rst); err != nil {
			c.logger.Error("Failed to emit tx results", zap.Stringer("tx_id", rst.ID), zap.Error(err))
		}
	}

	for _, tx := range ineffective {
		if tx.TxHeader == nil {
			logger.Warn("tx header not parsed", zap.Stringer("tx_id", tx.ID))
			continue
		}
		if !c.has(tx.ID) {
			RawTxCount.WithLabelValues(updated).Inc()
			if err := transactions.Add(db, &tx, time.Now()); err != nil {
				return err
			}
		}

		toCleanup[tx.Principal] = struct{}{}
		if _, ok := byPrincipal[tx.Principal]; ok {
			continue
		}
		if _, ok := c.pending[tx.Principal]; !ok {
			continue
		}
		toReset[tx.Principal] = struct{}{}
	}
	defer c.cleanupAccounts(maps.Keys(toCleanup)...)

	for principal := range byPrincipal {
		c.createAcctIfNotPresent(principal)
		nextNonce, balance := c.stateF(principal)
		logger.Debug("new account nonce/balance",
			zap.Stringer("address", principal),
			zap.Uint64("nonce", nextNonce),
			zap.Uint64("balance", balance),
		)
		t0 := time.Now()
		if err := c.pending[principal].resetAfterApply(logger, db, nextNonce, balance, lid); err != nil {
			logger.Error("failed to reset cache for principal",
				zap.Stringer("address", principal),
				zap.Error(err),
			)
			return err
		}
		acctResetDuration.Observe(float64(time.Since(t0)))
	}

	for principal, accCache := range c.pending {
		if _, ok := toCleanup[principal]; ok {
			continue
		}
		if accCache.moreInDB {
			toReset[principal] = struct{}{}
		}
	}
	for principal := range toReset {
		nextNonce, balance := c.stateF(principal)
		t2 := time.Now()
		if err := c.pending[principal].resetAfterApply(logger, db, nextNonce, balance, lid); err != nil {
			logger.Error("failed to reset cache for principal",
				zap.Stringer("address", principal),
				zap.Error(err),
			)
			return err
		}
		acctResetDuration.Observe(float64(time.Since(t2)))
	}
	return nil
}

func (c *Cache) RevertToLayer(db sql.StateDatabase, revertTo types.LayerID) error {
	if err := undoLayers(db, revertTo.Add(1)); err != nil {
		return err
	}

	if err := c.buildFromScratch(db); err != nil {
		return fmt.Errorf("building from scratch after revert: %w", err)
	}
	return nil
}

// GetProjection returns the projected nonce and balance for an account, including
// pending transactions that are paced in proposals/blocks but not yet applied to the state.
func (c *Cache) GetProjection(addr types.Address) (uint64, uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.pending[addr]; !ok {
		return c.stateF(addr)
	}
	return c.pending[addr].nextNonce(), c.pending[addr].availBalance()
}

// GetMempool returns all the transactions that eligible for a proposal/block.
func (c *Cache) GetMempool() map[types.Address][]*NanoTX {
	c.mu.Lock()
	defer c.mu.Unlock()

	all := make(map[types.Address][]*NanoTX)
	c.logger.Debug("cache has pending accounts", zap.Int("num_acct", len(c.pending)))
	for addr, accCache := range c.pending {
		txs := accCache.getMempool(c.logger.With(zap.Stringer("address", addr)))
		if len(txs) > 0 {
			all[addr] = txs
		}
	}
	return all
}

// checkApplyOrder returns an error if layers were not applied in order.
func checkApplyOrder(logger *zap.Logger, db sql.StateDatabase, toApply types.LayerID) error {
	lastApplied, err := layers.GetLastApplied(db)
	if err != nil {
		return fmt.Errorf("cache get last applied %w", err)
	}
	if toApply != lastApplied.Add(1) {
		logger.Error("layer not applied in order",
			zap.Stringer("expected", lastApplied.Add(1)),
			zap.Stringer("to_apply", toApply),
		)
		return errLayerNotInOrder
	}
	return nil
}

func addToProposal(db sql.StateDatabase, lid types.LayerID, pid types.ProposalID, tids []types.TransactionID) error {
	return db.WithTx(context.Background(), func(dbtx sql.Transaction) error {
		for _, tid := range tids {
			if err := transactions.AddToProposal(dbtx, tid, lid, pid); err != nil {
				return fmt.Errorf("add2prop %w", err)
			}
		}
		return nil
	})
}

func addToBlock(db sql.StateDatabase, lid types.LayerID, bid types.BlockID, tids []types.TransactionID) error {
	return db.WithTx(context.Background(), func(dbtx sql.Transaction) error {
		for _, tid := range tids {
			if err := transactions.AddToBlock(dbtx, tid, lid, bid); err != nil {
				return fmt.Errorf("add2block %w", err)
			}
		}
		return nil
	})
}

func undoLayers(db sql.StateDatabase, from types.LayerID) error {
	return db.WithTx(context.Background(), func(dbtx sql.Transaction) error {
		err := transactions.UndoLayers(dbtx, from)
		if err != nil {
			return fmt.Errorf("undo %w", err)
		}
		return nil
	})
}

func getNextIncluded(
	db sql.Executor,
	id types.TransactionID,
	after types.LayerID,
) (types.LayerID, types.BlockID, error) {
	bid, lid, err := transactions.TransactionInBlock(db, id, after)
	if err != nil && errors.Is(err, sql.ErrNotFound) {
		lid, err = transactions.TransactionInProposal(db, id, after)
		if err != nil && !errors.Is(err, sql.ErrNotFound) {
			return lid, bid, fmt.Errorf("get tx in next proposals %w", err)
		}
	} else if err != nil {
		return lid, bid, fmt.Errorf("get tx in next blocks %w", err)
	}
	return lid, bid, nil
}
