package hashsync

import (
	"context"
	"fmt"
	"math/rand"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/maps"
)

type rangeMessage struct {
	mtype MessageType
	x, y  Ordered
	fp    any
	count int
	keys  []Ordered
}

var _ SyncMessage = rangeMessage{}

func (m rangeMessage) Type() MessageType { return m.mtype }
func (m rangeMessage) X() Ordered        { return m.x }
func (m rangeMessage) Y() Ordered        { return m.y }
func (m rangeMessage) Fingerprint() any  { return m.fp }
func (m rangeMessage) Count() int        { return m.count }
func (m rangeMessage) Keys() []Ordered   { return m.keys }

func (m rangeMessage) String() string {
	return SyncMessageToString(m)
}

type fakeConduit struct {
	t    *testing.T
	msgs []rangeMessage
	resp []rangeMessage
}

var _ Conduit = &fakeConduit{}

func (fc *fakeConduit) gotoResponse() {
	fc.msgs = fc.resp
	fc.resp = nil
}

func (fc *fakeConduit) numItems() int {
	n := 0
	for _, m := range fc.msgs {
		n += len(m.Keys())
	}
	return n
}

func (fc *fakeConduit) NextMessage() (SyncMessage, error) {
	if len(fc.msgs) != 0 {
		m := fc.msgs[0]
		fc.msgs = fc.msgs[1:]
		return m, nil
	}

	return nil, nil
}

func (fc *fakeConduit) sendMsg(msg rangeMessage) {
	fc.resp = append(fc.resp, msg)
}

func (fc *fakeConduit) SendFingerprint(x, y Ordered, fingerprint any, count int) error {
	require.NotNil(fc.t, x)
	require.NotNil(fc.t, y)
	require.NotZero(fc.t, count)
	require.NotNil(fc.t, fingerprint)
	fc.sendMsg(rangeMessage{
		mtype: MessageTypeFingerprint,
		x:     x,
		y:     y,
		fp:    fingerprint,
		count: count,
	})
	return nil
}

func (fc *fakeConduit) SendEmptySet() error {
	fc.sendMsg(rangeMessage{mtype: MessageTypeEmptySet})
	return nil
}

func (fc *fakeConduit) SendEmptyRange(x, y Ordered) error {
	require.NotNil(fc.t, x)
	require.NotNil(fc.t, y)
	fc.sendMsg(rangeMessage{
		mtype: MessageTypeEmptyRange,
		x:     x,
		y:     y,
	})
	return nil
}

func (fc *fakeConduit) SendRangeContents(x, y Ordered, count int) error {
	require.NotNil(fc.t, x)
	require.NotNil(fc.t, y)
	fc.sendMsg(rangeMessage{
		mtype: MessageTypeRangeContents,
		x:     x,
		y:     y,
		count: count,
	})
	return nil
}

func (fc *fakeConduit) SendItems(count, itemChunkSize int, it Iterator) error {
	require.Positive(fc.t, count)
	require.NotZero(fc.t, count)
	require.NotNil(fc.t, it)
	for i := 0; i < count; i += itemChunkSize {
		msg := rangeMessage{mtype: MessageTypeItemBatch}
		n := min(itemChunkSize, count-i)
		for n > 0 {
			if it.Key() == nil {
				panic("fakeConduit.SendItems: went got to the end of the tree")
			}
			msg.keys = append(msg.keys, it.Key())
			it.Next()
			n--
		}
		fc.sendMsg(msg)
	}
	return nil
}

func (fc *fakeConduit) SendEndRound() error {
	fc.sendMsg(rangeMessage{mtype: MessageTypeEndRound})
	return nil
}

func (fc *fakeConduit) SendDone() error {
	fc.sendMsg(rangeMessage{mtype: MessageTypeDone})
	return nil
}

func (fc *fakeConduit) SendProbe(x, y Ordered, fingerprint any, sampleSize int) error {
	fc.sendMsg(rangeMessage{
		mtype: MessageTypeProbe,
		x:     x,
		y:     y,
		fp:    fingerprint,
		count: sampleSize,
	})
	return nil
}

func (fc *fakeConduit) SendProbeResponse(x, y Ordered, fingerprint any, count, sampleSize int, it Iterator) error {
	msg := rangeMessage{
		mtype: MessageTypeProbeResponse,
		x:     x,
		y:     y,
		fp:    fingerprint,
		count: count,
		keys:  make([]Ordered, sampleSize),
	}
	for n := 0; n < sampleSize; n++ {
		require.NotNil(fc.t, it.Key())
		msg.keys[n] = it.Key()
		it.Next()
	}
	fc.sendMsg(msg)
	return nil
}

func (fc *fakeConduit) ShortenKey(k Ordered) Ordered {
	return k
}

type dumbStoreIterator struct {
	ds *dumbStore
	n  int
}

var _ Iterator = &dumbStoreIterator{}

func (it *dumbStoreIterator) Equal(other Iterator) bool {
	o := other.(*dumbStoreIterator)
	if it.ds != o.ds {
		panic("comparing iterators from different dumbStores")
	}
	return it.n == o.n
}

func (it *dumbStoreIterator) Key() Ordered {
	return it.ds.keys[it.n]
}

func (it *dumbStoreIterator) Next() {
	if len(it.ds.keys) != 0 {
		it.n = (it.n + 1) % len(it.ds.keys)
	}
}

type dumbStore struct {
	keys []sampleID
}

var _ ItemStore = &dumbStore{}

func (ds *dumbStore) Add(ctx context.Context, k Ordered) error {
	id := k.(sampleID)
	if len(ds.keys) == 0 {
		ds.keys = []sampleID{id}
		return nil
	}
	p := slices.IndexFunc(ds.keys, func(other sampleID) bool {
		return other >= id
	})
	switch {
	case p < 0:
		ds.keys = append(ds.keys, id)
	case id == ds.keys[p]:
		// already present
	default:
		ds.keys = slices.Insert(ds.keys, p, id)
	}

	return nil
}

func (ds *dumbStore) iter(n int) Iterator {
	if n == -1 || n == len(ds.keys) {
		return nil
	}
	return &dumbStoreIterator{ds: ds, n: n}
}

func (ds *dumbStore) last() sampleID {
	if len(ds.keys) == 0 {
		panic("can't get the last element: zero items")
	}
	return ds.keys[len(ds.keys)-1]
}

func (ds *dumbStore) iterFor(s sampleID) Iterator {
	n := slices.Index(ds.keys, s)
	if n == -1 {
		panic("item not found: " + s)
	}
	return ds.iter(n)
}

func (ds *dumbStore) GetRangeInfo(preceding Iterator, x, y Ordered, count int) RangeInfo {
	if x == nil && y == nil {
		it := ds.Min()
		if it == nil {
			return RangeInfo{
				Fingerprint: "",
			}
		} else {
			x = it.Key()
			y = x
		}
	} else if x == nil || y == nil {
		panic("BUG: bad X or Y")
	}
	all := storeItemStr(ds)
	vx := x.(sampleID)
	vy := y.(sampleID)
	if preceding != nil && preceding.Key().Compare(x) > 0 {
		panic("preceding info after x")
	}
	fp, startStr, endStr := naiveRange(all, string(vx), string(vy), count)
	r := RangeInfo{
		Fingerprint: fp,
		Count:       len(fp),
	}
	if all != "" {
		if startStr == "" || endStr == "" {
			panic("empty startStr/endStr from naiveRange")
		}
		r.Start = ds.iterFor(sampleID(startStr))
		r.End = ds.iterFor(sampleID(endStr))
	}
	return r
}

func (ds *dumbStore) Min() Iterator {
	if len(ds.keys) == 0 {
		return nil
	}
	return &dumbStoreIterator{
		ds: ds,
		n:  0,
	}
}

func (ds *dumbStore) Max() Iterator {
	if len(ds.keys) == 0 {
		return nil
	}
	return &dumbStoreIterator{
		ds: ds,
		n:  len(ds.keys) - 1,
	}
}

func (ds *dumbStore) Copy() ItemStore {
	return &dumbStore{keys: slices.Clone(ds.keys)}
}

func (ds *dumbStore) Has(k Ordered) bool {
	for _, cur := range ds.keys {
		if k.Compare(cur) == 0 {
			return true
		}
	}
	return false
}

type verifiedStoreIterator struct {
	t         *testing.T
	knownGood Iterator
	it        Iterator
}

var _ Iterator = &verifiedStoreIterator{}

func (it verifiedStoreIterator) Equal(other Iterator) bool {
	o := other.(verifiedStoreIterator)
	eq1 := it.knownGood.Equal(o.knownGood)
	eq2 := it.it.Equal(o.it)
	assert.Equal(it.t, eq1, eq2, "iterators equal -- keys <%v> <%v> / <%v> <%v>",
		it.knownGood.Key(), it.it.Key(),
		o.knownGood.Key(), o.it.Key())
	assert.Equal(it.t, it.knownGood.Key(), it.it.Key(), "keys of equal iterators")
	return eq2
}

func (it verifiedStoreIterator) Key() Ordered {
	k1 := it.knownGood.Key()
	k2 := it.it.Key()
	assert.Equal(it.t, k1, k2, "keys")
	return k2
}

func (it verifiedStoreIterator) Next() {
	it.knownGood.Next()
	it.it.Next()
	assert.Equal(it.t, it.knownGood.Key(), it.it.Key(), "keys for Next()")
}

type verifiedStore struct {
	t            *testing.T
	knownGood    ItemStore
	store        ItemStore
	disableReAdd bool
	added        map[sampleID]struct{}
}

var _ ItemStore = &verifiedStore{}

func disableReAdd(s ItemStore) {
	if vs, ok := s.(*verifiedStore); ok {
		vs.disableReAdd = true
	}
}

func (vs *verifiedStore) Add(ctx context.Context, k Ordered) error {
	if vs.disableReAdd {
		_, found := vs.added[k.(sampleID)]
		require.False(vs.t, found, "hash sent twice: %v", k)
		if vs.added == nil {
			vs.added = make(map[sampleID]struct{})
		}
		vs.added[k.(sampleID)] = struct{}{}
	}
	if err := vs.knownGood.Add(ctx, k); err != nil {
		return fmt.Errorf("add to knownGood: %w", err)
	}
	if err := vs.store.Add(ctx, k); err != nil {
		return fmt.Errorf("add to store: %w", err)
	}
	return nil
}

func (vs *verifiedStore) GetRangeInfo(preceding Iterator, x, y Ordered, count int) RangeInfo {
	var ri1, ri2 RangeInfo
	if preceding != nil {
		p := preceding.(verifiedStoreIterator)
		ri1 = vs.knownGood.GetRangeInfo(p.knownGood, x, y, count)
		ri2 = vs.store.GetRangeInfo(p.it, x, y, count)
	} else {
		ri1 = vs.knownGood.GetRangeInfo(nil, x, y, count)
		ri2 = vs.store.GetRangeInfo(nil, x, y, count)
	}
	require.Equal(vs.t, ri1.Fingerprint, ri2.Fingerprint, "range info fingerprint")
	require.Equal(vs.t, ri1.Count, ri2.Count, "range info count")
	ri := RangeInfo{
		Fingerprint: ri2.Fingerprint,
		Count:       ri2.Count,
	}
	if ri1.Start == nil {
		require.Nil(vs.t, ri2.Start, "range info start")
		require.Nil(vs.t, ri1.End, "range info end (known good)")
		require.Nil(vs.t, ri2.End, "range info end")
	} else {
		require.NotNil(vs.t, ri2.Start, "range info start")
		require.Equal(vs.t, ri1.Start.Key(), ri2.Start.Key(), "range info start key")
		require.NotNil(vs.t, ri1.End, "range info end (known good)")
		require.NotNil(vs.t, ri2.End, "range info end")
		ri.Start = verifiedStoreIterator{
			t:         vs.t,
			knownGood: ri1.Start,
			it:        ri2.Start,
		}
	}
	if ri1.End == nil {
		require.Nil(vs.t, ri2.End, "range info end")
	} else {
		require.NotNil(vs.t, ri2.End, "range info end")
		require.Equal(vs.t, ri1.End.Key(), ri2.End.Key(), "range info end key")
		ri.End = verifiedStoreIterator{
			t:         vs.t,
			knownGood: ri1.End,
			it:        ri2.End,
		}
	}
	// QQQQQ: TODO: if count >= 0 and start+end != nil, do more calls to GetRangeInfo using resulting
	// end iterator key to make sure the range is correct
	return ri
}

func (vs *verifiedStore) Min() Iterator {
	m1 := vs.knownGood.Min()
	m2 := vs.store.Min()
	if m1 == nil {
		require.Nil(vs.t, m2, "Min")
		return nil
	} else {
		require.NotNil(vs.t, m2, "Min")
		require.Equal(vs.t, m1.Key(), m2.Key(), "Min key")
	}
	return verifiedStoreIterator{
		t:         vs.t,
		knownGood: m1,
		it:        m2,
	}
}

func (vs *verifiedStore) Max() Iterator {
	m1 := vs.knownGood.Max()
	m2 := vs.store.Max()
	if m1 == nil {
		require.Nil(vs.t, m2, "Max")
		return nil
	} else {
		require.NotNil(vs.t, m2, "Max")
		require.Equal(vs.t, m1.Key(), m2.Key(), "Max key")
	}
	return verifiedStoreIterator{
		t:         vs.t,
		knownGood: m1,
		it:        m2,
	}
}

func (vs *verifiedStore) Copy() ItemStore {
	return &verifiedStore{
		t:            vs.t,
		knownGood:    vs.knownGood.Copy(),
		store:        vs.store.Copy(),
		disableReAdd: vs.disableReAdd,
	}
}

func (vs *verifiedStore) Has(k Ordered) bool {
	h1 := vs.knownGood.Has(k)
	h2 := vs.store.Has(k)
	require.Equal(vs.t, h1, h2)
	return h2
}

type storeFactory func(t *testing.T) ItemStore

func makeDumbStore(t *testing.T) ItemStore {
	return &dumbStore{}
}

func makeSyncTreeStore(t *testing.T) ItemStore {
	return NewSyncTreeStore(sampleMonoid{})
}

func makeVerifiedSyncTreeStore(t *testing.T) ItemStore {
	return &verifiedStore{
		t:         t,
		knownGood: makeDumbStore(t),
		store:     makeSyncTreeStore(t),
	}
}

func makeStore(t *testing.T, f storeFactory, items string) ItemStore {
	s := f(t)
	for _, c := range items {
		require.NoError(t, s.Add(context.Background(), sampleID(c)))
	}
	return s
}

func storeItemStr(is ItemStore) string {
	it := is.Min()
	if it == nil {
		return ""
	}
	endAt := is.Min()
	r := ""
	for {
		r += string(it.Key().(sampleID))
		it.Next()
		if it.Equal(endAt) {
			return r
		}
	}
}

var testStores = []struct {
	name    string
	factory storeFactory
}{
	{
		name:    "dumb store",
		factory: makeDumbStore,
	},
	{
		name:    "monoid tree store",
		factory: makeSyncTreeStore,
	},
	{
		name:    "verified monoid tree store",
		factory: makeVerifiedSyncTreeStore,
	},
}

func forTestStores(t *testing.T, testFunc func(t *testing.T, factory storeFactory)) {
	for _, s := range testStores {
		t.Run(s.name, func(t *testing.T) {
			testFunc(t, s.factory)
		})
	}
}

// QQQQQ: rm
func dumpRangeMessages(t *testing.T, msgs []rangeMessage, fmt string, args ...any) {
	t.Logf(fmt, args...)
	for _, m := range msgs {
		t.Logf("  %s", m)
	}
}

func runSync(t *testing.T, syncA, syncB *RangeSetReconciler, maxRounds int) (nRounds, nMsg, nItems int) {
	fc := &fakeConduit{t: t}
	require.NoError(t, syncA.Initiate(fc))
	return doRunSync(fc, syncA, syncB, maxRounds)
}

func runBoundedSync(t *testing.T, syncA, syncB *RangeSetReconciler, x, y Ordered, maxRounds int) (nRounds, nMsg, nItems int) {
	fc := &fakeConduit{t: t}
	require.NoError(t, syncA.InitiateBounded(fc, x, y))
	return doRunSync(fc, syncA, syncB, maxRounds)
}

func doRunSync(fc *fakeConduit, syncA, syncB *RangeSetReconciler, maxRounds int) (nRounds, nMsg, nItems int) {
	var i int
	aDone, bDone := false, false
	// dumpRangeMessages(fc.t, fc.resp.msgs, "A %q -> B %q (init):", storeItemStr(syncA.is), storeItemStr(syncB.is))
	// dumpRangeMessages(fc.t, fc.resp.msgs, "A -> B (init):")
	for i = 0; ; i++ {
		if i == maxRounds {
			require.FailNow(fc.t, "too many rounds", "didn't reconcile in %d rounds", i)
		}
		fc.gotoResponse()
		nMsg += len(fc.msgs)
		nItems += fc.numItems()
		var err error
		bDone, err = syncB.Process(context.Background(), fc)
		require.NoError(fc.t, err)
		// a party should never send anything in response to the "done" message
		require.False(fc.t, aDone && !bDone, "A is done but B after that is not")
		// dumpRangeMessages(fc.t, fc.resp.msgs, "B %q -> A %q:", storeItemStr(syncA.is), storeItemStr(syncB.is))
		// dumpRangeMessages(fc.t, fc.resp.msgs, "B -> A:")
		if aDone && bDone {
			require.Empty(fc.t, fc.resp, "got messages from B in response to done msg from A")
			break
		}
		fc.gotoResponse()
		nMsg += len(fc.msgs)
		nItems += fc.numItems()
		aDone, err = syncA.Process(context.Background(), fc)
		require.NoError(fc.t, err)
		// dumpRangeMessages(fc.t, fc.msgs, "A %q --> B %q:", storeItemStr(syncB.is), storeItemStr(syncA.is))
		// dumpRangeMessages(fc.t, fc.resp.msgs, "A -> B:")
		require.False(fc.t, bDone && !aDone, "B is done but A after that is not")
		if aDone && bDone {
			require.Empty(fc.t, fc.resp, "got messages from A in response to done msg from B")
			break
		}
	}
	return i + 1, nMsg, nItems
}

func runProbe(t *testing.T, from, to *RangeSetReconciler) ProbeResult {
	fc := &fakeConduit{t: t}
	info, err := from.InitiateProbe(fc)
	require.NoError(t, err)
	return doRunProbe(fc, from, to, info)
}

func runBoundedProbe(t *testing.T, from, to *RangeSetReconciler, x, y Ordered) ProbeResult {
	fc := &fakeConduit{t: t}
	info, err := from.InitiateBoundedProbe(fc, x, y)
	require.NoError(t, err)
	return doRunProbe(fc, from, to, info)
}

func doRunProbe(fc *fakeConduit, from, to *RangeSetReconciler, info RangeInfo) ProbeResult {
	require.NotEmpty(fc.t, fc.resp, "empty initial round")
	fc.gotoResponse()
	done, err := to.Process(context.Background(), fc)
	require.True(fc.t, done)
	require.NoError(fc.t, err)
	fc.gotoResponse()
	pr, err := from.HandleProbeResponse(fc, info)
	require.NoError(fc.t, err)
	require.Nil(fc.t, fc.resp, "got messages from Probe in response to done msg")
	return pr
}

func TestRangeSync(t *testing.T) {
	forTestStores(t, func(t *testing.T, storeFactory storeFactory) {
		for _, tc := range []struct {
			name           string
			a, b           string
			finalA, finalB string
			x, y           string
			countA, countB int
			fpA, fpB       string
			maxRounds      [4]int
			sim            float64
		}{
			{
				name:      "empty sets",
				a:         "",
				b:         "",
				finalA:    "",
				finalB:    "",
				countA:    0,
				countB:    0,
				fpA:       "",
				fpB:       "",
				maxRounds: [4]int{1, 1, 1, 1},
				sim:       1,
			},
			{
				name:      "empty to non-empty",
				a:         "",
				b:         "abcd",
				finalA:    "abcd",
				finalB:    "abcd",
				countA:    0,
				countB:    4,
				fpA:       "",
				fpB:       "abcd",
				maxRounds: [4]int{2, 2, 2, 2},
				sim:       0,
			},
			{
				name:      "non-empty to empty",
				a:         "abcd",
				b:         "",
				finalA:    "abcd",
				finalB:    "abcd",
				countA:    4,
				countB:    0,
				fpA:       "abcd",
				fpB:       "",
				maxRounds: [4]int{2, 2, 2, 2},
				sim:       0,
			},
			{
				name:      "non-intersecting sets",
				a:         "ab",
				b:         "cd",
				finalA:    "abcd",
				finalB:    "abcd",
				countA:    2,
				countB:    2,
				fpA:       "ab",
				fpB:       "cd",
				maxRounds: [4]int{3, 2, 2, 2},
				sim:       0,
			},
			{
				name:      "intersecting sets",
				a:         "acdefghijklmn",
				b:         "bcdopqr",
				finalA:    "abcdefghijklmnopqr",
				finalB:    "abcdefghijklmnopqr",
				countA:    13,
				countB:    7,
				fpA:       "acdefghijklmn",
				fpB:       "bcdopqr",
				maxRounds: [4]int{4, 4, 3, 3},
				sim:       0.153,
			},
			{
				name:      "bounded reconciliation",
				a:         "acdefghijklmn",
				b:         "bcdopqr",
				finalA:    "abcdefghijklmn",
				finalB:    "abcdefgopqr",
				x:         "a",
				y:         "h",
				countA:    6,
				countB:    3,
				fpA:       "acdefg",
				fpB:       "bcd",
				maxRounds: [4]int{3, 3, 2, 2},
				sim:       0.333,
			},
			{
				name:      "bounded reconciliation with rollover",
				a:         "acdefghijklmn",
				b:         "bcdopqr",
				finalA:    "acdefghijklmnopqr",
				finalB:    "bcdhijklmnopqr",
				x:         "h",
				y:         "a",
				countA:    7,
				countB:    4,
				fpA:       "hijklmn",
				fpB:       "opqr",
				maxRounds: [4]int{4, 3, 3, 2},
				sim:       0,
			},
			{
				name:      "sync against 1-element set",
				a:         "bcd",
				b:         "a",
				finalA:    "abcd",
				finalB:    "abcd",
				countA:    3,
				countB:    1,
				fpA:       "bcd",
				fpB:       "a",
				maxRounds: [4]int{2, 2, 2, 2},
				sim:       0,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				for n, maxSendRange := range []int{1, 2, 3, 4} {
					t.Logf("maxSendRange: %d", maxSendRange)
					storeA := makeStore(t, storeFactory, tc.a)
					disableReAdd(storeA)
					syncA := NewRangeSetReconciler(storeA,
						WithMaxSendRange(maxSendRange),
						WithItemChunkSize(3))
					storeB := makeStore(t, storeFactory, tc.b)
					disableReAdd(storeB)
					syncB := NewRangeSetReconciler(storeB,
						WithMaxSendRange(maxSendRange),
						WithItemChunkSize(3))

					var (
						nRounds    int
						prBA, prAB ProbeResult
					)
					if tc.x == "" {
						prBA = runProbe(t, syncB, syncA)
						prAB = runProbe(t, syncA, syncB)
						nRounds, _, _ = runSync(t, syncA, syncB, tc.maxRounds[n])
					} else {
						x := sampleID(tc.x)
						y := sampleID(tc.y)
						prBA = runBoundedProbe(t, syncB, syncA, x, y)
						prAB = runBoundedProbe(t, syncA, syncB, x, y)
						nRounds, _, _ = runBoundedSync(t, syncA, syncB, x, y, tc.maxRounds[n])
					}
					t.Logf("%s: maxSendRange %d: %d rounds", tc.name, maxSendRange, nRounds)

					require.Equal(t, tc.countA, prBA.Count, "countA")
					require.Equal(t, tc.countB, prAB.Count, "countB")
					require.Equal(t, tc.fpA, prBA.FP, "fpA")
					require.Equal(t, tc.fpB, prAB.FP, "fpB")
					require.Equal(t, tc.finalA, storeItemStr(storeA), "finalA")
					require.Equal(t, tc.finalB, storeItemStr(storeB), "finalB")
					require.InDelta(t, tc.sim, prAB.Sim, 0.01, "prAB.Sim")
					require.InDelta(t, tc.sim, prBA.Sim, 0.01, "prBA.Sim")
				}
			})
		}
	})
}

func TestRandomSync(t *testing.T) {
	forTestStores(t, func(t *testing.T, storeFactory storeFactory) {
		var bytesA, bytesB []byte
		defer func() {
			if t.Failed() {
				t.Logf("Random sync failed: %q <-> %q", bytesA, bytesB)
			}
		}()
		for i := 0; i < 1000; i++ {
			var chars []byte
			for c := byte(33); c < 127; c++ {
				chars = append(chars, c)
			}

			bytesA = append([]byte(nil), chars...)
			rand.Shuffle(len(bytesA), func(i, j int) {
				bytesA[i], bytesA[j] = bytesA[j], bytesA[i]
			})
			bytesA = bytesA[:rand.Intn(len(bytesA))]
			storeA := makeStore(t, storeFactory, string(bytesA))

			bytesB = append([]byte(nil), chars...)
			rand.Shuffle(len(bytesB), func(i, j int) {
				bytesB[i], bytesB[j] = bytesB[j], bytesB[i]
			})
			bytesB = bytesB[:rand.Intn(len(bytesB))]
			storeB := makeStore(t, storeFactory, string(bytesB))

			keySet := make(map[byte]struct{})
			for _, c := range append(bytesA, bytesB...) {
				keySet[byte(c)] = struct{}{}
			}

			expectedSet := maps.Keys(keySet)
			slices.Sort(expectedSet)

			maxSendRange := rand.Intn(16) + 1
			syncA := NewRangeSetReconciler(storeA,
				WithMaxSendRange(maxSendRange),
				WithItemChunkSize(3))
			syncB := NewRangeSetReconciler(storeB,
				WithMaxSendRange(maxSendRange),
				WithItemChunkSize(3))

			runSync(t, syncA, syncB, max(len(expectedSet), 2)) // FIXME: less rounds!
			// t.Logf("maxSendRange %d a %d b %d n %d", maxSendRange, len(bytesA), len(bytesB), n)
			require.Equal(t, storeItemStr(storeA), storeItemStr(storeB))
			require.Equal(t, string(expectedSet), storeItemStr(storeA),
				"expected set for %q<->%q", bytesA, bytesB)
		}
	})
}

// TBD: make sure that requests with MessageTypeDone are never
//      answered!!!
// TBD: use logger for verbose logging (messages)
// TBD: in fakeConduit -- check item count against the iterator in
//      SendItems / SendItemsOnly!!
// TBD: record interaction using golden master in testRangeSync, for
//      both probe and sync, together with N of rounds / msgs / items
//      and don't check max rounds
