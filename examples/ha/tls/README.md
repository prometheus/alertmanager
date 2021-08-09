# TLS Transport Config Example

## Usage
1. Install dependencies:
   1. `go install github.com/cloudflare/cfssl/cmd/cfssl`
   2. `go install github.com/mattn/goreman`
2. Build Alertmanager (root of repository):
   1. `go mod download`
   1. `make build`.
2. `make start` (inside this directory).

## Testing
1. Start the cluster (as explained above)
2. Navigate to one of the Alertmanager instances at `localhost:9093`.
3. Create a silence.
4. Navigate to the other Alertmanager instance at `localhost:9094`.
5. Observe that the silence created in the other Alertmanager instance has been synchronized over to this instance.
6. Repeat.
