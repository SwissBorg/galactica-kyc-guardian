package api

import (
	"encoding/json"
)

const (
	CertificateStatusPending CertificateStatus = "PENDING"
	CertificateStatusDone    CertificateStatus = "DONE"
)

type ErrorResp struct {
	Error string `json:"error"`
}

type CertificateStatus string

type GenerateCertRequest struct {
	HolderCommitment string `json:"holder_commitment"`
	EncryptionPubKey string `json:"encryption_pub_key"`

	UserID  UserID  `json:"user_id" validate:"required,lte=64"`
	Profile Profile `json:"profile"`
}

type GetCertRequest struct {
	UserID UserID `json:"user_id"`
}

type GenerateCertResponse struct {
	Status CertificateStatus `json:"status"`
}

type GetCertResponse struct {
	Status      CertificateStatus `json:"status"`
	Certificate json.RawMessage   `json:"certificate"`
}

type UserID string

type Profile struct {
	Firstname   string `json:"firstname"`
	Lastname    string `json:"lastname"`
	DateOfBirth string `json:"date_of_birth"`
	Nationality string `json:"nationality"`
	Postcode    string `json:"postcode"`
}
