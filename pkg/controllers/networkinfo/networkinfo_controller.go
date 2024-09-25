/* Copyright © 2023 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: Apache-2.0 */

package networkinfo

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apimachineryruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"
	"github.com/vmware-tanzu/nsx-operator/pkg/controllers/common"
	"github.com/vmware-tanzu/nsx-operator/pkg/logger"
	"github.com/vmware-tanzu/nsx-operator/pkg/metrics"
	_ "github.com/vmware-tanzu/nsx-operator/pkg/nsx/ratelimiter"
	commonservice "github.com/vmware-tanzu/nsx-operator/pkg/nsx/services/common"
	"github.com/vmware-tanzu/nsx-operator/pkg/nsx/services/ipblocksinfo"
	"github.com/vmware-tanzu/nsx-operator/pkg/nsx/services/vpc"
)

var (
	log           = &logger.Log
	MetricResType = common.MetricResTypeNetworkInfo
)

// NetworkInfoReconciler NetworkInfoReconcile reconciles a NetworkInfo object
// Actually it is more like a shell, which is used to manage nsx VPC
type NetworkInfoReconciler struct {
	Client              client.Client
	Scheme              *apimachineryruntime.Scheme
	Service             *vpc.VPCService
	IPBlocksInfoService *ipblocksinfo.IPBlocksInfoService
	Recorder            record.EventRecorder
}

func (r *NetworkInfoReconciler) deleteStaleVPCs(ctx context.Context, ns string) error {
	isShared, err := r.Service.IsSharedVPCNamespaceByNS(ns)
	if err != nil {
		return fmt.Errorf("failed to check if Namespace is shared for NS %s: %w", ns, err)
	}
	if isShared {
		log.Info("shared Namespace, skipping deletion of NSX VPC", "Namespace", ns)
		return nil
	}
	// Retrieve stale VPCs associated with the Namespace
	staleVPCs := r.Service.GetVPCsByNamespace(ns)
	if len(staleVPCs) == 0 {
		log.Info("no VPCs found in store, skipping deletion of NSX VPC", "Namespace", ns)
		return nil
	}
	var deleteErrs []error
	for _, nsxVPC := range staleVPCs {
		if nsxVPC.Path == nil {
			log.Error(nil, "VPC path is nil, skipping", "VPC", nsxVPC)
			continue
		}
		if err := r.Service.DeleteVPC(*nsxVPC.Path); err != nil {
			log.Error(err, "failed to delete VPC in NSX", "VPC", nsxVPC.Path)
			deleteErrs = append(deleteErrs, fmt.Errorf("failed to delete VPC %s: %w", *nsxVPC.Path, err))
		}
	}
	if len(deleteErrs) > 0 {
		return fmt.Errorf("multiple errors occurred while deleting VPCs: %v", deleteErrs)
	}

	// Update the VPCNetworkConfiguration Status
	ncName, err := r.Service.GetNetworkconfigNameFromNS(ns)
	if err != nil {
		return fmt.Errorf("failed to get VPCNetworkConfiguration for Namespace when deleting stale VPCs %s: %w", ns, err)
	}
	deleteVPCNetworkConfigurationStatus(ctx, r.Client, ncName, staleVPCs, r.Service.ListVPC())
	return nil
}

func (r *NetworkInfoReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	startTime := time.Now()
	defer func() {
		log.Info("Finished reconciling NetworkInfo", "NetworkInfo", req.NamespacedName, "time", time.Since(startTime))
	}()
	metrics.CounterInc(r.Service.NSXConfig, metrics.ControllerSyncTotal, common.MetricResTypeNetworkInfo)

	networkInfoCR := &v1alpha1.NetworkInfo{}

	if err := r.Client.Get(ctx, req.NamespacedName, networkInfoCR); err != nil {
		if err := r.deleteStaleVPCs(ctx, networkInfoCR.GetNamespace()); err != nil {
			deleteFail(r, ctx, networkInfoCR, &err, r.Client)
			log.Error(err, "failed to delete stale nsx VPC, would retry exponentially", "NetworkInfo", req.NamespacedName)
			return common.ResultRequeue, err
		}
		log.Error(err, "unable to fetch NetworkInfo CR", "req", req.NamespacedName)
		return common.ResultNormal, client.IgnoreNotFound(err)
	}
	if !networkInfoCR.ObjectMeta.DeletionTimestamp.IsZero() {
		metrics.CounterInc(r.Service.NSXConfig, metrics.ControllerDeleteTotal, common.MetricResTypeNetworkInfo)
		if err := r.deleteStaleVPCs(ctx, networkInfoCR.GetNamespace()); err != nil {
			deleteFail(r, ctx, networkInfoCR, &err, r.Client)
			log.Error(err, "failed to delete stale nsx VPC, would retry exponentially", "NetworkInfo", req.NamespacedName)
			return common.ResultRequeue, err
		}
		deleteSuccess(r, ctx, networkInfoCR)
	}
	metrics.CounterInc(r.Service.NSXConfig, metrics.ControllerUpdateTotal, common.MetricResTypeNetworkInfo)
	// TODO:
	// 1. check whether the logic to get VPC network config can be replaced by GetVPCNetworkConfigByNamespace
	// 2. sometimes the variable nc points to a VPCNetworkInfo, sometimes it's a VPCNetworkConfiguration, we need to distinguish between them.
	ncName, err := r.Service.GetNetworkconfigNameFromNS(networkInfoCR.Namespace)
	if err != nil {
		log.Error(err, "failed to get network config name for VPC when creating NSX VPC", "NetworkInfo", networkInfoCR.Name)
		updateFail(r, ctx, networkInfoCR, &err, r.Client, nil)
		return common.ResultRequeueAfter10sec, err
	}
	nc, _exist := r.Service.GetVPCNetworkConfig(ncName)
	if !_exist {
		message := fmt.Sprintf("failed to read network config %s when creating NSX VPC", ncName)
		log.Info(message)
		updateFail(r, ctx, networkInfoCR, &err, r.Client, nil)
		return common.ResultRequeueAfter10sec, errors.New(message)
	}
	log.Info("got network config from store", "NetworkConfig", ncName)
	vpcNetworkConfiguration := &v1alpha1.VPCNetworkConfiguration{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: commonservice.SystemVPCNetworkConfigurationName}, vpcNetworkConfiguration)
	if err != nil {
		log.Error(err, "failed to get system VPCNetworkConfiguration")
		updateFail(r, ctx, networkInfoCR, &err, r.Client, nil)
		return common.ResultRequeueAfter10sec, err
	}
	gatewayConnectionReady, _, err := getGatewayConnectionStatus(ctx, vpcNetworkConfiguration)
	if err != nil {
		log.Error(err, "failed to get the gateway connection status", "req", req.NamespacedName)
		return common.ResultRequeueAfter10sec, err
	}

	gatewayConnectionReason := ""
	if !gatewayConnectionReady {
		if ncName == commonservice.SystemVPCNetworkConfigurationName {
			gatewayConnectionReady, gatewayConnectionReason, err = r.Service.ValidateGatewayConnectionStatus(&nc)
			log.Info("got the gateway connection status", "gatewayConnectionReady", gatewayConnectionReady, "gatewayConnectionReason", gatewayConnectionReason)
			if err != nil {
				log.Error(err, "failed to validate the edge and gateway connection", "org", nc.Org, "project", nc.NSXProject)
				updateFail(r, ctx, networkInfoCR, &err, r.Client, nil)
				return common.ResultRequeueAfter10sec, err
			}
			setVPCNetworkConfigurationStatusWithGatewayConnection(ctx, r.Client, vpcNetworkConfiguration, gatewayConnectionReady, gatewayConnectionReason)
		} else {
			log.Info("skipping reconciling the network info because the system gateway connection is not ready", "NetworkInfo", req.NamespacedName)
			return common.ResultRequeueAfter60sec, nil
		}
	}
	lbProvider := r.Service.GetLBProvider()
	createdVpc, err := r.Service.CreateOrUpdateVPC(networkInfoCR, &nc, lbProvider)
	if err != nil {
		log.Error(err, "create VPC failed, would retry exponentially", "VPC", req.NamespacedName)
		updateFail(r, ctx, networkInfoCR, &err, r.Client, nil)
		return common.ResultRequeueAfter10sec, err
	}

	var privateIPs []string
	var vpcConnectivityProfilePath string
	var nsxLBSPath string
	isPreCreatedVPC := vpc.IsPreCreatedVPC(nc)
	if isPreCreatedVPC {
		privateIPs = createdVpc.PrivateIps
		vpcConnectivityProfilePath = *createdVpc.VpcConnectivityProfile
		// Retrieve NSX lbs path if Avi is not used with the pre-created VPC.
		if createdVpc.LoadBalancerVpcEndpoint == nil || createdVpc.LoadBalancerVpcEndpoint.Enabled == nil ||
			!*createdVpc.LoadBalancerVpcEndpoint.Enabled {
			nsxLBSPath, err = r.Service.GetLBSsFromNSXByVPC(*createdVpc.Path)
			if err != nil {
				log.Error(err, "failed to get NSX LBS path with pre-created VPC", "VPC", createdVpc.Path)
				updateFail(r, ctx, networkInfoCR, &err, r.Client, nil)
				return common.ResultRequeueAfter10sec, err
			}
		}
	} else {
		privateIPs = nc.PrivateIPs
		vpcConnectivityProfilePath = nc.VPCConnectivityProfile
	}

	snatIP, path, cidr := "", "", ""

	vpcConnectivityProfile, err := r.Service.GetVpcConnectivityProfile(&nc, vpcConnectivityProfilePath)
	if err != nil {
		log.Error(err, "get VpcConnectivityProfile failed, would retry exponentially", "VPC", req.NamespacedName)
		updateFail(r, ctx, networkInfoCR, &err, r.Client, nil)
		return common.ResultRequeueAfter10sec, err
	}
	hasExternalIPs := true
	if ncName == commonservice.SystemVPCNetworkConfigurationName {
		if len(vpcConnectivityProfile.ExternalIpBlocks) == 0 {
			hasExternalIPs = false
			log.Error(err, "there is no ExternalIPBlock in VPC ConnectivityProfile", "VPC", req.NamespacedName)
		}
		setVPCNetworkConfigurationStatusWithNoExternalIPBlock(ctx, r.Client, vpcNetworkConfiguration, hasExternalIPs)
	}
	// currently, auto snat is not exposed, and use default value True
	// checking autosnat to support future extension in VPC configuration
	autoSnatEnabled := r.Service.IsEnableAutoSNAT(vpcConnectivityProfile)
	if autoSnatEnabled {
		snatIP, err = r.Service.GetDefaultSNATIP(*createdVpc)
		if err != nil {
			log.Error(err, "failed to read default SNAT ip from VPC", "VPC", createdVpc.Id)
			state := &v1alpha1.VPCState{
				Name:                    *createdVpc.DisplayName,
				DefaultSNATIP:           "",
				LoadBalancerIPAddresses: "",
				PrivateIPs:              privateIPs,
			}
			updateFail(r, ctx, networkInfoCR, &err, r.Client, state)
			return common.ResultRequeueAfter10sec, err
		}
	}
	if ncName == commonservice.SystemVPCNetworkConfigurationName {
		vpcNetworkConfiguration := &v1alpha1.VPCNetworkConfiguration{}
		err := r.Client.Get(ctx, types.NamespacedName{Name: ncName}, vpcNetworkConfiguration)
		if err != nil {
			log.Error(err, "failed to get VPCNetworkConfiguration", "Name", ncName)
			updateFail(r, ctx, networkInfoCR, &err, r.Client, nil)
			return common.ResultRequeueAfter10sec, err
		}
		log.Info("got the AutoSnat status", "autoSnatEnabled", autoSnatEnabled, "req", req.NamespacedName)
		setVPCNetworkConfigurationStatusWithSnatEnabled(ctx, r.Client, vpcNetworkConfiguration, autoSnatEnabled)
	}

	// if lb VPC enabled, read avi subnet path and cidr
	// nsx bug, if set LoadBalancerVpcEndpoint.Enabled to false, when read this VPC back,
	// LoadBalancerVpcEndpoint.Enabled will become a nil pointer.
	if lbProvider == vpc.AVILB && createdVpc.LoadBalancerVpcEndpoint != nil && createdVpc.LoadBalancerVpcEndpoint.Enabled != nil && *createdVpc.LoadBalancerVpcEndpoint.Enabled {
		path, cidr, err = r.Service.GetAVISubnetInfo(*createdVpc)
		if err != nil {
			log.Error(err, "failed to read lb subnet path and cidr", "VPC", createdVpc.Id)
			state := &v1alpha1.VPCState{
				Name:                    *createdVpc.DisplayName,
				DefaultSNATIP:           snatIP,
				LoadBalancerIPAddresses: "",
				PrivateIPs:              privateIPs,
			}
			updateFail(r, ctx, networkInfoCR, &err, r.Client, state)
			return common.ResultRequeueAfter10sec, err
		}
	}

	state := &v1alpha1.VPCState{
		Name:                    *createdVpc.DisplayName,
		DefaultSNATIP:           snatIP,
		LoadBalancerIPAddresses: cidr,
		PrivateIPs:              privateIPs,
		VPCPath:                 *createdVpc.Path,
	}

	if !isPreCreatedVPC {
		nsxLBSPath = r.Service.GetDefaultNSXLBSPathByVPC(*createdVpc.Id)
	}
	// AKO needs to know the AVI subnet path created by NSX
	setVPCNetworkConfigurationStatusWithLBS(ctx, r.Client, ncName, state.Name, path, nsxLBSPath, *createdVpc.Path)
	updateSuccess(r, ctx, networkInfoCR, r.Client, state, nc.Name, path)
	if ncName == commonservice.SystemVPCNetworkConfigurationName && (!gatewayConnectionReady || !autoSnatEnabled || !hasExternalIPs) {
		log.Info("requeuing the NetworkInfo CR because VPCNetworkConfiguration system is not ready", "gatewayConnectionReason", gatewayConnectionReason, "autoSnatEnabled", autoSnatEnabled, "hasExternalIPs", hasExternalIPs, "req", req)
		return common.ResultRequeueAfter60sec, nil
	}
	return common.ResultNormal, nil
}

func (r *NetworkInfoReconciler) setupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.NetworkInfo{}).
		WithOptions(
			controller.Options{
				MaxConcurrentReconciles: common.NumReconcile(),
			}).
		Watches(
			// For created/removed network config, add/remove from VPC network config cache,
			// and update IPBlocksInfo.
			// For modified network config, currently only support appending ips to public ip blocks,
			// update network config in cache and update nsx VPC object.
			&v1alpha1.VPCNetworkConfiguration{},
			&VPCNetworkConfigurationHandler{
				Client:              mgr.GetClient(),
				vpcService:          r.Service,
				ipBlocksInfoService: r.IPBlocksInfoService,
			},
			builder.WithPredicates(VPCNetworkConfigurationPredicate)).
		Complete(r)
}

// Start setup manager and launch GC
func (r *NetworkInfoReconciler) Start(mgr ctrl.Manager) error {
	err := r.setupWithManager(mgr)
	if err != nil {
		return err
	}
	return nil
}

// CollectGarbage logic for NSX VPC is that:
// 1. list all current existing namespace in kubernetes
// 2. list all the NSX VPC in vpcStore
// 3. loop all the NSX VPC to get its namespace, check if the namespace still exist
// 4. if ns do not exist anymore, delete the NSX VPC resource
// it implements the interface GarbageCollector method.
func (r *NetworkInfoReconciler) CollectGarbage(ctx context.Context) {
	startTime := time.Now()
	defer func() {
		log.Info("VPC garbage collection completed", "time", time.Since(startTime))
	}()
	// read all NSX VPC from VPC store
	nsxVPCList := r.Service.ListVPC()
	if len(nsxVPCList) == 0 {
		log.Info("No NSX VPCs found in the store, skipping garbage collection")
		return
	}

	// read all namespaces from k8s
	namespaces := &corev1.NamespaceList{}
	err := r.Client.List(ctx, namespaces)
	if err != nil {
		log.Error(err, "Failed to list Kubernetes Namespaces")
		return
	}
	nsSet := sets.NewString()
	for _, ns := range namespaces.Items {
		nsSet.Insert(ns.Name)
	}

	for i, nsxVPC := range nsxVPCList {
		nsxVPCNamespace := getNamespaceFromNSXVPC(&nsxVPCList[i])
		if nsSet.Has(nsxVPCNamespace) {
			continue
		}
		log.Info("Garbage collecting NSX VPC object", "VPC ID", nsxVPC.Id, "Namespace", nsxVPCNamespace)
		metrics.CounterInc(r.Service.NSXConfig, metrics.ControllerDeleteTotal, common.MetricResTypeNetworkInfo)

		if err = r.Service.DeleteVPC(*nsxVPC.Path); err != nil {
			log.Error(err, "Failed to delete NSX VPC", "VPC ID", nsxVPC.Id, "Namespace", nsxVPCNamespace)
			metrics.CounterInc(r.Service.NSXConfig, metrics.ControllerDeleteFailTotal, common.MetricResTypeNetworkInfo)
			continue
		}

		metrics.CounterInc(r.Service.NSXConfig, metrics.ControllerDeleteSuccessTotal, common.MetricResTypeNetworkInfo)
		log.Info("Successfully deleted NSX VPC", "VPC ID", nsxVPC.Id)
	}
}
