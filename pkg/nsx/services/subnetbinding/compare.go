package subnetbinding

import (
	"github.com/vmware/vsphere-automation-sdk-go/runtime/data"
	"github.com/vmware/vsphere-automation-sdk-go/services/nsxt/model"

	"github.com/vmware-tanzu/nsx-operator/pkg/nsx/services/common"
)

type (
	SubnetConnectionBindingMap model.SubnetConnectionBindingMap
)

func (m *SubnetConnectionBindingMap) Key() string {
	return *m.Id
}

func (m *SubnetConnectionBindingMap) Value() data.DataValue {
	// NSX treats an omitted SubnetAssociation as TRUNK. Normalize the effective
	// value before comparison so nil and explicit TRUNK are semantically equal.
	association := m.SubnetAssociation
	if association == nil || *association == "" {
		trunk := model.SubnetConnectionBindingMap_SUBNET_ASSOCIATION_TRUNK
		association = &trunk
	}
	s := &SubnetConnectionBindingMap{
		Id:                m.Id,
		DisplayName:       m.DisplayName,
		Tags:              m.Tags,
		SubnetPath:        m.SubnetPath,
		SubnetAssociation: association,
		VlanTrafficTag:    m.VlanTrafficTag,
	}
	dataValue, _ := ComparableToSubnetConnectionBindingMap(s).GetDataValue__()
	return dataValue
}

func SubnetConnectionBindingMapToComparable(bindingMap *model.SubnetConnectionBindingMap) common.Comparable {
	return (*SubnetConnectionBindingMap)(bindingMap)
}

func ComparableToSubnetConnectionBindingMap(bindingMap common.Comparable) *model.SubnetConnectionBindingMap {
	return (*model.SubnetConnectionBindingMap)(bindingMap.(*SubnetConnectionBindingMap))
}
