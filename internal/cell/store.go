package cell

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const pingTimeout = 2 * time.Second

const schema = `
create table if not exists orders (
	id          bigserial primary key,
	customer_id text        not null,
	item        text        not null,
	amount      numeric     not null,
	created_at  timestamptz not null default now()
);
create index if not exists orders_customer_idx on orders (customer_id);
`

type Order struct {
	ID         int64     `json:"id"`
	CustomerID string    `json:"customer_id"`
	Item       string    `json:"item"`
	Amount     float64   `json:"amount"`
	CreatedAt  time.Time `json:"created_at"`
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Bootstrap(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, schema)
	return err
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) CreateOrder(ctx context.Context, customerID, item string, amount float64) (Order, error) {
	const q = `
		insert into orders (customer_id, item, amount)
		values ($1, $2, $3)
		returning id, customer_id, item, amount, created_at`

	var o Order
	err := s.pool.QueryRow(ctx, q, customerID, item, amount).
		Scan(&o.ID, &o.CustomerID, &o.Item, &o.Amount, &o.CreatedAt)
	return o, err
}

func (s *Store) OrdersByCustomer(ctx context.Context, customerID string) ([]Order, error) {
	const q = `
		select id, customer_id, item, amount, created_at
		from orders
		where customer_id = $1
		order by created_at desc`

	rows, err := s.pool.Query(ctx, q, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []Order
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.CustomerID, &o.Item, &o.Amount, &o.CreatedAt); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	return orders, rows.Err()
}
