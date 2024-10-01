package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/dgraph-io/badger/v4"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/galactica-corp/guardians-sdk/pkg/keymanagement"
	"github.com/iden3/go-iden3-crypto/babyjub"
	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/swissborg/galactica-kyc-guardian/config"
	"github.com/swissborg/galactica-kyc-guardian/internal/api"
	"github.com/swissborg/galactica-kyc-guardian/internal/zkcert"
)

func main() {
	log.SetFormatter(&log.JSONFormatter{})

	log.Info("api service init...")
	defer log.Info("api service stop")

	ctx, cancelCancel := context.WithCancel(context.Background())

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	if err := godotenv.Load(".env"); err != nil {
		var pathError *fs.PathError
		if !errors.As(err, &pathError) {
			log.Fatalf("parsing .env file: %v", err)
		}
	}

	configPath := os.Getenv("CONFIG_PATH")
	ethereumPrivateKey := os.Getenv("PRIVATE_KEY")
	certSigningKey := os.Getenv("SIGNING_KEY")

	yamlFile, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("yamlFile.Get err #%v .path: %s", err, configPath)
	}

	cfg := config.Config{}
	err = yaml.Unmarshal(yamlFile, &cfg)
	if err != nil {
		log.Fatalf("unmarshal: %v", err)
	}

	providerKey, err := crypto.HexToECDSA(ethereumPrivateKey)
	if err != nil {
		log.Fatalf("prepare provider key: %v", err)
	}

	signingKey, err := prepareBabyJubSigningKey(certSigningKey, providerKey)
	if err != nil {
		log.Fatalf("prepare signing key: %v", err)
	}

	certGenerator, err := zkcert.NewService(
		providerKey,
		signingKey,
		cfg.RegistryAddress,
		cfg.Node,
		cfg.MerkleProofService.URL,
		cfg.MerkleProofService.TLS,
	)
	if err != nil {
		log.Fatalf("failed to create cert generator %v", err)
	}

	opt := badger.DefaultOptions("").WithInMemory(true)
	db, err := badger.Open(opt)
	if err != nil {
		log.Fatalf("failed to open badger %v", err)
	}
	defer db.Close()

	server := api.NewServer(certGenerator, db)

	go func() {
		if err := server.Start(cfg.APIConf); err != nil && (!errors.Is(err, http.ErrServerClosed)) {
			log.WithError(err).Fatal("shutting down the server")
		}
	}()

	waiting := make(chan struct{})
	go func() {
		defer close(waiting)
		select {
		case <-quit:
			log.Info("Gracefully stopping…")
			cancelCancel()

			if err := server.Stop(); err != nil {
				log.WithError(err).Fatal()
			}
		case <-ctx.Done():
			return
		}
	}()
	<-waiting
	log.Info("🏁 finished.")
}

func prepareBabyJubSigningKey(certSigningKey string, privateKey *ecdsa.PrivateKey) (babyjub.PrivateKey, error) {
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
		signingKey, err = keymanagement.DeriveEdDSAKeyFromEthereumPrivateKey(privateKey)
		if err != nil {
			return signingKey, fmt.Errorf("inferring signing key: %w", err)
		}
	}
	return signingKey, nil
}
