package bootstrap

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/di"
	"github.com/kbukum/gokit/logger"
)

// ComponentStatus holds the tracked status of a component during bootstrap.
type ComponentStatus struct {
	Name    string
	Status  string
	Healthy bool
}

// InfrastructureInfo holds detailed infrastructure component information.
type InfrastructureInfo struct {
	Name          string
	ComponentName string // Internal component name (for deduplication with ComponentStatus)
	Type          string // e.g. "database", "server", "kafka", "redis"
	Status        string
	Details       string
	Port          int
	Healthy       bool
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
// For non-component infrastructure (e.g., auth config), componentName can be empty.
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

// trackInfrastructureWithComponent is like TrackInfrastructure but also records
// the internal component name for deduplication with the Components section.
func (s *Summary) trackInfrastructureWithComponent(name, componentName, componentType, status, details string, port int, healthy bool) {
	s.infrastructure = append(s.infrastructure, InfrastructureInfo{
		Name:          name,
		ComponentName: componentName,
		Type:          componentType,
		Status:        status,
		Details:       details,
		Port:          port,
		Healthy:       healthy,
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

// DisplaySummary prints the bootstrap summary.
// It auto-collects infrastructure, routes, and health from the component
// registry and DI registrations from the container. Manual Track* calls
// are only needed for non-component items (e.g., auth config).
func (s *Summary) DisplaySummary(registry *component.Registry, container di.Container, log *logger.Logger) {
	ctx := context.Background()

	// --- Auto-collect from registry ---
	s.collectFromRegistry(ctx, registry)

	// Header
	fmt.Printf("\n")
	fmt.Printf("🚀 \033[1;32m%s\033[0m \033[1;36mv%s\033[0m started in \033[33m%.2fs\033[0m\n\n",
		s.serviceName, s.version, s.startupDuration.Seconds())

	// Infrastructure (auto-discovered from Describable components + manual entries)
	if len(s.infrastructure) > 0 {
		fmt.Printf("\033[1m📊 Infrastructure\033[0m\n")
		for i, inf := range s.infrastructure {
			prefix := treePrefix(i, len(s.infrastructure))
			icon := statusIcon(inf.Status, inf.Healthy)
			details := inf.Details
			if inf.Port > 0 && !strings.Contains(details, fmt.Sprintf(":%d", inf.Port)) {
				details = fmt.Sprintf("%s (:%d)", details, inf.Port)
			}
			fmt.Printf("   %s %s %s: %s\n", prefix, icon, inf.Name, details)
		}
		fmt.Printf("\n")
	}

	// Component health summary
	healthResults := registry.HealthAll(ctx)
	if len(healthResults) > 0 {
		healthy := 0
		for _, h := range healthResults {
			if h.Status == component.StatusHealthy {
				healthy++
			}
		}
		total := len(healthResults)
		if healthy == total {
			fmt.Printf("✅ All components healthy (%d/%d)\n", healthy, total)
		} else {
			fmt.Printf("⚠️  Some components have issues (%d/%d healthy)\n", healthy, total)
		}
	}

	// DI registrations (auto-discovered from container)
	s.displayDIRegistrations(container)

	// Business layer (manually tracked — project-specific service details)
	if len(s.business) > 0 {
		fmt.Printf("\n\033[1m💼 Business Layer\033[0m\n")
		for i, b := range s.business {
			prefix := treePrefix(i, len(s.business))
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

	// Routes (auto-discovered from RouteProvider components + manual entries)
	if len(s.routes) > 0 {
		fmt.Printf("\n\033[1m🌐 Routes (%d)\033[0m\n", len(s.routes))
		for i, r := range s.routes {
			prefix := treePrefix(i, len(s.routes))
			fmt.Printf("   %s %s%-7s\033[0m %s → %s\n", prefix, methodColor(r.Method), r.Method, r.Path, r.Handler)
		}
	}

	// Consumers
	if len(s.consumers) > 0 {
		fmt.Printf("\n\033[1m📨 Consumers\033[0m\n")
		for i, c := range s.consumers {
			prefix := treePrefix(i, len(s.consumers))
			fmt.Printf("   %s %s (group: %s, topic: %s) [%s]\n", prefix, c.Name, c.Group, c.Topic, c.Status)
		}
	}

	// Clients
	if len(s.clients) > 0 {
		fmt.Printf("\n\033[1m🔌 Clients\033[0m\n")
		for i, c := range s.clients {
			prefix := treePrefix(i, len(s.clients))
			fmt.Printf("   %s %s → %s [%s] (%s)\n", prefix, c.Name, c.Target, c.Type, c.Status)
		}
	}

	// Health issues — only show when something is NOT healthy
	if len(healthResults) > 0 {
		var unhealthy []component.ComponentHealth
		for _, h := range healthResults {
			if h.Status != component.StatusHealthy {
				unhealthy = append(unhealthy, h)
			}
		}
		if len(unhealthy) > 0 {
			fmt.Printf("\n\033[1m🏥 Health Issues\033[0m\n")
			for i, h := range unhealthy {
				prefix := treePrefix(i, len(unhealthy))
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

// collectFromRegistry auto-discovers infrastructure, routes, and health
// from registered components. Called at the start of DisplaySummary.
func (s *Summary) collectFromRegistry(ctx context.Context, registry *component.Registry) {
	if registry == nil {
		return
	}

	for _, c := range registry.All() {
		// Auto-discover infrastructure from Describable components
		if d, ok := c.(component.Describable); ok {
			desc := d.Describe()
			h := c.Health(ctx)
			healthy := h.Status == component.StatusHealthy
			status := "active"
			if !healthy {
				status = string(h.Status)
			}
			displayName := desc.Name
			if displayName == "" {
				displayName = c.Name()
			}
			// Avoid duplicates (if manually tracked too)
			found := false
			for _, inf := range s.infrastructure {
				if inf.ComponentName == c.Name() {
					found = true
					break
				}
			}
			if !found {
				s.infrastructure = append(s.infrastructure, InfrastructureInfo{
					Name:          displayName,
					ComponentName: c.Name(),
					Type:          desc.Type,
					Status:        status,
					Details:       desc.Details,
					Port:          desc.Port,
					Healthy:       healthy,
				})
			}
		}

		// Auto-discover routes from RouteProvider components
		if rp, ok := c.(component.RouteProvider); ok {
			for _, r := range rp.Routes() {
				s.routes = append(s.routes, RouteInfo{
					Method:  r.Method,
					Path:    r.Path,
					Handler: r.Handler,
				})
			}
		}
	}
}

// displayDIRegistrations shows DI container registrations grouped by type.
func (s *Summary) displayDIRegistrations(container di.Container) {
	if container == nil {
		return
	}

	regs := container.Registrations()
	if len(regs) == 0 {
		return
	}

	// Group by prefix (service.*, repository.*, handler.*)
	type group struct {
		label string
		icon  string
		items []di.RegistrationInfo
	}

	groups := []group{
		{label: "service", icon: "⚙️"},
		{label: "repository", icon: "📁"},
		{label: "handler", icon: "🎯"},
		{label: "client", icon: "🔌"},
		{label: "producer", icon: "📤"},
		{label: "consumer", icon: "📥"},
	}

	var infra []di.RegistrationInfo

	// Sort for deterministic output
	sort.Slice(regs, func(i, j int) bool {
		return regs[i].Key < regs[j].Key
	})

	for _, reg := range regs {
		matched := false
		for i, g := range groups {
			if strings.HasPrefix(reg.Key, g.label+".") {
				groups[i].items = append(groups[i].items, reg)
				matched = true
				break
			}
		}
		if !matched {
			infra = append(infra, reg)
		}
	}

	// Only display if we have categorized registrations
	hasGrouped := false
	for _, g := range groups {
		if len(g.items) > 0 {
			hasGrouped = true
			break
		}
	}
	if !hasGrouped {
		return
	}

	fmt.Printf("\n\033[1m📦 DI Container (%d registrations)\033[0m\n", len(regs))

	// Count total displayable groups
	totalGroups := 0
	for _, g := range groups {
		if len(g.items) > 0 {
			totalGroups++
		}
	}
	if len(infra) > 0 {
		totalGroups++
	}

	displayIdx := 0
	for _, g := range groups {
		if len(g.items) == 0 {
			continue
		}
		displayIdx++
		isLast := displayIdx == totalGroups
		groupPrefix := treePrefix(displayIdx-1, totalGroups)
		// continuation line: "│   " if not last group, "    " if last
		cont := "│"
		if isLast {
			cont = " "
		}

		fmt.Printf("   %s %s %ss (%d)\n", groupPrefix, g.icon, g.label, len(g.items))
		for j, item := range g.items {
			name := strings.TrimPrefix(item.Key, g.label+".")
			status := "✅"
			if item.Mode == di.Lazy && !item.Initialized {
				status = "💤"
			}
			itemPrefix := treePrefix(j, len(g.items))
			fmt.Printf("   %s   %s %s %s\n", cont, itemPrefix, status, name)
		}
	}

	if len(infra) > 0 {
		displayIdx++
		groupPrefix := treePrefix(displayIdx-1, totalGroups)
		fmt.Printf("   %s 🔧 infrastructure (%d)\n", groupPrefix, len(infra))
		for j, item := range infra {
			status := "✅"
			if item.Mode == di.Lazy && !item.Initialized {
				status = "💤"
			}
			itemPrefix := treePrefix(j, len(infra))
			fmt.Printf("      %s %s %s\n", itemPrefix, status, item.Key)
		}
	}
}

// treePrefix returns the correct tree connector for position i of n items.
func treePrefix(i, n int) string {
	if i == n-1 {
		return "└──"
	}
	return "├──"
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

func methodColor(method string) string {
	switch method {
	case "GET":
		return "\033[32m" // Green
	case "POST":
		return "\033[33m" // Yellow
	case "PUT":
		return "\033[36m" // Cyan
	case "PATCH":
		return "\033[35m" // Magenta
	case "DELETE":
		return "\033[31m" // Red
	default:
		return "\033[0m"
	}
}
