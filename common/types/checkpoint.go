package types

type Checkpoint struct {
	Command string    `json:"command"`
	Version string    `json:"version"`
	Data    InnerData `json:"data"`
}

type InnerData struct {
	CheckpointId string                      `json:"id"`
	Atxs         []AtxSnapshot               `json:"atxs"`
	Accounts     []AccountSnapshot           `json:"accounts"`
	Marriages    map[ATXID][]MarriageSnaphot `json:"marriages"`
}

type AtxSnapshot struct {
	ID             []byte `json:"id"`
	Epoch          uint32 `json:"epoch"`
	CommitmentAtx  []byte `json:"commitmentAtx"`
	MarriageAtx    []byte `json:"marriageAtx"`
	VrfNonce       uint64 `json:"vrfNonce"`
	BaseTickHeight uint64 `json:"baseTickHeight"`
	TickCount      uint64 `json:"tickCount"`
	PublicKey      []byte `json:"publicKey"`
	Sequence       uint64 `json:"sequence"`
	Coinbase       []byte `json:"coinbase"`
	// total effective units
	NumUnits uint32 `json:"numUnits"`
	// actual units per smesher
	Units map[NodeID]uint32 `json:"units"`
}

type AccountSnapshot struct {
	Address  []byte `json:"address"`
	Balance  uint64 `json:"balance"`
	Nonce    uint64 `json:"nonce"`
	Template []byte `json:"template"`
	State    []byte `json:"state"`
}

type MarriageSnaphot struct {
	Index     int    `json:"index"`
	MarriedTo []byte `json:"marriedTo"`
	Signer    []byte `json:"signer"`
	Signature []byte `json:"signature"`
}
