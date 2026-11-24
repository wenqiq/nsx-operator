package nsxserviceaccount

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	mpmodel "github.com/vmware/vsphere-automation-sdk-go/services/nsxt-mp/nsx/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/vmware-tanzu/nsx-operator/pkg/apis/legacy/v1alpha1"
	"github.com/vmware-tanzu/nsx-operator/pkg/config"
	"github.com/vmware-tanzu/nsx-operator/pkg/nsx"
	"github.com/vmware-tanzu/nsx-operator/pkg/nsx/services/common"
	nsxutil "github.com/vmware-tanzu/nsx-operator/pkg/nsx/util"
)

func (s *NSXServiceAccountService) desiredTenancy(ctx context.Context, obj *v1alpha1.NSXServiceAccount) (bool, string, error) {
	if s.NSXConfig == nil || s.NSXConfig.CoeConfig == nil || (!s.NSXConfig.CoeConfig.EnableVPCNetwork && !config.HasVPCNamespaces()) {
		return false, "", nil
	}
	if !s.NSXClient.NSXCheckVersion(nsx.AntreaMultiTenancy) {
		return false, "", nil
	}
	namespace := &corev1.Namespace{}
	if err := s.Client.Get(ctx, types.NamespacedName{Name: obj.Namespace}, namespace); err != nil {
		return false, "", err
	}
	if config.IsPerNamespaceProvidersSupported() && !config.IsVPCNamespace(namespace) {
		return false, "", nil
	}
	if s.VPCService == nil {
		return false, "", fmt.Errorf("VPC service is unavailable for native Antrea tenancy")
	}
	vpcs := s.VPCService.ListVPCInfo(obj.Namespace)
	if len(vpcs) != 1 || vpcs[0].OrgID == "" || vpcs[0].ProjectID == "" || vpcs[0].VPCID == "" {
		return false, "", fmt.Errorf("expected one realized namespace VPC for %s, got %d", obj.Namespace, len(vpcs))
	}
	return true, vpcs[0].GetVPCPath(), nil
}

// ReconcileMultiTenancy runs after restore and publishes status only after every NSX write succeeds.
func (s *NSXServiceAccountService) ReconcileMultiTenancy(ctx context.Context, obj *v1alpha1.NSXServiceAccount) error {
	enabled, path, err := s.desiredTenancy(ctx, obj)
	if err != nil {
		return err
	}
	if !enabled || (obj.Status.MultiTenancy && obj.Status.VPCPath == path) {
		return nil
	}
	return s.CreateOrUpdateNSXServiceAccount(ctx, obj)
}

// ensureNativeRegistration deliberately uses JSON for scope_path / node_id until generated SDKs
// expose them. The cluster transport retains normal authentication, retries and error handling.
func (s *NSXServiceAccountService) ensureNativeRegistration(name, scopePath, cert string, existingID *string, obj *v1alpha1.NSXServiceAccount) (string, error) {
	ccpObj := s.ClusterControlPlaneStore.GetByKey(name)
	hasCCP := len(s.ClusterControlPlaneStore.GetByIndex(common.TagScopeNSXServiceAccountCRUID, string(obj.UID))) > 0
	if hasCCP != (ccpObj != nil) {
		return "", fmt.Errorf("conflicting CCP ownership for %s", name)
	}
	piObj := s.PrincipalIdentityStore.GetByKey(name)
	hasPI := len(s.PrincipalIdentityStore.GetByIndex(common.TagScopeNSXServiceAccountCRUID, string(obj.UID))) > 0
	if hasPI != (piObj != nil) {
		return "", fmt.Errorf("conflicting PI ownership for %s", name)
	}
	path := "policy/api/v1/infra/sites/default/enforcement-points/default/cluster-control-planes/" + url.PathEscape(name)
	nodeID := ""
	if existingID != nil {
		nodeID = *existingID
	}
	// A previous PUT may have succeeded before its response or cache update failed.
	// Always re-read the remote object so retries use its revision and node ID.
	body := make(map[string]interface{})
	getErr := s.NSXClient.Cluster.HttpGetAndDecode(path, &body)
	if getErr != nil && !errors.Is(getErr, nsxutil.HttpNotFoundError) {
		return "", getErr
	}
	if getErr == nil {
		owner := ""
		if tags, ok := body["tags"].([]interface{}); ok {
			for _, value := range tags {
				tag, ok := value.(map[string]interface{})
				if ok && tag["scope"] == common.TagScopeNSXServiceAccountCRUID {
					owner, _ = tag["tag"].(string)
				}
			}
		}
		if owner != string(obj.UID) {
			return "", fmt.Errorf("remote CCP %s belongs to another account", name)
		}
		remoteID, _ := body["node_id"].(string)
		if remoteID == "" {
			return "", fmt.Errorf("CCP %s has no node ID", name)
		}
		if nodeID != "" && nodeID != remoteID {
			return "", fmt.Errorf("CCP node ID conflicts with recorded identity")
		}
		nodeID = remoteID
		if current, _ := body["scope_path"].(string); current != "" && current != scopePath {
			return "", fmt.Errorf("CCP %s already belongs to scope %s", name, current)
		}

		// NSX validates immutability. Missing inventory is expected before the adapter first starts.
		inventoryPath := "api/v1/fabric/container-clusters/" + url.PathEscape(nodeID)
		var inventory map[string]interface{}
		err := s.NSXClient.Cluster.HttpGetAndDecode(inventoryPath, &inventory)
		if err != nil {
			if !errors.Is(err, nsxutil.HttpNotFoundError) {
				return "", err
			}
		} else {
			if old, _ := inventory["node_id"].(string); old != "" && old != nodeID {
				return "", fmt.Errorf("inventory node ID conflicts with CCP %s", name)
			}
			if inventory["node_id"] != nodeID {
				_, err = s.NSXClient.Cluster.HttpPost("api/v1/inventory/container/"+url.PathEscape(nodeID)+"?action=updates", map[string]interface{}{
					"container_inventory_objects": []interface{}{map[string]interface{}{"object_update_type": "UPDATE", "container_object": map[string]interface{}{"resource_type": "ContainerCluster", "external_id": nodeID, "node_id": nodeID}}},
				})
				if err != nil {
					return "", err
				}
			}
		}
	} else if err := checkCCPCapacityBackoff(); err != nil {
		return "", err
	}

	// Read the current JSON object so PUT retains revision and immutable legacy fields.
	tags := make([]map[string]interface{}, 0)
	for _, tag := range s.buildBasicTags(obj) {
		tags = append(tags, map[string]interface{}{"scope": tag.Scope, "tag": tag.Tag})
	}
	body["resource_type"] = antreaClusterResourceType
	body["certificate"] = cert
	body["scope_path"] = scopePath
	body["node_type"] = "ANTREA_NODE"
	body["tags"] = tags
	if nodeID != "" {
		body["node_id"] = nodeID
	}
	if _, err := s.NSXClient.Cluster.HttpPut(path, body); err != nil {
		if nsxutil.IsCCPConnectionCapacityFullError(err) {
			MarkCCPConnectionCapacityFull()
		}
		return "", err
	}
	updated, err := s.NSXClient.ClusterControlPlanesClient.Get(siteId, enforcementpointId, name)
	if err != nil {
		return "", nsxutil.TransNSXApiError(err)
	}
	if updated.NodeId == nil || *updated.NodeId == "" {
		return "", fmt.Errorf("CCP %s has no realized node ID", name)
	}
	if nodeID != "" && nodeID != *updated.NodeId {
		return "", fmt.Errorf("CCP node ID changed during registration")
	}
	nodeID = *updated.NodeId
	s.ClusterControlPlaneStore.Add(&updated)

	// Recreate only a PI whose scoped role changed, keeping its name, node ID and certificate.
	if piObj != nil {
		pi := piObj.(*mpmodel.PrincipalIdentity)
		if principalIdentityHasScope(pi, scopePath) {
			if err := s.updatePICert(pi, name, cert); err != nil {
				return "", err
			}
			return nodeID, nil
		}
		if pi.Id == nil {
			return "", fmt.Errorf("PI %s has no ID", name)
		}
		if err := s.NSXClient.PrincipalIdentitiesClient.Delete(*pi.Id); err != nil {
			return "", nsxutil.TransNSXApiError(err)
		}
		s.PrincipalIdentityStore.Delete(pi)
	}
	pi, err := s.NSXClient.WithCertificateClient.Create(mpmodel.PrincipalIdentityWithCertificate{
		IsProtected: &isProtectedTrue, Name: &name, NodeId: &name, CertificatePem: &cert,
		RolesForPaths: []mpmodel.RolesForPath{
			{Path: &readerPath, Roles: []mpmodel.Role{{Role: &readerRole}}},
			{Path: &scopePath, Roles: []mpmodel.Role{{Role: &vpcRole}}},
		}, Tags: common.ConvertTagsToMPTags(s.buildBasicTags(obj)),
	})
	if err != nil {
		return "", nsxutil.TransNSXApiError(err)
	}
	s.PrincipalIdentityStore.Add(&pi)
	return nodeID, nil
}

func principalIdentityHasScope(pi *mpmodel.PrincipalIdentity, path string) bool {
	if pi.Role != nil && *pi.Role != "" {
		return false
	}
	if len(pi.RolesForPaths) != 2 {
		return false
	}
	reader, tenant := false, false
	for _, binding := range pi.RolesForPaths {
		if binding.Path == nil || len(binding.Roles) != 1 || binding.Roles[0].Role == nil {
			return false
		}
		switch {
		case *binding.Path == readerPath && *binding.Roles[0].Role == readerRole:
			reader = true
		case *binding.Path == path && *binding.Roles[0].Role == vpcRole:
			tenant = true
		default:
			return false
		}
	}
	return reader && tenant
}
