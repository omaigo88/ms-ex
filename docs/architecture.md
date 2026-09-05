# Architecture

Three independent Go services build a toy "order a spaceship" system:

- **OrderService** — HTTP API (`:8080`), the only service a client talks to directly.
- **InventoryService** — gRPC (`:50051`), owns the parts catalog and stock.
- **PaymentService** — gRPC (`:50052`), processes payments.

Each service is split into the same three layers (`<service>/pkg/{api,service,repository}`):

- `api` — transport adapter (HTTP handler / gRPC server). Converts wire types
  ⇄ domain types, maps domain errors to status codes.
- `service` — business logic. Depends only on narrow interfaces
  (`Repository`, `InventoryClient`, `PaymentClient`), never on transport types.
- `repository` — storage (in-memory for both Order and Inventory; Payment is
  stateless).

## Component overview

```mermaid
flowchart TB
    Client(["Client"])

    subgraph OrderService["OrderService — HTTP :8080"]
        direction TB
        OrderAPI["api\n(HTTP handler)"]
        OrderSvc["service\n(business logic)"]
        OrderRepo[("repository\n(in-memory orders)")]
        OrderAPI --> OrderSvc
        OrderSvc --> OrderRepo
    end

    subgraph InventoryService["InventoryService — gRPC :50051"]
        direction TB
        InvAPI["api\n(gRPC server)"]
        InvSvc["service\n(business logic)"]
        InvRepo[("repository\n(in-memory parts)")]
        InvAPI --> InvSvc
        InvSvc --> InvRepo
    end

    subgraph PaymentService["PaymentService — gRPC :50052"]
        direction TB
        PaySvc["service\n(validation +\ntransaction UUID)"]
        PayAPI["api\n(gRPC server)"]
        PayAPI --> PaySvc
    end

    Client -- "HTTP JSON" --> OrderAPI
    OrderSvc -- "gRPC ListParts" --> InvAPI
    OrderSvc -- "gRPC PayOrder" --> PayAPI
```

`OrderService.service` is the only place that talks to the other two
services — it holds `InventoryClient`/`PaymentClient` interfaces (satisfied by
the real generated gRPC clients), which is exactly what gets swapped for
mocks in `order/pkg/service/service_test.go`.

## Example scenario: create an order, then pay for it

This is the same flow `scripts/e2e_test.sh` (`task test:e2e`) drives against
the real, independently-running processes.

```mermaid
sequenceDiagram
    actor Client
    participant OAPI as Order api
    participant OSvc as Order service
    participant ORepo as Order repository
    participant Inv as Inventory (gRPC)
    participant Pay as Payment (gRPC)

    Client->>OAPI: POST /api/v1/orders<br/>{hull_uuid, engine_uuid}
    OAPI->>OSvc: CreateOrder(input)
    OSvc->>Inv: ListParts(uuids)
    Inv-->>OSvc: parts [price, stock_quantity]
    Note over OSvc: sum prices,<br/>reject if any stock_quantity <= 0
    OSvc->>ORepo: Save(order, status=PENDING_PAYMENT)
    OSvc-->>OAPI: order
    OAPI-->>Client: 201 {order_uuid, total_price}

    Client->>OAPI: POST /api/v1/orders/{id}/pay<br/>{payment_method}
    OAPI->>OSvc: PayOrder(id, method)
    OSvc->>ORepo: Get(id)
    ORepo-->>OSvc: order (must be PENDING_PAYMENT)
    OSvc->>Pay: PayOrder(order_uuid, method)
    Pay-->>OSvc: transaction_uuid
    OSvc->>ORepo: Save(order, status=PAID)
    OSvc-->>OAPI: order
    OAPI-->>Client: 200 {transaction_uuid}
```

## Error mapping

Business-rule failures are plain sentinel errors in `service` (e.g.
`ErrComponentOutOfStock`, `ErrOrderAlreadyFinal`, `ErrPartNotFound`) — the
`api` layer is the only place that knows about HTTP/gRPC status codes, and
maps them via `errors.Is`:

| Service layer error | Order HTTP | Inventory/Payment gRPC |
|---|---|---|
| not found (order / part / component) | 404 | `codes.NotFound` |
| invalid input (empty/malformed UUID, bad enum) | 400 (ogen validation) | `codes.InvalidArgument` |
| order already paid/cancelled | 409 | — |
| component out of stock | 409 | — |
| anything else | 500 | `codes.Internal` |
