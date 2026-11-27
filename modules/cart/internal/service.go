package core

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/FeliceGeracitano/go-modular-monolith/pkg/apperr"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/id"
)

var (
	ErrUnknownCustomer   = apperr.Invalid("cart.unknown_customer", "customer does not exist")
	ErrUnknownProduct    = apperr.Invalid("cart.unknown_product", "product does not exist")
	ErrInvalidQuantity   = apperr.Invalid("cart.invalid_quantity", "quantity must be positive")
	ErrInsufficientStock = apperr.Invalid("cart.insufficient_stock", "requested quantity exceeds available stock")
	ErrMixedCurrencies   = apperr.Invalid("cart.mixed_currencies", "all cart items must use the same currency")
	ErrItemNotFound      = apperr.NotFound("cart.item_not_found", "item not found in cart")
)

type Item struct {
	SKU       string
	Name      string
	Quantity  int64
	UnitPrice int64
	Currency  string
}

type Cart struct {
	CustomerID string
	Currency   string
	Items      []Item
}

func (c Cart) TotalAmount() int64 {
	var total int64
	for _, it := range c.Items {
		total += it.UnitPrice * it.Quantity
	}
	return total
}

type ProductInfo struct {
	SKU      string
	Name     string
	Price    int64
	Currency string
	Stock    int64
}

type Repository interface {
	Get(ctx context.Context, customerID string) (Cart, error)
	Save(ctx context.Context, cart Cart) error
	Clear(ctx context.Context, customerID string) error
}

type CustomerDirectory interface {
	Exists(ctx context.Context, customerID string) (bool, error)
}

type ProductCatalog interface {
	GetBySKU(ctx context.Context, sku string) (ProductInfo, bool, error)
}

type Service struct {
	repo      Repository
	customers CustomerDirectory
	products  ProductCatalog
}

func NewService(repo Repository, customers CustomerDirectory, products ProductCatalog) *Service {
	return &Service{repo: repo, customers: customers, products: products}
}

func (s *Service) Get(ctx context.Context, customerID string) (Cart, error) {
	if !id.Valid(customerID) {
		return Cart{}, ErrUnknownCustomer
	}
	exists, err := s.customers.Exists(ctx, customerID)
	if err != nil {
		return Cart{}, err
	}
	if !exists {
		return Cart{}, ErrUnknownCustomer
	}
	return s.repo.Get(ctx, customerID)
}

func (s *Service) AddItem(ctx context.Context, customerID, sku string, quantity int64) (Cart, error) {
	if quantity <= 0 {
		return Cart{}, ErrInvalidQuantity
	}
	cart, err := s.Get(ctx, customerID)
	if err != nil {
		return Cart{}, err
	}
	sku = strings.ToUpper(strings.TrimSpace(sku))
	prod, found, err := s.products.GetBySKU(ctx, sku)
	if err != nil {
		return Cart{}, err
	}
	if !found {
		return Cart{}, ErrUnknownProduct
	}
	if cart.Currency != "" && cart.Currency != prod.Currency {
		return Cart{}, ErrMixedCurrencies
	}

	idx := slices.IndexFunc(cart.Items, func(it Item) bool { return it.SKU == prod.SKU })
	newQty := quantity
	if idx >= 0 {
		newQty += cart.Items[idx].Quantity
	}
	if newQty > prod.Stock {
		return Cart{}, ErrInsufficientStock
	}

	item := Item{
		SKU:       prod.SKU,
		Name:      prod.Name,
		Quantity:  newQty,
		UnitPrice: prod.Price,
		Currency:  prod.Currency,
	}
	if idx >= 0 {
		cart.Items[idx] = item
	} else {
		cart.Items = append(cart.Items, item)
	}
	cart.Currency = prod.Currency
	slices.SortFunc(cart.Items, func(a, b Item) int { return strings.Compare(a.SKU, b.SKU) })

	if err := s.repo.Save(ctx, cart); err != nil {
		return Cart{}, err
	}
	return cart, nil
}

func (s *Service) RemoveItem(ctx context.Context, customerID, sku string) (Cart, error) {
	cart, err := s.Get(ctx, customerID)
	if err != nil {
		return Cart{}, err
	}
	sku = strings.ToUpper(strings.TrimSpace(sku))
	idx := slices.IndexFunc(cart.Items, func(it Item) bool { return it.SKU == sku })
	if idx < 0 {
		return Cart{}, ErrItemNotFound
	}
	cart.Items = slices.Delete(cart.Items, idx, idx+1)
	if len(cart.Items) == 0 {
		cart.Currency = ""
	}
	if err := s.repo.Save(ctx, cart); err != nil {
		return Cart{}, err
	}
	return cart, nil
}

func (s *Service) Clear(ctx context.Context, customerID string) error {
	return s.repo.Clear(ctx, customerID)
}

type MemoryStore struct {
	mu    sync.RWMutex
	carts map[string]Cart
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{carts: make(map[string]Cart)}
}

func (s *MemoryStore) Get(_ context.Context, customerID string) (Cart, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.carts[customerID]
	if !ok {
		return Cart{CustomerID: customerID, Items: []Item{}}, nil
	}
	c.Items = slices.Clone(c.Items)
	return c, nil
}

func (s *MemoryStore) Save(_ context.Context, cart Cart) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cart.Items = slices.Clone(cart.Items)
	s.carts[cart.CustomerID] = cart
	return nil
}

func (s *MemoryStore) Clear(_ context.Context, customerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.carts, customerID)
	return nil
}
