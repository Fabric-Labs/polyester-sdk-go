package services

// mustReply returns a decoded reply, failing the test (by panicking) when the
// decoder refused it.
func mustReply[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
