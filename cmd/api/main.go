package main

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/dgraph-io/badger/v4"
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
	privKey := os.Getenv("PRIVATE_KEY")
	signingKey := os.Getenv("SIGNING_KEY")

	yamlFile, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("yamlFile.Get err #%v .path: %s", err, configPath)
	}

	cfg := config.Config{}
	err = yaml.Unmarshal(yamlFile, &cfg)
	if err != nil {
		log.Fatalf("unmarshal: %v", err)
	}

	certGenerator, err := zkcert.NewService(
		privKey,
		cfg.RegistryAddress,
		cfg.Node,
		cfg.MerkleProofService.URL,
		cfg.MerkleProofService.TLS,
		signingKey,
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
