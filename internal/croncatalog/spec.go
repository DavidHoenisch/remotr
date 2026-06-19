package croncatalog

import (
	"bytes"

	"github.com/DavidHoenisch/remotr/internal/models"
	"gopkg.in/yaml.v3"
)

// JobSpecYAML serializes one cron job for agent execution.
func JobSpecYAML(job models.CronJob) ([]byte, error) {
	state := models.CronState{Crons: []models.CronJob{job}}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	if err := enc.Encode(state); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
