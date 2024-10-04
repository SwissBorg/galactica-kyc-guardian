package zkcert

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"time"

	merkleproof "github.com/Galactica-corp/merkle-proof-service/gen/galactica/merkle"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/galactica-corp/guardians-sdk/cmd"
	"github.com/galactica-corp/guardians-sdk/pkg/contracts"
	"github.com/galactica-corp/guardians-sdk/pkg/merkle"
	"github.com/galactica-corp/guardians-sdk/pkg/zkcertificate"
	"github.com/iden3/go-iden3-crypto/babyjub"
	log "github.com/sirupsen/logrus"
	"github.com/swissborg/galactica-kyc-guardian/internal/tq"
)

var errRequiresRetry = errors.New("requires a retry")

type Service struct {
	EthClient         *ethclient.Client
	merkleProofClient merkleproof.QueryClient
	providerKey       *ecdsa.PrivateKey
	registry          *contracts.ZkCertificateRegistry
	registryAddress   common.Address
	rpcURL            string
	signingKey        babyjub.PrivateKey
	taskQueue         *tq.Queue
}

func NewService(
	providerKey *ecdsa.PrivateKey,
	signingKey babyjub.PrivateKey,
	registryAddress common.Address,
	rpcURL string,
	merkleProofURL string,
	merkleProofTLS bool,
) (*Service, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	ethClient, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return nil, fmt.Errorf("connect to ethereum node: %v", err)
	}

	merkleProofClient, err := merkle.ConnectToMerkleProofService(merkleProofURL, merkleProofTLS)
	if err != nil {
		return nil, fmt.Errorf("connect to merkle proof service: %v", err)
	}

	registry, err := contracts.NewZkCertificateRegistry(registryAddress, ethClient)
	if err != nil {
		return nil, fmt.Errorf("load record registry: %w", err)
	}

	taskQueue := tq.NewQueue(100)

	return &Service{
		rpcURL:            rpcURL,
		EthClient:         ethClient,
		merkleProofClient: merkleProofClient,
		providerKey:       providerKey,
		registry:          registry,
		registryAddress:   registryAddress,
		signingKey:        signingKey,
		taskQueue:         taskQueue,
	}, nil
}

func (s *Service) Close() {
	s.taskQueue.Wait()
	s.EthClient.Close()
}

func (s *Service) CreateZKCert(
	holderCommitment zkcertificate.HolderCommitment,
	inputs zkcertificate.KYCInputs,
) (*zkcertificate.Certificate[zkcertificate.KYCContent], error) {
	if err := inputs.Validate(); err != nil {
		return nil, fmt.Errorf("validate inputs: %w", err)
	}

	content, err := inputs.FFEncode()
	if err != nil {
		return nil, fmt.Errorf("encode inputs to finite field: %w", err)
	}

	/* one year expiration */
	expirationDate := time.Now().AddDate(1, 0, 0)

	return cmd.CreateZKCert(content, holderCommitment, s.signingKey, expirationDate)
}

func (s *Service) AddZKCertToQueue(
	ctx context.Context,
	certificate zkcertificate.Certificate[zkcertificate.KYCContent],
	callback func(zkcertificate.IssuedCertificate[zkcertificate.KYCContent], error),
) {
	s.taskQueue.Add(tq.NewTask(
		func() (zkcertificate.IssuedCertificate[zkcertificate.KYCContent], error) {
			// This is to give the merkle proof service time to "see" the latest merkle proof root
			time.Sleep(3 * time.Second)

			_, issuedCert, err := cmd.IssueZKCert(ctx, certificate, s.EthClient, s.merkleProofClient, s.registryAddress, s.providerKey)
			if err != nil {
				log.WithError(err).Error("issue zk certificate")
				return zkcertificate.IssuedCertificate[zkcertificate.KYCContent]{}, err
			}

			return issuedCert, err
		},
		callback,
		errRequiresRetry,
	))
}

func (s *Service) EncryptZKCert(
	holderCommitment zkcertificate.HolderCommitment,
	issuedCert zkcertificate.IssuedCertificate[zkcertificate.KYCContent],
) (zkcertificate.EncryptedCertificate, error) {
	return cmd.EncryptZKCert(issuedCert, holderCommitment)
}
