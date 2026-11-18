// Package tests contains the API tests.
//
// Layout: everything is spun up in-process within a single test binary.
//   - IAM, Inventory and Payment (gRPC) are spun up via bufconn.Listener
//     (a virtual transport with no network), clients reach them via
//     grpc.NewClient with a custom dialer
//   - Order (HTTP) is spun up via httptest.NewServer (a real TCP listener on
//     127.0.0.1 with a random port); Order internally talks to Inventory/Payment
//     through the same bufconn clients
//   - Authentication is real: Order's HTTP middleware and Inventory's gRPC
//     interceptor call IAM (in-memory users/sessions). TestMain registers and
//     logs in a test user; httpClient sends its session as a Bearer token.
//
// This stack is started once in TestMain — no Docker, no hardcoded ports,
// no task up. The tests are fast and self-contained.
package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	iamAPI "github.com/omaigo88/iam/pkg/api"
	iamRepository "github.com/omaigo88/iam/pkg/repository"
	iamService "github.com/omaigo88/iam/pkg/service"
	inventoryAPI "github.com/omaigo88/inventory/pkg/api"
	inventoryDomain "github.com/omaigo88/inventory/pkg/domain"
	inventoryRepository "github.com/omaigo88/inventory/pkg/repository"
	inventoryService "github.com/omaigo88/inventory/pkg/service"
	orderAPI "github.com/omaigo88/order/pkg/api"
	orderRepository "github.com/omaigo88/order/pkg/repository"
	orderService "github.com/omaigo88/order/pkg/service"
	"github.com/omaigo88/order/tests/testutil"
	paymentAPI "github.com/omaigo88/payment/pkg/api"
	paymentService "github.com/omaigo88/payment/pkg/service"
	"github.com/omaigo88/shared/pkg/auth"
	authv1 "github.com/omaigo88/shared/pkg/proto/auth/v1"
	inventoryv1 "github.com/omaigo88/shared/pkg/proto/inventory/v1"
	paymentv1 "github.com/omaigo88/shared/pkg/proto/payment/v1"
)

// Preloaded UUIDs and part prices (from inventory/pkg/repository/memory.go)
const (
	HullAluminumUUID   = "550e8400-e29b-41d4-a716-446655440001" // 500000 kopecks (5000 RUB)
	HullTitaniumUUID   = "550e8400-e29b-41d4-a716-446655440002" // 1500000 kopecks (15000 RUB)
	EngineIonCUUID     = "550e8400-e29b-41d4-a716-446655440003" // 300000 kopecks (3000 RUB)
	EngineIonBUUID     = "550e8400-e29b-41d4-a716-446655440004" // 800000 kopecks (8000 RUB)
	ShieldEnergyUUID   = "550e8400-e29b-41d4-a716-446655440005" // 400000 kopecks (4000 RUB)
	WeaponLaserUUID    = "550e8400-e29b-41d4-a716-446655440006" // 250000 kopecks (2500 RUB)
	HullOutOfStockUUID = "550e8400-e29b-41d4-a716-446655440007" // 2000000 kopecks (20000 RUB), stock=0

	// Prices in kopecks
	HullAluminumPrice   = 500000
	HullTitaniumPrice   = 1500000
	EngineIonCPrice     = 300000
	EngineIonBPrice     = 800000
	ShieldEnergyPrice   = 400000
	WeaponLaserPrice    = 250000
	HullOutOfStockPrice = 2000000
)

const bufSize = 1024 * 1024

var (
	iamLis *bufconn.Listener
	invLis *bufconn.Listener
	payLis *bufconn.Listener

	authClient authv1.AuthServiceClient
	// inventoryClient calls Inventory directly as the test user (session in
	// outgoing metadata).
	inventoryClient inventoryv1.InventoryServiceClient
	// orderInventoryClient is the client Order itself uses: it forwards the
	// session from ctx, so a call without one is unauthenticated.
	orderInventoryClient inventoryv1.InventoryServiceClient
	paymentClient        paymentv1.PaymentServiceClient

	// testSession / testUserUUID belong to the user TestMain registers.
	testSession  string
	testUserUUID string

	// httpClient authenticates as the test user; anonClient sends no token.
	httpClient = &http.Client{Timeout: 5 * time.Second, Transport: bearerTransport{token: func() string { return testSession }}}
	anonClient = &http.Client{Timeout: 5 * time.Second}
	ts         *httptest.Server

	// orderSvc is exposed so tests can drive transitions that, outside this
	// suite, arrive over Kafka (e.g. MarkAssembled on ShipAssembled).
	orderSvc *orderService.Service
)

// bearerTransport adds "Authorization: Bearer <token>" unless the request
// already carries an Authorization header.
type bearerTransport struct {
	token func() string
}

func (b bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("Authorization") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("Authorization", "Bearer "+b.token())
	}

	return http.DefaultTransport.RoundTrip(req)
}

// sessionMetadataInterceptor attaches a fixed session to every outgoing call.
func sessionMetadataInterceptor(session func() string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		return invoker(metadata.AppendToOutgoingContext(ctx, auth.SessionMetadataKey, session()), method, req, reply, cc, opts...)
	}
}

func bufClient(lis *bufconn.Listener, opts ...grpc.DialOption) *grpc.ClientConn {
	opts = append(
		opts,
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)

	conn, err := grpc.NewClient("passthrough:///bufnet", opts...)
	if err != nil {
		slog.Error("bufconn gRPC client failed", "error", err)
		os.Exit(1)
	}

	return conn
}

func serve(srv *grpc.Server, lis *bufconn.Listener, name string) {
	go func() {
		if err := srv.Serve(lis); err != nil {
			slog.Error(name+" gRPC server failed", "error", err)
			os.Exit(1)
		}
	}()
}

// registerAndLogin creates a user and returns its UUID and a fresh session.
func registerAndLogin(ctx context.Context, login, password string) (userUUID, session string, err error) {
	reg, err := authClient.Register(ctx, &authv1.RegisterRequest{Login: login, Password: password})
	if err != nil {
		return "", "", err
	}

	lr, err := authClient.Login(ctx, &authv1.LoginRequest{Login: login, Password: password})
	if err != nil {
		return "", "", err
	}

	return reg.GetUserUuid(), lr.GetSessionUuid(), nil
}

// orderBaseURL returns the base URL for order HTTP tests
func orderBaseURL() string {
	return ts.URL
}

// strPtr returns a pointer to the given string
func strPtr(s string) *string {
	return &s
}

// TestMain starts all services before the tests and stops them afterward
func TestMain(m *testing.M) {
	ctx := context.Background()

	// 1. IAM gRPC via bufconn (in-memory users and sessions)
	iamLis = bufconn.Listen(bufSize)
	iamGRPCServer := grpc.NewServer()
	iamSvc := iamService.NewService(iamRepository.NewUserMemoryRepository(), iamRepository.NewSessionMemoryRepository(), 24*time.Hour)
	authv1.RegisterAuthServiceServer(iamGRPCServer, iamAPI.NewAuthServer(iamSvc))
	serve(iamGRPCServer, iamLis, "iam")

	iamConn := bufClient(iamLis)
	authClient = authv1.NewAuthServiceClient(iamConn)

	var err error

	testUserUUID, testSession, err = registerAndLogin(ctx, "api-test-user", "api-test-password")
	if err != nil {
		slog.Error("register/login test user failed", "error", err)
		os.Exit(1)
	}

	// 2. Inventory gRPC via bufconn, behind the IAM auth interceptor
	invLis = bufconn.Listen(bufSize)
	invGRPCServer := grpc.NewServer(grpc.UnaryInterceptor(inventoryAPI.AuthInterceptor(authClient)))
	invRepo := inventoryRepository.NewMemoryRepository()
	invSvc := inventoryService.NewService(invRepo, inventoryDomain.NewCompatibilityChecker(), inventoryService.NewNoopTrManager())
	inventoryv1.RegisterInventoryServiceServer(invGRPCServer, inventoryAPI.NewServer(invSvc))
	serve(invGRPCServer, invLis, "inventory")

	invConn := bufClient(invLis, grpc.WithUnaryInterceptor(sessionMetadataInterceptor(func() string { return testSession })))
	inventoryClient = inventoryv1.NewInventoryServiceClient(invConn)

	orderInvConn := bufClient(invLis, grpc.WithUnaryInterceptor(auth.UnaryClientInterceptor()))
	orderInventoryClient = inventoryv1.NewInventoryServiceClient(orderInvConn)

	// 3. Payment gRPC via bufconn
	payLis = bufconn.Listen(bufSize)
	payGRPCServer := grpc.NewServer()
	paySvc := paymentService.NewService()
	paymentv1.RegisterPaymentServiceServer(payGRPCServer, paymentAPI.NewServer(paySvc))
	serve(payGRPCServer, payLis, "payment")

	payConn := bufClient(payLis)
	paymentClient = paymentv1.NewPaymentServiceClient(payConn)

	// 4. Order HTTP via httptest, behind the IAM auth middleware
	orderRepo := orderRepository.NewMemoryRepository()
	orderSvc = orderService.NewService(
		orderInventoryClient, paymentClient, orderRepo,
		orderService.NewNoopTrManager(), orderService.NewNoopOrderPaidPublisher(),
	)
	h := orderAPI.NewHandler(orderSvc)
	orderServer, err := orderAPI.SetupServer(h)
	if err != nil {
		slog.Error("order HTTP server setup failed", "error", err)
		os.Exit(1)
	}
	ts = httptest.NewServer(orderAPI.AuthMiddleware(authClient)(orderServer))

	code := m.Run()

	ts.Close()
	_ = invConn.Close()
	_ = orderInvConn.Close()
	_ = payConn.Close()
	_ = iamConn.Close()
	invGRPCServer.Stop()
	payGRPCServer.Stop()
	iamGRPCServer.Stop()
	os.Exit(code)
}

// HTTP request/response types

// CreateOrderRequest represents the request body for creating an order
type CreateOrderRequest struct {
	HullUUID   string  `json:"hull_uuid"`
	EngineUUID string  `json:"engine_uuid"`
	ShieldUUID *string `json:"shield_uuid,omitempty"`
	WeaponUUID *string `json:"weapon_uuid,omitempty"`
}

// CreateOrderResponse represents the response for creating an order
type CreateOrderResponse struct {
	OrderUUID  string `json:"order_uuid"`
	TotalPrice int64  `json:"total_price"`
}

// PayOrderRequest represents the request body for paying for an order
type PayOrderRequest struct {
	PaymentMethod string `json:"payment_method"`
}

// PayOrderResponse represents the response for paying for an order
type PayOrderResponse struct {
	TransactionUUID string `json:"transaction_uuid"`
}

// CancelOrderResponse represents the response for cancelling an order (empty)
type CancelOrderResponse struct{}

// OrderDTO represents an order in the API response
type OrderDTO struct {
	OrderUUID       string  `json:"order_uuid"`
	UserUUID        string  `json:"user_uuid"`
	HullUUID        string  `json:"hull_uuid"`
	EngineUUID      string  `json:"engine_uuid"`
	ShieldUUID      *string `json:"shield_uuid"`
	WeaponUUID      *string `json:"weapon_uuid"`
	TotalPrice      int64   `json:"total_price"`
	TransactionUUID *string `json:"transaction_uuid"`
	PaymentMethod   *string `json:"payment_method"`
	Status          string  `json:"status"`
	CreatedAt       string  `json:"created_at"`
}

// ErrorResponse represents an error response from the API
type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Helper HTTP functions

func createOrder(t *testing.T, req *CreateOrderRequest) (*CreateOrderResponse, *http.Response) {
	t.Helper()

	jsonBody, err := json.Marshal(req)
	require.NoError(t, err)

	httpReq, err := http.NewRequest(http.MethodPost, orderBaseURL()+"/api/v1/orders", bytes.NewReader(jsonBody))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)

	if resp.StatusCode == http.StatusCreated {
		var result CreateOrderResponse
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)
		return &result, resp
	}

	return nil, resp
}

func getOrder(t *testing.T, orderUUID string) (*OrderDTO, *http.Response) {
	t.Helper()

	resp, err := httpClient.Get(orderBaseURL() + "/api/v1/orders/" + orderUUID)
	require.NoError(t, err)

	if resp.StatusCode == http.StatusOK {
		var result OrderDTO
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)
		return &result, resp
	}

	return nil, resp
}

func payOrder(t *testing.T, orderUUID string, req *PayOrderRequest) (*PayOrderResponse, *http.Response) {
	t.Helper()

	jsonBody, err := json.Marshal(req)
	require.NoError(t, err)

	httpReq, err := http.NewRequest(http.MethodPost, orderBaseURL()+"/api/v1/orders/"+orderUUID+"/pay", bytes.NewReader(jsonBody))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)

	if resp.StatusCode == http.StatusOK {
		var result PayOrderResponse
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)
		return &result, resp
	}

	return nil, resp
}

func cancelOrder(t *testing.T, orderUUID string) (*CancelOrderResponse, *http.Response) {
	t.Helper()

	httpReq, err := http.NewRequest(http.MethodPost, orderBaseURL()+"/api/v1/orders/"+orderUUID+"/cancel", nil)
	require.NoError(t, err)

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)

	if resp.StatusCode == http.StatusOK {
		var result CancelOrderResponse
		err = json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)
		return &result, resp
	}

	return nil, resp
}

// InventoryService tests (gRPC)

func TestInventory_GetPart_Success(t *testing.T) {
	resp, err := inventoryClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{
		Uuid: HullAluminumUUID,
	})
	require.NoError(t, err)

	part := resp.GetPart()
	assert.Equal(t, HullAluminumUUID, part.GetUuid())
	assert.Equal(t, int64(HullAluminumPrice), part.GetPrice())
	assert.Equal(t, inventoryv1.PartType_PART_TYPE_HULL, part.GetPartType())
	assert.NotEmpty(t, part.GetName())
	assert.NotNil(t, part.GetCreatedAt())
}

func TestInventory_GetPart_AllTypes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		uuid     string
		price    int64
		partType inventoryv1.PartType
	}{
		{"Hull Aluminum", HullAluminumUUID, HullAluminumPrice, inventoryv1.PartType_PART_TYPE_HULL},
		{"Hull Titanium", HullTitaniumUUID, HullTitaniumPrice, inventoryv1.PartType_PART_TYPE_HULL},
		{"Engine Ion C", EngineIonCUUID, EngineIonCPrice, inventoryv1.PartType_PART_TYPE_ENGINE},
		{"Engine Ion B", EngineIonBUUID, EngineIonBPrice, inventoryv1.PartType_PART_TYPE_ENGINE},
		{"Shield Energy", ShieldEnergyUUID, ShieldEnergyPrice, inventoryv1.PartType_PART_TYPE_SHIELD},
		{"Weapon Laser", WeaponLaserUUID, WeaponLaserPrice, inventoryv1.PartType_PART_TYPE_WEAPON},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resp, err := inventoryClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{
				Uuid: tc.uuid,
			})
			require.NoError(t, err)

			part := resp.GetPart()
			assert.Equal(t, tc.uuid, part.GetUuid())
			assert.Equal(t, tc.price, part.GetPrice())
			assert.Equal(t, tc.partType, part.GetPartType())
		})
	}
}

func TestInventory_GetPart_NotFound(t *testing.T) {
	_, err := inventoryClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{
		Uuid: uuid.New().String(),
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.NotFound)
}

func TestInventory_GetPart_EmptyUUID(t *testing.T) {
	_, err := inventoryClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{
		Uuid: "",
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestInventory_GetPart_InvalidUUID(t *testing.T) {
	_, err := inventoryClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{
		Uuid: "invalid-uuid-format",
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestInventory_ListParts_All(t *testing.T) {
	resp, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		PartType: inventoryv1.PartType_PART_TYPE_UNSPECIFIED,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 7)
}

func TestInventory_ListParts_ByType_Hull(t *testing.T) {
	resp, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		PartType: inventoryv1.PartType_PART_TYPE_HULL,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 3)

	for _, part := range resp.GetParts() {
		assert.Equal(t, inventoryv1.PartType_PART_TYPE_HULL, part.GetPartType())
	}
}

func TestInventory_ListParts_ByType_Engine(t *testing.T) {
	resp, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		PartType: inventoryv1.PartType_PART_TYPE_ENGINE,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 2)

	for _, part := range resp.GetParts() {
		assert.Equal(t, inventoryv1.PartType_PART_TYPE_ENGINE, part.GetPartType())
	}
}

func TestInventory_ListParts_ByType_Shield(t *testing.T) {
	resp, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		PartType: inventoryv1.PartType_PART_TYPE_SHIELD,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 1)
	assert.Equal(t, ShieldEnergyUUID, resp.GetParts()[0].GetUuid())
}

func TestInventory_ListParts_ByType_Weapon(t *testing.T) {
	resp, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		PartType: inventoryv1.PartType_PART_TYPE_WEAPON,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 1)
	assert.Equal(t, WeaponLaserUUID, resp.GetParts()[0].GetUuid())
}

func TestInventory_ListParts_SortedByName(t *testing.T) {
	resp, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		PartType: inventoryv1.PartType_PART_TYPE_UNSPECIFIED,
	})
	require.NoError(t, err)

	parts := resp.GetParts()
	for i := 1; i < len(parts); i++ {
		assert.LessOrEqual(t, parts[i-1].GetName(), parts[i].GetName(),
			"parts should be sorted by name in alphabetical order")
	}
}

// ListParts.uuids tests

func TestInventory_ListParts_ByUuids_Success(t *testing.T) {
	uuids := []string{HullAluminumUUID, EngineIonCUUID, ShieldEnergyUUID}

	resp, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		Uuids: uuids,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 3)

	// Check that the expected parts were returned
	returnedUUIDs := make([]string, len(resp.GetParts()))
	for i, part := range resp.GetParts() {
		returnedUUIDs[i] = part.GetUuid()
	}
	assert.ElementsMatch(t, uuids, returnedUUIDs)
}

func TestInventory_ListParts_ByUuids_PreservesOrder(t *testing.T) {
	// Request in a specific order (engine, hull, weapon)
	uuids := []string{EngineIonCUUID, HullAluminumUUID, WeaponLaserUUID}

	resp, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		Uuids: uuids,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 3)

	// Check that the order matches the request
	for i, part := range resp.GetParts() {
		assert.Equal(t, uuids[i], part.GetUuid(),
			"part at index %d should match the order of the requested UUIDs", i)
	}
}

func TestInventory_ListParts_ByUuids_IgnoresPartType(t *testing.T) {
	// Request with both uuids AND part_type (part_type should be ignored)
	uuids := []string{HullAluminumUUID, EngineIonCUUID}

	resp, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		Uuids:    uuids,
		PartType: inventoryv1.PartType_PART_TYPE_WEAPON, // Should be ignored
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 2)

	// Check that we got Hull and Engine, not Weapons
	assert.Equal(t, HullAluminumUUID, resp.GetParts()[0].GetUuid())
	assert.Equal(t, EngineIonCUUID, resp.GetParts()[1].GetUuid())
}

func TestInventory_ListParts_ByUuids_NotFound(t *testing.T) {
	// Include one non-existent UUID
	nonExistentUUID := uuid.New().String()
	uuids := []string{HullAluminumUUID, nonExistentUUID, EngineIonCUUID}

	_, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		Uuids: uuids,
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.NotFound)
}

func TestInventory_ListParts_ByUuids_InvalidUUID(t *testing.T) {
	uuids := []string{HullAluminumUUID, "invalid-uuid-format"}

	_, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		Uuids: uuids,
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestInventory_ListParts_ByUuids_SingleUUID(t *testing.T) {
	uuids := []string{WeaponLaserUUID}

	resp, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		Uuids: uuids,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 1)
	assert.Equal(t, WeaponLaserUUID, resp.GetParts()[0].GetUuid())
	assert.Equal(t, int64(WeaponLaserPrice), resp.GetParts()[0].GetPrice())
}

func TestInventory_ListParts_ByUuids_AllParts(t *testing.T) {
	// Request all 7 parts by UUID
	uuids := []string{
		HullAluminumUUID, HullTitaniumUUID,
		EngineIonCUUID, EngineIonBUUID,
		ShieldEnergyUUID, WeaponLaserUUID,
		HullOutOfStockUUID,
	}

	resp, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		Uuids: uuids,
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 7)

	// Check that the order matches the request order
	for i, part := range resp.GetParts() {
		assert.Equal(t, uuids[i], part.GetUuid())
	}
}

// PaymentService tests (gRPC)

func TestPayment_PayOrder_Success_Card(t *testing.T) {
	resp, err := paymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     uuid.New().String(),
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_CARD,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetTransactionUuid())

	// Check that the transaction UUID is valid
	_, err = uuid.Parse(resp.GetTransactionUuid())
	assert.NoError(t, err)
}

func TestPayment_PayOrder_Success_SBP(t *testing.T) {
	resp, err := paymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     uuid.New().String(),
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_SBP,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetTransactionUuid())
}

func TestPayment_PayOrder_Success_CreditCard(t *testing.T) {
	resp, err := paymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     uuid.New().String(),
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_CREDIT_CARD,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetTransactionUuid())
}

func TestPayment_PayOrder_Success_InvestorMoney(t *testing.T) {
	resp, err := paymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     uuid.New().String(),
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_INVESTOR_MONEY,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetTransactionUuid())
}

func TestPayment_PayOrder_EmptyOrderUUID(t *testing.T) {
	_, err := paymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     "",
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_CARD,
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestPayment_PayOrder_UnspecifiedMethod(t *testing.T) {
	_, err := paymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     uuid.New().String(),
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_UNSPECIFIED,
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.InvalidArgument)
}

func TestPayment_PayOrder_UniqueTransactions(t *testing.T) {
	orderUUID := uuid.New().String()

	resp1, err := paymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     orderUUID,
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_CARD,
	})
	require.NoError(t, err)

	resp2, err := paymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     orderUUID,
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_CARD,
	})
	require.NoError(t, err)

	assert.NotEqual(t, resp1.GetTransactionUuid(), resp2.GetTransactionUuid(),
		"each payment should generate a unique transaction UUID")
}

// OrderService tests (HTTP)

func TestOrder_Create_Success_MinimalParts(t *testing.T) {
	req := &CreateOrderRequest{
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}

	result, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.OrderUUID)
	assert.Equal(t, int64(HullAluminumPrice+EngineIonCPrice), result.TotalPrice)
}

func TestOrder_Create_Success_AllParts(t *testing.T) {
	req := &CreateOrderRequest{
		HullUUID:   HullTitaniumUUID,
		EngineUUID: EngineIonBUUID,
		ShieldUUID: strPtr(ShieldEnergyUUID),
		WeaponUUID: strPtr(WeaponLaserUUID),
	}

	result, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.OrderUUID)

	expectedTotal := int64(HullTitaniumPrice + EngineIonBPrice + ShieldEnergyPrice + WeaponLaserPrice)
	assert.Equal(t, expectedTotal, result.TotalPrice)
}

func TestOrder_Create_VerifyTotalPrice(t *testing.T) {
	req := &CreateOrderRequest{
		HullUUID:   HullAluminumUUID, // 500000
		EngineUUID: EngineIonCUUID,   // 300000
	}

	result, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Equal(t, int64(800000), result.TotalPrice, "500000 + 300000 = 800000")
}

func TestOrder_Create_HullNotFound(t *testing.T) {
	req := &CreateOrderRequest{
		HullUUID:   uuid.New().String(),
		EngineUUID: EngineIonCUUID,
	}

	_, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestOrder_Create_EngineNotFound(t *testing.T) {
	req := &CreateOrderRequest{
		HullUUID:   HullAluminumUUID,
		EngineUUID: uuid.New().String(),
	}

	_, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestOrder_Create_ShieldNotFound(t *testing.T) {
	req := &CreateOrderRequest{
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
		ShieldUUID: strPtr(uuid.New().String()),
	}

	_, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestOrder_Create_WeaponNotFound(t *testing.T) {
	req := &CreateOrderRequest{
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
		WeaponUUID: strPtr(uuid.New().String()),
	}

	_, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestOrder_Get_Success(t *testing.T) {
	// First create an order
	createReq := &CreateOrderRequest{
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	defer func() { _ = createResp.Body.Close() }()
	require.NotNil(t, createResult)

	// Get the order
	order, resp := getOrder(t, createResult.OrderUUID)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, order)
	assert.Equal(t, createResult.OrderUUID, order.OrderUUID)
	assert.Equal(t, HullAluminumUUID, order.HullUUID)
	assert.Equal(t, EngineIonCUUID, order.EngineUUID)
	assert.Equal(t, createResult.TotalPrice, order.TotalPrice)
}

func TestOrder_Get_VerifyStatus_PendingPayment(t *testing.T) {
	// Create an order
	createReq := &CreateOrderRequest{
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	defer func() { _ = createResp.Body.Close() }()
	require.NotNil(t, createResult)

	// Get and check the status
	order, resp := getOrder(t, createResult.OrderUUID)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "PENDING_PAYMENT", order.Status)
}

func TestOrder_Get_NotFound(t *testing.T) {
	_, resp := getOrder(t, uuid.New().String())
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestOrder_Pay_Success_Card(t *testing.T) {
	// Create an order
	createReq := &CreateOrderRequest{
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	defer func() { _ = createResp.Body.Close() }()
	require.NotNil(t, createResult)

	// Pay for the order
	payReq := &PayOrderRequest{PaymentMethod: "CARD"}
	payResult, resp := payOrder(t, createResult.OrderUUID, payReq)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, payResult)
	assert.NotEmpty(t, payResult.TransactionUUID)
}

func TestOrder_Pay_VerifyStatusChange(t *testing.T) {
	// Create an order
	createReq := &CreateOrderRequest{
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	defer func() { _ = createResp.Body.Close() }()
	require.NotNil(t, createResult)

	// Pay for the order
	payReq := &PayOrderRequest{PaymentMethod: "CARD"}
	_, payResp := payOrder(t, createResult.OrderUUID, payReq)
	defer func() { _ = payResp.Body.Close() }()

	// Get and check the status change to PAID
	order, getResp := getOrder(t, createResult.OrderUUID)
	defer func() { _ = getResp.Body.Close() }()

	require.Equal(t, http.StatusOK, getResp.StatusCode)
	assert.Equal(t, "PAID", order.Status)
	assert.NotNil(t, order.TransactionUUID)
	assert.NotNil(t, order.PaymentMethod)
	assert.Equal(t, "CARD", *order.PaymentMethod)
}

func TestOrder_Pay_NotFound(t *testing.T) {
	payReq := &PayOrderRequest{PaymentMethod: "CARD"}
	_, resp := payOrder(t, uuid.New().String(), payReq)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestOrder_Pay_AlreadyPaid(t *testing.T) {
	// Create an order
	createReq := &CreateOrderRequest{
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	defer func() { _ = createResp.Body.Close() }()
	require.NotNil(t, createResult)

	// Pay for the order the first time
	payReq := &PayOrderRequest{PaymentMethod: "CARD"}
	_, payResp1 := payOrder(t, createResult.OrderUUID, payReq)
	defer func() { _ = payResp1.Body.Close() }()

	// Try to pay again (should be a conflict error)
	_, payResp2 := payOrder(t, createResult.OrderUUID, payReq)
	defer func() { _ = payResp2.Body.Close() }()

	require.Equal(t, http.StatusConflict, payResp2.StatusCode)
}

func TestOrder_Pay_AlreadyCancelled(t *testing.T) {
	// Create an order
	createReq := &CreateOrderRequest{
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	defer func() { _ = createResp.Body.Close() }()
	require.NotNil(t, createResult)

	// Cancel the order
	_, cancelResp := cancelOrder(t, createResult.OrderUUID)
	defer func() { _ = cancelResp.Body.Close() }()

	// Try to pay for the cancelled order (should be a conflict error)
	payReq := &PayOrderRequest{PaymentMethod: "CARD"}
	_, payResp := payOrder(t, createResult.OrderUUID, payReq)
	defer func() { _ = payResp.Body.Close() }()

	require.Equal(t, http.StatusConflict, payResp.StatusCode)
}

func TestOrder_Cancel_Success(t *testing.T) {
	// Create an order
	createReq := &CreateOrderRequest{
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	defer func() { _ = createResp.Body.Close() }()
	require.NotNil(t, createResult)

	// Cancel the order
	_, resp := cancelOrder(t, createResult.OrderUUID)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestOrder_Cancel_VerifyStatusChange(t *testing.T) {
	// Create an order
	createReq := &CreateOrderRequest{
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	defer func() { _ = createResp.Body.Close() }()
	require.NotNil(t, createResult)

	// Cancel the order
	_, cancelResp := cancelOrder(t, createResult.OrderUUID)
	defer func() { _ = cancelResp.Body.Close() }()

	// Get and check the status change to CANCELLED
	order, getResp := getOrder(t, createResult.OrderUUID)
	defer func() { _ = getResp.Body.Close() }()

	require.Equal(t, http.StatusOK, getResp.StatusCode)
	assert.Equal(t, "CANCELLED", order.Status)
}

func TestOrder_Cancel_NotFound(t *testing.T) {
	_, resp := cancelOrder(t, uuid.New().String())
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestOrder_Cancel_AlreadyPaid(t *testing.T) {
	// Create an order
	createReq := &CreateOrderRequest{
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	defer func() { _ = createResp.Body.Close() }()
	require.NotNil(t, createResult)

	// Pay for the order
	payReq := &PayOrderRequest{PaymentMethod: "CARD"}
	_, payResp := payOrder(t, createResult.OrderUUID, payReq)
	defer func() { _ = payResp.Body.Close() }()

	// Try to cancel the paid order (should be a conflict error)
	_, cancelResp := cancelOrder(t, createResult.OrderUUID)
	defer func() { _ = cancelResp.Body.Close() }()

	require.Equal(t, http.StatusConflict, cancelResp.StatusCode)
}

func TestOrder_Cancel_AlreadyCancelled(t *testing.T) {
	// Create an order
	createReq := &CreateOrderRequest{
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	defer func() { _ = createResp.Body.Close() }()
	require.NotNil(t, createResult)

	// Cancel the order the first time
	_, cancelResp1 := cancelOrder(t, createResult.OrderUUID)
	defer func() { _ = cancelResp1.Body.Close() }()

	// Try to cancel again (should be a conflict error)
	_, cancelResp2 := cancelOrder(t, createResult.OrderUUID)
	defer func() { _ = cancelResp2.Body.Close() }()

	require.Equal(t, http.StatusConflict, cancelResp2.StatusCode)
}

// Additional validation tests

func TestOrder_Create_WithWeaponOnly(t *testing.T) {
	req := &CreateOrderRequest{
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
		WeaponUUID: strPtr(WeaponLaserUUID),
	}

	result, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NotNil(t, result)
	expectedTotal := int64(HullAluminumPrice + EngineIonCPrice + WeaponLaserPrice)
	assert.Equal(t, expectedTotal, result.TotalPrice)
}

func TestOrder_Pay_AllMethods(t *testing.T) {
	t.Parallel()

	methods := []string{"CARD", "SBP", "CREDIT_CARD", "INVESTOR_MONEY"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			t.Parallel()

			// Create an order
			createReq := &CreateOrderRequest{
				HullUUID:   HullAluminumUUID,
				EngineUUID: EngineIonCUUID,
			}
			createResult, createResp := createOrder(t, createReq)
			defer func() { _ = createResp.Body.Close() }()
			require.NotNil(t, createResult)

			// Pay using this method
			payReq := &PayOrderRequest{PaymentMethod: method}
			payResult, resp := payOrder(t, createResult.OrderUUID, payReq)
			defer func() { _ = resp.Body.Close() }()

			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.NotNil(t, payResult)
			assert.NotEmpty(t, payResult.TransactionUUID)

			// Check that the payment method was saved
			order, getResp := getOrder(t, createResult.OrderUUID)
			defer func() { _ = getResp.Body.Close() }()
			require.NotNil(t, order.PaymentMethod)
			assert.Equal(t, method, *order.PaymentMethod)
		})
	}
}

func TestOrder_Get_WithOptionalParts(t *testing.T) {
	shieldUUID := ShieldEnergyUUID
	weaponUUID := WeaponLaserUUID
	req := &CreateOrderRequest{
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
		ShieldUUID: &shieldUUID,
		WeaponUUID: &weaponUUID,
	}

	createResult, createResp := createOrder(t, req)
	defer func() { _ = createResp.Body.Close() }()
	require.NotNil(t, createResult)

	// Get the order and check that the optional parts were saved
	order, resp := getOrder(t, createResult.OrderUUID)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, order.ShieldUUID)
	require.NotNil(t, order.WeaponUUID)
	assert.Equal(t, shieldUUID, *order.ShieldUUID)
	assert.Equal(t, weaponUUID, *order.WeaponUUID)
}

func TestPayment_PayOrder_InvalidUUIDFormat(t *testing.T) {
	_, err := paymentClient.PayOrder(context.Background(), &paymentv1.PayOrderRequest{
		OrderUuid:     "invalid-uuid-format",
		PaymentMethod: paymentv1.PaymentMethod_PAYMENT_METHOD_CARD,
	})
	require.Error(t, err)
	testutil.AssertGRPCStatus(t, err, codes.InvalidArgument)
}

// Full lifecycle tests

func TestOrder_FullLifecycle_CreatePayGet(t *testing.T) {
	// 1. Create an order
	createReq := &CreateOrderRequest{
		HullUUID:   HullTitaniumUUID,
		EngineUUID: EngineIonBUUID,
		ShieldUUID: strPtr(ShieldEnergyUUID),
	}
	createResult, createResp := createOrder(t, createReq)
	defer func() { _ = createResp.Body.Close() }()
	require.NotNil(t, createResult)
	assert.NotEmpty(t, createResult.OrderUUID)

	expectedTotal := int64(HullTitaniumPrice + EngineIonBPrice + ShieldEnergyPrice)
	assert.Equal(t, expectedTotal, createResult.TotalPrice)

	// 2. Get the order (check PENDING_PAYMENT)
	order1, getResp1 := getOrder(t, createResult.OrderUUID)
	defer func() { _ = getResp1.Body.Close() }()
	assert.Equal(t, "PENDING_PAYMENT", order1.Status)
	assert.Nil(t, order1.TransactionUUID)

	// 3. Pay for the order
	payReq := &PayOrderRequest{PaymentMethod: "SBP"}
	payResult, payResp := payOrder(t, createResult.OrderUUID, payReq)
	defer func() { _ = payResp.Body.Close() }()
	require.NotNil(t, payResult)
	assert.NotEmpty(t, payResult.TransactionUUID)

	// 4. Get the order (check PAID)
	order2, getResp2 := getOrder(t, createResult.OrderUUID)
	defer func() { _ = getResp2.Body.Close() }()

	assert.Equal(t, "PAID", order2.Status)
	require.NotNil(t, order2.TransactionUUID)
	assert.Equal(t, payResult.TransactionUUID, *order2.TransactionUUID)
	require.NotNil(t, order2.PaymentMethod)
	assert.Equal(t, "SBP", *order2.PaymentMethod)
}

func TestOrder_FullLifecycle_CreateCancelGet(t *testing.T) {
	// 1. Create an order
	createReq := &CreateOrderRequest{
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	defer func() { _ = createResp.Body.Close() }()
	require.NotNil(t, createResult)

	// 2. Get the order (check PENDING_PAYMENT)
	order1, getResp1 := getOrder(t, createResult.OrderUUID)
	defer func() { _ = getResp1.Body.Close() }()
	assert.Equal(t, "PENDING_PAYMENT", order1.Status)

	// 3. Cancel the order
	_, cancelResp := cancelOrder(t, createResult.OrderUUID)
	defer func() { _ = cancelResp.Body.Close() }()

	// 4. Get the order (check CANCELLED)
	order2, getResp2 := getOrder(t, createResult.OrderUUID)
	defer func() { _ = getResp2.Body.Close() }()

	assert.Equal(t, "CANCELLED", order2.Status)
	assert.Nil(t, order2.TransactionUUID)
}

func TestOrder_FullLifecycle_AllPartsPayGet(t *testing.T) {
	// Full lifecycle with all 4 parts: hull + engine + shield + weapon
	shieldUUID := ShieldEnergyUUID
	weaponUUID := WeaponLaserUUID
	createReq := &CreateOrderRequest{
		HullUUID:   HullTitaniumUUID,
		EngineUUID: EngineIonBUUID,
		ShieldUUID: &shieldUUID,
		WeaponUUID: &weaponUUID,
	}

	// 1. Create an order
	createResult, createResp := createOrder(t, createReq)
	defer func() { _ = createResp.Body.Close() }()
	require.NotNil(t, createResult)

	expectedTotal := int64(HullTitaniumPrice + EngineIonBPrice + ShieldEnergyPrice + WeaponLaserPrice)
	assert.Equal(t, expectedTotal, createResult.TotalPrice)

	// 2. Check all parts in the GET response
	order1, getResp1 := getOrder(t, createResult.OrderUUID)
	defer func() { _ = getResp1.Body.Close() }()
	assert.Equal(t, HullTitaniumUUID, order1.HullUUID)
	assert.Equal(t, EngineIonBUUID, order1.EngineUUID)
	require.NotNil(t, order1.ShieldUUID)
	assert.Equal(t, shieldUUID, *order1.ShieldUUID)
	require.NotNil(t, order1.WeaponUUID)
	assert.Equal(t, weaponUUID, *order1.WeaponUUID)

	// 3. Pay for the order
	payReq := &PayOrderRequest{PaymentMethod: "CREDIT_CARD"}
	payResult, payResp := payOrder(t, createResult.OrderUUID, payReq)
	defer func() { _ = payResp.Body.Close() }()
	require.NotNil(t, payResult)

	// 4. Check the final state
	order2, getResp2 := getOrder(t, createResult.OrderUUID)
	defer func() { _ = getResp2.Body.Close() }()

	assert.Equal(t, "PAID", order2.Status)
	require.NotNil(t, order2.PaymentMethod)
	assert.Equal(t, "CREDIT_CARD", *order2.PaymentMethod)
}

// Out-of-stock tests (StockQuantity <= 0)

func TestOrder_Create_OutOfStock(t *testing.T) {
	req := &CreateOrderRequest{
		HullUUID:   HullOutOfStockUUID,
		EngineUUID: EngineIonCUUID,
	}

	_, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusConflict, resp.StatusCode)
}

// Incompatible components test (CompatibilityChecker)

func TestOrder_Create_IncompatibleParts(t *testing.T) {
	// Titanium Hull (mass 3000kg) exceeds Ion Engine C's rating (max 1500kg) —
	// see inventory/pkg/domain's CompatibilityChecker.
	req := &CreateOrderRequest{
		HullUUID:   HullTitaniumUUID,
		EngineUUID: EngineIonCUUID,
	}

	result, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusConflict, resp.StatusCode)
	require.Nil(t, result)
}

// ogen validation tests (400 Bad Request)

func TestOrder_Create_InvalidBody_EmptyJSON(t *testing.T) {
	httpReq, err := http.NewRequest(http.MethodPost, orderBaseURL()+"/api/v1/orders", bytes.NewReader([]byte("{}")))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Create_InvalidBody_NotJSON(t *testing.T) {
	httpReq, err := http.NewRequest(http.MethodPost, orderBaseURL()+"/api/v1/orders", bytes.NewReader([]byte("not json")))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Create_InvalidBody_MissingHullUUID(t *testing.T) {
	body := `{"engine_uuid": "` + EngineIonCUUID + `"}`
	httpReq, err := http.NewRequest(http.MethodPost, orderBaseURL()+"/api/v1/orders", bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Create_InvalidBody_MissingEngineUUID(t *testing.T) {
	body := `{"hull_uuid": "` + HullAluminumUUID + `"}`
	httpReq, err := http.NewRequest(http.MethodPost, orderBaseURL()+"/api/v1/orders", bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Create_InvalidBody_InvalidHullUUID(t *testing.T) {
	body := `{"hull_uuid": "not-a-uuid", "engine_uuid": "` + EngineIonCUUID + `"}`
	httpReq, err := http.NewRequest(http.MethodPost, orderBaseURL()+"/api/v1/orders", bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Get_InvalidUUIDInPath(t *testing.T) {
	resp, err := httpClient.Get(orderBaseURL() + "/api/v1/orders/not-a-uuid")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Pay_InvalidUUIDInPath(t *testing.T) {
	body := `{"payment_method": "CARD"}`
	httpReq, err := http.NewRequest(http.MethodPost, orderBaseURL()+"/api/v1/orders/not-a-uuid/pay", bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Pay_InvalidPaymentMethod(t *testing.T) {
	// Create an order
	createReq := &CreateOrderRequest{
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	defer func() { _ = createResp.Body.Close() }()
	require.NotNil(t, createResult)

	// Try to pay with an invalid method (ogen rejects it)
	body := `{"payment_method": "BITCOIN"}`
	httpReq, err := http.NewRequest(http.MethodPost,
		orderBaseURL()+"/api/v1/orders/"+createResult.OrderUUID+"/pay",
		bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Pay_MissingPaymentMethod(t *testing.T) {
	// Create an order
	createReq := &CreateOrderRequest{
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	defer func() { _ = createResp.Body.Close() }()
	require.NotNil(t, createResult)

	// Try to pay without payment_method
	body := `{}`
	httpReq, err := http.NewRequest(http.MethodPost,
		orderBaseURL()+"/api/v1/orders/"+createResult.OrderUUID+"/pay",
		bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Pay_EmptyBody(t *testing.T) {
	// Create an order
	createReq := &CreateOrderRequest{
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
	}
	createResult, createResp := createOrder(t, createReq)
	defer func() { _ = createResp.Body.Close() }()
	require.NotNil(t, createResult)

	httpReq, err := http.NewRequest(http.MethodPost,
		orderBaseURL()+"/api/v1/orders/"+createResult.OrderUUID+"/pay",
		bytes.NewReader([]byte("")))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestOrder_Cancel_InvalidUUIDInPath(t *testing.T) {
	httpReq, err := http.NewRequest(http.MethodPost, orderBaseURL()+"/api/v1/orders/not-a-uuid/cancel", nil)
	require.NoError(t, err)

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// Tests with shield only (no weapon)

func TestOrder_Create_WithShieldOnly(t *testing.T) {
	req := &CreateOrderRequest{
		HullUUID:   HullAluminumUUID,
		EngineUUID: EngineIonCUUID,
		ShieldUUID: strPtr(ShieldEnergyUUID),
	}

	result, resp := createOrder(t, req)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NotNil(t, result)
	expectedTotal := int64(HullAluminumPrice + EngineIonCPrice + ShieldEnergyPrice)
	assert.Equal(t, expectedTotal, result.TotalPrice)
}

// Inventory test: a part with zero stock

func TestInventory_GetPart_OutOfStock(t *testing.T) {
	resp, err := inventoryClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{
		Uuid: HullOutOfStockUUID,
	})
	require.NoError(t, err)

	part := resp.GetPart()
	assert.Equal(t, HullOutOfStockUUID, part.GetUuid())
	assert.Equal(t, int64(HullOutOfStockPrice), part.GetPrice())
	assert.Equal(t, int64(0), part.GetStockQuantity())
	assert.Equal(t, inventoryv1.PartType_PART_TYPE_HULL, part.GetPartType())
}

func TestInventory_ListParts_ByUuids_EmptyList(t *testing.T) {
	// Empty UUID list (should return all parts when filtering by type UNSPECIFIED)
	resp, err := inventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{
		Uuids: []string{},
	})
	require.NoError(t, err)
	assert.Len(t, resp.GetParts(), 7)
}

// Authentication tests

func TestOrder_Create_UserComesFromSession(t *testing.T) {
	// A user_uuid in the body is not part of the request schema any more and
	// must not override the session's user.
	body := `{"user_uuid": "` + uuid.NewString() + `", "hull_uuid": "` + HullAluminumUUID + `", "engine_uuid": "` + EngineIonCUUID + `"}`
	httpReq, err := http.NewRequest(http.MethodPost, orderBaseURL()+"/api/v1/orders", bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var created CreateOrderResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))

	order, getResp := getOrder(t, created.OrderUUID)
	defer func() { _ = getResp.Body.Close() }()
	require.Equal(t, http.StatusOK, getResp.StatusCode)
	assert.Equal(t, testUserUUID, order.UserUUID)
}

func TestOrder_Create_DifferentUsersOwnTheirOrders(t *testing.T) {
	otherUser, otherSession, err := registerAndLogin(context.Background(), "other-"+uuid.NewString(), "pw")
	require.NoError(t, err)

	body := `{"hull_uuid": "` + HullAluminumUUID + `", "engine_uuid": "` + EngineIonCUUID + `"}`
	httpReq, err := http.NewRequest(http.MethodPost, orderBaseURL()+"/api/v1/orders", bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+otherSession)

	resp, err := httpClient.Do(httpReq)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var created CreateOrderResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))

	order, getResp := getOrder(t, created.OrderUUID)
	defer func() { _ = getResp.Body.Close() }()
	assert.Equal(t, otherUser, order.UserUUID)
}

// unauthorizedRequests covers every Order endpoint.
func unauthorizedRequests(t *testing.T) []*http.Request {
	t.Helper()

	id := uuid.NewString()
	newReq := func(method, path, body string) *http.Request {
		req, err := http.NewRequest(method, orderBaseURL()+path, bytes.NewReader([]byte(body)))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")

		return req
	}

	return []*http.Request{
		newReq(http.MethodPost, "/api/v1/orders", `{"hull_uuid": "`+HullAluminumUUID+`", "engine_uuid": "`+EngineIonCUUID+`"}`),
		newReq(http.MethodGet, "/api/v1/orders/"+id, ""),
		newReq(http.MethodPost, "/api/v1/orders/"+id+"/pay", `{"payment_method": "CARD"}`),
		newReq(http.MethodPost, "/api/v1/orders/"+id+"/cancel", ""),
	}
}

func assertUnauthorized(t *testing.T, resp *http.Response) {
	t.Helper()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	var body ErrorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, http.StatusUnauthorized, body.Code)
}

func TestAuth_NoToken_Unauthorized(t *testing.T) {
	for _, req := range unauthorizedRequests(t) {
		t.Run(req.Method+" "+req.URL.Path, func(t *testing.T) {
			resp, err := anonClient.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assertUnauthorized(t, resp)
		})
	}
}

func TestAuth_InvalidToken_Unauthorized(t *testing.T) {
	for _, token := range []string{uuid.NewString(), "not-a-uuid"} {
		t.Run(token, func(t *testing.T) {
			req := unauthorizedRequests(t)[1]
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := anonClient.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assertUnauthorized(t, resp)
		})
	}
}

func TestAuth_LogoutRevokesSession(t *testing.T) {
	_, session, err := registerAndLogin(context.Background(), "logout-"+uuid.NewString(), "pw")
	require.NoError(t, err)

	createWith := func() *http.Response {
		req := unauthorizedRequests(t)[0]
		req.Header.Set("Authorization", "Bearer "+session)

		resp, doErr := anonClient.Do(req)
		require.NoError(t, doErr)

		return resp
	}

	resp := createWith()
	_ = resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	_, err = authClient.Logout(context.Background(), &authv1.LogoutRequest{SessionUuid: session})
	require.NoError(t, err)

	resp = createWith()
	defer func() { _ = resp.Body.Close() }()
	assertUnauthorized(t, resp)
}

func TestAuth_InventoryRequiresSession(t *testing.T) {
	// orderInventoryClient forwards only a session found on ctx; none here.
	_, err := orderInventoryClient.ListParts(context.Background(), &inventoryv1.ListPartsRequest{})
	testutil.AssertGRPCStatus(t, err, codes.Unauthenticated)

	_, err = orderInventoryClient.ListParts(auth.WithSessionUUID(context.Background(), uuid.NewString()), &inventoryv1.ListPartsRequest{})
	testutil.AssertGRPCStatus(t, err, codes.Unauthenticated)

	resp, err := orderInventoryClient.ListParts(auth.WithSessionUUID(context.Background(), testSession), &inventoryv1.ListPartsRequest{})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetParts())
}

// ASSEMBLED tests. Outside this suite the transition is driven by a
// ShipAssembled Kafka event; here the service method is called directly —
// with no session on ctx, exactly like the Kafka consumer, which works
// because Inventory exempts CommitParts from authentication.

func stockOf(t *testing.T, partUUID string) int64 {
	t.Helper()

	resp, err := inventoryClient.GetPart(context.Background(), &inventoryv1.GetPartRequest{Uuid: partUUID})
	require.NoError(t, err)

	return resp.GetPart().GetStockQuantity()
}

func TestOrder_Assembled_CommitsPartsAndBlocksCancelAndPay(t *testing.T) {
	createResult, createResp := createOrder(t, &CreateOrderRequest{
		HullUUID:   HullTitaniumUUID,
		EngineUUID: EngineIonBUUID,
	})
	defer func() { _ = createResp.Body.Close() }()
	require.NotNil(t, createResult)

	_, payResp := payOrder(t, createResult.OrderUUID, &PayOrderRequest{PaymentMethod: "CARD"})
	defer func() { _ = payResp.Body.Close() }()
	require.Equal(t, http.StatusOK, payResp.StatusCode)

	hullStockBefore := stockOf(t, HullTitaniumUUID)
	engineStockBefore := stockOf(t, EngineIonBUUID)

	require.NoError(t, orderSvc.MarkAssembled(context.Background(), uuid.MustParse(createResult.OrderUUID)))

	order, getResp := getOrder(t, createResult.OrderUUID)
	defer func() { _ = getResp.Body.Close() }()
	require.Equal(t, "ASSEMBLED", order.Status)

	assert.Equal(t, hullStockBefore-1, stockOf(t, HullTitaniumUUID), "hull should be written off")
	assert.Equal(t, engineStockBefore-1, stockOf(t, EngineIonBUUID), "engine should be written off")

	// A redelivered ShipAssembled must not write the parts off twice.
	require.NoError(t, orderSvc.MarkAssembled(context.Background(), uuid.MustParse(createResult.OrderUUID)))
	assert.Equal(t, hullStockBefore-1, stockOf(t, HullTitaniumUUID))

	_, cancelResp := cancelOrder(t, createResult.OrderUUID)
	defer func() { _ = cancelResp.Body.Close() }()
	assert.Equal(t, http.StatusConflict, cancelResp.StatusCode)

	_, repayResp := payOrder(t, createResult.OrderUUID, &PayOrderRequest{PaymentMethod: "CARD"})
	defer func() { _ = repayResp.Body.Close() }()
	assert.Equal(t, http.StatusConflict, repayResp.StatusCode)
}
