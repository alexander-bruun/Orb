// Package objstore provides an abstraction over storage backends for audio files.
package objstore

import (
	"context"
	"io"
)

// ObjectStore is the interface all storage backends implement.
type ObjectStore interface {
	// Put stores a new object. r is read exactly once; size is the total byte count.
	Put(ctx context.Context, key string, r io.Reader, size int64) error
	// GetRange returns a reader for [offset, offset+length) bytes of the object.
	GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error)
	// Delete removes an object. A non-existent key is not an error.
	Delete(ctx context.Context, key string) error
	// Exists reports whether the object with the given key is present.
	Exists(ctx context.Context, key string) (bool, error)
	// Size returns the byte length of the object.
	Size(ctx context.Context, key string) (int64, error)
}
