package repository

import (
	"sync"

	"github.com/google/uuid"
)

// memoryRepository is an in-memory, thread-safe Repository implementation.
type memoryRepository struct {
	mu     sync.RWMutex
	orders map[uuid.UUID]Order
}

// NewMemoryRepository creates a new empty order repository.
func NewMemoryRepository() Repository {
	return &memoryRepository{
		orders: make(map[uuid.UUID]Order),
	}
}

// Save creates or updates an order.
func (r *memoryRepository) Save(order Order) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.orders[order.OrderUUID] = order
}

// Get returns an order by UUID.
func (r *memoryRepository) Get(id uuid.UUID) (Order, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	order, ok := r.orders[id]

	return order, ok
}
