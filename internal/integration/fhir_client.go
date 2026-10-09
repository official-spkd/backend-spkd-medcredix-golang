package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type FHIRClient struct {
	BaseURL    string
	AuthToken  string
	HTTPClient *http.Client
}

func NewFHIRClient(baseURL, token string) *FHIRClient {
	return &FHIRClient{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		AuthToken: token,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second, // MedCredix tidak boleh hang bila SIMRS down
		},
	}
}

// TestFHIRResource memanggil GET {base}/{resource}?_summary=count dan mengembalikan
// (berhasil, jumlah record, error). ctx dipakai agar pengujian massal bisa dibatalkan.
func (c *FHIRClient) TestFHIRResource(ctx context.Context, resourceType string) (bool, int, error) {
	endpoint := fmt.Sprintf("%s/%s?_summary=count", c.BaseURL, url.PathEscape(resourceType))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, 0, err
	}
	if c.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.AuthToken)
	}
	req.Header.Set("Accept", "application/fhir+json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return false, 0, fmt.Errorf("gagal menghubungi server eksternal: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, 0, fmt.Errorf("server merespons dengan HTTP %d", resp.StatusCode)
	}

	var bundle struct {
		ResourceType string `json:"resourceType"`
		Total        int    `json:"total"`
	}
	// Batasi 1 MB agar respons tak wajar tidak menghabiskan memori.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&bundle); err != nil {
		return false, 0, fmt.Errorf("respons bukan JSON FHIR yang valid: %w", err)
	}
	if bundle.ResourceType != "Bundle" {
		return false, 0, fmt.Errorf("respons bukan Bundle FHIR (resourceType=%q)", bundle.ResourceType)
	}
	return true, bundle.Total, nil
}