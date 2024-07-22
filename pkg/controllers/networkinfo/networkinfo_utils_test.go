package networkinfo

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vmware-tanzu/nsx-operator/pkg/apis/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/golang/mock/gomock"
	"github.com/vmware-tanzu/nsx-operator/pkg/config"
	mock_client "github.com/vmware-tanzu/nsx-operator/pkg/mock/controller-runtime/client"
	"github.com/vmware-tanzu/nsx-operator/pkg/nsx/services/common"
	"github.com/vmware-tanzu/nsx-operator/pkg/nsx/services/vpc"
	"k8s.io/apimachinery/pkg/runtime"
)

type fakeRecorder struct {
	records map[string]int
}

func (recorder *fakeRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	count, found := recorder.records[message]
	if !found {
		recorder.records[message] = 1
	} else {
		recorder.records[message] = count + 1
	}
}
func (recorder *fakeRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
}
func (recorder *fakeRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
}

func (recorder *fakeRecorder) countEvent() int {
	message := "NetworkInfo CR has been updated with VPC state"
	count, found := recorder.records[message]
	if !found {
		return 0
	}
	return count
}

func newRecorder() *fakeRecorder {
	return &fakeRecorder{
		records: make(map[string]int),
	}
}

func TestUpdateNetworkInfoVPCStates(t *testing.T) {
	service := &vpc.VPCService{
		Service: common.Service{
			NSXConfig: &config.NSXOperatorConfig{
				NsxConfig: &config.NsxConfig{
					EnforcementPoint: "vmc-enforcementpoint",
				},
			},
		},
	}
	mockCtl := gomock.NewController(t)
	k8sClient := mock_client.NewMockClient(mockCtl)
	r := &NetworkInfoReconciler{
		Client:  k8sClient,
		Scheme:  nil,
		Service: service,
	}

	ctx := context.Background()
	ns := "ns1"
	vpcState := &v1alpha1.VPCState{
		Name:                    "vpc1",
		VPCPath:                 "vpc1-path",
		DefaultSNATIP:           "AVI endpoint not found",
		LoadBalancerIPAddresses: "99.99.99.0/28",
		PrivateIPs:              []string{"1.1.1.0/24", "1.1.2.0/24"},
	}

	for _, tc := range []struct {
		name     string
		mockFunc func()
		expCount int
	}{
		{
			name: "error when listing NetworkInfo CRs",
			mockFunc: func() {
				k8sClient.EXPECT().List(gomock.Any(), &v1alpha1.NetworkInfoList{}, client.InNamespace(ns)).
					Return(errors.New("failure with list"))
			},
		},
		{
			name: "no NetworkInfo CRs were created",
			mockFunc: func() {
				k8sClient.EXPECT().List(gomock.Any(), &v1alpha1.NetworkInfoList{}, client.InNamespace(ns)).
					Return(nil).Do(func(ctx context.Context, obj client.ObjectList, opts ...client.ListOption) error {
					netList := obj.(*v1alpha1.NetworkInfoList)
					netList.Items = nil
					return nil
				})
			},
		},
		{
			name: "NetworkInfo has no VPC states",
			mockFunc: func() {
				k8sClient.EXPECT().List(gomock.Any(), &v1alpha1.NetworkInfoList{}, client.InNamespace(ns)).
					Return(nil).Do(func(ctx context.Context, obj client.ObjectList, opts ...client.ListOption) error {
					netList := obj.(*v1alpha1.NetworkInfoList)
					netList.Items = []v1alpha1.NetworkInfo{
						{
							ObjectMeta: metav1.ObjectMeta{Name: ns, Namespace: ns},
						},
					}
					return nil
				})
				k8sClient.EXPECT().Update(gomock.Any(), &v1alpha1.NetworkInfo{
					ObjectMeta: metav1.ObjectMeta{Name: ns, Namespace: ns},
					VPCs:       []v1alpha1.VPCState{*vpcState},
				}).Return(nil).Times(1)
			},
			expCount: 1,
		},
		{
			name: "VPC state has no changes",
			mockFunc: func() {
				k8sClient.EXPECT().List(gomock.Any(), &v1alpha1.NetworkInfoList{}, client.InNamespace(ns)).
					Return(nil).Do(func(ctx context.Context, obj client.ObjectList, opts ...client.ListOption) error {
					netList := obj.(*v1alpha1.NetworkInfoList)
					netList.Items = []v1alpha1.NetworkInfo{
						{
							ObjectMeta: metav1.ObjectMeta{Name: ns, Namespace: ns},
							VPCs: []v1alpha1.VPCState{
								{
									Name:                    "vpc1",
									VPCPath:                 "vpc1-path",
									DefaultSNATIP:           "AVI endpoint not found",
									LoadBalancerIPAddresses: "99.99.99.0/28",
									PrivateIPs:              []string{"1.1.2.0/24", "1.1.1.0/24"},
								},
							},
						},
					}
					return nil
				})
			},
		},
		{
			name: "failed to update private IPs with VPC state",
			mockFunc: func() {
				k8sClient.EXPECT().List(gomock.Any(), &v1alpha1.NetworkInfoList{}, client.InNamespace(ns)).
					Return(nil).Do(func(ctx context.Context, obj client.ObjectList, opts ...client.ListOption) error {
					netList := obj.(*v1alpha1.NetworkInfoList)
					netList.Items = []v1alpha1.NetworkInfo{
						{
							ObjectMeta: metav1.ObjectMeta{Name: ns, Namespace: ns},
							VPCs: []v1alpha1.VPCState{
								{
									Name:                    "vpc1",
									VPCPath:                 "vpc1-path",
									DefaultSNATIP:           "AVI endpoint not found",
									LoadBalancerIPAddresses: "99.99.99.0/28",
									PrivateIPs:              []string{"1.1.1.0/24"},
								},
							},
						},
					}
					return nil
				})
				k8sClient.EXPECT().Update(gomock.Any(), &v1alpha1.NetworkInfo{
					ObjectMeta: metav1.ObjectMeta{Name: ns, Namespace: ns},
					VPCs:       []v1alpha1.VPCState{*vpcState},
				}).Return(errors.New("failed to update NetworkInfo")).Times(1)
			},
		},
		{
			name: "new private IPs is added in VPC state",
			mockFunc: func() {
				k8sClient.EXPECT().List(gomock.Any(), &v1alpha1.NetworkInfoList{}, client.InNamespace(ns)).
					Return(nil).Do(func(ctx context.Context, obj client.ObjectList, opts ...client.ListOption) error {
					netList := obj.(*v1alpha1.NetworkInfoList)
					netList.Items = []v1alpha1.NetworkInfo{
						{
							ObjectMeta: metav1.ObjectMeta{Name: ns, Namespace: ns},
							VPCs: []v1alpha1.VPCState{
								{
									Name:                    "vpc1",
									VPCPath:                 "vpc1-path",
									DefaultSNATIP:           "AVI endpoint not found",
									LoadBalancerIPAddresses: "99.99.99.0/28",
									PrivateIPs:              []string{"1.1.1.0/24"},
								},
							},
						},
					}
					return nil
				})
				k8sClient.EXPECT().Update(gomock.Any(), &v1alpha1.NetworkInfo{
					ObjectMeta: metav1.ObjectMeta{Name: ns, Namespace: ns},
					VPCs:       []v1alpha1.VPCState{*vpcState},
				}).Return(nil).Times(1)
			},
			expCount: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := newRecorder()
			r.Recorder = recorder
			tc.mockFunc()
			updateNetworkInfoVPCStates(r, ctx, ns, vpcState)
			assert.Equal(t, tc.expCount, recorder.countEvent())
		})
	}
}
