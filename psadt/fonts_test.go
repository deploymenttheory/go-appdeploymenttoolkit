package psadt

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTestFont assembles a minimal sfnt font carrying a single name-table
// record (Windows platform) with the given nameID and value.
func buildTestFont(nameID uint16, value string) []byte {
	// String storage: UTF-16BE encoded value.
	units := utf16.Encode([]rune(value))
	str := make([]byte, len(units)*2)
	for i, u := range units {
		binary.BigEndian.PutUint16(str[i*2:], u)
	}

	// Name table: header (6) + one record (12) + string storage.
	name := make([]byte, 6+12+len(str))
	binary.BigEndian.PutUint16(name[0:], 0)                 // format
	binary.BigEndian.PutUint16(name[2:], 1)                 // count
	binary.BigEndian.PutUint16(name[4:], 18)                // stringOffset (6 + 12)
	binary.BigEndian.PutUint16(name[6:], platformWindows)   // platformID
	binary.BigEndian.PutUint16(name[8:], 1)                 // encodingID
	binary.BigEndian.PutUint16(name[10:], 0x409)            // languageID
	binary.BigEndian.PutUint16(name[12:], nameID)           // nameID
	binary.BigEndian.PutUint16(name[14:], uint16(len(str))) // length
	binary.BigEndian.PutUint16(name[16:], 0)                // offset within storage
	copy(name[18:], str)

	// Offset table (12) + one table record (16); name data follows at 28.
	const nameOffset = 28
	out := make([]byte, nameOffset+len(name))
	binary.BigEndian.PutUint32(out[0:], 0x00010000) // sfntVersion
	binary.BigEndian.PutUint16(out[4:], 1)          // numTables
	// Table record at offset 12: tag(4), checksum(4), offset(4), length(4).
	copy(out[12:16], "name")                                // tag
	binary.BigEndian.PutUint32(out[20:], nameOffset)        // table data offset
	binary.BigEndian.PutUint32(out[24:], uint32(len(name))) // table data length
	copy(out[nameOffset:], name)
	return out
}

func TestFontTitle(t *testing.T) {
	t.Run("full name preferred", func(t *testing.T) {
		title, err := fontTitle(buildTestFont(nameIDFullName, "Test Font Regular"))
		require.NoError(t, err)
		assert.Equal(t, "Test Font Regular", title)
	})

	t.Run("falls back to family name", func(t *testing.T) {
		title, err := fontTitle(buildTestFont(nameIDFontFamily, "Test Family"))
		require.NoError(t, err)
		assert.Equal(t, "Test Family", title)
	})

	t.Run("truncated data errors", func(t *testing.T) {
		_, err := fontTitle([]byte{0, 1, 2})
		assert.Error(t, err)
	})
}

func TestFontRegistryName(t *testing.T) {
	dir := t.TempDir()
	fontPath := filepath.Join(dir, "myfont.ttf")
	require.NoError(t, os.WriteFile(fontPath, buildTestFont(nameIDFullName, "My Font"), 0o644))
	assert.Equal(t, "My Font (TrueType)", fontRegistryName(fontPath, " (TrueType)"))

	// Unreadable/invalid font content falls back to the base file name.
	bad := filepath.Join(dir, "broken.otf")
	require.NoError(t, os.WriteFile(bad, []byte("not a font"), 0o644))
	assert.Equal(t, "broken (OpenType)", fontRegistryName(bad, " (OpenType)"))
}

func TestFontTypeSuffixes(t *testing.T) {
	assert.Equal(t, " (TrueType)", fontTypeSuffixes[".ttf"])
	assert.Equal(t, " (TrueType)", fontTypeSuffixes[".ttc"])
	assert.Equal(t, " (OpenType)", fontTypeSuffixes[".otf"])
}

func TestResolveFontRemoval(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "arial.ttf"), []byte("x"), 0o644))
	values := map[string]string{"Arial (TrueType)": "arial.ttf"}

	t.Run("by file name", func(t *testing.T) {
		display, file := resolveFontRemoval("arial.ttf", values, dir)
		assert.Equal(t, "Arial (TrueType)", display)
		assert.Equal(t, "arial.ttf", file)
	})

	t.Run("by display name", func(t *testing.T) {
		display, file := resolveFontRemoval("Arial (TrueType)", values, dir)
		assert.Equal(t, "Arial (TrueType)", display)
		assert.Equal(t, "arial.ttf", file)
	})

	t.Run("not registered", func(t *testing.T) {
		display, file := resolveFontRemoval("missing.ttf", values, dir)
		assert.Empty(t, display)
		assert.Empty(t, file)
	})
}
