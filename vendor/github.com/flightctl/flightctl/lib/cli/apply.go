package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	apiClient "github.com/flightctl/flightctl/lib/api/client"
	"github.com/flightctl/flightctl/lib/apipublic/v1alpha1"
)

func ApplyRepository(ctx context.Context, token string, server string, r *v1alpha1.Repository) error {
	c, err := apiClientFromToken(token, server)
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	buf, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshalling resource: %w", err)
	}

	var res *apiClient.ReplaceRepositoryResponse
	res, err = c.ReplaceRepositoryWithBodyWithResponse(ctx, *r.Metadata.Name, "application/json", bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("applying repository: %w", err)
	}

	if res.StatusCode() != http.StatusOK && res.StatusCode() != http.StatusCreated {
		return fmt.Errorf("applying repository: status code %d", res.StatusCode())
	}

	return nil
}
