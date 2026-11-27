package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSpanRoute(t *testing.T) {
	assert.Equal(t, "/api/v1/orders", spanRoute("/api/v1/orders"))
	assert.Equal(t, "/api/v1/orders/{order_uuid}/pay",
		spanRoute("/api/v1/orders/b00c3b63-994c-4f51-8381-31bf8537fe82/pay"))
	assert.Equal(t, "/api/v1/orders/not-a-uuid", spanRoute("/api/v1/orders/not-a-uuid"))
}
