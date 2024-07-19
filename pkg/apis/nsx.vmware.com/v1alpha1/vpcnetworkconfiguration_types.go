/* Copyright © 2022-2023 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: Apache-2.0 */

// +kubebuilder:object:generate=true
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	AccessModePublic       string = "Public"
	AccessModePrivate      string = "Private"
	AccessModeProject      string = "PrivateTGW"
	LoadBalancerSizeSmall  string = "SMALL"
	LoadBalancerSizeMedium string = "MEDIUM"
	LoadBalancerSizeLarge  string = "LARGE"
	LoadBalancerSizeXlarge string = "XLARGE"
)

// VPCNetworkConfigurationSpec defines the desired state of VPCNetworkConfiguration.
// There is a default VPCNetworkConfiguration that applies to Namespaces
// do not have a VPCNetworkConfiguration assigned. When a field is not set
// in a Namespace's VPCNetworkConfiguration, the Namespace will use the value
// in the default VPCNetworkConfiguration.
type VPCNetworkConfigurationSpec struct {
	// NSX-T Project the Namespace associated with.
	NsxProject string `json:"nsxProject,omitempty"`

	// VpcConnectivityProfile ID. This profile has configuration related to creating VPC transit gateway attachment.
	VpcConnectivityProfile string `json:"vpcConnectivityProfile,omitempty"`

	// Private IPs.
	PrivateIPs []string `json:"privateIPs,omitempty"`

	// ShortID specifies Identifier to use when displaying VPC context in logs.
	// Less than equal to 8 characters.
	// +kubebuilder:validation:MaxLength=8
	// +optional
	ShortID string `json:"shortID,omitempty"`

	// NSX path of the VPC the Namespace associated with.
	// If vpc is set, only defaultIPv4SubnetSize and defaultSubnetAccessMode
	// take effect, other fields are ignored.
	// +optional
	VPC string `json:"vpc,omitempty"`

	// +kubebuilder:validation:Enum=SMALL;MEDIUM;LARGE;XLARGE
	LoadBalancerSize string `json:"loadBalancerSize,omitempty"`

	// Default size of Subnet based upon estimated workload count.
	// Defaults to 26.
	// +kubebuilder:default=26
	DefaultSubnetSize int `json:"defaultSubnetSize,omitempty"`
	// PodSubnetAccessMode defines the access mode of the default SubnetSet for PodVM.
	// Must be Public or Private.
	// +kubebuilder:validation:Enum=Public;Private;PrivateTGW
	PodSubnetAccessMode string `json:"podSubnetAccessMode,omitempty"`
}

// VPCNetworkConfigurationStatus defines the observed state of VPCNetworkConfiguration
type VPCNetworkConfigurationStatus struct {
	// VPCs describes VPC info, now it includes lb Subnet info which are needed for AKO.
	VPCs []VPCInfo `json:"vpcs,omitempty"`
}

// VPCInfo defines VPC info needed by tenant admin.
type VPCInfo struct {
	// VPC name.
	Name string `json:"name"`
	// AVISESubnetPath is the NSX Policy Path for the AVI SE Subnet.
	AVISESubnetPath string `json:"lbSubnetPath,omitempty"`
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// VPCNetworkConfiguration is the Schema for the vpcnetworkconfigurations API.
// +kubebuilder:resource:scope="Cluster"
// +kubebuilder:printcolumn:name="NsxProject",type=string,JSONPath=`.spec.nsxProject`,description="NsxProject the Namespace associated with"
// +kubebuilder:printcolumn:name="PrivateIPs",type=string,JSONPath=`.spec.privateIPs`,description="PrivateIPs assigned to the Namespace"
type VPCNetworkConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VPCNetworkConfigurationSpec   `json:"spec,omitempty"`
	Status VPCNetworkConfigurationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VPCNetworkConfigurationList contains a list of VPCNetworkConfiguration.
type VPCNetworkConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VPCNetworkConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VPCNetworkConfiguration{}, &VPCNetworkConfigurationList{})
}
