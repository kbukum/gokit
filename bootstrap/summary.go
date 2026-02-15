package bootstrap

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/skillsenselab/gokit/component"
	"github.com/skillsenselab/gokit/logger"
)

// ComponentStatus holds the tracked status of a component during bootstrap.
type ComponentStatus struct {
	Name    string
	Status  string
	Healthy bool
}

// InfrastructureInfo holds detailed infrastructure component information.
type InfrastructureInfo struct {
	Name    string
	Type    string // e.g. "database", "server", "kafka", "redis"
	Status  string
	Details string
	Port    int
	Healthy bool
}

// BusinessComponentInfo represents a business-layer component (service, repository, handler).
type BusinessComponentInfo struct {
	Name         string
	Type         string // "service", "repository", "handler"
	Status       string
	Dependencies []string
}

// RouteInfo represents a registered HTTP route.
type RouteInfo struct {
	Method  string
	Path    string
	Handler string
}

// ConsumerInfo represents a message consumer (e.g. Kafka).
type ConsumerInfo struct {
	Name   string
	Group  string
	Topic  string
	Status string
}

// ClientInfo represents an external client connection.
type ClientInfo struct {
	Name   string
	Target string
	Status string
	Type   string // "grpc", "http", etc.
}

// Summary tracks and displays the application bootstrap process.
type Summary struct {
	serviceName     string
	version         string
	startupDuration time.Duration
	components      []ComponentStatus
	infrastructure  []InfrastructureInfo
	business        []BusinessComponentInfo
	routes          []RouteInfo
	consumers       []ConsumerInfo
	clients         []ClientInfo
}

// NewSummary creates a new bootstrap summary tracker.
func NewSummary(serviceName, version string) *Summary {
	return &Summary{
		serviceName:    serviceName,
		version:        version,
		components:     make([]ComponentStatus, 0),
		infrastructure: make([]InfrastructureInfo, 0),
		business:       make([]BusinessComponentInfo, 0),
		routes:         make([]RouteInfo, 0),
		consumers:      make([]ConsumerInfo, 0),
		clients:        make([]ClientInfo, 0),
	}
}

// SetStartupDuration records the total startup time.
func (s *Summary) SetStartupDuration(d time.Duration) {
	s.startupDuration = d
}

// TrackComponent adds a component's bootstrap status to the summary.
func (s *Summary) TrackComponent(name, status string, healthy bool) {
	s.components = append(s.components, ComponentStatus{
		Name:    name,
		Status:  status,
		Healthy: healthy,
	})
}

// TrackInfrastructure adds an infrastructure component with detailed metadata.
func (s *Summary) TrackInfrastructure(name, componentType, status, details string, port int, healthy bool) {
	s.infrastructure = append(s.infrastructure, InfrastructureInfo{
		Name:    name,
		Type:    componentType,
		Status:  status,
		Details: details,
		Port:    port,
		Healthy: healthy,
	})
}

// TrackBusinessComponent records a business-layer component.
func (s *Summary) TrackBusinessComponent(name, componentType, status string, dependencies []string) {
	s.business = append(s.business, BusinessComponentInfo{
		Name:         name,
		Type:         componentType,
		Status:       status,
		Dependencies: dependencies,
	})
}

// TrackRoute records an HTTP route.
func (s *Summary) TrackRoute(method, path, handler string) {
	s.routes = append(s.routes, RouteInfo{
		Method:  method,
		Path:    path,
		Handler: handler,
	})
}

// TrackConsumer records a message consumer.
func (s *Summary) TrackConsumer(name, group, topic, status string) {
	s.consumers = append(s.consumers, ConsumerInfo{
		Name:   name,
		Group:  group,
		Topic:  topic,
		Status: status,
	})
}

// TrackClient records an external client connection.
func (s *Summary) TrackClient(name, target, status, clientType string) {
	s.clients = append(s.clients, ClientInfo{
		Name:   name,
		Target: target,
		Status: status,
		Type:   clientType,
	})
}

// DisplaySummary prints the bootstrap summary including live health from the registry.
func (s *Summary) DisplaySummary(registry *component.Registry, log *logger.Logger) {
	// Header
	fmt.Printf("\n")
	fmt.Printf("🚀 %s v%s started in %.2fs\n\n",
		s.serviceName, s.version, s.startupDuration.Seconds())

	// Infrastructure (detailed)
	if len(s.infrastructure) > 0 {
		fmt.Printf("📊 Infrastructure\n")
		for i, inf := range s.infrastructure {
			prefix := "├──"
			if i == len(s.infrastructure)-1 && len(s.components) == 0 {
				prefix = "└──"
			}
			icon := statusIcon(inf.Status, inf.Healthy)
			details := inf.Details
			if inf.Port > 0 {
				details = fmt.Sprintf("%s (:%d)", details, inf.Port)
			}
			fmt.Printf("   %s %s %s: %s\n", prefix, icon, inf.Name, details)
		}
		fmt.Printf("\n")
	}

	// Components
	if len(s.components) > 0 {
		fmt.Printf("📦 Components\n")
		healthy := 0
		for i, c := range s.components {
			prefix := "├──"
			if i == len(s.components)-1 {
				prefix = "└──"
			}
			icon := statusIcon(c.Status, c.Healthy)
			fmt.Printf("   %s %s %s (%s)\n", prefix, icon, c.Name, c.Status)
			if c.Healthy {
				healthy++
			}
		}
		fmt.Printf("\n")

		total := len(s.components)
		if healthy == total {
			fmt.Printf("✅ All components healthy (%d/%d)\n", healthy, total)
		} else {
			fmt.Printf("⚠️  Some components have issues (%d/%d healthy)\n", healthy, total)
		}
	}

	if len(s.infrastructure) == 0 && len(s.components) == 0 {
		fmt.Printf("   └── No components registered\n")
	}

	// Business layer
	if len(s.business) > 0 {
		fmt.Printf("\n💼 Business Layer\n")
		for i, b := range s.business {
			prefix := "├──"
			if i == len(s.business)-1 {
				prefix = "└──"
			}
			fmt.Printf("   %s %s [%s] (%s)\n", prefix, businessIcon(b.Type), b.Name, b.Status)
			for j, dep := range b.Dependencies {
				depPrefix := "│   ├──"
				if i == len(s.business)-1 {
					depPrefix = "    ├──"
				}
				if j == len(b.Dependencies)-1 {
					if i == len(s.business)-1 {
						depPrefix = "    └──"
					} else {
						depPrefix = "│   └──"
					}
				}
				fmt.Printf("   %s 🔗 %s\n", depPrefix, dep)
			}
		}
	}

	// Routes
	if len(s.routes) > 0 {
		fmt.Printf("\n🌐 Routes (%d)\n", len(s.routes))
		for i, r := range s.routes {
			prefix := "├──"
			if i == len(s.routes)-1 {
				prefix = "└──"
			}
			fmt.Printf("   %s %-7s %s → %s\n", prefix, r.Method, r.Path, r.Handler)
		}
	}

	// Consumers
	if len(s.consumers) > 0 {
		fmt.Printf("\n📨 Consumers\n")
		for i, c := range s.consumers {
			prefix := "├──"
			if i == len(s.consumers)-1 {
				prefix = "└──"
			}
			fmt.Printf("   %s %s (group: %s, topic: %s) [%s]\n", prefix, c.Name, c.Group, c.Topic, c.Status)
		}
	}

	// Clients
	if len(s.clients) > 0 {
		fmt.Printf("\n🔌 Clients\n")
		for i, c := range s.clients {
			prefix := "├──"
			if i == len(s.clients)-1 {
				prefix = "└──"
			}
			fmt.Printf("   %s %s → %s [%s] (%s)\n", prefix, c.Name, c.Target, c.Type, c.Status)
		}
	}

	// Live health check
	if registry != nil {
		healthResults := registry.HealthAll(context.Background())
		if len(healthResults) > 0 {
			fmt.Printf("\n🏥 Health Check\n")
			for i, h := range healthResults {
				prefix := "├──"
				if i == len(healthResults)-1 {
					prefix = "└──"
				}
				icon := healthStatusIcon(h.Status)
				msg := ""
				if h.Message != "" {
					msg = fmt.Sprintf(" — %s", h.Message)
				}
				fmt.Printf("   %s %s %s: %s%s\n", prefix, icon, h.Name, strings.ToLower(string(h.Status)), msg)
			}
		}
	}

	fmt.Printf("\n")
}

func statusIcon(status string, healthy bool) string {
	if !healthy {
		return "❌"
	}
	switch status {
	case "active", "initialized", "connected", "healthy":
		return "✅"
	case "lazy":
		return "⚡"
	case "inactive", "disabled":
		return "⏸️"
	case "error", "failed":
		return "❌"
	default:
		return "⚠️"
	}
}

func healthStatusIcon(status component.HealthStatus) string {
	switch status {
	case component.StatusHealthy:
		return "✅"
	case component.StatusDegraded:
		return "⚠️"
	case component.StatusUnhealthy:
		return "❌"
	default:
		return "❓"
	}
}

func businessIcon(compType string) string {
	switch compType {
	case "service":
		return "⚙️"
	case "repository":
		return "📁"
	case "handler":
		return "🎯"
	default:
		return "💼"
	}
}
