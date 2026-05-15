package bankan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateLabel_Valid(t *testing.T) {
	l := Label{ID: "ab1", Name: "Bug", Color: "#ef4444"}
	assert.NoError(t, ValidateLabel(l))
}

func TestValidateLabel_ShortHex(t *testing.T) {
	l := Label{ID: "ab1", Name: "Bug", Color: "#abc"}
	assert.NoError(t, ValidateLabel(l))
}

func TestValidateLabel_MissingID(t *testing.T) {
	l := Label{ID: "", Name: "Bug", Color: "#ef4444"}
	assert.Error(t, ValidateLabel(l))
}

func TestValidateLabel_MissingName(t *testing.T) {
	l := Label{ID: "ab1", Name: "", Color: "#ef4444"}
	assert.Error(t, ValidateLabel(l))
}

func TestValidateLabel_BadColor(t *testing.T) {
	l := Label{ID: "ab1", Name: "Bug", Color: "red"}
	assert.Error(t, ValidateLabel(l))
}

func TestValidateLabels_Unique(t *testing.T) {
	labels := []Label{
		{ID: "a1", Name: "Bug", Color: "#ff0000"},
		{ID: "b2", Name: "Feature", Color: "#0000ff"},
	}
	assert.NoError(t, ValidateLabels(labels))
}

func TestValidateLabels_DuplicateID(t *testing.T) {
	labels := []Label{
		{ID: "a1", Name: "Bug", Color: "#ff0000"},
		{ID: "a1", Name: "Feature", Color: "#0000ff"},
	}
	assert.Error(t, ValidateLabels(labels))
}

func TestValidateLabels_DuplicateName(t *testing.T) {
	labels := []Label{
		{ID: "a1", Name: "Bug", Color: "#ff0000"},
		{ID: "b2", Name: "bug", Color: "#0000ff"}, // same name, different case
	}
	assert.Error(t, ValidateLabels(labels))
}

func TestFindLabelByID(t *testing.T) {
	labels := []Label{
		{ID: "a1", Name: "Bug", Color: "#ff0000"},
		{ID: "b2", Name: "Feature", Color: "#0000ff"},
	}

	l, ok := FindLabelByID(labels, "b2")
	require.True(t, ok)
	assert.Equal(t, "Feature", l.Name)

	_, ok = FindLabelByID(labels, "xx")
	assert.False(t, ok)
}

func TestFindLabelByName(t *testing.T) {
	labels := []Label{
		{ID: "a1", Name: "Bug", Color: "#ff0000"},
	}

	l, ok := FindLabelByName(labels, "BUG")
	require.True(t, ok)
	assert.Equal(t, "a1", l.ID)

	_, ok = FindLabelByName(labels, "missing")
	assert.False(t, ok)
}
