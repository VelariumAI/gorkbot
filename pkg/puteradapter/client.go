package puteradapter

import "context"

// Client is the mockable Puter operation sink.
type Client interface {
	FSRead(ctx context.Context, path PuterWorkspacePath) ([]byte, error)
	FSWrite(ctx context.Context, path PuterWorkspacePath, data []byte) error
	FSDelete(ctx context.Context, path PuterWorkspacePath) error
	FSMove(ctx context.Context, from, to PuterWorkspacePath) error
	KVGet(ctx context.Context, key PuterKVKey) ([]byte, error)
	KVSet(ctx context.Context, key PuterKVKey, value []byte) error
	KVDelete(ctx context.Context, key PuterKVKey) error
}
