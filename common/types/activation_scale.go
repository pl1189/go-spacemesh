// Code generated by github.com/spacemeshos/go-scale/scalegen. DO NOT EDIT.

// nolint
package types

import (
	"github.com/spacemeshos/go-scale"
)

func (t *ATXMetadata) EncodeScale(enc *scale.Encoder) (total int, err error) {
	{
		n, err := scale.EncodeCompact32(enc, uint32(t.PublishEpoch))
		if err != nil {
			return total, err
		}
		total += n
	}
	{
		n, err := scale.EncodeByteArray(enc, t.MsgHash[:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (t *ATXMetadata) DecodeScale(dec *scale.Decoder) (total int, err error) {
	{
		field, n, err := scale.DecodeCompact32(dec)
		if err != nil {
			return total, err
		}
		total += n
		t.PublishEpoch = EpochID(field)
	}
	{
		n, err := scale.DecodeByteArray(dec, t.MsgHash[:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (t *MerkleProof) EncodeScale(enc *scale.Encoder) (total int, err error) {
	{
		n, err := scale.EncodeStructSliceWithLimit(enc, t.Nodes, 32)
		if err != nil {
			return total, err
		}
		total += n
	}
	{
		n, err := scale.EncodeCompact64(enc, uint64(t.LeafIndex))
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (t *MerkleProof) DecodeScale(dec *scale.Decoder) (total int, err error) {
	{
		field, n, err := scale.DecodeStructSliceWithLimit[Hash32](dec, 32)
		if err != nil {
			return total, err
		}
		total += n
		t.Nodes = field
	}
	{
		field, n, err := scale.DecodeCompact64(dec)
		if err != nil {
			return total, err
		}
		total += n
		t.LeafIndex = uint64(field)
	}
	return total, nil
}

func (t *EpochActiveSet) EncodeScale(enc *scale.Encoder) (total int, err error) {
	{
		n, err := scale.EncodeCompact32(enc, uint32(t.Epoch))
		if err != nil {
			return total, err
		}
		total += n
	}
	{
		n, err := scale.EncodeStructSliceWithLimit(enc, t.Set, 8000000)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (t *EpochActiveSet) DecodeScale(dec *scale.Decoder) (total int, err error) {
	{
		field, n, err := scale.DecodeCompact32(dec)
		if err != nil {
			return total, err
		}
		total += n
		t.Epoch = EpochID(field)
	}
	{
		field, n, err := scale.DecodeStructSliceWithLimit[ATXID](dec, 8000000)
		if err != nil {
			return total, err
		}
		total += n
		t.Set = field
	}
	return total, nil
}
