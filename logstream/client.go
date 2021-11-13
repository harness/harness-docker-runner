package logstream

import "context"

// Client defines a log service client.
type Client interface {
	// Upload upload the full log history to the data store
	Upload(ctx context.Context, key string, lines []*Line) error

	// Open opens the data stream.
	Open(ctx context.Context, key string) error

	// Close closes the data stream.
	Close(ctx context.Context, key string) error

	// Write writes logs to the data stream.
	Write(ctx context.Context, key string, lines []*Line) error
}
