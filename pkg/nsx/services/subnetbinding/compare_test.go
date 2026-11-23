package subnetbinding

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vmware/vsphere-automation-sdk-go/services/nsxt/model"

	servicecommon "github.com/vmware-tanzu/nsx-operator/pkg/nsx/services/common"
)

func TestSubnetConnectionBindingMapCompareTreatsUnsetAssociationAsTrunk(t *testing.T) {
	existing := &model.SubnetConnectionBindingMap{
		Id:                String("binding-1"),
		SubnetAssociation: String(model.SubnetConnectionBindingMap_SUBNET_ASSOCIATION_TRUNK),
	}
	desired := &model.SubnetConnectionBindingMap{
		Id: String("binding-1"),
	}

	require.False(t, servicecommon.CompareResource(
		SubnetConnectionBindingMapToComparable(existing),
		SubnetConnectionBindingMapToComparable(desired),
	))
}
