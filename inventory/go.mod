// TODO: Change the module name github.com/omaigo88 to your own and update all imports
module github.com/omaigo88/inventory

go 1.26.0

require (
	github.com/avito-tech/go-transaction-manager/drivers/pgxv5/v2 v2.0.4
	github.com/avito-tech/go-transaction-manager/trm/v2 v2.0.4
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.11.0
	github.com/omaigo88/shared v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.12.1
	google.golang.org/grpc v1.79.2
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	go.opentelemetry.io/otel/sdk v1.42.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
)

replace github.com/omaigo88/shared => ../shared
