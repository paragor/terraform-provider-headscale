// Copyright (c) HashiCorp, Inc.

package headscaleclient

import (
	"context"

	"google.golang.org/grpc/credentials"
)

type tokenAuth struct {
	token string
}

func NewGRPCTokenAuth(token string) credentials.PerRPCCredentials {
	return &tokenAuth{token: token}
}

// Return value is mapped to request headers.
func (t *tokenAuth) GetRequestMetadata(
	ctx context.Context,
	in ...string,
) (map[string]string, error) {
	return map[string]string{
		"authorization": "Bearer " + t.token,
	}, nil
}

func (*tokenAuth) RequireTransportSecurity() bool {
	return true
}
