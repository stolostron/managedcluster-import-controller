package source

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	workv1 "open-cluster-management.io/api/work/v1"

	"open-cluster-management.io/sdk-go/test/integration/cloudevents/store"
)

func StatusHashGetter(obj *store.Resource) (string, error) {
	statusBytes, err := json.Marshal(&workv1.ManifestWorkStatus{Conditions: obj.Status.Conditions})
	if err != nil {
		return "", fmt.Errorf("failed to marshal resource status, %v", err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(statusBytes)), nil
}
