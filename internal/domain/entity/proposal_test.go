package entity

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// OptionsJSON is the persisted column; only the decoded Options field may be
// serialized as "options", otherwise the JSON payload carries a duplicate key.
func TestProposalJSON_SerializesOptionsOnce(t *testing.T) {
	proposal := &Proposal{
		ID:          "p-1",
		Title:       "Election",
		OptionsJSON: `["candidate-A","candidate-B"]`,
		Options:     []string{"candidate-A", "candidate-B"},
	}

	raw, err := json.Marshal(proposal)
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(string(raw), `"options"`))

	var decoded struct {
		Options []string `json:"options"`
	}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, []string{"candidate-A", "candidate-B"}, decoded.Options)
}
