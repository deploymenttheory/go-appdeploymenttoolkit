// Package msipkg hosts the Windows Installer domain logic behind the psadt
// MSI facade functions: the msiexec exit-code message catalog, MSI database
// property access (Windows only) and uninstall-registry application
// enumeration through the regkey backend seam.
//
// Everything that does not require msi.dll is kept portable so it can be
// unit-tested on every platform.
package msipkg

import "fmt"

// msiExitCodeMessages is the Go port of the message catalog surfaced by
// PSADT's Get-ADTMsiExitCodeMessage / MsiUtilities.GetExceptionForMsiExitCode:
// the standard Windows Installer (msiexec.exe) exit codes and the messages
// msimsg.dll associates with them, suffixed with the WIN32_ERROR name.
var msiExitCodeMessages = map[int]string{
	0:    "The action completed successfully. (ERROR_SUCCESS)",
	13:   "The data is invalid. (ERROR_INVALID_DATA)",
	87:   "One of the parameters was invalid. (ERROR_INVALID_PARAMETER)",
	120:  "This function is not available for this platform. It is only available on Windows 2000 and Windows XP with Windows Installer version 2.0. (ERROR_CALL_NOT_IMPLEMENTED)",
	1259: "This error code only occurs when using Windows Installer version 2.0 and Windows XP or later. If Windows Installer determines a product may be incompatible with the current operating system, it displays a dialog box informing the user and asking whether to try to install anyway. This error code is returned if the user chooses not to try the installation. (ERROR_APPHELP_BLOCK)",
	1601: "The Windows Installer service could not be accessed. Contact your support personnel to verify that the Windows Installer service is properly registered. (ERROR_INSTALL_SERVICE_FAILURE)",
	1602: "The user cancelled installation. (ERROR_INSTALL_USEREXIT)",
	1603: "A fatal error occurred during installation. (ERROR_INSTALL_FAILURE)",
	1604: "Installation suspended, incomplete. (ERROR_INSTALL_SUSPEND)",
	1605: "This action is only valid for products that are currently installed. (ERROR_UNKNOWN_PRODUCT)",
	1606: "The feature identifier is not registered. (ERROR_UNKNOWN_FEATURE)",
	1607: "The component identifier is not registered. (ERROR_UNKNOWN_COMPONENT)",
	1608: "This is an unknown property. (ERROR_UNKNOWN_PROPERTY)",
	1609: "The handle is in an invalid state. (ERROR_INVALID_HANDLE_STATE)",
	1610: "The configuration data for this product is corrupt. Contact your support personnel. (ERROR_BAD_CONFIGURATION)",
	1611: "The component qualifier not present. (ERROR_INDEX_ABSENT)",
	1612: "The installation source for this product is not available. Verify that the source exists and that you can access it. (ERROR_INSTALL_SOURCE_ABSENT)",
	1613: "This installation package cannot be installed by the Windows Installer service. You must install a Windows service pack that contains a newer version of the Windows Installer service. (ERROR_INSTALL_PACKAGE_VERSION)",
	1614: "The product is uninstalled. (ERROR_PRODUCT_UNINSTALLED)",
	1615: "The SQL query syntax is invalid or unsupported. (ERROR_BAD_QUERY_SYNTAX)",
	1616: "The record field does not exist. (ERROR_INVALID_FIELD)",
	1618: "Another installation is already in progress. Complete that installation before proceeding with this install. (ERROR_INSTALL_ALREADY_RUNNING)",
	1619: "This installation package could not be opened. Verify that the package exists and is accessible, or contact the application vendor to verify that this is a valid Windows Installer package. (ERROR_INSTALL_PACKAGE_OPEN_FAILED)",
	1620: "This installation package could not be opened. Contact the application vendor to verify that this is a valid Windows Installer package. (ERROR_INSTALL_PACKAGE_INVALID)",
	1621: "There was an error starting the Windows Installer service user interface. Contact your support personnel. (ERROR_INSTALL_UI_FAILURE)",
	1622: "There was an error opening installation log file. Verify that the specified log file location exists and is writable. (ERROR_INSTALL_LOG_FAILURE)",
	1623: "This language of this installation package is not supported by your system. (ERROR_INSTALL_LANGUAGE_UNSUPPORTED)",
	1624: "There was an error applying transforms. Verify that the specified transform paths are valid. (ERROR_INSTALL_TRANSFORM_FAILURE)",
	1625: "This installation is forbidden by system policy. Contact your system administrator. (ERROR_INSTALL_PACKAGE_REJECTED)",
	1626: "The function could not be executed. (ERROR_FUNCTION_NOT_CALLED)",
	1627: "The function failed during execution. (ERROR_FUNCTION_FAILED)",
	1628: "An invalid or unknown table was specified. (ERROR_INVALID_TABLE)",
	1629: "The data supplied is the wrong type. (ERROR_DATATYPE_MISMATCH)",
	1630: "Data of this type is not supported. (ERROR_UNSUPPORTED_TYPE)",
	1631: "The Windows Installer service failed to start. Contact your support personnel. (ERROR_CREATE_FAILED)",
	1632: "The Temp folder is either full or inaccessible. Verify that the Temp folder exists and that you can write to it. (ERROR_INSTALL_TEMP_UNWRITABLE)",
	1633: "This installation package is not supported on this platform. Contact your application vendor. (ERROR_INSTALL_PLATFORM_UNSUPPORTED)",
	1634: "Component is not used on this machine. (ERROR_INSTALL_NOTUSED)",
	1635: "This patch package could not be opened. Verify that the patch package exists and is accessible, or contact the application vendor to verify that this is a valid Windows Installer patch package. (ERROR_PATCH_PACKAGE_OPEN_FAILED)",
	1636: "This patch package could not be opened. Contact the application vendor to verify that this is a valid Windows Installer patch package. (ERROR_PATCH_PACKAGE_INVALID)",
	1637: "This patch package cannot be processed by the Windows Installer service. You must install a Windows service pack that contains a newer version of the Windows Installer service. (ERROR_PATCH_PACKAGE_UNSUPPORTED)",
	1638: "Another version of this product is already installed. Installation of this version cannot continue. To configure or remove the existing version of this product, use Add/Remove Programs in Control Panel. (ERROR_PRODUCT_VERSION)",
	1639: "Invalid command line argument. Consult the Windows Installer SDK for detailed command-line help. (ERROR_INVALID_COMMAND_LINE)",
	1640: "The current user is not permitted to perform installations from a client session of a server running the Terminal Server role service. (ERROR_INSTALL_REMOTE_DISALLOWED)",
	1641: "The installer has initiated a restart. This message is indicative of a success. (ERROR_SUCCESS_REBOOT_INITIATED)",
	1642: "The installer cannot install the upgrade patch because the program being upgraded may be missing or the upgrade patch updates a different version of the program. Verify that the program to be upgraded exists on your computer and that you have the correct upgrade patch. (ERROR_PATCH_TARGET_NOT_FOUND)",
	1643: "The patch package is not permitted by system policy. (ERROR_PATCH_PACKAGE_REJECTED)",
	1644: "One or more customizations are not permitted by system policy. (ERROR_INSTALL_TRANSFORM_REJECTED)",
	1645: "Windows Installer does not permit installation from a Remote Desktop Connection. (ERROR_INSTALL_REMOTE_PROHIBITED)",
	1646: "The patch package is not a removable patch package. (ERROR_PATCH_REMOVAL_UNSUPPORTED)",
	1647: "The patch is not applied to this product. (ERROR_UNKNOWN_PATCH)",
	1648: "No valid sequence could be found for the set of patches. (ERROR_PATCH_NO_SEQUENCE)",
	1649: "Patch removal was disallowed by policy. (ERROR_PATCH_REMOVAL_DISALLOWED)",
	1650: "The XML patch data is invalid. (ERROR_INVALID_PATCH_XML)",
	1651: "Administrative user failed to apply patch for a per-user managed or a per-machine application that is in advertised state. (ERROR_PATCH_MANAGED_ADVERTISED_PRODUCT)",
	1652: "Windows Installer is not accessible when the computer is in Safe Mode. Exit Safe Mode and try again or try using System Restore to return your computer to a previous state. (ERROR_INSTALL_SERVICE_SAFEBOOT)",
	1653: "Could not perform a multiple-package transaction because rollback has been disabled. Multiple-package installations cannot run if rollback is disabled. (ERROR_FAIL_FAST_EXCEPTION)",
	1654: "The app that you are trying to run is not supported on this version of Windows. (ERROR_INSTALL_REJECTED)",
	1707: "The installation operation completed successfully. (ERROR_SUCCESS)",
	3010: "A restart is required to complete the install. This message is indicative of a success. This does not include installs where the ForceReboot action is run. (ERROR_SUCCESS_REBOOT_REQUIRED)",
}

// ExitCodeMessage returns the descriptive message for a Windows Installer
// (msiexec.exe) exit code, mirroring PSADT's Get-ADTMsiExitCodeMessage.
// Unknown codes yield a generic message carrying the code.
func ExitCodeMessage(code int) string {
	if msg, ok := msiExitCodeMessages[code]; ok {
		return msg
	}
	return fmt.Sprintf("Unknown MSI exit code (%d).", code)
}

// RebootExitCodes returns the msiexec exit codes indicating a required or
// initiated reboot (PSADT's default RebootExitCodes for MSI operations).
func RebootExitCodes() []int {
	return []int{1641, 3010}
}

// SuccessExitCodes returns the msiexec exit codes PSADT treats as successful
// MSI outcomes (including the reboot codes).
func SuccessExitCodes() []int {
	return []int{0, 1707, 3010, 1641}
}

// IsRebootExitCode reports whether the code signals a pending reboot.
func IsRebootExitCode(code int) bool {
	return code == 1641 || code == 3010
}

// IsSuccessExitCode reports whether the code is a successful MSI outcome.
func IsSuccessExitCode(code int) bool {
	switch code {
	case 0, 1707, 3010, 1641:
		return true
	default:
		return false
	}
}
