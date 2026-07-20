// Package configmgr triggers Configuration Manager (SCCM/MECM) client
// schedule tasks via the ROOT\CCM SMS_Client.TriggerSchedule WMI method.
// The schedule-ID catalog and the schedule-GUID composition are portable and
// unit-tested; the WMI invocation lives in trigger_windows.go.
package configmgr

import (
	"fmt"
	"strings"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"
)

// ScheduleID is a Configuration Manager client schedule identifier, ported
// from PSADT's PSADT.ConfigMgr.TriggerScheduleId enum.
type ScheduleID uint16

// Schedule IDs. Each value packs a high and low byte: (high<<8)|low.
const (
	HardwareInventory                        ScheduleID = 0x0001
	SoftwareInventory                        ScheduleID = 0x0002
	HeartbeatDiscovery                       ScheduleID = 0x0003
	SoftwareInventoryFileCollection          ScheduleID = 0x0010
	IDMIFCollection                          ScheduleID = 0x0011
	ClientMachineAuthentication              ScheduleID = 0x0012
	RequestMachinePolicy                     ScheduleID = 0x0021
	EvaluateMachinePolicy                    ScheduleID = 0x0022
	RefreshDefaultMp                         ScheduleID = 0x0023
	RefreshLocationServices                  ScheduleID = 0x0024
	LocationServicesCleanup                  ScheduleID = 0x0025
	PolicyAgentRequestAssignment             ScheduleID = 0x0026
	PolicyAgentEvaluateAssignment            ScheduleID = 0x0027
	SoftwareMeteringReport                   ScheduleID = 0x0031
	SourceUpdate                             ScheduleID = 0x0032
	ClearProxySettingsCache                  ScheduleID = 0x0037
	PolicyAgentCleanup                       ScheduleID = 0x0040
	UserPolicyAgentCleanup                   ScheduleID = 0x0041
	PolicyAgentValidateMachinePolicy         ScheduleID = 0x0042
	PolicyAgentValidateUserPolicy            ScheduleID = 0x0043
	CertificateMaintenance                   ScheduleID = 0x0051
	PeerDistributionPointStatus              ScheduleID = 0x0061
	PeerDistributionPointProvisioning        ScheduleID = 0x0062
	ComplianceIntervalEnforcement            ScheduleID = 0x0071
	SoftwareUpdatesAgentAssignmentEvaluation ScheduleID = 0x0108
	SoftwareUpdatesScan                      ScheduleID = 0x0113
	UpdateStorePolicy                        ScheduleID = 0x0114
	ApplicationManagerPolicyAction           ScheduleID = 0x0121
	ApplicationManagerUserPolicyAction       ScheduleID = 0x0122
	ApplicationManagerGlobalEvaluationAction ScheduleID = 0x0123
	DiscoveryDataCollectionCycle             ScheduleID = 0x0103
	FileCollectionCycle                      ScheduleID = 0x0104
)

var scheduleNames = map[string]ScheduleID{
	"hardwareinventory":                        HardwareInventory,
	"softwareinventory":                        SoftwareInventory,
	"heartbeatdiscovery":                       HeartbeatDiscovery,
	"softwareinventoryfilecollection":          SoftwareInventoryFileCollection,
	"idmifcollection":                          IDMIFCollection,
	"clientmachineauthentication":              ClientMachineAuthentication,
	"requestmachinepolicy":                     RequestMachinePolicy,
	"evaluatemachinepolicy":                    EvaluateMachinePolicy,
	"refreshdefaultmp":                         RefreshDefaultMp,
	"refreshlocationservices":                  RefreshLocationServices,
	"locationservicescleanup":                  LocationServicesCleanup,
	"policyagentrequestassignment":             PolicyAgentRequestAssignment,
	"policyagentevaluateassignment":            PolicyAgentEvaluateAssignment,
	"softwaremeteringreport":                   SoftwareMeteringReport,
	"sourceupdate":                             SourceUpdate,
	"clearproxysettingscache":                  ClearProxySettingsCache,
	"policyagentcleanup":                       PolicyAgentCleanup,
	"userpolicyagentcleanup":                   UserPolicyAgentCleanup,
	"policyagentvalidatemachinepolicy":         PolicyAgentValidateMachinePolicy,
	"policyagentvalidateuserpolicy":            PolicyAgentValidateUserPolicy,
	"certificatemaintenance":                   CertificateMaintenance,
	"complianceintervalenforcement":            ComplianceIntervalEnforcement,
	"softwareupdatesagentassignmentevaluation": SoftwareUpdatesAgentAssignmentEvaluation,
	"softwareupdatesscan":                      SoftwareUpdatesScan,
	"updatestorepolicy":                        UpdateStorePolicy,
	"applicationmanagerpolicyaction":           ApplicationManagerPolicyAction,
	"applicationmanageruserpolicyaction":       ApplicationManagerUserPolicyAction,
	"applicationmanagerglobalevaluationaction": ApplicationManagerGlobalEvaluationAction,
	"discoverydatacollectioncycle":             DiscoveryDataCollectionCycle,
	"filecollectioncycle":                      FileCollectionCycle,
}

// ParseScheduleID resolves a PSADT schedule-ID name (case-insensitive).
func ParseScheduleID(name string) (ScheduleID, error) {
	if id, ok := scheduleNames[strings.ToLower(strings.TrimSpace(name))]; ok {
		return id, nil
	}
	return 0, winerr.Wrap("configmgr: unknown schedule id "+name, winerr.ErrInvalidOption)
}

// ScheduleGUID composes the sScheduleID argument SMS_Client.TriggerSchedule
// expects: a GUID whose final two bytes carry the schedule's high and low
// bytes, formatted with braces — e.g. SoftwareUpdatesScan (0x0113) yields
// "{00000000-0000-0000-0000-000000000113}".
func (id ScheduleID) ScheduleGUID() string {
	high := byte(id >> 8)
	low := byte(id & 0xFF)
	// A .NET Guid built from the 16-byte array {0×14, high, low} renders its
	// final 6-byte group (bytes 10-15) as "00000000HHLL".
	return fmt.Sprintf("{00000000-0000-0000-0000-00000000%02X%02X}", high, low)
}
