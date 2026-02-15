package di

// PkgNames defines the base layer component names for the pkg bootstrap layer.
// Projects embed this struct in their own shared/service DI names.
type PkgNames struct {
	// Core infrastructure
	Config           string
	Logger           string
	Database         string
	Redis            string
	ServiceRegistry  string
	ServiceDiscovery string

	// HTTP/gRPC servers
	HTTPServer    string
	GRPCServer    string
	UnifiedServer string

	// Messaging
	KafkaProducer      string
	KafkaConsumer      string
	KafkaConsumerGroup string
}

// Pkg contains all component names for the pkg bootstrap layer.
var Pkg = PkgNames{
	// Core infrastructure
	Config:           "config",
	Logger:           "logger",
	Database:         "database",
	Redis:            "redis",
	ServiceRegistry:  "service_registry",
	ServiceDiscovery: "service_discovery",

	// HTTP/gRPC servers
	HTTPServer:    "http_server",
	GRPCServer:    "grpc_server",
	UnifiedServer: "unified_server",

	// Messaging
	KafkaProducer:      "kafka_producer",
	KafkaConsumer:      "kafka_consumer",
	KafkaConsumerGroup: "kafka_consumer_group",
}
