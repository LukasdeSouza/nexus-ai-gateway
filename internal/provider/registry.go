package provider

import (
	"fmt"
	"sync"
)

// Registry manages initialized provider adapter instances.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry initializes an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

// Register registers or overrides a provider adapter.
func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.ID()] = p
}

// Get retrieves a provider by its identifier.
func (r *Registry) Get(id string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[id]
	return p, ok
}

// MustGet returns provider or panics if unregistered.
func (r *Registry) MustGet(id string) Provider {
	p, ok := r.Get(id)
	if !ok {
		panic(fmt.Sprintf("provider %s is not registered", id))
	}
	return p
}

// All returns a slice of all registered providers.
func (r *Registry) All() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		list = append(list, p)
	}
	return list
}
