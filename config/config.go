package config

import "github.com/ethereum/go-ethereum/common"

type Config struct {
	APIConf            APIConf            `yaml:"APIConf"`
	RegistryAddress    common.Address     `yaml:"RegistryAddress"`
	Node               string             `yaml:"Node"`
	MerkleProofService MerkleProofService `yaml:"MerkleProofService"`
}

type APIConf struct {
	Port        string `yaml:"Port" default:"8081"`
	Host        string `yaml:"Host" default:"0.0.0.0"`
	CORSEnabled bool   `yaml:"CORSEnabled"`
	CORSOrigin  string `yaml:"CORSOrigin"`
}

type MerkleProofService struct {
	URL string `yaml:"URL"`
	TLS bool   `yaml:"TLS"`
}
