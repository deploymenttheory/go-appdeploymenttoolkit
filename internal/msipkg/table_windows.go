package msipkg

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	ais "github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/applicationinstallationandservicing"
)

// msiNoMoreItems is ERROR_NO_MORE_ITEMS, MsiViewFetch's end-of-rows code.
const msiNoMoreItems = 259

// modmsi/procMsiOpenDatabaseW: the go-bindings-win32 MsiOpenDatabase wrapper
// marshals szPersist as a UTF-16 string, but the MSIDBOPEN_* persist modes
// are small integers cast to LPCTSTR, which a string parameter cannot
// express. We therefore call MsiOpenDatabaseW directly with the numeric
// persist mode (the bindings' MSIDBOPEN_* uintptr constants).
var (
	modmsi               = windows.NewLazySystemDLL("msi.dll")
	procMsiOpenDatabaseW = modmsi.NewProc("MsiOpenDatabaseW")
)

// openMsiDatabase opens an MSI database with a numeric MSIDBOPEN_* persist
// mode, returning the database handle.
func openMsiDatabase(path string, persist uintptr) (ais.MSIHANDLE, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, fmt.Errorf("msipkg: encoding path %s: %w", path, err)
	}
	var handle ais.MSIHANDLE
	code, _, _ := procMsiOpenDatabaseW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		persist,
		uintptr(unsafe.Pointer(&handle)),
	)
	//#nosec G115 -- MsiOpenDatabaseW returns a UINT; the syscall result fits 32 bits
	if err := winerr.FromMsi("MsiOpenDatabaseW "+path, uint32(code)); err != nil {
		return 0, err
	}
	return handle, nil
}

// closeMsiHandle releases an MSI handle, ignoring the (informational) result.
func closeMsiHandle(handle ais.MSIHANDLE) {
	if handle != 0 {
		_ = ais.MsiCloseHandle(handle)
	}
}

// closeMsiView closes and releases a view handle.
func closeMsiView(view ais.MSIHANDLE) {
	if view != 0 {
		_ = ais.MsiViewClose(view)
		_ = ais.MsiCloseHandle(view)
	}
}

// recordString reads a string field from an MSI record, growing the buffer
// when the first call reports ERROR_MORE_DATA.
func recordString(record ais.MSIHANDLE, field uint32) (string, error) {
	buf := make([]uint16, 256)
	size := uint32(len(buf)) //#nosec G115 -- buffer length is bounded by the allocations above
	code := ais.MsiRecordGetString(record, field, foundation.PWSTR(&buf[0]), &size)
	if code == uint32(windows.ERROR_MORE_DATA) {
		buf = make([]uint16, size+1)
		size = uint32(len(buf)) //#nosec G115 -- buffer length derives from the API-reported size
		code = ais.MsiRecordGetString(record, field, foundation.PWSTR(&buf[0]), &size)
	}
	if err := winerr.FromMsi("MsiRecordGetString", code); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf[:size]), nil
}

// escapeMsiSQL doubles single quotes for MSI SQL string literals.
func escapeMsiSQL(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// openExecutedView opens and executes a view over the database.
func openExecutedView(db ais.MSIHANDLE, query string) (ais.MSIHANDLE, error) {
	var view ais.MSIHANDLE
	if err := winerr.FromMsi(
		"MsiDatabaseOpenView",
		ais.MsiDatabaseOpenView(db, query, &view),
	); err != nil {
		return 0, err
	}
	if err := winerr.FromMsi("MsiViewExecute", ais.MsiViewExecute(view, 0)); err != nil {
		closeMsiView(view)
		return 0, err
	}
	return view, nil
}

// TableProperty reads a single Property-table value from an MSI database,
// mirroring Get-ADTMsiTableProperty for one property. A missing property
// returns an error wrapping winerr.ErrNotFound.
func TableProperty(msiPath, property string) (string, error) {
	if property == "" {
		return "", winerr.Wrap("msipkg: property name is required", winerr.ErrInvalidOption)
	}
	db, err := openMsiDatabase(msiPath, ais.MSIDBOPEN_READONLY)
	if err != nil {
		return "", err
	}
	defer closeMsiHandle(db)
	query := "SELECT `Value` FROM `Property` WHERE `Property` = '" + escapeMsiSQL(property) + "'"
	view, err := openExecutedView(db, query)
	if err != nil {
		return "", err
	}
	defer closeMsiView(view)
	var record ais.MSIHANDLE
	code := ais.MsiViewFetch(view, &record)
	if code == msiNoMoreItems {
		return "", winerr.Wrap("msipkg: property "+property+" in "+msiPath, winerr.ErrNotFound)
	}
	if err := winerr.FromMsi("MsiViewFetch", code); err != nil {
		return "", err
	}
	defer closeMsiHandle(record)
	return recordString(record, 1)
}

// AllProperties reads the whole Property table of an MSI database as a map,
// mirroring Get-ADTMsiTableProperty without a -Property filter.
func AllProperties(msiPath string) (map[string]string, error) {
	db, err := openMsiDatabase(msiPath, ais.MSIDBOPEN_READONLY)
	if err != nil {
		return nil, err
	}
	defer closeMsiHandle(db)
	view, err := openExecutedView(db, "SELECT `Property`, `Value` FROM `Property`")
	if err != nil {
		return nil, err
	}
	defer closeMsiView(view)
	props := map[string]string{}
	for {
		var record ais.MSIHANDLE
		code := ais.MsiViewFetch(view, &record)
		if code == msiNoMoreItems {
			return props, nil
		}
		if err := winerr.FromMsi("MsiViewFetch", code); err != nil {
			return nil, err
		}
		name, err := recordString(record, 1)
		if err == nil {
			var value string
			if value, err = recordString(record, 2); err == nil {
				props[name] = value
			}
		}
		closeMsiHandle(record)
		if err != nil {
			return nil, err
		}
	}
}

// SetProperty updates or inserts a Property-table row in an MSI database,
// mirroring Set-ADTMsiProperty (transacted open plus commit).
func SetProperty(msiPath, property, value string) error {
	if property == "" {
		return winerr.Wrap("msipkg: property name is required", winerr.ErrInvalidOption)
	}
	db, err := openMsiDatabase(msiPath, ais.MSIDBOPEN_TRANSACT)
	if err != nil {
		return err
	}
	defer closeMsiHandle(db)

	_, err = TableProperty(msiPath, property)
	exists := err == nil

	name, val := escapeMsiSQL(property), escapeMsiSQL(value)
	query := "INSERT INTO `Property` (`Property`, `Value`) VALUES ('" + name + "', '" + val + "')"
	if exists {
		query = "UPDATE `Property` SET `Value` = '" + val + "' WHERE `Property` = '" + name + "'"
	}
	view, err := openExecutedView(db, query)
	if err != nil {
		return err
	}
	closeMsiView(view)
	return winerr.FromMsi("MsiDatabaseCommit", ais.MsiDatabaseCommit(db))
}

// CreatePropertyTransform ports MsiUtilities.CreatePropertyTransformFile: it
// copies the MSI to a temp file, optionally applies an existing transform,
// assigns the given Property-table values, and generates newTransformPath as
// the difference between the modified copy and the original database.
func CreatePropertyTransform(
	msiPath, newTransformPath, applyTransformPath string,
	properties map[string]string,
) error {
	if len(properties) == 0 {
		return winerr.Wrap(
			"msipkg: at least one transform property is required",
			winerr.ErrInvalidOption,
		)
	}
	baseName := strings.TrimSuffix(filepath.Base(msiPath), filepath.Ext(msiPath))
	tempMsiPath := filepath.Join(
		os.TempDir(),
		baseName+"_"+strconv.Itoa(os.Getpid())+".transform.msi",
	)
	if err := copyFile(msiPath, tempMsiPath); err != nil {
		return err
	}
	defer func() { _ = os.Remove(tempMsiPath) }()

	original, err := openMsiDatabase(msiPath, ais.MSIDBOPEN_READONLY)
	if err != nil {
		return err
	}
	defer closeMsiHandle(original)

	if err := assignTransformProperties(tempMsiPath, applyTransformPath, properties); err != nil {
		return err
	}

	modified, err := openMsiDatabase(tempMsiPath, ais.MSIDBOPEN_READONLY)
	if err != nil {
		return err
	}
	defer closeMsiHandle(modified)

	if err := os.Remove(newTransformPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("msipkg: removing existing transform %s: %w", newTransformPath, err)
	}
	if err := winerr.FromMsi("MsiDatabaseGenerateTransform",
		ais.MsiDatabaseGenerateTransform(modified, original, newTransformPath)); err != nil {
		return err
	}
	// Validate against every condition, matching MsiUtilities' "all flags" mask.
	const validateAll = ais.MSITRANSFORM_VALIDATE(int32(ais.MSITRANSFORM_VALIDATE_UPGRADECODE)<<1 - 1)
	return winerr.FromMsi("MsiCreateTransformSummaryInfo",
		ais.MsiCreateTransformSummaryInfo(modified, original, newTransformPath,
			ais.MSITRANSFORM_ERROR_NONE, validateAll))
}

// assignTransformProperties opens the temp database transacted, applies the
// optional baseline transform and assigns the Property-table rows.
func assignTransformProperties(
	tempMsiPath, applyTransformPath string,
	properties map[string]string,
) error {
	db, err := openMsiDatabase(tempMsiPath, ais.MSIDBOPEN_TRANSACT)
	if err != nil {
		return err
	}
	defer closeMsiHandle(db)
	if applyTransformPath != "" {
		if err := winerr.FromMsi("MsiDatabaseApplyTransform",
			ais.MsiDatabaseApplyTransform(db, applyTransformPath, 0)); err != nil {
			return err
		}
	}
	view, err := openExecutedView(db, "SELECT `Property`, `Value` FROM `Property`")
	if err != nil {
		return err
	}
	defer closeMsiView(view)
	record := ais.MsiCreateRecord(2)
	if record == 0 {
		return winerr.Wrap("msipkg: MsiCreateRecord", winerr.ErrNotFound)
	}
	defer closeMsiHandle(record)
	for name, value := range properties {
		if strings.TrimSpace(name) == "" {
			return winerr.Wrap(
				"msipkg: transform property names must be non-empty",
				winerr.ErrInvalidOption,
			)
		}
		if err := winerr.FromMsi(
			"MsiRecordSetString",
			ais.MsiRecordSetString(record, 1, name),
		); err != nil {
			return err
		}
		if err := winerr.FromMsi(
			"MsiRecordSetString",
			ais.MsiRecordSetString(record, 2, value),
		); err != nil {
			return err
		}
		if err := winerr.FromMsi("MsiViewModify",
			ais.MsiViewModify(view, ais.MSIMODIFY_ASSIGN, record)); err != nil {
			return err
		}
	}
	return winerr.FromMsi("MsiDatabaseCommit", ais.MsiDatabaseCommit(db))
}

// copyFile copies src to dst, replacing dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src) //#nosec G304 -- copying the caller-designated MSI package
	if err != nil {
		return fmt.Errorf("msipkg: opening %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst) //#nosec G304 -- temp copy path composed above
	if err != nil {
		return fmt.Errorf("msipkg: creating %s: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("msipkg: copying %s: %w", src, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("msipkg: closing %s: %w", dst, err)
	}
	return nil
}
