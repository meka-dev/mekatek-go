package mekabuild

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// Signer is a consumer contract for the Builder. It models a subset of the
// methods provided by a Tendermint private validator.
type Signer interface {
	SignMekatekBuildBlockRequest(*BuildBlockRequest) error
	SignMekatekRegisterChallenge(*RegisterChallenge) error
}

// BuildBlockRequest represents a request from a validator to the build endpoint
// of the builder API. In order to meet the pattern used by other signable types
// in Tendermint, it contains a Signature field that needs to be set by callers.
// See SignableBytes for more detail.
type BuildBlockRequest struct {
	ChainID          string   `json:"chain_id"`
	Height           int64    `json:"height"`
	ValidatorAddress string   `json:"validator_address"`
	MaxBytes         int64    `json:"max_bytes"`
	MaxGas           int64    `json:"max_gas"`
	Txs              [][]byte `json:"txs"`

	Signature []byte `json:"signature"`
}

// SignableBytes returns a stable byte representation of the signable fields of
// a BuildBlockRequest. After constructing a complete value, callers should
// invoke this method, sign the returned bytes with the private key that
// corresponds to the validator address, and set the Signature field to that
// signature.
func (req *BuildBlockRequest) SignableBytes() []byte {
	return BuildBlockRequestSignableBytes(req.ChainID, req.Height, req.ValidatorAddress, req.MaxBytes, req.MaxGas, req.Txs)
}

// BuildBlockRequestSignableBytes returns a stable byte representation of a
// BuildBlockRequest represented by the provided parameters.
func BuildBlockRequestSignableBytes(chainID string, height int64, validatorAddr string, maxBytes, maxGas int64, txs [][]byte) []byte {
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
	mustEncode(&sb, uint64(len(txs)))
	for _, tx := range txs {
		mustEncode(&sb, uint64(len(tx)))
		mustEncode(&sb, tx)
	}
	return sb.Bytes()
}

// BuildBlockResponse is returned by the build endpoint of the builder API.
type BuildBlockResponse struct {
	Txs              [][]byte `json:"txs"`
	ValidatorPayment string   `json:"validator_payment,omitempty"`
}

//
//
//

// RegisterChallenge is used by the builder API during registration to establish
// the identity of the registering validator. In order to meet the pattern used
// by other signable types in Tendermint, it contains a Signature field that
// needs to be set by callers. See SignableBytes for more detail.
type RegisterChallenge struct {
	Bytes     []byte `json:"bytes"`
	Signature []byte `json:"signature,omitempty"`
}

// SignableBytes returns a stable byte representation of the challenge. After
// constructing a complete value, callers should invoke this method, sign the
// returned bytes with the private key that corresponds to the validator address
// provided during registration, and set the Signature field to that signature.
func (rc *RegisterChallenge) SignableBytes() []byte {
	return RegisterChallengeSignableBytes(rc.Bytes)
}

// RegisterChallengeSignableBytes returns a stable byte representation of the
// RegisterChallenge represented by the provided parameters.
func RegisterChallengeSignableBytes(ch []byte) []byte {
	// XXX: Changing the order or the set of fields that are signed will cause
	// verification failures unless both the signer and verifier are updated.
	// Tread carefully.

	// SECURITY 🚨 We prefix the signable bytes with a constant so that in an
	// unauthenticated remote signer deployment an internal actor can't sign
	// arbitrary bytes with an RPC.

	var sb bytes.Buffer
	mustEncode(&sb, []byte(`register-challenge`))
	mustEncode(&sb, uint64(len(ch)))
	mustEncode(&sb, ch)
	return sb.Bytes()
}

func mustEncode(w io.Writer, v interface{}) {
	if err := binary.Write(w, binary.LittleEndian, v); err != nil {
		panic(fmt.Errorf("encode %T (%v): %w", v, v, err))
	}
}
