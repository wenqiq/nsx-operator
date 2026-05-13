package subnetipreservation

import (
	"strings"

	"github.com/vmware/vsphere-automation-sdk-go/services/nsxt/model"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"
	"github.com/vmware-tanzu/nsx-operator/pkg/nsx/services/common"
	"github.com/vmware-tanzu/nsx-operator/pkg/util"
)

// NSX DynamicIpAddressReservation IP address type values.
// The CRD uses "IPV4IPV6" (no underscore) while NSX uses "IPV4_IPV6" (with underscore).
const (
	nsxIPAddressTypeIPv4     = "IPV4"
	nsxIPAddressTypeIPv6     = "IPV6"
	nsxIPAddressTypeIPv4IPv6 = "IPV4_IPV6"
)

// ipAddressTypeToNSX maps an IPAddressType to the NSX DynamicIpAddressReservation IpAddressType.
// It accepts both the current mixed-case CRD enum values ("IPv4"/"IPv6"/"IPv4IPv6") and the
// legacy all-caps values ("IPV4"/"IPV6"/"IPV4IPV6") that may be stored in older Subnet CRs.
// The CRD uses "IPV4IPV6" (no underscore) while NSX uses "IPV4_IPV6" (with underscore).
func ipAddressTypeToNSX(ipAddressType v1alpha1.IPAddressType) string {
	switch strings.ToUpper(string(ipAddressType)) {
	case "IPV6":
		return nsxIPAddressTypeIPv6
	case "IPV4IPV6":
		return nsxIPAddressTypeIPv4IPv6
	default:
		return nsxIPAddressTypeIPv4
	}
}

func (s *IPReservationService) buildDynamicIPReservation(ipReservation *v1alpha1.SubnetIPReservation, subnetPath string) *model.DynamicIpAddressReservation {
	tags := util.BuildBasicTags(getCluster(s), ipReservation, "")
	ipAddressType := ipAddressTypeToNSX(ipReservation.Spec.IPAddressType)
	nsxIPReservation := &model.DynamicIpAddressReservation{
		NumberOfIps:   common.Int64(int64(ipReservation.Spec.NumberOfIPs)),
		Tags:          tags,
		Id:            common.String(s.buildIPReservationID(ipReservation, subnetPath)),
		DisplayName:   common.String(ipReservation.Name),
		IpAddressType: &ipAddressType,
	}
	return nsxIPReservation
}

func (s *IPReservationService) buildStaticIPReservation(ipReservation *v1alpha1.SubnetIPReservation, subnetPath string) *model.StaticIpAddressReservation {
	tags := util.BuildBasicTags(getCluster(s), ipReservation, "")
	var reservedIPs []string
	// If ReservedIPs is not set, it implies this is a restore call for SubnetIPReservation with numberOfIPs
	// Use IPs in CR status to build the NSX Static IPReservation
	if len(ipReservation.Spec.ReservedIPs) == 0 {
		reservedIPs = ipReservation.Status.IPs
		log.Debug("Build Static IPReservation for restored SubnetIPReservation with IPs", "IPs", ipReservation.Status.IPs)
	} else {
		reservedIPs = ipReservation.Spec.ReservedIPs
	}
	nsxIPReservation := &model.StaticIpAddressReservation{
		ReservedIps: reservedIPs,
		Tags:        tags,
		Id:          common.String(s.buildIPReservationID(ipReservation, subnetPath)),
		DisplayName: common.String(ipReservation.Name),
	}
	return nsxIPReservation
}

func getCluster(service *IPReservationService) string {
	return service.NSXConfig.Cluster
}

// buildIPReservationID generates the ID of NSX SubnetIPReservation resource, its format is like this,
// ${SubnetIPReservation_CR}.name_hash(${parent_VpcSubnet}.Path)[:5], e.g., ipreservation1_823ca. Note, if
// the generated id has collision with the existing NSX SubnetIPReservation.id, a random UUID is used as
// an alternative of the parent path to generate the hash suffix.
func (s *IPReservationService) buildIPReservationID(ipReservation *v1alpha1.SubnetIPReservation, subnetPath string) string {
	idCR := &v1.ObjectMeta{
		Name: ipReservation.GetName(),
		UID:  types.UID(subnetPath),
	}
	return common.BuildUniqueIDWithRandomUUID(idCR, util.GenerateIDByObject, func(id string) bool {
		return s.DynamicIPReservationStore.GetByKey(id) != nil || s.StaticIPReservationStore.GetByKey(id) != nil
	})
}
