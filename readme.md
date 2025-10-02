# Galactica KYC Guardian

Backend service that generates encrypted zk KYC certificates for the Galactica blockchain based on SwissBorg KYC data.

## Requirements

To run this project, you need to have the following secrets passed to the application via environment variables:

- `BLOCKCHAIN_NODE`: URL of the blockchain node, if it is not the public one
- `CONFIG_PATH`: Path to the config file
- `PRIVATE_KEY`: ECDSA private key for blockchain interactions
- `SIGNING_KEY`: EdDSA private key for ZK certificate signing

These can be set in a `.env` file for local development.

For production environment, follow this guide on [setup to become a guardian](https://docs.galactica.com/galactica-developer-documentation/guardian-guide/setup-to-become-a-guardian).

> [!WARNING]
> The private key should be whitelisted in the Guardians Registry to be able to sign transactions.
>
> Guardians Registry contract address for Cassiopeia is `0xBcf3641f4A4a0fF102F5800291f09e2Bd87c0a53`.

## Configuration

A YAML configuration file is required with the following structure:

```yaml
APIConf:
  Host: "0.0.0.0"
  Port: 8080

# Galactica node URL
Node: https://galactica-cassiopeia.g.alchemy.com/public

# zk KYC Registry contract address on Galactica
RegistryAddress: 0xFe35EF5D1E8488a6b06BD35434613917e7d9760f # Cassiopeia

# Merkle proof service, can be self-hosted: https://github.com/galactica-corp/merkle-proof-service
MerkleProofService:
  URL: grpc-merkle-proof-service.galactica.com:443
  TLS: true
```

## Setup

To provide the required secrets, you can create a `.env` file in the root of the project:

```sh
make config # copy .sample.env to .env
```

Then update the configurations in your local `.env` file.

## API

To start the API server, run:

```sh
make api
```

The API server will be available at `http://localhost:8080`.

## Endpoints

This endpoint starts the computation of a new certificate, taking as input the user's profile.

```
POST /cert/generate
```

Request body:

```json
{
  "encryption_pub_key": "OEotdsfEuoiqM7ob2KJEQemhWodn87hZNFv890q4xGw=",
  "holder_commitment": "4586425042444163335895417167611444541749813513569901646582116352074512113476",
  "user_id": "12345",
  "profile": {
    "firstname": "Bob",
    "lastname": "Norman",
    "date_of_birth": "2006-01-02",
    "nationality": "CH",
    "postcode": "1006"
  }
}
```

Response:

```json
{
  "status": "PENDING"
}
```

This endpoint get the status of the certificate and its value when computed.

```
POST /cert/get
```

Request body:

```json
{
  "user_id":"12345"
}
```

Response:

```json
{
  "status": "DONE",
  "certificate":{}
}
```
