# Galactica KYC Guardian


## Setup

Run the server directly:
```bash
go run main.go
```

Build and run the Docker image:
```bash
docker build -t galactica-kyc-guardian .
docker run --rm -p 8080:8080 -t galactica-kyc-guardian
```

## API Contract

The HTTP server is binded to port 8080.

### Proof endpoint
```
POST /proof
```

Request body:
```json
{
  "profile": {
    "firstname": "John",
    "lastname": "Doe",
    "date_of_birth": "1987-01-01",
    "nationality": "FR",
    "residential_address": {
      "country": "CH",
      "postcode": "1003"
    }
  }
}
```

Response body:
```json
{
  "proof": "<encoded proof>" 
}
```

This endpoint computes the ZK-proof, taking as input the user's profile.


### Health endpoint

```
GET /health
```

A standard health endpoint is exposed, to be used in health probes.
It returns a `200 OK` with an empty body. 


## Environment

List of configuration variables:
- XXX:XXX
