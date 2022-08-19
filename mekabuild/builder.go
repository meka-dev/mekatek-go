package mekabuild

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/meka-dev/mekatek-go/mekabuild/internal"
)

// Builder provides an interface to the builder API for validators. It's
// intended to be constructed and stored in a Tendermint node, and invoked
// whenever the validator becomes a proposer and should propose a block.
//
// Builders, like all types and functions in this package, are build and managed
// in our Tendermint fork, and shouldn't need to be used directly.
type Builder struct {
	baseurl       *url.URL
	client        *http.Client
	signer        Signer
	chainID       string
	validatorAddr string
	paymentAddr   string

	mu         sync.Mutex
	registered bool
}

// NewBuilder returns a usable builder which has not yet registered with the
// builder API. The provided HTTP client is used to make requests to provided
// builder API URL.
//
// The signer is used to verify the integrity of requests, and is implemented by
// the (Mekatek-patched) Tendermint private validator.
//
// The validator address should be the public address of the calling validator
// as represented on chain, which is normally uppercase hex encoded. The payment
// address should be a valid Bech32 encoded address that can be used as a
// recipient in bank send transactions.
func NewBuilder(cli *http.Client, apiURL *url.URL, s Signer, chainID, validatorAddr, paymentAddr string) *Builder {
	return &Builder{
		baseurl:       apiURL,
		client:        cli,
		signer:        s,
		chainID:       chainID,
		validatorAddr: validatorAddr,
		paymentAddr:   paymentAddr,
	}
}

// Register the validator, as defined by the parmaeters passed to the
// constructor, with the builder API. Register is stateful, meaning once a given
// instance of a builder has successfully registered, subsequent calls to
// Register will be no-ops.
func (b *Builder) Register(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.registered {
		return nil
	}

	// First we apply, get a challenge, sign it, and save the signature.
	var (
		challengeID string
		signature   []byte
	)
	{
		req := internal.ApplyRequest{
			ChainID:          b.chainID,
			ValidatorAddress: b.validatorAddr,
			PaymentAddress:   b.paymentAddr,
		}

		var resp internal.ApplyResponse

		if err := b.do(ctx, "/v0/register", &req, &resp); err != nil {
			return fmt.Errorf("registration application: %w", err)
		}

		ch := RegisterChallenge{
			Bytes: resp.Challenge,
		}

		if err := b.signer.SignMekatekRegisterChallenge(&ch); err != nil {
			return fmt.Errorf("sign register challenge: %w", err)
		}

		challengeID = resp.ChallengeID
		signature = ch.Signature
	}

	// Then we register with the saved challenge ID and signature.
	{
		req := internal.RegisterRequest{
			ChallengeID: challengeID,
			Signature:   signature,
		}

		var resp internal.RegisterResponse

		if err := b.do(ctx, "/v0/register", &req, &resp); err != nil {
			return fmt.Errorf("registration response: %w", err)
		}
	}

	// We have successfully registered.
	b.registered = true
	return nil
}

// BuildBlock submits a build request to the builder API.
func (b *Builder) BuildBlock(ctx context.Context, req *BuildBlockRequest) (*BuildBlockResponse, error) {
	if err := b.Register(ctx); err != nil {
		return nil, fmt.Errorf("register validator: %w", err)
	}

	if err := b.signer.SignMekatekBuildBlockRequest(req); err != nil {
		return nil, fmt.Errorf("sign request: %w", err)
	}

	var resp BuildBlockResponse
	if err := b.do(ctx, "/v0/build", req, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (b *Builder) do(ctx context.Context, path string, req, resp interface{}) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	u := b.baseurl
	u.Path = path
	uri := u.String()

	r, err := http.NewRequestWithContext(ctx, "POST", uri, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	r.Header.Set("content-type", "application/json")

	res, err := b.client.Do(r)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}

	defer res.Body.Close()

	body, err = io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("response code %d (%s)", res.StatusCode, strings.TrimSpace(string(body)))
	}

	if err = json.Unmarshal(body, resp); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	return nil
}
