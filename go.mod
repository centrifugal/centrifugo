module main

go 1.13

require (
	github.com/FZambia/statik v0.1.2-0.20180217151304-b9f012bb2a1b
	github.com/FZambia/viper-lite v0.0.0-20171108064948-d5a31e6aa18b
	github.com/centrifugal/centrifuge v0.2.2
	github.com/gogo/protobuf v1.3.1 // indirect
	github.com/gorilla/securecookie v1.1.1 // indirect
	github.com/mattn/go-isatty v0.0.3
	github.com/mitchellh/mapstructure v1.1.2 // indirect
	github.com/pelletier/go-toml v1.6.0 // indirect
	github.com/prometheus/client_golang v0.9.2
	github.com/rs/zerolog v1.17.2
	github.com/satori/go.uuid v1.2.0
	github.com/spf13/cast v1.3.0 // indirect
	github.com/spf13/cobra v0.0.3
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	golang.org/x/crypto v0.0.0-20191128160524-b544559bb6d1
	google.golang.org/genproto v0.0.0-20191115221424-83cc0476cb11 // indirect
	google.golang.org/grpc v1.19.0
	gopkg.in/yaml.v2 v2.2.7 // indirect
	internal/admin v0.0.0-00010101000000-000000000000
	internal/api v0.0.0-00010101000000-000000000000
	internal/health v0.0.0-00010101000000-000000000000
	internal/metrics/graphite v0.0.0-00010101000000-000000000000
	internal/middleware v0.0.0-00010101000000-000000000000
	internal/webui v0.0.0-00010101000000-000000000000
)

replace (
	internal/admin => ./internal/admin
	internal/api => ./internal/api
	internal/health => ./internal/health
	internal/metrics/graphite => ./internal/metrics/graphite
	internal/middleware => ./internal/middleware
	internal/webui => ./internal/webui
)
