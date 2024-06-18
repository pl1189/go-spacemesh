package dbsync

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math/bits"
	"os"
	"strconv"

	"golang.org/x/exp/slices"
)

const (
	fingerprintBytes = 12
	cachedBits       = 24
	cachedSize       = 1 << cachedBits
	cacheMask        = cachedSize - 1
	maxIDBytes       = 32
	bit63            = 1 << 63
)

type fingerprint [fingerprintBytes]byte

func (fp fingerprint) String() string {
	return hex.EncodeToString(fp[:])
}

func (fp *fingerprint) update(h []byte) {
	for n := range *fp {
		(*fp)[n] ^= h[n]
	}
}

func hexToFingerprint(s string) fingerprint {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("bad hex fingerprint: " + err.Error())
	}
	var fp fingerprint
	if len(b) != len(fp) {
		panic("bad hex fingerprint")
	}
	copy(fp[:], b)
	return fp
}

const (
	nodeFlagLeaf = 1 << 31
	nodeFlagMask = nodeFlagLeaf
)

// NOTE: all leafs are on the last level

type node struct {
	// 16-byte structure with alignment
	// The cache is 512 MiB per 1<<24 (16777216) IDs
	fp fingerprint
	c  uint32
}

func (node *node) empty() bool {
	return node.c == 0
}

func (node *node) leaf() bool {
	return node.c&nodeFlagLeaf != 0
}

func (node *node) count() uint32 {
	if node.leaf() {
		return 1
	}
	return node.c
}

type cacheIndex uint32

const (
	prefixLenBits = 6
	prefixLenMask = 1<<prefixLenBits - 1
	prefixBitMask = ^uint64(prefixLenMask)
	maxPrefixLen  = 64 - prefixLenBits
)

type prefix uint64

func (p prefix) len() int {
	return int(p & prefixLenMask)
}

func (p prefix) bits() uint64 {
	return uint64(p >> prefixLenBits)
}

func (p prefix) left() prefix {
	l := uint64(p) & prefixLenMask
	if l == maxPrefixLen {
		panic("BUG: max prefix len reached")
	}
	return prefix((uint64(p)&prefixBitMask)<<1 + l + 1)
}

func (p prefix) right() prefix {
	return p.left() + (1 << prefixLenBits)
}

func (p prefix) cacheIndex() (cacheIndex, bool) {
	if l := p.len(); l <= cachedBits {
		// Notation: prefix(cacheIndex)
		//
		//          empty(0)
		//         /       \
		//        /         \
		//       /           \
		//     0(1)          1(2)
		//    /   \         /   \
		//   /     \       /     \
		// 00(3)  01(4)  10(5)  11(6)

		// indexing starts at 1
		// left:  n = n*2
		// right: n = n*2+1
		// but in the end we substract 1 to make it 0-based again

		return cacheIndex(p.bits() | (1 << l) - 1), true
	}
	return 0, false
}

func (p prefix) String() string {
	if p.len() == 0 {
		return "<0>"
	}
	b := fmt.Sprintf("%064b", p.bits())
	return fmt.Sprintf("<%d:%s>", p.len(), b[64-p.len():])
}

func load64(h []byte) uint64 {
	return binary.BigEndian.Uint64(h[:8])
}

func hashPrefix(h []byte, nbits int) prefix {
	if nbits < 0 || nbits > maxPrefixLen {
		panic("BUG: bad prefix length")
	}
	if nbits == 0 {
		return 0
	}
	v := load64(h)
	return prefix((v>>(64-nbits-prefixLenBits))&prefixBitMask + uint64(nbits))
}

func preFirst0(h []byte) prefix {
	l := min(maxPrefixLen, bits.LeadingZeros64(^load64(h)))
	return hashPrefix(h, l)
}

func preFirst1(h []byte) prefix {
	l := min(maxPrefixLen, bits.LeadingZeros64(load64(h)))
	return hashPrefix(h, l)
}

func commonPrefix(a, b []byte) prefix {
	v1 := load64(a)
	v2 := load64(b)
	l := uint64(min(maxPrefixLen, bits.LeadingZeros64(v1^v2)))
	return prefix((v1>>(64-l))<<prefixLenBits + l)
}

type fpResult struct {
	fp    fingerprint
	count uint32
}

type aggResult struct {
	tails []uint64
	fp    fingerprint
	count uint32
	itype int
}

func (r *aggResult) update(node *node) {
	r.fp.update(node.fp[:])
	r.count += node.c
}

type fpTree struct {
	nodes [cachedSize * 2]node
}

func (ft *fpTree) pushDown(node *node, p prefix) {
	if p.len() >= cachedBits {
		return
	}
	pushDownBit := node.c & (1 << (cachedBits - 1 - p.len()))
	var pushDownPrefix prefix
	if pushDownBit == 0 {
		pushDownPrefix = p.left()
	} else {
		pushDownPrefix = p.right()
	}
	pushDownIdx, haveIdx := pushDownPrefix.cacheIndex()
	if !haveIdx {
		panic("BUG: no idx for pushDownPrefix")
	}
	pushDownNode := &ft.nodes[pushDownIdx]

	// QQQQQ: rm
	idx, _ := p.cacheIndex()
	fmt.Fprintf(os.Stderr, "QQQQQ: idx: %d pushDownIdx: %d c: %d\n", idx, pushDownIdx, pushDownNode.c)

	if !pushDownNode.empty() {
		panic("BUG: non-empty push down node")
	}
	pushDownNode.c = node.c
	pushDownNode.fp = node.fp
}

func (ft *fpTree) addHash(h []byte) {
	var p prefix
	v := binary.BigEndian.Uint64(h[:8])
	vFull := v
	for {
		idx, haveIdx := p.cacheIndex()
		if !haveIdx {
			panic("BUG: no cache idx")
		}
		node := &ft.nodes[idx]
		switch {
		case node.empty():
			node.c = uint32(vFull>>(64-cachedBits)) | nodeFlagLeaf
			node.fp.update(h[:])
			fmt.Fprintf(os.Stderr, "QQQQQ: leaf at idx: %d\n", idx)
			return
		case node.leaf():
			// push down the old leaf
			ft.pushDown(node, p)
			node.c = 2
			node.fp.update(h[:])
		default:
			node.c++
			node.fp.update(h[:])
		}
		switch {
		case !haveIdx:
			panic("BUG: no cache idx")
		case p.len() > cachedBits:
			panic("BUG: prefix too long")
		case p.len() == cachedBits:
			return
		case v&bit63 == 0:
			p = p.left()
		default:
			p = p.right()
		}
		v <<= 1
	}
}

func (ft *fpTree) aggregateLeft(v uint64, p prefix, r *aggResult) {
	if p.len() >= cachedBits {
		r.tails = append(r.tails, p.bits()<<(24-p.len()))
		fmt.Fprintf(os.Stderr, "QQQQQ: aggregateLeft: %016x %s: add tail\n", v, p)
		return
	}
	idx, gotIdx := p.right().cacheIndex()
	if !gotIdx {
		panic("BUG: no idx")
	}
	node := &ft.nodes[idx]
	if node.leaf() {
		r.tails = append(r.tails, uint64(node.c & ^uint32(nodeFlagMask)))
		return
	}
	if bit := v & (1 << (63 - p.len())); bit == 0 {
		r.update(node)
		fmt.Fprintf(os.Stderr, "QQQQQ: aggregateLeft: %016x %s: 0 -> add count %d fp %s + go left\n", v, p,
			node.c, node.fp)
		ft.aggregateLeft(v, p.left(), r)
	} else {
		fmt.Fprintf(os.Stderr, "QQQQQ: aggregateLeft: %016x %s: 1 -> go right\n", v, p)
		ft.aggregateLeft(v, p.right(), r)
	}

	// switch {
	// case p.len() >= cachedBits:
	// 	r.tails = append(r.tails, p)
	// 	fmt.Fprintf(os.Stderr, "QQQQQ: aggregateLeft: %016x %s: add tail\n", v, p)
	// case bit == 0:
	// 	idx, gotIdx := p.right().cacheIndex()
	// 	if !gotIdx {
	// 		panic("BUG: no idx")
	// 	}
	// 	r.update(&ft.nodes[idx])
	// 	fmt.Fprintf(os.Stderr, "QQQQQ: aggregateLeft: %016x %s: 0 -> add count %d fp %s\n", v, p,
	// 		ft.nodes[idx].c, ft.nodes[idx].fp)
	// 	ft.aggregateLeft(v, p.left(), r)
	// default:
	// 	fmt.Fprintf(os.Stderr, "QQQQQ: aggregateLeft: %016x %s: 1 -> go right\n", v, p)
	// 	ft.aggregateLeft(v, p.right(), r)
	// }
}

func (ft *fpTree) aggregateRight(v uint64, p prefix, r *aggResult) {
	if p.len() >= cachedBits {
		r.tails = append(r.tails, p.bits()<<(24-p.len()))
		fmt.Fprintf(os.Stderr, "QQQQQ: aggregateRight: %016x %s: add tail\n", v, p)
		return
	}
	idx, gotIdx := p.left().cacheIndex()
	if !gotIdx {
		panic("BUG: no idx")
	}
	node := &ft.nodes[idx]
	if node.leaf() {
		r.tails = append(r.tails, uint64(node.c & ^uint32(nodeFlagMask)))
		return
	}
	if bit := v & (1 << (63 - p.len())); bit == 0 {
		fmt.Fprintf(os.Stderr, "QQQQQ: aggregateRight: %016x %s: 1 -> go left\n", v, p)
		ft.aggregateRight(v, p.left(), r)
	} else {
		r.update(node)
		fmt.Fprintf(os.Stderr, "QQQQQ: aggregateRight: %016x %s: 0 -> add count %d fp %s + go right\n", v, p,
			node.c, node.fp)
		ft.aggregateRight(v, p.right(), r)
	}

	// bit := v & (1 << (63 - p.len()))
	// switch {
	// case p.len() >= cachedBits:
	// 	r.tails = append(r.tails, p)
	// 	fmt.Fprintf(os.Stderr, "QQQQQ: aggregateRight: %016x %s: add tail\n", v, p)
	// case bit == 0:
	// 	fmt.Fprintf(os.Stderr, "QQQQQ: aggregateRight: %016x %s: 0 -> go left\n", v, p)
	// 	ft.aggregateRight(v, p.left(), r)
	// default:
	// 	idx, gotIdx := p.left().cacheIndex()
	// 	if !gotIdx {
	// 		panic("BUG: no idx")
	// 	}
	// 	r.update(&ft.nodes[idx])
	// 	fmt.Fprintf(os.Stderr, "QQQQQ: aggregateRight: %016x %s: 1 -> add count %d fp %s + go right\n", v, p,
	// 		ft.nodes[idx].c, ft.nodes[idx].fp)
	// 	ft.aggregateRight(v, p.right(), r)
	// }
}

func (ft *fpTree) aggregateInterval(x, y []byte) aggResult {
	var r aggResult
	r.itype = bytes.Compare(x, y)
	switch {
	case r.itype == 0:
		// the whole set
		r.update(&ft.nodes[0])
	case r.itype < 0:
		// "proper" interval: [x; lca); (lca; y)
		p := commonPrefix(x, y)
		ft.aggregateLeft(load64(x), p.left(), &r)
		ft.aggregateRight(load64(y), p.right(), &r)
	default:
		// inverse interval: [min; y); [x; max]
		ft.aggregateRight(load64(y), preFirst1(y), &r)
		ft.aggregateLeft(load64(x), preFirst0(x), &r)
	}
	return r
}

func (ft *fpTree) dumpNode(w io.Writer, p prefix, indent, dir string) {
	idx, gotIdx := p.cacheIndex()
	if !gotIdx {
		return
	}
	node := &ft.nodes[idx]
	if node.empty() {
		return
	}
	var countStr string
	leaf := node.leaf()
	if leaf {
		countStr = "LEAF"
	} else if node.empty() {
		countStr = "EMPTY"
	} else {
		countStr = strconv.Itoa(int(node.count()))
	}
	fmt.Fprintf(w, "%s%s%s %s\n", indent, dir, node.fp, countStr)
	if !leaf {
		indent += "  "
		ft.dumpNode(w, p.left(), indent, "l: ")
		ft.dumpNode(w, p.right(), indent, "r: ")
	}
}

func (ft *fpTree) dump(w io.Writer) {
	if ft.nodes[0].c == 0 {
		fmt.Fprintln(w, "empty tree")
	} else {
		ft.dumpNode(w, 0, "", "")
	}
}

type inMemFPTree struct {
	tree fpTree
	ids  [cachedSize][][]byte
}

func (mft *inMemFPTree) addHash(h []byte) {
	mft.tree.addHash(h)
	idx := load64(h) >> (64 - cachedBits)
	s := mft.ids[idx]
	n := slices.IndexFunc(s, func(cur []byte) bool {
		return bytes.Compare(cur, h) > 0
	})
	if n < 0 {
		mft.ids[idx] = append(s, h)
	} else {
		mft.ids[idx] = slices.Insert(s, n, h)
	}
}

func (mft *inMemFPTree) aggregateInterval(x, y []byte) fpResult {
	r := mft.tree.aggregateInterval(x, y)
	for _, t := range r.tails {
		ids := mft.ids[t]
		for _, id := range ids {
			// FIXME: this can be optimized as the IDs are ordered
			if idWithinInterval(id, x, y, r.itype) {
				r.fp.update(id)
				r.count++
			}
		}
	}
	return fpResult{fp: r.fp, count: r.count}
}

func idWithinInterval(id, x, y []byte, itype int) bool {
	switch itype {
	case 0:
		return true
	case -1:
		return bytes.Compare(id, x) >= 0 && bytes.Compare(id, y) < 0
	default:
		return bytes.Compare(id, y) < 0 || bytes.Compare(id, x) >= 0
	}
}

// TBD: perhaps use json-based SELECTs
// TBD: extra cache for after-24bit entries
// TBD: benchmark 24-bit limit (not going beyond the cache)
// TBD: optimize, get rid of binary.BigEndian.*
