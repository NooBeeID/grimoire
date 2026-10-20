package bus

import (
	"context"
	"fmt"
	"reflect"
)

// CallTyped memanggil endpoint dan menegaskan tipe respons T.
func CallTyped[T any](ctx context.Context, b *Registry, endpoint string, req any) (T, error) {
	var zero T
	if b == nil {
		return zero, ErrNilRegistry
	}
	res, err := b.Call(ctx, endpoint, req)
	if err != nil {
		return zero, err
	}
	out, ok := res.(T)
	if !ok {
		return zero, fmt.Errorf("%w: endpoint=%s want=%s got=%T", ErrBadResponseType, endpoint, reflect.TypeFor[T](), res)
	}
	return out, nil
}

// Expect menegaskan tipe request di handler provider.
func Expect[T any](req any) (T, error) {
	out, ok := req.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("%w: want=%s got=%T", ErrBadRequestType, reflect.TypeFor[T](), req)
	}
	return out, nil
}
