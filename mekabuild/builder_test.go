package mekabuild_test

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/meka-dev/mekatek-go/mekabuild"
)

func TestBuilderBuild(t *testing.T) {
	var (
		ctx           = context.Background()
		rng           = rand.Reader
		chainID       = "other-chain-id"
		keyBar        = newMockKey(t, "bar", rng)
		api           = newMockAPI()
		server        = newTestServer(t, api)
		client        = &http.Client{}
		apiURL, _     = url.Parse(server.URL)
		signer        = keyBar
		validatorAddr = keyBar.addr
	)

	api.addPublicKey(chainID, keyBar.addr, keyBar.PublicKey)

	builder := mekabuild.NewBuilder(client, apiURL, signer, chainID, validatorAddr)
	resp, err := builder.BuildBlock(ctx, &mekabuild.BuildBlockRequest{
		ChainID:          chainID,
		Height:           10,
		ValidatorAddress: validatorAddr,
		MaxBytes:         100_000,
		MaxGas:           100_000,
		Txs:              [][]byte{[]byte(`tx1`), []byte(`tx2`)},
	})
	if err != nil {
		t.Fatalf("build block failed: %v", err)
	}

	if want, have := 2, len(resp.Txs); want != have {
		t.Errorf("tx count: want %d, have %d", want, have)
	}

	if want, have := fmt.Sprintf("2 %s coins", chainID), resp.ValidatorPayment; want != have {
		t.Errorf("payment: want %q, have %q", want, have)
	}
}

//
//
//

type mockAPI struct {
	publicKeys map[string][]byte
	validators map[string]*mockValidator
}

func newMockAPI() *mockAPI {
	return &mockAPI{
		publicKeys: map[string][]byte{},
		validators: map[string]*mockValidator{},
	}
}

func (a *mockAPI) addPublicKey(chainID, addr string, publicKey []byte) {
	a.publicKeys[makeID(chainID, addr)] = publicKey
}

func (a *mockAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/v1/build":
		var req mekabuild.BuildBlockRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Errorf("decode request: %w", err).Error(), http.StatusBadRequest)
			return
		}

		id := makeID(req.ChainID, req.ValidatorAddress)
		publicKey, ok := a.publicKeys[id]
		if !ok {
			http.Error(w, "validator not in valset", http.StatusBadRequest)
			return
		}

		msg := mekabuild.BuildBlockRequestSignBytes(
			req.ChainID,
			req.Height,
			req.ValidatorAddress,
			req.MaxBytes,
			req.MaxGas,
			mekabuild.HashTxs(req.Txs...),
		)
		if !verify(publicKey, msg, req.Signature) {
			http.Error(w, "bad signature", http.StatusBadRequest)
			return
		}

		a.validators[id] = &mockValidator{chainID: req.ChainID, validatorAddr: req.ValidatorAddress}

		json.NewEncoder(w).Encode(mekabuild.BuildBlockResponse{
			Txs:              req.Txs,
			ValidatorPayment: fmt.Sprintf("%d %s coins", len(req.Txs), req.ChainID),
		})

	default:
		http.Error(w, fmt.Sprintf("unknown mock API route %s", r.URL.Path), http.StatusNotFound)
	}
}

//
//
//

func newTestServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(mekabuild.GunzipRequestMiddleware(h))
	t.Cleanup(server.Close)
	return server
}

func makeID(chainID, addr string) string {
	return chainID + ":" + addr
}

type mockValidator struct {
	chainID       string
	validatorAddr string
}

type mockKey struct {
	addr string
	ed25519.PublicKey
	ed25519.PrivateKey
}

func newMockKey(t *testing.T, addr string, rng io.Reader) *mockKey {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rng)
	if err != nil {
		t.Fatal(err)
	}
	return &mockKey{
		addr:       addr,
		PublicKey:  public,
		PrivateKey: private,
	}
}

func (k *mockKey) SignBuildBlockRequest(r *mekabuild.BuildBlockRequest) error {
	msg := mekabuild.BuildBlockRequestSignBytes(
		r.ChainID,
		r.Height,
		r.ValidatorAddress,
		r.MaxBytes,
		r.MaxGas,
		mekabuild.HashTxs(r.Txs...),
	)
	sig, err := k.PrivateKey.Sign(nil, msg, crypto.Hash(0))
	if err != nil {
		return err
	}
	r.Signature = sig
	return nil
}

func verify(publicKey, msg, sig []byte) bool {
	return ed25519.Verify(publicKey, msg, sig)
}
