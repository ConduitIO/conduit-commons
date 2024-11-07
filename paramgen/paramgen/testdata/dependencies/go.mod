module example.com/test2

go 1.23

toolchain go1.23.2

require (
	example.com/test v0.0.0
	github.com/conduitio/conduit-commons v0.3.0
)

require (
	github.com/goccy/go-json v0.10.3 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	google.golang.org/protobuf v1.35.1 // indirect
)

replace example.com/test => ../basic

replace github.com/conduitio/conduit-commons => ../../../../
