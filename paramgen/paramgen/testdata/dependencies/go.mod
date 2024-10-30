module example.com/test2

go 1.22.4

require (
	github.com/conduitio/conduit-commons v0.3.0
	example.com/test v0.0.0
)

require (
	github.com/goccy/go-json v0.10.3 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
)

replace example.com/test => ../basic