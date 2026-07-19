package psadt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeKB(t *testing.T) {
	assert.Equal(t, "KB2549864", normalizeKB("kb2549864"))
	assert.Equal(t, "KB2549864", normalizeKB("2549864"))
	assert.Equal(t, "KB2549864", normalizeKB(" KB2549864 "))
	assert.Equal(t, "", normalizeKB(""))
}
