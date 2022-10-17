package mekabuild

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync/atomic"
)

// Builder provides an interface to the builder API for validators. It's
// intended to be constructed and stored in a Tendermint node, and invoked
// whenever the validator becomes a proposer and should propose a block.
//
// Builders, like all types and functions in this package, are constructed and
// managed within Tendermint, and shouldn't need to be used directly.
type Builder struct {
	baseurl       *url.URL
	client        *http.Client
	signer        Signer
	chainID       string
	validatorAddr string
	paymentAddr   string

	disableCompression int32 // atomic
}

// NewBuilder returns a usable builder. The provided HTTP client is used to make
// requests to the provided builder API URL.
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

// SetCompression enables or disables compression of HTTP request data from the
// builder client to the builder API. By default, compression is enabled.
func (b *Builder) SetCompression(enabled bool) {
	if enabled {
		atomic.StoreInt32(&b.disableCompression, 0)
	} else {
		atomic.StoreInt32(&b.disableCompression, 1)
	}
}

// BuildBlock submits a build request to the builder API.
func (b *Builder) BuildBlock(ctx context.Context, req *BuildBlockRequest) (*BuildBlockResponse, error) {
	if err := b.signer.SignBuildBlockRequest(req); err != nil {
		return nil, fmt.Errorf("sign request: %w", err)
	}

	var resp BuildBlockResponse
	if err := b.do(ctx, "/v0/build", req, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (b *Builder) do(ctx context.Context, path string, req, resp interface{}) error {
	u := b.baseurl
	u.Path = path
	uri := u.String()

	compress := atomic.LoadInt32(&b.disableCompression) != 0

	pr, pw := io.Pipe()
	go func() {
		switch {
		case compress: // normal path
			zw := gzip.NewWriter(pw)
			enc := json.NewEncoder(zw)
			if err := enc.Encode(req); err != nil {
				pw.CloseWithError(err)
				return
			}
			if err := zw.Flush(); err != nil {
				pw.CloseWithError(err)
				return
			}

		case !compress: // usually for tests
			enc := json.NewEncoder(pw)
			if err := enc.Encode(req); err != nil {
				pw.CloseWithError(err)
				return
			}
		}
		pw.Close()
	}()

	r, err := http.NewRequestWithContext(ctx, "POST", uri, pr)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	r.Header.Set("content-type", "application/json")
	r.Header.Set("zenith-chain-id", b.chainID)

	if compress {
		r.Header.Set("content-encoding", "gzip")
	}

	res, err := b.client.Do(r)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		var resp struct {
			Error string `json:"error"`
		}

		if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
			resp.Error = fmt.Errorf("unmarshal error: %w", err).Error()
		}

		return fmt.Errorf("response code %d (%s)", res.StatusCode, resp.Error)
	}

	if err := json.NewDecoder(res.Body).Decode(resp); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	return nil
}
