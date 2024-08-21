package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	iso "github.com/DarkJian/iso3166"
	"github.com/dgraph-io/badger/v4"
	"github.com/galactica-corp/guardians-sdk/pkg/zkcertificate"
	"github.com/labstack/echo/v4"
	log "github.com/sirupsen/logrus"
	"github.com/stasundr/decimal"

	"github.com/swissborg/galactica-kyc-guardian/internal/zkcert"
)

type Handlers struct {
	inMem     *badger.DB
	generator *zkcert.Service
}

func NewHandlers(generator *zkcert.Service,
	mem *badger.DB) *Handlers {
	return &Handlers{
		inMem:     mem,
		generator: generator,
	}
}

func (h *Handlers) GenerateCert(c echo.Context) error {
	var req GenerateCertRequest

	if err := c.Bind(&req); err != nil {
		log.WithError(err).Error("bind gen cert request")
		return c.JSON(http.StatusBadRequest, ErrorResp{
			Error: ErrParsReq.Error(),
		})
	}

	log.Printf("REQUEST: %+v", req)

	if err := c.Validate(req); err != nil {
		log.WithError(err).Error("validate gen cert request")
		return c.JSON(http.StatusBadRequest, ErrorResp{
			Error: err.Error(),
		})
	}

	var holderCommitment zkcertificate.HolderCommitment
	dec, ok := decimal.NewDecimalFromString(req.HolderCommitment)
	if !ok {
		log.Errorf("%s: %s", ErrParsCommitment.Error(), req.HolderCommitment)
		return c.JSON(http.StatusBadRequest, ErrorResp{
			Error: ErrParsCommitment.Error(),
		})
	}

	holderCommitment.CommitmentHash = zkcertificate.HashFromBigInt(dec.ToBig())

	decoded, err := base64.StdEncoding.DecodeString(req.EncryptionPubKey)
	if err != nil {
		log.WithError(err).Errorf("%s: %s", ErrDecodePubKey.Error(), req.EncryptionPubKey)
		return c.JSON(http.StatusBadRequest, ErrorResp{
			Error: ErrDecodePubKey.Error(),
		})
	}
	holderCommitment.EncryptionKey = decoded

	if err := holderCommitment.Validate(); err != nil {
		log.WithError(err).Errorf("%s: %s", ErrValidateCommitment.Error(), holderCommitment)
		return c.JSON(http.StatusBadRequest, ErrorResp{
			Error: ErrValidateCommitment.Error(),
		})
	}

	log.Info("holder commitment validated")

	d, err := time.Parse(time.DateOnly, req.Profile.DateOfBirth)
	if err != nil {
		log.WithError(err).Error(ErrParsDate)
		return c.JSON(http.StatusBadRequest, ErrorResp{
			Error: ErrParsReq.Error(),
		})
	}

	code, err := iso.I3166().ALPHA2().GetAlpha3(req.Profile.Nationality)
	if err != nil {
		log.WithError(err).Error(ErrParsNationality)
		return c.JSON(http.StatusBadRequest, ErrorResp{
			Error: ErrParsNationality.Error(),
		})
	}

	inputs := zkcertificate.KYCInputs{
		Surname:      req.Profile.Firstname,
		Forename:     req.Profile.Lastname,
		YearOfBirth:  uint16(d.Year()),
		MonthOfBirth: uint8(d.Month()),
		DayOfBirth:   uint8(d.Day()),
		Citizenship:  code,
		Postcode:     req.Profile.Postcode,
		Country:      code,
	}

	cert, err := h.generator.CreateZKCert(holderCommitment, inputs)
	if err != nil {
		log.WithError(err).Error(ErrCertGenerating)
		return c.JSON(http.StatusInternalServerError, ErrorResp{
			Error: ErrCertGenerating.Error(),
		})
	}

	hc := stripToSix(cert.HolderCommitment)

	log.
		WithField("holderCommitment", hc).
		WithField("userID", req.UserID).
		WithField("contentHash", cert.ContentHash).
		Info("cert created")

	callback := func(issuedCert *zkcertificate.IssuedCertificate[zkcertificate.KYCContent], err error) {
		log.WithField("holderCommitment", hc).
			WithField("userID", req.UserID).
			Info("certificate issued")

		encryptedCert, err := zkcert.EncryptKYCzkCert(holderCommitment, issuedCert)
		if err != nil {
			log.WithError(err).Error("encrypting cert")
			return
		}

		log.WithField("holderCommitment", hc).
			WithField("userID", req.UserID).
			Info("cert encrypted")

		b, err := json.Marshal(encryptedCert)
		if err != nil {
			log.WithError(err).Error("marshaling cert")
			return
		}
		if err = addCertToDB(h.inMem, req.UserID, b); err != nil {
			log.WithError(err)
			return
		}

		log.WithField("holderCommitment", hc).
			WithField("userID", req.UserID).
			Info("certificate added to db")
	}

	err = h.generator.AddZKCertToQueue(context.Background(), cert, callback)
	if err != nil {
		log.WithError(err).Error(ErrAddCertToQueue)
		return c.JSON(http.StatusInternalServerError, ErrorResp{
			Error: ErrAddCertToQueue.Error(),
		})
	}

	// set nil cert to userID key means
	// that certificate status is pending
	err = addCertToDB(h.inMem, req.UserID, nil)
	if err != nil {
		log.WithError(err).Error(ErrAddCertToDB)
		return c.JSON(http.StatusInternalServerError, ErrorResp{
			Error: ErrAddCertToDB.Error(),
		})
	}

	return c.JSON(http.StatusOK, GenerateCertResponse{
		Status: CertificateStatusPending,
	})
}

func (h *Handlers) GetCert(c echo.Context) error {
	var req GetCertRequest

	if err := c.Bind(&req); err != nil {
		log.WithError(err).Error("bind get cert request")
		return c.JSON(http.StatusBadRequest, ErrorResp{
			Error: ErrParsReq.Error(),
		})
	}

	log.Printf("REQUEST: %+v", req)

	certificate, err := readCertFromDB(h.inMem, req.UserID)
	if err != nil {
		log.WithError(err).Error(ErrReadCertStatus)
		return c.JSON(http.StatusInternalServerError, ErrorResp{
			Error: fmt.Sprintf("%v:%v", ErrReadCertStatus, err),
		})
	}

	// if userID key exists but certificate is empty
	// means certificate status is pending
	if certificate == "" {
		return c.JSON(http.StatusOK, GetCertResponse{
			Certificate: nil,
			Status:      CertificateStatusPending,
		})
	}

	return c.JSON(http.StatusOK, GetCertResponse{
		Certificate: json.RawMessage(certificate),
		Status:      CertificateStatusDone,
	})
}
