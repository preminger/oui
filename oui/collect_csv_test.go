package oui_test

import (
	"testing"

	"github.com/preminger/oui/v2/internal/logger"
	"github.com/preminger/oui/v2/oui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_CollectAll(t *testing.T) {
	results, err := oui.CollectAll(nil, logger.New())
	require.NoError(t, err)
	assert.IsType(t, []*oui.VendorDef{}, results)
	assert.True(t, len(results) > 5)
}
