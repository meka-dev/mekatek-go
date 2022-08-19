package internal

type RegistrationRequest struct {
	ApplyRequest
	RegisterRequest
}

type ApplyRequest struct {
	ChainID          string `json:"chain_id"`
	ValidatorAddress string `json:"validator_address"`
	PaymentAddress   string `json:"payment_address"`
}

type ApplyResponse struct {
	ChallengeID string `json:"challenge_id"`
	Challenge   []byte `json:"challenge"`
}

type RegisterRequest struct {
	ChallengeID string `json:"challenge_id"`
	Signature   []byte `json:"signature"`
}

type RegisterResponse struct {
	Result string `json:"result"`
}
