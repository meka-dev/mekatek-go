package mekabuild_test

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/meka-dev/mekatek-go/mekabuild"
	"github.com/meka-dev/mekatek-go/mekabuild/internal"
)

func TestBuilderRegister(t *testing.T) {
	var (
		ctx           = context.Background()
		rng           = rand.Reader
		chainID       = "my-chain-id"
		keyFoo        = newMockKey(t, "foo", rng)
		api           = newMockAPI()
		server        = newTestServer(t, api)
		client        = &http.Client{}
		apiURL, _     = url.Parse(server.URL)
		signer        = keyFoo
		validatorAddr = keyFoo.addr
		paymentAddr   = "my-payment-addr"
	)

	api.addPublicKey(chainID, keyFoo.addr, keyFoo.PublicKey)

	builder := mekabuild.NewBuilder(client, apiURL, signer, chainID, validatorAddr, paymentAddr)
	if api.isRegistered(chainID, keyFoo.addr) {
		t.Errorf("registered before registration?")
	}

	if err := builder.Register(ctx); err != nil {
		t.Errorf("registration failed: %v", err)
	}

	if !api.isRegistered(chainID, keyFoo.addr) {
		t.Errorf("registration didn't seem to take effect")
	}

	if err := builder.Register(ctx); err != nil {
		t.Errorf("re-registration gave error: %v", err)
	}

	badBuilder := mekabuild.NewBuilder(client, apiURL, signer, chainID, "xyz", paymentAddr)
	if err := badBuilder.Register(ctx); err == nil {
		t.Errorf("registration of unknown validator incorrectly succeeded")
	}
}

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
		paymentAddr   = "some-payment-addr"
	)

	api.addPublicKey(chainID, keyBar.addr, keyBar.PublicKey)

	builder := mekabuild.NewBuilder(client, apiURL, signer, chainID, validatorAddr, paymentAddr)
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
	challenges map[string]*mockChallenge
	validators map[string]*mockValidator
}

func newMockAPI() *mockAPI {
	return &mockAPI{
		publicKeys: map[string][]byte{},
		challenges: map[string]*mockChallenge{},
		validators: map[string]*mockValidator{},
	}
}

func (a *mockAPI) addPublicKey(chainID, addr string, publicKey []byte) {
	a.publicKeys[makeID(chainID, addr)] = publicKey
}

func (a *mockAPI) isRegistered(chainID, addr string) bool {
	_, ok := a.validators[makeID(chainID, addr)]
	return ok
}

func (a *mockAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/v0/register":
		var req internal.RegistrationRequest
		json.NewDecoder(r.Body).Decode(&req)
		isApply := req.ChallengeID == ""
		isRegister := !isApply
		switch {
		case isApply:
			validator := &mockValidator{chainID: req.ApplyRequest.ChainID, validatorAddr: req.ApplyRequest.ValidatorAddress, paymentAddr: req.ApplyRequest.PaymentAddress}
			challenge := &mockChallenge{challengeID: hex.EncodeToString(randomBytes(16)), challenge: randomBytes(10), validator: validator}
			a.challenges[challenge.challengeID] = challenge
			json.NewEncoder(w).Encode(internal.ApplyResponse{ChallengeID: challenge.challengeID, Challenge: challenge.challenge})

		case isRegister:
			challenge, ok := a.challenges[req.RegisterRequest.ChallengeID]
			if !ok {
				http.Error(w, "no such challenge ID", http.StatusBadRequest)
				return
			}
			delete(a.challenges, req.RegisterRequest.ChallengeID)
			publicKey, ok := a.publicKeys[challenge.validator.id()]
			if !ok {
				http.Error(w, fmt.Sprintf("no public key for %q", challenge.validator.id()), http.StatusBadRequest)
				return
			}
			msg := mekabuild.RegisterChallengeSignableBytes(challenge.challenge)
			if !verify(publicKey, msg, req.Signature) {
				http.Error(w, "bad signature", http.StatusBadRequest)
				return
			}
			a.validators[challenge.validator.id()] = challenge.validator
			json.NewEncoder(w).Encode(internal.RegisterResponse{Result: "success"})
		}

	case "/v0/build":
		var req mekabuild.BuildBlockRequest
		json.NewDecoder(r.Body).Decode(&req)
		id := makeID(req.ChainID, req.ValidatorAddress)
		_, ok := a.validators[id]
		if !ok {
			http.Error(w, fmt.Sprintf("unknown validator %s", id), http.StatusBadRequest)
			return
		}
		publicKey, ok := a.publicKeys[id]
		if !ok {
			http.Error(w, fmt.Sprintf("no public key for %q", id), http.StatusBadRequest)
			return
		}
		msg := req.SignableBytes()
		if !verify(publicKey, msg, req.Signature) {
			http.Error(w, "bad signature", http.StatusBadRequest)
			return
		}
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
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	return server
}

func makeID(chainID, addr string) string {
	return chainID + ":" + addr
}

type mockChallenge struct {
	challengeID string
	challenge   []byte
	validator   *mockValidator
}

type mockValidator struct {
	chainID       string
	validatorAddr string
	paymentAddr   string
}

func (v *mockValidator) id() string {
	return makeID(v.chainID, v.validatorAddr)
}

func randomBytes(n int) []byte {
	msg := make([]byte, n)
	_, err := rand.Read(msg)
	if err != nil {
		panic(fmt.Errorf("rand.Read: %v", err))
	}
	return msg
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

func (k *mockKey) SignMekatekBuildBlockRequest(r *mekabuild.BuildBlockRequest) error {
	msg := r.SignableBytes()
	sig, err := k.PrivateKey.Sign(nil, msg, crypto.Hash(0))
	if err != nil {
		return err
	}
	r.Signature = sig
	return nil
}

func (k *mockKey) SignMekatekRegisterChallenge(c *mekabuild.RegisterChallenge) error {
	msg := c.SignableBytes()
	sig, err := k.PrivateKey.Sign(nil, msg, crypto.Hash(0))
	if err != nil {
		return err
	}
	c.Signature = sig
	return nil
}

func verify(publicKey, msg, sig []byte) bool {
	return ed25519.Verify(publicKey, msg, sig)
}