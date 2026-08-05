// TODO: Поменяй имя модуля github.com/omaigo88 на своё и обнови все импорты
module github.com/omaigo88/inventory

go 1.26.0

require (
	github.com/google/uuid v1.6.0
	github.com/omaigo88/shared v0.0.0-00010101000000-000000000000
	google.golang.org/grpc v1.79.2
	google.golang.org/protobuf v1.36.11
)

require (
	go.opentelemetry.io/otel/sdk v1.42.0 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
)

replace github.com/omaigo88/shared => ../shared
