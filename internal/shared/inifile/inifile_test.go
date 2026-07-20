package inifile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/errs"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.ini")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestReadValue(t *testing.T) {
	path := writeTemp(t, "[One]\r\nkey=value\r\nother = has = equals \r\n\r\n[Two]\r\nkey=second\r\n")

	v, err := ReadValue(path, "One", "key")
	require.NoError(t, err)
	assert.Equal(t, "value", v)

	// Values may contain '='; whitespace around key and value is trimmed.
	v, err = ReadValue(path, "One", "other")
	require.NoError(t, err)
	assert.Equal(t, "has = equals", v)

	// Section scoping: same key name in a later section.
	v, err = ReadValue(path, "Two", "key")
	require.NoError(t, err)
	assert.Equal(t, "second", v)
}

func TestReadValueCaseInsensitive(t *testing.T) {
	path := writeTemp(t, "[Section]\r\nKey=value\r\n")

	v, err := ReadValue(path, "sEcTiOn", "kEy")
	require.NoError(t, err)
	assert.Equal(t, "value", v)
}

func TestReadValueLastWinsForDuplicates(t *testing.T) {
	path := writeTemp(t, "[S]\r\nk=first\r\nk=last\r\n")

	v, err := ReadValue(path, "S", "k")
	require.NoError(t, err)
	assert.Equal(t, "last", v)
}

func TestReadValueStripsQuotes(t *testing.T) {
	path := writeTemp(t, "[S]\r\ndq=\" padded \"\r\nsq='single'\r\nmix=\"unbalanced'\r\n")

	v, err := ReadValue(path, "S", "dq")
	require.NoError(t, err)
	assert.Equal(t, " padded ", v)

	v, err = ReadValue(path, "S", "sq")
	require.NoError(t, err)
	assert.Equal(t, "single", v)

	v, err = ReadValue(path, "S", "mix")
	require.NoError(t, err)
	assert.Equal(t, "\"unbalanced'", v)
}

func TestReadValueNotFound(t *testing.T) {
	path := writeTemp(t, "[S]\r\nk=v\r\n")

	_, err := ReadValue(path, "S", "missing")
	assert.ErrorIs(t, err, errs.ErrNotFound)

	_, err = ReadValue(path, "Missing", "k")
	assert.ErrorIs(t, err, errs.ErrNotFound)

	_, err = ReadValue(filepath.Join(t.TempDir(), "absent.ini"), "S", "k")
	assert.ErrorIs(t, err, errs.ErrNotFound)
}

func TestReadSection(t *testing.T) {
	path := writeTemp(t, "[S]\r\n; comment line\r\na=1\r\nb=2\r\nb=3\r\nnoequals\r\n[T]\r\nc=4\r\n")

	m, err := ReadSection(path, "S")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "1", "b": "3"}, m)

	_, err = ReadSection(path, "Missing")
	assert.ErrorIs(t, err, errs.ErrNotFound)
}

func TestReadSectionNames(t *testing.T) {
	path := writeTemp(t, "[One]\r\na=1\r\n[Two]\r\n[Three] trailing junk\r\n")

	names, err := ReadSectionNames(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"One", "Two", "Three"}, names)
}

func TestWriteValueCreatesFileAndSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.ini")

	require.NoError(t, WriteValue(path, "S", "k", "v"))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "[S]\r\nk=v\r\n", string(raw))
}

func TestWriteValueUpdatesInPlace(t *testing.T) {
	path := writeTemp(t, "; header comment\r\n[S]\r\na=1\r\nk=old\r\nb=2\r\n\r\n[T]\r\nc=3\r\n")

	require.NoError(t, WriteValue(path, "S", "k", "new"))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "; header comment\r\n[S]\r\na=1\r\nk=new\r\nb=2\r\n\r\n[T]\r\nc=3\r\n", string(raw))
}

func TestWriteValueAppendsBeforeBlankSeparator(t *testing.T) {
	path := writeTemp(t, "[S]\r\na=1\r\n\r\n[T]\r\nc=3\r\n")

	require.NoError(t, WriteValue(path, "S", "b", "2"))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "[S]\r\na=1\r\nb=2\r\n\r\n[T]\r\nc=3\r\n", string(raw))
}

func TestWriteValueRemovesDuplicateKeys(t *testing.T) {
	path := writeTemp(t, "[S]\r\nk=first\r\nk=second\r\n")

	require.NoError(t, WriteValue(path, "S", "K", "only"))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "[S]\r\nK=only\r\n", string(raw))
}

func TestWriteValueNormalizesToCRLF(t *testing.T) {
	path := writeTemp(t, "[S]\na=1\n")

	require.NoError(t, WriteValue(path, "S", "b", "2"))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "[S]\r\na=1\r\nb=2\r\n", string(raw))
}

func TestEncodingRoundTripUTF8BOM(t *testing.T) {
	path := writeTemp(t, "\xEF\xBB\xBF[S]\r\na=1\r\n")

	require.NoError(t, WriteValue(path, "S", "b", "ümläut"))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "\xEF\xBB\xBF[S]\r\na=1\r\nb=ümläut\r\n", string(raw))

	v, err := ReadValue(path, "S", "b")
	require.NoError(t, err)
	assert.Equal(t, "ümläut", v)
}

func TestEncodingRoundTripUTF16LE(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wide.ini")
	require.NoError(t, os.WriteFile(path, encodeUTF16LE("[S]\r\na=世界\r\n"), 0o600))

	v, err := ReadValue(path, "S", "a")
	require.NoError(t, err)
	assert.Equal(t, "世界", v)

	require.NoError(t, WriteValue(path, "S", "b", "two"))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	// The file stays UTF-16LE with BOM.
	require.GreaterOrEqual(t, len(raw), 2)
	assert.Equal(t, []byte{0xFF, 0xFE}, raw[:2])
	assert.Equal(t, "[S]\r\na=世界\r\nb=two\r\n", decodeUTF16LE(raw[2:]))
}

func TestDeleteValue(t *testing.T) {
	path := writeTemp(t, "[S]\r\na=1\r\nk=x\r\nk=y\r\nb=2\r\n")

	require.NoError(t, DeleteValue(path, "S", "k"))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "[S]\r\na=1\r\nb=2\r\n", string(raw))

	// Idempotent for missing keys, sections and files.
	require.NoError(t, DeleteValue(path, "S", "k"))
	require.NoError(t, DeleteValue(path, "Missing", "k"))
	require.NoError(t, DeleteValue(filepath.Join(t.TempDir(), "absent.ini"), "S", "k"))
}

func TestDeleteSection(t *testing.T) {
	path := writeTemp(t, "[One]\r\na=1\r\n[Two]\r\nb=2\r\n[Three]\r\nc=3\r\n")

	require.NoError(t, DeleteSection(path, "Two"))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "[One]\r\na=1\r\n[Three]\r\nc=3\r\n", string(raw))

	// Idempotent for missing sections and files.
	require.NoError(t, DeleteSection(path, "Two"))
	require.NoError(t, DeleteSection(filepath.Join(t.TempDir(), "absent.ini"), "One"))
}

func TestWriteSectionReplacesBody(t *testing.T) {
	path := writeTemp(t, "[One]\r\nold=1\r\nstale=2\r\n\r\n[Two]\r\nb=2\r\n")

	require.NoError(t, WriteSection(path, "One", map[string]string{"z": "26", "a": "1"}))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	// Keys are written sorted; the blank separator line is preserved.
	assert.Equal(t, "[One]\r\na=1\r\nz=26\r\n\r\n[Two]\r\nb=2\r\n", string(raw))
}

func TestWriteSectionCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.ini")

	require.NoError(t, WriteSection(path, "S", map[string]string{"a": "1"}))

	m, err := ReadSection(path, "S")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "1"}, m)
}

func TestWriteSectionEmptyContentEmptiesSection(t *testing.T) {
	path := writeTemp(t, "[S]\r\na=1\r\n")

	require.NoError(t, WriteSection(path, "S", nil))

	m, err := ReadSection(path, "S")
	require.NoError(t, err)
	assert.Empty(t, m)
}

func TestUnknownLinesPreserved(t *testing.T) {
	original := "; top comment\r\n\r\n[S]\r\n; section comment\r\na=1\r\ngarbage line\r\n"
	path := writeTemp(t, original)

	require.NoError(t, WriteValue(path, "S", "a", "1"))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(raw))
}

func TestEmptyFileRoundTrip(t *testing.T) {
	path := writeTemp(t, "")

	_, err := ReadValue(path, "S", "k")
	assert.ErrorIs(t, err, errs.ErrNotFound)

	require.NoError(t, WriteValue(path, "S", "k", "v"))
	v, err := ReadValue(path, "S", "k")
	require.NoError(t, err)
	assert.Equal(t, "v", v)
}

func TestSectionHeaderWithBracketInValue(t *testing.T) {
	path := writeTemp(t, "[S]\r\nk=[not a section]\r\n")

	// A "key=[...]" line is a value, not a section header.
	names, err := ReadSectionNames(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"S"}, names)

	v, err := ReadValue(path, "S", "k")
	require.NoError(t, err)
	assert.Equal(t, "[not a section]", v)
}

func TestLongRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trip.ini")

	sections := []string{"Alpha", "Beta", "Gamma"}
	for _, s := range sections {
		for _, k := range []string{"one", "two", "three"} {
			require.NoError(t, WriteValue(path, s, k, s+"-"+k))
		}
	}
	for _, s := range sections {
		m, err := ReadSection(path, s)
		require.NoError(t, err)
		assert.Len(t, m, 3)
		assert.Equal(t, s+"-two", m["two"])
	}

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, 3, strings.Count(string(raw), "["))
}
