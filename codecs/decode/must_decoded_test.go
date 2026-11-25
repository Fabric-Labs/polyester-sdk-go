package decode

// mustDecoded returns v, panicking on err: a decode the test expects to
// succeed.
func mustDecoded[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
