package component

// Healthy returns a healthy report for the named component.
func Healthy(name string) Health {
	return Health{Name: name, Status: StatusHealthy}
}

// Degraded returns a degraded report for the named component with an explanatory message.
func Degraded(name, message string) Health {
	return Health{Name: name, Status: StatusDegraded, Message: message}
}

// Unhealthy returns an unhealthy report for the named component with an explanatory message.
func Unhealthy(name, message string) Health {
	return Health{Name: name, Status: StatusUnhealthy, Message: message}
}

// IsHealthy reports whether the status is StatusHealthy.
func (h Health) IsHealthy() bool {
	return h.Status == StatusHealthy
}
