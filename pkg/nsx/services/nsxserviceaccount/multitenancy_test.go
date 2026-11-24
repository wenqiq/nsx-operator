package nsxserviceaccount

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/require"
	mpmodel "github.com/vmware/vsphere-automation-sdk-go/services/nsxt-mp/nsx/model"
	"github.com/vmware/vsphere-automation-sdk-go/services/nsxt/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/vmware-tanzu/nsx-operator/pkg/apis/legacy/v1alpha1"
	"github.com/vmware-tanzu/nsx-operator/pkg/config"
	"github.com/vmware-tanzu/nsx-operator/pkg/nsx"
	"github.com/vmware-tanzu/nsx-operator/pkg/nsx/services/common"
	nsxutil "github.com/vmware-tanzu/nsx-operator/pkg/nsx/util"
)

type tenancyVPCProvider struct {
	common.VPCServiceProvider
	vpcs []common.VPCResourceInfo
}

func (p tenancyVPCProvider) ListVPCInfo(string) []common.VPCResourceInfo { return p.vpcs }

func TestDesiredAntreaTenancy(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	config.SetPerNamespaceProvidersSupportedForTest(false)
	t.Cleanup(func() { config.SetPerNamespaceProvidersSupportedForTest(false) })
	for _, tc := range []struct {
		name      string
		supported bool
		count     int
		want      bool
		wantErr   bool
	}{
		{"old NSX", false, 0, false, false}, {"VPC not ready", true, 0, false, true}, {"native VPC", true, 1, true, false}, {"ambiguous VPC", true, 2, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			patches := gomonkey.ApplyMethod(reflect.TypeOf(&nsx.Client{}), "NSXCheckVersion", func(_ *nsx.Client, feature int) bool {
				require.Equal(t, nsx.AntreaMultiTenancy, feature)
				return tc.supported
			})
			defer patches.Reset()
			vpcs := make([]common.VPCResourceInfo, tc.count)
			for i := range vpcs {
				vpcs[i] = common.VPCResourceInfo{OrgID: "default", ProjectID: "custom", VPCID: "actual"}
			}
			s := &NSXServiceAccountService{Service: common.Service{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "tenant"}}).Build(), NSXClient: &nsx.Client{}, NSXConfig: &config.NSXOperatorConfig{CoeConfig: &config.CoeConfig{EnableVPCNetwork: true}}}, VPCService: tenancyVPCProvider{vpcs: vpcs}}
			enabled, path, err := s.desiredTenancy(context.Background(), &v1alpha1.NSXServiceAccount{ObjectMeta: metav1.ObjectMeta{Namespace: "tenant"}})
			require.Equal(t, tc.wantErr, err != nil)
			require.Equal(t, tc.want, enabled)
			if tc.want {
				require.Equal(t, "/orgs/default/projects/custom/vpcs/actual", path)
			}
		})
	}
}

func TestNativeTenancyMigrationOrderingAndRetry(t *testing.T) {
	for _, failScope := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "retry scope failure"}[failScope], func(t *testing.T) {
			name, id, uid, path := "cluster", "node-1", "account-uid", "/orgs/default/projects/custom/vpcs/real"
			tags := []model.Tag{{Scope: common.String(common.TagScopeNSXServiceAccountCRUID), Tag: &uid}}
			s := &NSXServiceAccountService{Service: common.Service{NSXConfig: &config.NSXOperatorConfig{CoeConfig: &config.CoeConfig{Cluster: "supervisor"}}, NSXClient: &nsx.Client{Cluster: &nsx.Cluster{}, ClusterControlPlanesClient: &fakeClusterControlPlanesClient{}, PrincipalIdentitiesClient: &fakePrincipalIdentitiesClient{}, WithCertificateClient: &fakeWithCertificateClient{}}}}
			s.SetUpStore()
			s.ClusterControlPlaneStore.Add(&model.ClusterControlPlane{Id: &name, NodeId: &id, Tags: tags})
			piID := "old-pi"
			oldPath := "/orgs/default/projects/legacy/vpcs/dummy"
			s.PrincipalIdentityStore.Add(&mpmodel.PrincipalIdentity{Id: &piID, Name: &name, NodeId: &name, Tags: common.ConvertTagsToMPTags(tags), RolesForPaths: []mpmodel.RolesForPath{{Path: &oldPath, Roles: []mpmodel.Role{{Role: &vpcRole}}}}})
			var order []string
			patched := gomonkey.NewPatches()
			defer patched.Reset()
			patched.ApplyMethod(reflect.TypeOf(&nsx.Cluster{}), "HttpGetAndDecode", func(_ *nsx.Cluster, path string, result interface{}) error {
				if strings.HasPrefix(path, "policy/") {
					*(result.(*map[string]interface{})) = map[string]interface{}{"node_id": id, "vhc_path": oldPath, "_revision": float64(7), "tags": []interface{}{map[string]interface{}{"scope": common.TagScopeNSXServiceAccountCRUID, "tag": uid}}}
					return nil
				}
				*(result.(*map[string]interface{})) = map[string]interface{}{"external_id": id}
				return nil
			})
			patched.ApplyMethod(reflect.TypeOf(&nsx.Cluster{}), "HttpPost", func(_ *nsx.Cluster, _ string, _ interface{}) (map[string]interface{}, error) {
				order = append(order, "inventory")
				return map[string]interface{}{}, nil
			})
			failed := false
			patched.ApplyMethod(reflect.TypeOf(&nsx.Cluster{}), "HttpPut", func(_ *nsx.Cluster, _ string, body interface{}) (map[string]interface{}, error) {
				order = append(order, "scope")
				require.Equal(t, path, body.(map[string]interface{})["scope_path"])
				require.Equal(t, float64(7), body.(map[string]interface{})["_revision"])
				require.Equal(t, oldPath, body.(map[string]interface{})["vhc_path"])
				if failScope && !failed {
					failed = true
					return nil, errors.New("scope unavailable")
				}
				return map[string]interface{}{}, nil
			})
			patched.ApplyMethod(reflect.TypeOf(&fakeClusterControlPlanesClient{}), "Get", func(_ *fakeClusterControlPlanesClient, _, _, _ string) (model.ClusterControlPlane, error) {
				return model.ClusterControlPlane{Id: &name, NodeId: &id, Tags: tags}, nil
			})
			patched.ApplyMethod(reflect.TypeOf(&fakePrincipalIdentitiesClient{}), "Delete", func(_ *fakePrincipalIdentitiesClient, _ string) error { order = append(order, "delete PI"); return nil })
			patched.ApplyMethod(reflect.TypeOf(&fakeWithCertificateClient{}), "Create", func(_ *fakeWithCertificateClient, request mpmodel.PrincipalIdentityWithCertificate) (mpmodel.PrincipalIdentity, error) {
				order = append(order, "create PI")
				require.Equal(t, "certificate", *request.CertificatePem)
				require.Equal(t, path, *request.RolesForPaths[1].Path)
				return mpmodel.PrincipalIdentity{Name: &name, NodeId: &name, Tags: common.ConvertTagsToMPTags(tags), RolesForPaths: request.RolesForPaths}, nil
			})
			obj := &v1alpha1.NSXServiceAccount{ObjectMeta: metav1.ObjectMeta{UID: types.UID(uid)}}
			got, err := s.ensureNativeRegistration(name, path, "certificate", &id, obj)
			if failScope {
				require.Error(t, err)
				require.Equal(t, []string{"inventory", "scope"}, order)
				require.NotNil(t, s.PrincipalIdentityStore.GetByKey(name))
				order = nil
				s.ClusterControlPlaneStore.Delete(&model.ClusterControlPlane{Id: &name}) // Simulate a restart before the successful remote write was cached.
				got, err = s.ensureNativeRegistration(name, path, "certificate", &id, obj)
			}
			require.NoError(t, err)
			require.Equal(t, id, got)
			require.Equal(t, []string{"inventory", "scope", "delete PI", "create PI"}, order)
			require.False(t, obj.Status.MultiTenancy, "the caller publishes status only after success")
		})
	}
}

func TestNativeMigrationResumesAfterPIDeleted(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	config.SetPerNamespaceProvidersSupportedForTest(false)
	name, id, uid := "cluster", "node", "uid"
	s := &NSXServiceAccountService{Service: common.Service{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "tenant"}}).Build(), NSXClient: &nsx.Client{}, NSXConfig: &config.NSXOperatorConfig{CoeConfig: &config.CoeConfig{EnableVPCNetwork: true}}}, VPCService: tenancyVPCProvider{vpcs: []common.VPCResourceInfo{{OrgID: "default", ProjectID: "custom", VPCID: "real"}}}}
	s.SetUpStore()
	s.ClusterControlPlaneStore.Add(&model.ClusterControlPlane{Id: &name, NodeId: &id, Tags: []model.Tag{{Scope: common.String(common.TagScopeNSXServiceAccountCRUID), Tag: &uid}}})
	patches := gomonkey.ApplyMethod(reflect.TypeOf(&nsx.Client{}), "NSXCheckVersion", func(_ *nsx.Client, _ int) bool { return true })
	defer patches.Reset()
	resumed := false
	patches.ApplyMethod(reflect.TypeOf(s), "CreateOrUpdateNSXServiceAccount", func(_ *NSXServiceAccountService, _ context.Context, _ *v1alpha1.NSXServiceAccount) error {
		resumed = true
		return nil
	})
	obj := &v1alpha1.NSXServiceAccount{ObjectMeta: metav1.ObjectMeta{UID: types.UID(uid), Namespace: "tenant"}, Status: v1alpha1.NSXServiceAccountStatus{ClusterName: name, ClusterID: id, Phase: v1alpha1.NSXServiceAccountPhaseRealized}}
	require.NoError(t, s.RestoreRealizedNSXServiceAccount(context.Background(), obj))
	require.True(t, resumed)
}

func TestNativePrincipalIdentityScope(t *testing.T) {
	path := "/orgs/default/projects/p/vpcs/v"
	desired := []mpmodel.RolesForPath{{Path: &readerPath, Roles: []mpmodel.Role{{Role: &readerRole}}}, {Path: &path, Roles: []mpmodel.Role{{Role: &vpcRole}}}}
	require.True(t, principalIdentityHasScope(&mpmodel.PrincipalIdentity{RolesForPaths: desired}, path))
	extra := "/orgs/default/projects/other/vpcs/other"
	require.False(t, principalIdentityHasScope(&mpmodel.PrincipalIdentity{RolesForPaths: append(desired, mpmodel.RolesForPath{Path: &extra, Roles: []mpmodel.Role{{Role: &vpcRole}}})}, path))
	require.False(t, principalIdentityHasScope(&mpmodel.PrincipalIdentity{Role: common.String("enterprise_admin"), RolesForPaths: desired}, path))
}

func TestNativeCertificateUpdatePreservesScope(t *testing.T) {
	name, id, scope, cert := "cluster", "node", "/orgs/default/projects/p/vpcs/v", "new-certificate"
	s := &NSXServiceAccountService{Service: common.Service{NSXConfig: &config.NSXOperatorConfig{CoeConfig: &config.CoeConfig{EnableVPCNetwork: true}}, NSXClient: &nsx.Client{Cluster: &nsx.Cluster{}, ClusterControlPlanesClient: &fakeClusterControlPlanesClient{}}}}
	s.SetUpStore()
	patches := gomonkey.ApplyMethod(reflect.TypeOf(&nsx.Client{}), "NSXCheckVersion", func(_ *nsx.Client, _ int) bool { return true })
	defer patches.Reset()
	patches.ApplyMethod(reflect.TypeOf(&nsx.Cluster{}), "HttpGetAndDecode", func(_ *nsx.Cluster, _ string, result interface{}) error {
		*(result.(*map[string]interface{})) = map[string]interface{}{"scope_path": scope, "_revision": float64(4), "node_id": id}
		return nil
	})
	patches.ApplyMethod(reflect.TypeOf(&nsx.Cluster{}), "HttpPut", func(_ *nsx.Cluster, _ string, body interface{}) (map[string]interface{}, error) {
		fields := body.(map[string]interface{})
		require.Equal(t, scope, fields["scope_path"])
		require.Equal(t, float64(4), fields["_revision"])
		require.Equal(t, cert, fields["certificate"])
		return nil, nil
	})
	patches.ApplyMethod(reflect.TypeOf(&fakeClusterControlPlanesClient{}), "Get", func(_ *fakeClusterControlPlanesClient, _, _, _ string) (model.ClusterControlPlane, error) {
		return model.ClusterControlPlane{Id: &name, NodeId: &id}, nil
	})
	_, err := s.updateClusterControlPlane(name, model.ClusterControlPlane{Certificate: &cert})
	require.NoError(t, err)
}

func TestNewNativeRegistration(t *testing.T) {
	name, id, uid, path := "cluster", "node", "uid", "/orgs/default/projects/custom/vpcs/real"
	s := &NSXServiceAccountService{Service: common.Service{NSXConfig: &config.NSXOperatorConfig{CoeConfig: &config.CoeConfig{Cluster: "supervisor"}}, NSXClient: &nsx.Client{Cluster: &nsx.Cluster{}, ClusterControlPlanesClient: &fakeClusterControlPlanesClient{}, WithCertificateClient: &fakeWithCertificateClient{}}}}
	s.SetUpStore()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethod(reflect.TypeOf(&nsx.Cluster{}), "HttpGetAndDecode", func(_ *nsx.Cluster, _ string, _ interface{}) error { return nsxutil.HttpNotFoundError })
	patches.ApplyMethod(reflect.TypeOf(&nsx.Cluster{}), "HttpPut", func(_ *nsx.Cluster, _ string, body interface{}) (map[string]interface{}, error) {
		fields := body.(map[string]interface{})
		require.Equal(t, path, fields["scope_path"])
		require.Equal(t, "ANTREA_NODE", fields["node_type"])
		require.NotContains(t, fields, "vhc_path")
		require.NotContains(t, fields, "node_id")
		return nil, nil
	})
	patches.ApplyMethod(reflect.TypeOf(&fakeClusterControlPlanesClient{}), "Get", func(_ *fakeClusterControlPlanesClient, _, _, _ string) (model.ClusterControlPlane, error) {
		return model.ClusterControlPlane{Id: &name, NodeId: &id, Tags: []model.Tag{{Scope: common.String(common.TagScopeNSXServiceAccountCRUID), Tag: &uid}}}, nil
	})
	patches.ApplyMethod(reflect.TypeOf(&fakeWithCertificateClient{}), "Create", func(_ *fakeWithCertificateClient, request mpmodel.PrincipalIdentityWithCertificate) (mpmodel.PrincipalIdentity, error) {
		require.Equal(t, path, *request.RolesForPaths[1].Path)
		return mpmodel.PrincipalIdentity{Name: &name, NodeId: &name, Tags: request.Tags, RolesForPaths: request.RolesForPaths}, nil
	})
	obj := &v1alpha1.NSXServiceAccount{ObjectMeta: metav1.ObjectMeta{UID: types.UID(uid)}}
	got, err := s.ensureNativeRegistration(name, path, "certificate", nil, obj)
	require.NoError(t, err)
	require.Equal(t, id, got)
}
