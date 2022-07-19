package mekatek

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
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
	PubKey() (bytes []byte, typ, addr string, err error)
	// TODO: Change to Sign(*BuildBlockRequest)
	Sign(p []byte) ([]byte, error)
}

func NewBuilder(
	chainID string,
	apiURL *url.URL,
	apiTimeout time.Duration,
	paymentAddr string,
	p Proposer,
) (Builder, error) {
	pubKeyBytes, pubKeyType, _, err := p.PubKey()
	if err != nil {
		return nil, fmt.Errorf("get public key from validator: %w", err)
	}

	bb, err := newHTTPBlockBuilder(apiURL, apiTimeout, p)
	if err != nil {
		return nil, fmt.Errorf("create HTTP block builder: %w", err)
	}

	if _, err = bb.RegisterProposer(context.Background(), &registerProposerRequest{
		ChainID:        chainID,
		PaymentAddress: paymentAddr,
		PubKeyBytes:    pubKeyBytes,
		PubKeyType:     pubKeyType,
	}); err != nil {
		return nil, fmt.Errorf("register proposer: %w", err)
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

func newHTTPBlockBuilder(
	baseurl *url.URL,
	timeout time.Duration,
	p Proposer,
) (*httpBlockBuilder, error) {
	_, _, addr, err := p.PubKey()
	if err != nil {
		return nil, fmt.Errorf("get proposer public key: %w", err)
	}

	return &httpBlockBuilder{
		baseurl:      baseurl,
		client:       &http.Client{Timeout: timeout},
		proposerAddr: addr,
		proposer:     p,
	}, nil
}

func (b *httpBlockBuilder) BuildBlock(
	ctx context.Context,
	req *BuildBlockRequest,
) (*BuildBlockResponse, error) {
	var resp BuildBlockResponse
	return &resp, b.do(ctx, "/v0/build", req, &resp)
}

func (b *httpBlockBuilder) RegisterProposer(
	ctx context.Context,
	req *registerProposerRequest,
) (*registerProposerResponse, error) {
	var resp registerProposerResponse
	return &resp, b.do(ctx, "/v0/register", req, &resp)
}

func (b *httpBlockBuilder) do(ctx context.Context, path string, req, resp interface{}) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	// TODO: SECURITY 🚨 review, do we need to sign other things than the body?
	// What about nonces (e.g. timestamp)? Are replay attacks possible or exploitable here?
	signature, err := b.proposer.Sign(body)
	if err != nil {
		return fmt.Errorf("signature failed: %w", err)
	}

	u := b.baseurl
	u.Path = path
	uri := u.String()

	r, err := http.NewRequestWithContext(ctx, "POST", uri, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	r.Header.Set("content-type", "application/json")
	r.Header.Set("mekatek-proposer-address", b.proposerAddr)
	r.Header.Set("mekatek-request-signature", hex.EncodeToString(signature))

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

func (r *BuildBlockRequest) SignatureBytes() []byte {
	// XXX: Changing the order or the set of fields that are signed
	// will cause verification failures unless both the signer and verifier
	// are updated. Tread carefully.
	var sb bytes.Buffer
	sb.WriteString(r.ProposerAddress)
	sb.WriteString(r.ChainID)
	binary.Write(&sb, binary.LittleEndian, r.Height)
	binary.Write(&sb, binary.LittleEndian, r.MaxBytes)
	binary.Write(&sb, binary.LittleEndian, r.MaxGas)
	for _, tx := range r.Txs {
		sb.Write(tx)
	}
	return sb.Bytes()
}

type BuildBlockResponse struct {
	Txs [][]byte `json:"txs"`
}

type registerProposerRequest struct {
	ChainID        string `json:"chain_id"`
	PaymentAddress string `json:"payment_address"`
	PubKeyBytes    []byte `json:"pub_key_bytes"`
	PubKeyType     string `json:"pub_key_type"`
}

type registerProposerResponse struct {
	Result string `json:"result"`
}
