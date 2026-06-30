package decode

// Void ignores RPC response payloads for void-like methods.
func Void[T any](*T) struct{} {
	return struct{}{}
}
