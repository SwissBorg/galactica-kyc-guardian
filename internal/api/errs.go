package api

import "fmt"

var (
	ErrParsReq            = fmt.Errorf("parsing request failed")
	ErrParsCommitment     = fmt.Errorf("parsing commitment hash failed")
	ErrValidateCommitment = fmt.Errorf("validating holder commitment failed")
	ErrParsDate           = fmt.Errorf("parsing profile date failed")
	ErrParsNationality    = fmt.Errorf("parsing profile nationality failed")
	ErrCertGenerating     = fmt.Errorf("generating cert failed")
	ErrReadCertStatus     = fmt.Errorf("reading cert status failed")
	ErrAddCertToQueue     = fmt.Errorf("adding cert to queue failed")
	ErrAddCertToDB        = fmt.Errorf("adding cert to DB failed")
	ErrDecodePubKey       = fmt.Errorf("decode pub key string failed")
)
