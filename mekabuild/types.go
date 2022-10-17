package mekabuild

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
)

// Signer is a consumer contract for the Builder. It models a subset of the
// methods provided by a Tendermint private validator.
type Signer interface {
	SignBuildBlockRequest(*BuildBlockRequest) error
}

// BuildBlockRequest represents a request from a validator to the build endpoint
// of the builder API. In order to meet the pattern used by other signable types
// in Tendermint, it contains a Signature field that needs to be set by callers.
// See BuildBlockRequestSignBytes for more detail.
type BuildBlockRequest struct {
	ChainID          string   `json:"chain_id"`
	Height           int64    `json:"height"`
	ValidatorAddress string   `json:"validator_address"`
	MaxBytes         int64    `json:"max_bytes"`
	MaxGas           int64    `json:"max_gas"`
	Txs              [][]byte `json:"txs"`

	Signature []byte `json:"signature"`
}

// HashTxs returns the sha256 sum of all given txs.
// Pass this to BuildBlockRequestSignBytes txsHash argument.
func HashTxs(txs ...[]byte) []byte {
	h := sha256.New()
	for _, tx := range txs {
		h.Write(tx)
	}
	return h.Sum(nil)
}

// BuildBlockRequestSignBytes returns a stable byte representation of a
// BuildBlockRequest represented by the provided parameters.
func BuildBlockRequestSignBytes(chainID string, height int64, validatorAddr string, maxBytes, maxGas int64, txsHash []byte) []byte {
	// XXX: Changing the order or the set of fields that are signed will cause
	// verification failures unless both the signer and verifier are updated.
	// Tread carefully.

	// SECURITY 🚨 We prefix the signable bytes with a constant so that in an
	// unauthenticated remote signer deployment, an internal actor can't sign
	// arbitrary bytes with an RPC.

	var sb bytes.Buffer
	mustEncode(&sb, []byte(`build-block-request`))
	mustEncode(&sb, uint64(len([]byte(chainID))))
	mustEncode(&sb, []byte(chainID))
	mustEncode(&sb, height)
	mustEncode(&sb, uint64(len([]byte(validatorAddr))))
	mustEncode(&sb, []byte(validatorAddr))
	mustEncode(&sb, maxBytes)
	mustEncode(&sb, maxGas)
	mustEncode(&sb, uint64(len(txsHash)))
	mustEncode(&sb, txsHash)
	return sb.Bytes()
}

// BuildBlockResponse is returned by the build endpoint of the builder API.
type BuildBlockResponse struct {
	Txs              [][]byte `json:"txs"`
	ValidatorPayment string   `json:"validator_payment,omitempty"`
}

func mustEncode(w io.Writer, v interface{}) {
	if err := binary.Write(w, binary.LittleEndian, v); err != nil {
		panic(fmt.Errorf("encode %T (%v): %w", v, v, err))
	}
}
