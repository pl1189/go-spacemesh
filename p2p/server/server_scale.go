// Code generated by github.com/spacemeshos/go-scale/scalegen. DO NOT EDIT.

// nolint
package server

import (
	"github.com/spacemeshos/go-scale"
)

func (t *Response) EncodeScale(enc *scale.Encoder) (total int, err error) {
	{
		n, err := scale.EncodeByteSliceWithLimit(enc, t.Data, 272629760)
		if err != nil {
			return total, err
		}
		total += n
	}
	{
		n, err := scale.EncodeStringWithLimit(enc, string(t.Error), 1024)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (t *Response) DecodeScale(dec *scale.Decoder) (total int, err error) {
	{
		field, n, err := scale.DecodeByteSliceWithLimit(dec, 272629760)
		if err != nil {
			return total, err
		}
		total += n
		t.Data = field
	}
	{
		field, n, err := scale.DecodeStringWithLimit(dec, 1024)
		if err != nil {
			return total, err
		}
		total += n
		t.Error = string(field)
	}
	return total, nil
}
