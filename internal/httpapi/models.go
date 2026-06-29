package httpapi

import (
	"net/http"
	"sort"

	"github.com/Pedro-0101/gix-server/internal/config"
)

type modelInfo struct {
	ID          string  `json:"id"`
	InputPrice  float64 `json:"inputPrice"`
	OutputPrice float64 `json:"outputPrice"`
}

func (s *Server) listModels(w http.ResponseWriter, r *http.Request) {
	models := make([]modelInfo, 0, len(config.ModelPrices))
	for id, p := range config.ModelPrices {
		models = append(models, modelInfo{
			ID:          id,
			InputPrice:  p.InputPrice,
			OutputPrice: p.OutputPrice,
		})
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})
	writeJSON(w, http.StatusOK, models)
}
