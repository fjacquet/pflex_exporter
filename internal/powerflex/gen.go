package powerflex

import "github.com/fjacquet/pflex_exporter/internal/models"

// PowerFlex cluster generation classifications.
const (
	GenerationGen1    = "gen1"
	GenerationGen2    = "gen2"
	GenerationUnknown = "unknown"
)

// detectGeneration classifies a cluster by inspecting storage pool data layouts,
// mirroring Dell's Gen1 check: FineGranularity/MediumGranularity pools indicate Gen1
// (mirroring), ErasureCoding indicates Gen2. This exporter targets Gen1 only.
func detectGeneration(in *models.Instances) string {
	pools := in.Get(models.TypeStoragePool)
	if len(pools) == 0 {
		return GenerationUnknown
	}
	sawGen1 := false
	for _, p := range pools {
		switch p.DataLayout {
		case "ErasureCoding":
			return GenerationGen2
		case "FineGranularity", "MediumGranularity":
			sawGen1 = true
		}
	}
	if sawGen1 {
		return GenerationGen1
	}
	return GenerationUnknown
}
