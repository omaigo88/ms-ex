// TODO: Change the module name github.com/omaigo88/week_1 to your own and update all imports
module github.com/omaigo88/payment

go 1.26.0

require (
	github.com/stretchr/testify v1.12.1
	google.golang.org/grpc v1.79.2
)

require (
	github.com/stretchr/objx v0.5.3 // indirect
	go.opentelemetry.io/otel/sdk v1.42.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
)

require (
	github.com/google/uuid v1.6.0
	github.com/omaigo88/shared v0.0.0-00010101000000-000000000000
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace github.com/omaigo88/shared => ../shared
