package logger

import (
	"strings"
	"time"
)

// ComponentRegistry tracks components during bootstrap for summary display.
type ComponentRegistry struct {
	startTime      time.Time
	infrastructure []InfraComponent
	services       []ServiceComponent
	repositories   []RepositoryComponent
	clients        []ClientComponent
	handlers       []HandlerComponent
	consumers      []ConsumerComponent
	// apiPrefix holds the configured API prefix (eg: /api/v1)
	apiPrefix string
}

// InfraComponent represents an infrastructure dependency (database, cache, broker, etc.).
type InfraComponent struct {
	Name    string
	Type    string // "database", "kafka", "redis", "server"
	Status  string // "active", "inactive", "error"
	Details string
}

// ServiceComponent represents a business-logic service.
type ServiceComponent struct {
	Name         string
	Status       string // "lazy", "initialized", "active"
	Dependencies []string
}

// RepositoryComponent represents a data-access repository.
type RepositoryComponent struct {
	Name   string
	Store  string // "PostgreSQL", "Redis", etc.
	Status string
}

// ClientComponent represents an external client (gRPC, HTTP, etc.).
type ClientComponent struct {
	Name   string
	Target string // "gRPC:9090", "HTTP:8080"
	Status string
}

// HandlerComponent represents an HTTP handler/route.
type HandlerComponent struct {
	Method  string // "GET", "POST", etc.
	Path    string
	Handler string
}

// ConsumerComponent represents a message consumer (e.g. Kafka).
type ConsumerComponent struct {
	Name       string
	Group      string
	Topic      string
	Partitions int
	Status     string
}

// ComponentRegistryInstance is the global component registry.
var ComponentRegistryInstance = NewComponentRegistry()

// NewComponentRegistry creates a new component registry.
func NewComponentRegistry() *ComponentRegistry {
	return &ComponentRegistry{
		startTime:      time.Now(),
		infrastructure: make([]InfraComponent, 0),
		services:       make([]ServiceComponent, 0),
		repositories:   make([]RepositoryComponent, 0),
		clients:        make([]ClientComponent, 0),
		handlers:       make([]HandlerComponent, 0),
		consumers:      make([]ConsumerComponent, 0),
	}
}

// SetAPIPrefix sets the API prefix (for example "/api/v1") so route grouping
// can be done using the configured prefix instead of hard-coded values.
func (r *ComponentRegistry) SetAPIPrefix(prefix string) {
	r.apiPrefix = strings.TrimRight(prefix, "/")
}

// APIPrefix returns the configured API prefix.
func (r *ComponentRegistry) APIPrefix() string {
	return r.apiPrefix
}

// StartTime returns the registry creation time (bootstrap start).
func (r *ComponentRegistry) StartTime() time.Time {
	return r.startTime
}

// RegisterInfrastructure registers an infrastructure component.
func (r *ComponentRegistry) RegisterInfrastructure(name, componentType, status, details string) {
	r.infrastructure = append(r.infrastructure, InfraComponent{
		Name:    name,
		Type:    componentType,
		Status:  status,
		Details: details,
	})
}

// RegisterService registers a service component.
func (r *ComponentRegistry) RegisterService(name, status string, dependencies []string) {
	r.services = append(r.services, ServiceComponent{
		Name:         name,
		Status:       status,
		Dependencies: dependencies,
	})
}

// RegisterRepository registers a repository component.
func (r *ComponentRegistry) RegisterRepository(name, store, status string) {
	r.repositories = append(r.repositories, RepositoryComponent{
		Name:   name,
		Store:  store,
		Status: status,
	})
}

// RegisterClient registers an external client.
func (r *ComponentRegistry) RegisterClient(name, target, status string) {
	r.clients = append(r.clients, ClientComponent{
		Name:   name,
		Target: target,
		Status: status,
	})
}

// RegisterHandler registers an HTTP handler.
func (r *ComponentRegistry) RegisterHandler(method, path, handler string) {
	r.handlers = append(r.handlers, HandlerComponent{
		Method:  method,
		Path:    path,
		Handler: handler,
	})
}

// RegisterConsumer registers a message consumer.
func (r *ComponentRegistry) RegisterConsumer(name, groupID, topic string, partitions int, status string) {
	r.consumers = append(r.consumers, ConsumerComponent{
		Name:       name,
		Group:      groupID,
		Topic:      topic,
		Partitions: partitions,
		Status:     status,
	})
}

// Infrastructure returns all registered infrastructure components.
func (r *ComponentRegistry) Infrastructure() []InfraComponent {
	return r.infrastructure
}

// Services returns all registered service components.
func (r *ComponentRegistry) Services() []ServiceComponent {
	return r.services
}

// Repositories returns all registered repository components.
func (r *ComponentRegistry) Repositories() []RepositoryComponent {
	return r.repositories
}

// Clients returns all registered client components.
func (r *ComponentRegistry) Clients() []ClientComponent {
	return r.clients
}

// Handlers returns all registered handler components.
func (r *ComponentRegistry) Handlers() []HandlerComponent {
	return r.handlers
}

// SetHandlers replaces the handler list (useful when collecting routes dynamically).
func (r *ComponentRegistry) SetHandlers(handlers []HandlerComponent) {
	r.handlers = handlers
}

// Consumers returns all registered consumer components.
func (r *ComponentRegistry) Consumers() []ConsumerComponent {
	return r.consumers
}
