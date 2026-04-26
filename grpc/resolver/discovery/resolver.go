package discovery

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/grpc/resolver"

	disc "github.com/kbukum/gokit/discovery"
	"github.com/kbukum/gokit/logger"
)

// discoveryResolver watches a discovery backend and pushes address updates to gRPC.
type discoveryResolver struct {
	serviceName string
	discovery   disc.Discovery
	cc          resolver.ClientConn
	log         *logger.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func newDiscoveryResolver(
	serviceName string,
	discovery disc.Discovery,
	cc resolver.ClientConn,
	log *logger.Logger,
) *discoveryResolver {
	ctx, cancel := context.WithCancel(context.Background())
	return &discoveryResolver{
		serviceName: serviceName,
		discovery:   discovery,
		cc:          cc,
		log:         log,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// start begins watching the discovery backend for address changes.
func (r *discoveryResolver) start() {
	// Do an initial resolution synchronously so gRPC has addresses immediately.
	r.resolve()

	// Start background watch for live updates.
	r.wg.Add(1)
	go r.watch()
}

// resolve performs a one-shot discovery query and pushes results to gRPC.
func (r *discoveryResolver) resolve() {
	instances, err := r.discovery.Discover(r.ctx, r.serviceName)
	if err != nil {
		r.log.Warn("Discovery resolve failed", map[string]interface{}{
			"service": r.serviceName,
			"error":   err.Error(),
		})
		r.cc.ReportError(fmt.Errorf("discovery resolve %q: %w", r.serviceName, err))
		return
	}

	r.updateAddresses(instances)
}

// watch uses Discovery.Watch() for live address updates.
func (r *discoveryResolver) watch() {
	defer r.wg.Done()

	ch, err := r.discovery.Watch(r.ctx, r.serviceName)
	if err != nil {
		r.log.Warn("Discovery watch failed, falling back to one-shot resolve", map[string]interface{}{
			"service": r.serviceName,
			"error":   err.Error(),
		})
		return
	}

	for {
		select {
		case <-r.ctx.Done():
			return
		case instances, ok := <-ch:
			if !ok {
				return
			}
			r.updateAddresses(instances)
		}
	}
}

// updateAddresses converts discovery instances to gRPC addresses and pushes to the ClientConn.
func (r *discoveryResolver) updateAddresses(instances []disc.ServiceInstance) {
	if len(instances) == 0 {
		r.log.Warn("No healthy instances found", map[string]interface{}{
			"service": r.serviceName,
		})
		r.cc.ReportError(fmt.Errorf("%w: %s", disc.ErrNoHealthyEndpoints, r.serviceName))
		return
	}

	addrs := make([]resolver.Address, 0, len(instances))
	for i := range instances {
		inst := &instances[i]
		addrs = append(addrs, resolver.Address{
			Addr:       fmt.Sprintf("%s:%d", inst.Address, inst.Port),
			ServerName: inst.Name,
		})
	}

	r.log.Debug("Updating gRPC addresses", map[string]interface{}{
		"service": r.serviceName,
		"count":   len(addrs),
	})

	if err := r.cc.UpdateState(resolver.State{Addresses: addrs}); err != nil {
		r.log.Warn("Failed to update resolver state", map[string]interface{}{
			"service": r.serviceName,
			"error":   err.Error(),
		})
	}
}

// ResolveNow is called by gRPC when it needs to re-resolve (e.g., after connection failure).
func (r *discoveryResolver) ResolveNow(_ resolver.ResolveNowOptions) {
	go r.resolve()
}

// Close stops the resolver and releases resources.
func (r *discoveryResolver) Close() {
	r.cancel()
	r.wg.Wait()
}

// Compile-time check.
var _ resolver.Resolver = (*discoveryResolver)(nil)
