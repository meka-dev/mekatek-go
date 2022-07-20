package mekatek

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var (
	defaultBlockBuilderAPIRURL = &url.URL{Scheme: "https", Host: "api.mekatek.xyz"}
	defaultBlockBuilderTimeout = 1 * time.Second
)

type Builder interface {
	BuildBlock(context.Context, *BuildBlockRequest) (*BuildBlockResponse, error)
}

type Proposer interface {
	Address() (string, error)
	SignBuildBlockRequest(*BuildBlockRequest) error
	SignRegisterChallenge(*RegisterChallenge) error
}

func NewBuilder(
	chainID string,
	apiURL *url.URL,
	apiTimeout time.Duration,
	paymentAddr string,
	p Proposer,
) (Builder, error) {
	addr, err := p.Address()
	if err != nil {
		return nil, fmt.Errorf("get proposer address: %w", err)
	}

	bb := &httpBlockBuilder{
		baseurl:      apiURL,
		client:       &http.Client{Timeout: apiTimeout},
		proposerAddr: addr,
		proposer:     p,
	}

	req := &RegisterRequest{
		ChainID:         chainID,
		ProposerAddress: addr,
		PaymentAddress:  paymentAddr,
		SignedChallenge: nil,
	}

	resp, err := bb.Register(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("register proposer: %w", err)
	}

	if resp.Result == "registered" {
		return bb, nil
	}

	if resp.Result != "challenged" {
		return nil, fmt.Errorf("unexpected register result %q", resp.Result)
	}

	if resp.Challenge == nil || len(resp.Challenge.Bytes) == 0 {
		return nil, fmt.Errorf("empty challenge")
	}

	if err = p.SignRegisterChallenge(resp.Challenge); err != nil {
		return nil, fmt.Errorf("sign register challenge failed: %w", err)
	}

	req.SignedChallenge = resp.Challenge

	resp, err = bb.Register(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("register with signed challenge failed: %w", err)
	}

	if resp.Result != "registered" {
		return nil, fmt.Errorf("unexpected register result: want registered, got %s", resp.Result)
	}

	return bb, nil
}

func GetURIFromEnv(envkey string) *url.URL {
	s := os.Getenv(envkey)
	if s == "" {
		return defaultBlockBuilderAPIRURL
	}

	if !strings.HasPrefix(s, "http") {
		s = defaultBlockBuilderAPIRURL.Scheme + s
	}

	u, err := url.Parse(s)
	if err != nil {
		return defaultBlockBuilderAPIRURL
	}

	return &url.URL{
		Scheme: defaultBlockBuilderAPIRURL.Scheme, // default URL defines scheme (e.g. HTTPS)
		Host:   u.Host,                            // provided URI defines host:port
		Path:   defaultBlockBuilderAPIRURL.Path,   // default URL defines path
	}
}

func GetDurationFromEnv(envkey string) time.Duration {
	s := os.Getenv(envkey)
	if s == "" {
		return defaultBlockBuilderTimeout
	}

	d, err := time.ParseDuration(s)
	if err != nil {
		return defaultBlockBuilderTimeout
	}

	return d
}

//
//
//

type httpBlockBuilder struct {
	baseurl      *url.URL
	client       *http.Client
	proposerAddr string
	proposer     Proposer
}

func (b *httpBlockBuilder) BuildBlock(
	ctx context.Context,
	req *BuildBlockRequest,
) (*BuildBlockResponse, error) {
	if err := b.proposer.SignBuildBlockRequest(req); err != nil {
		return nil, fmt.Errorf("failed to sign request: %w", err)
	}

	var resp BuildBlockResponse
	return &resp, b.do(ctx, "/v0/build", req, &resp)
}

func (b *httpBlockBuilder) Register(
	ctx context.Context,
	req *RegisterRequest,
) (*RegisterResponse, error) {
	var resp RegisterResponse
	return &resp, b.do(ctx, "/v0/register", req, &resp)
}

func (b *httpBlockBuilder) do(ctx context.Context, path string, req, resp interface{}) error {
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

	body, err = ioutil.ReadAll(res.Body)
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

//
//
//

type BuildBlockRequest struct {
	ProposerAddress string   `json:"proposer_address"`
	ChainID         string   `json:"chain_id"`
	Height          int64    `json:"height"`
	MaxBytes        int64    `json:"max_bytes"`
	MaxGas          int64    `json:"max_gas"`
	Txs             [][]byte `json:"txs"`
	Signature       []byte   `json:"signature"`
}

// BuildBlockRequestSignatureBytes returns a stable byte representation of the signable
// fields of a BuildBlockRequest.
func BuildBlockRequestSignatureBytes(
	proposerAddress string,
	chainID string,
	height int64,
	maxBytes int64,
	maxGas int64,
	txs [][]byte,
) []byte {
	// XXX: Changing the order or the set of fields that are signed
	// will cause verification failures unless both the signer and verifier
	// are updated. Tread carefully.
	var sb bytes.Buffer
	sb.WriteString(proposerAddress)
	sb.WriteString(chainID)
	binary.Write(&sb, binary.LittleEndian, height)
	binary.Write(&sb, binary.LittleEndian, maxBytes)
	binary.Write(&sb, binary.LittleEndian, maxGas)
	for _, tx := range txs {
		sb.Write(tx)
	}
	return sb.Bytes()
}

type BuildBlockResponse struct {
	Txs [][]byte `json:"txs"`
}

type RegisterRequest struct {
	ChainID         string             `json:"chain_id"`
	ProposerAddress string             `json:"proposer_address"`
	PaymentAddress  string             `json:"payment_address"`
	SignedChallenge *RegisterChallenge `json:"signed_challenge,omitempty"`
}

type RegisterResponse struct {
	Result    string             `json:"result"`
	Challenge *RegisterChallenge `json:"challenge,omitempty"`
}

type RegisterChallenge struct {
	Bytes     []byte `json:"bytes"`
	Signature []byte `json:"signature,omitempty"`
}
