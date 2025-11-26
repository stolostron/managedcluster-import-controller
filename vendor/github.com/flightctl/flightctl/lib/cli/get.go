package cli

import (
	"context"
	"fmt"

	"github.com/flightctl/flightctl/lib/api/client"
)

func GetRepository(ctx context.Context, token string, server string, name string) (*client.ReadRepositoryResponse, error) {
	c, err := apiClientFromToken(token, server)
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	// Call API to get repository
	response, err := c.ReadRepositoryWithResponse(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("reading repository %s: %w", name, err)
	}

	return response, nil
}

func GetDevice(ctx context.Context, token string, server string, name string) (*client.ReadDeviceResponse, error) {
	c, err := apiClientFromToken(token, server)
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	// Call API to get device
	response, err := c.ReadDeviceWithResponse(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("reading device %s: %w", name, err)
	}

	return response, nil
}
