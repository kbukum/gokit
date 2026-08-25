module github.com/kbukum/gokit/storage/s3

go 1.26.0

toolchain go1.26.6

require (
	github.com/aws/aws-sdk-go-v2 v1.43.6
	github.com/aws/aws-sdk-go-v2/config v1.32.37
	github.com/aws/aws-sdk-go-v2/credentials v1.19.36
	github.com/aws/aws-sdk-go-v2/service/s3 v1.107.2
	github.com/kbukum/gokit v0.3.0-alpha.1
	github.com/kbukum/gokit/storage v0.3.0-alpha.1
)

require (
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.18 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.37 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.37 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.37 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.38 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.30 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.37 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.38 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.5.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.33.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.38.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.45.6 // indirect
	github.com/aws/smithy-go v1.27.8 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.29.0 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.22 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/client_golang v1.24.1 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.70.1 // indirect
	github.com/prometheus/procfs v0.21.1 // indirect
	github.com/rs/zerolog v1.35.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.45.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc v0.21.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp v0.21.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.45.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.45.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.45.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.45.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.45.0 // indirect
	go.opentelemetry.io/otel/log v0.21.0 // indirect
	go.opentelemetry.io/otel/metric v1.45.0 // indirect
	go.opentelemetry.io/otel/sdk v1.45.0 // indirect
	go.opentelemetry.io/otel/sdk/log v0.21.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.45.0 // indirect
	go.opentelemetry.io/otel/trace v1.45.0 // indirect
	go.opentelemetry.io/proto/otlp v1.11.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260803160001-6ac0973c030d // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260807164820-c8921c73eeea // indirect
	google.golang.org/grpc v1.83.0 // indirect
	google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af // indirect
)

replace (
	github.com/kbukum/gokit => ../../
	github.com/kbukum/gokit/storage => ../
)
