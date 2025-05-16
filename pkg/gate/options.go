package gate

import (
	"gate/pkg/gate/db"
)

// Option is a function that configures a Gate instance
type Option func(*options)

type options struct {
	store db.Store
}

// WithStore sets the database store for the Gate instance
func WithStore(store db.Store) Option {
	return func(o *options) {
		o.store = store
	}
}
