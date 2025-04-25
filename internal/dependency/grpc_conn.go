package dependency

import (
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	"github.com/fdogov/trading/internal/config"
)

// NewGrpcConn creates a new gRPC connection
func NewGrpcConn(cfg config.Dependency) (*grpc.ClientConn, error) {
	keepAliveParams := keepalive.ClientParameters{
		Time:                10 * time.Second,
		Timeout:             5 * time.Second,
		PermitWithoutStream: true,
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepAliveParams),
	}

	conn, err := grpc.Dial(cfg.Address, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial grpc server: %w", err)
	}

	return conn, nil
}
