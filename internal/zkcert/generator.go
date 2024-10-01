package zkcert

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"time"

	merkleproof "github.com/Galactica-corp/merkle-proof-service/gen/galactica/merkle"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/galactica-corp/guardians-sdk/pkg/contracts"
	"github.com/galactica-corp/guardians-sdk/pkg/encryption"
	"github.com/galactica-corp/guardians-sdk/pkg/merkle"
	"github.com/galactica-corp/guardians-sdk/pkg/zkcertificate"
	"github.com/iden3/go-iden3-crypto/babyjub"
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
	ethereumPrivateKey string,
	registryAddress common.Address,
	rpcURL string,
	merkleProofURL string,
	merkleProofTLS bool,
	certSigningKey string,
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

	providerKey, err := crypto.HexToECDSA(ethereumPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("prepare provider key: %w", err)
	}

	signingKey, err := prepareBabyJubSigningKey(certSigningKey, ethereumPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("prepare signing key: %w", err)
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

	certificateContent, err := inputs.FFEncode()
	if err != nil {
		return nil, fmt.Errorf("encode inputs to finite field: %w", err)
	}

	contentHash, err := certificateContent.Hash()
	if err != nil {
		return nil, fmt.Errorf("hash certificate content: %w", err)
	}

	signature, err := zkcertificate.SignCertificate(s.signingKey, contentHash, holderCommitment.CommitmentHash)
	if err != nil {
		return nil, fmt.Errorf("sign certificate: %w", err)
	}

	salt, err := generateRandomSalt()
	if err != nil {
		return nil, fmt.Errorf("generate random salt: %w", err)
	}

	/* one year expiration */
	expirationDate := time.Now().AddDate(1, 0, 0)

	certificate, err := zkcertificate.New(
		holderCommitment.CommitmentHash,
		certificateContent,
		s.signingKey.Public(),
		signature,
		salt,
		expirationDate,
	)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	return certificate, nil
}

func (s *Service) AddZKCertToQueue(
	ctx context.Context,
	certificate *zkcertificate.Certificate[zkcertificate.KYCContent],
	callback func(*zkcertificate.IssuedCertificate[zkcertificate.KYCContent], error),
) error {
	err := s.ensureProviderIsGuardian(crypto.PubkeyToAddress(s.providerKey.PublicKey))
	if err != nil {
		return fmt.Errorf("ensure provider is guardian: %w", err)
	}

	s.taskQueue.Add(tq.NewTask(
		func() (struct{}, error) {
			err := s.registerToQueue(ctx, certificate.LeafHash)
			return struct{}{}, err
		},
		func(result struct{}, err error) {
			s.addZKCertToQueue(ctx, certificate, callback)
		},
		errRequiresRetry,
	))

	return nil
}

func (s *Service) addZKCertToQueue(
	ctx context.Context,
	certificate *zkcertificate.Certificate[zkcertificate.KYCContent],
	callback func(*zkcertificate.IssuedCertificate[zkcertificate.KYCContent], error),
) {
	myTurn, err := s.registry.CheckZkCertificateHashInQueue(&bind.CallOpts{Context: ctx}, certificate.LeafHash.Bytes32())
	if err != nil {
		callback(nil, fmt.Errorf("retrieve zkCertificate hash to check: %w", err))
		return
	}

	if !myTurn {
		callback(nil, errRequiresRetry)
		return
	}

	s.taskQueue.Add(tq.NewTask(
		func() (*zkcertificate.IssuedCertificate[zkcertificate.KYCContent], error) {
			// This is to give the merkle proof service time to "see" the new zkCertificate
			time.Sleep(3 * time.Second)

			return s.issueZKCert(ctx, certificate)
		},
		callback,
		errRequiresRetry,
	))
}

func (s *Service) ensureProviderIsGuardian(providerAddress common.Address) error {
	guardianRegistryAddress, err := s.registry.GuardianRegistry(&bind.CallOpts{})
	if err != nil {
		return fmt.Errorf("retrieve guardian registry address: %w", err)
	}

	guardianRegistry, err := contracts.NewGuardianRegistry(guardianRegistryAddress, s.EthClient)
	if err != nil {
		return fmt.Errorf("bind guardian registry contract: %w", err)
	}

	guardian, err := guardianRegistry.Guardians(&bind.CallOpts{}, providerAddress)
	if err != nil {
		return fmt.Errorf("retrieve guardian whitelist status: %w", err)
	}

	if !guardian.Whitelisted {
		return fmt.Errorf("provider %s is not a guardian yet", providerAddress)
	}

	return nil
}

func (s *Service) registerToQueue(ctx context.Context, leafHash zkcertificate.Hash) error {
	auth, err := s.getAuth(ctx)
	if err != nil {
		return fmt.Errorf("create transaction signer from private key: %w", err)
	}

	tx, err := s.registry.RegisterToQueue(auth, leafHash.Bytes32())
	if err != nil {
		exists, checkErr := s.registry.CheckZkCertificateHashInQueue(&bind.CallOpts{}, leafHash.Bytes32())
		if checkErr != nil {
			return fmt.Errorf("register to queue failed: %w, also failed to check if zkCertificateHash is in queue: %w", err, checkErr)
		}
		if exists {
			return nil
		}
		return fmt.Errorf("register to queue failed: %w", err)
	}

	if tx != nil {
		receipt, err := bind.WaitMined(ctx, s.EthClient, tx)
		if err != nil {
			return fmt.Errorf("wait until queue registration transaction is mined: %w", err)
		}
		if receipt.Status == 0 {
			return fmt.Errorf("queue registration transaction %q failed", receipt.TxHash)
		}
	}

	return nil
}

func (s *Service) issueZKCert(
	ctx context.Context,
	certificate *zkcertificate.Certificate[zkcertificate.KYCContent],
) (*zkcertificate.IssuedCertificate[zkcertificate.KYCContent], error) {
	auth, err := s.getAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("create transaction signer from private key: %w", err)
	}

	emptyLeafIndex, proof, err := merkle.GetEmptyLeafProof(ctx, s.merkleProofClient, s.registryAddress.Hex())
	if err != nil {
		return nil, fmt.Errorf("find empty tree leaf: %v", err)
	}

	tx, err := s.registry.AddZkCertificate(
		auth,
		big.NewInt(int64(emptyLeafIndex)),
		certificate.LeafHash.Bytes32(),
		encodeMerkleProof(proof),
	)
	if err != nil {
		return nil, fmt.Errorf("construct transaction to add record to registry: %v", err)
	}

	if receipt, err := bind.WaitMined(ctx, s.EthClient, tx); err != nil {
		return nil, fmt.Errorf("wait until transaction is mined: %v", err)
	} else if receipt.Status == 0 {
		return nil, fmt.Errorf("transaction %q falied", receipt.TxHash)
	}

	issuedCert := build(*certificate, s.registryAddress, int(emptyLeafIndex), proof, tx.ChainId())

	return issuedCert, nil
}

func encodeMerkleProof(proof merkle.Proof) [][32]byte {
	res := make([][32]byte, len(proof.Path))

	for i, node := range proof.Path {
		res[i] = node.Value.Bytes32()
	}

	return res
}

func build(
	certificate zkcertificate.Certificate[zkcertificate.KYCContent],
	registryAddress common.Address,
	leafIndex int,
	proof merkle.Proof,
	chainID *big.Int,
) *zkcertificate.IssuedCertificate[zkcertificate.KYCContent] {
	return &zkcertificate.IssuedCertificate[zkcertificate.KYCContent]{
		Certificate: certificate,
		Registration: zkcertificate.RegistrationDetails{
			Address:   registryAddress,
			Revocable: true,
			LeafIndex: leafIndex,
			ChainID:   chainID,
		},
		MerkleProof: proof,
	}
}

type EncryptedCert struct {
	encryption.EncryptedMessage `json:",inline"`
	HolderCommitment            zkcertificate.Hash `json:"holderCommitment"`
}

func EncryptKYCzkCert(
	holderCommitment zkcertificate.HolderCommitment,
	certificate any,
) (*EncryptedCert, error) {
	encryptedMessage, err := encryption.EncryptWithPadding([32]byte(holderCommitment.EncryptionKey), certificate)
	if err != nil {
		return nil, fmt.Errorf("encrypt certificate: %w", err)
	}

	return &EncryptedCert{
		EncryptedMessage: encryptedMessage,
		HolderCommitment: holderCommitment.CommitmentHash,
	}, err
}

func (s *Service) getAuth(ctx context.Context) (*bind.TransactOpts, error) {
	chainID, err := s.EthClient.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("retrieve chain id: %w", err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(s.providerKey, chainID)
	if err != nil {
		return nil, fmt.Errorf("create transaction signer from private key: %w", err)
	}

	return auth, nil
}

func generateRandomSalt() (int64, error) {
	salt, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64)) // [0, MaxInt64)
	if err != nil {
		return 0, fmt.Errorf("generate random salt: %w", err)
	}

	randomSalt := salt.Int64() + 1 // [1, MaxInt64]

	return randomSalt, nil
}

func inferBabyJubSigningKeyFromEthereumPrivateKey(ethereumPrivateKey string) (babyjub.PrivateKey, error) {
	privateKey := []byte(ethereumPrivateKey)
	res := make([]byte, hex.DecodedLen(len(privateKey)))

	var byteErr hex.InvalidByteError
	if _, err := hex.Decode(res, privateKey); errors.As(err, &byteErr) {
		return babyjub.PrivateKey{}, fmt.Errorf("invalid hex character %q in private key", byte(byteErr))
	} else if err != nil {
		return babyjub.PrivateKey{}, errors.New("invalid hex data for private key")
	}

	signingKey := babyjub.PrivateKey(res)

	return signingKey, nil
}

func prepareBabyJubSigningKey(certSigningKey string, ethereumPrivateKey string) (babyjub.PrivateKey, error) {
	var signingKey babyjub.PrivateKey
	if certSigningKey != "" {
		keyBytes, err := hex.DecodeString(certSigningKey)
		if err != nil {
			return signingKey, fmt.Errorf("invalid hex string: %w", err)
		}
		if len(keyBytes) != 32 {
			return signingKey, fmt.Errorf("invalid key length: expected 32 bytes, got %d", len(keyBytes))
		}
		copy(signingKey[:], keyBytes)
	} else {
		var err error
		signingKey, err = inferBabyJubSigningKeyFromEthereumPrivateKey(ethereumPrivateKey)
		if err != nil {
			return signingKey, fmt.Errorf("inferring signing key: %w", err)
		}
	}
	return signingKey, nil
}
