package rental

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type HTTPRentalPolicy struct {
	baseURL string
	client  *http.Client
}

func NewHTTPRentalPolicy(baseURL string) *HTTPRentalPolicy {
	return &HTTPRentalPolicy{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

func (p *HTTPRentalPolicy) IsAllowed(equipment Equipment, quantity, days int) (bool, error) {
	body, err := json.Marshal(map[string]any{
		"equipmentId": equipment.ID, "quantity": quantity, "days": days,
	})
	if err != nil {
		return false, err
	}
	response, err := p.client.Post(p.baseURL+"/rental-policy/check", "application/json", bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("rental policy request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false, fmt.Errorf("rental policy: HTTP %d", response.StatusCode)
	}
	var result struct {
		Allowed *bool `json:"allowed"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("rental policy response: %w", err)
	}
	if result.Allowed == nil {
		return false, fmt.Errorf("rental policy response: missing allowed")
	}
	return *result.Allowed, nil
}
