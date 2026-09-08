package market

import (
	_ "embed"
	"encoding/json"
)

// ETFLabel is read-only research metadata; it never alters ranking eligibility.
type ETFLabel struct {
	Market            string `json:"-"`
	Code              string `json:"-"`
	TrackingIndexID   string `json:"trackingIndexId"`
	TrackingIndexName string `json:"trackingIndexName"`
	AssetCategory     string `json:"assetCategory"`
	SourceURL         string `json:"sourceURL"`
	VerifiedAt        string `json:"verifiedAt"`
}

//go:embed etf_metadata.json
var etfMetadataJSON []byte

func ETFLabelFor(marketName, code string) *ETFLabel {
	var labels []ETFLabel
	if json.Unmarshal(etfMetadataJSON, &labels) != nil {
		return nil
	}
	for index := range labels {
		if labels[index].Market == marketName && labels[index].Code == code {
			label := labels[index]
			return &label
		}
	}
	return nil
}
