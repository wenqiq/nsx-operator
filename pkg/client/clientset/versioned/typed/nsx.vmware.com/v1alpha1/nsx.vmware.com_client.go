/* Copyright © 2024 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: Apache-2.0 */

// Code generated by client-gen. DO NOT EDIT.

package v1alpha1

import (
	"net/http"

	v1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/nsx.vmware.com/v1alpha1"
	"github.com/vmware-tanzu/nsx-operator/pkg/client/clientset/versioned/scheme"
	rest "k8s.io/client-go/rest"
)

type NsxV1alpha1Interface interface {
	RESTClient() rest.Interface
	IPPoolsGetter
	NSXServiceAccountsGetter
	NetworkInfosGetter
	SecurityPoliciesGetter
	StaticRoutesGetter
	SubnetsGetter
	SubnetPortsGetter
	SubnetSetsGetter
	VPCsGetter
	VPCNetworkConfigurationsGetter
}

// NsxV1alpha1Client is used to interact with features provided by the nsx.vmware.com group.
type NsxV1alpha1Client struct {
	restClient rest.Interface
}

func (c *NsxV1alpha1Client) IPPools(namespace string) IPPoolInterface {
	return newIPPools(c, namespace)
}

func (c *NsxV1alpha1Client) NSXServiceAccounts(namespace string) NSXServiceAccountInterface {
	return newNSXServiceAccounts(c, namespace)
}

func (c *NsxV1alpha1Client) NetworkInfos(namespace string) NetworkInfoInterface {
	return newNetworkInfos(c, namespace)
}

func (c *NsxV1alpha1Client) SecurityPolicies(namespace string) SecurityPolicyInterface {
	return newSecurityPolicies(c, namespace)
}

func (c *NsxV1alpha1Client) StaticRoutes(namespace string) StaticRouteInterface {
	return newStaticRoutes(c, namespace)
}

func (c *NsxV1alpha1Client) Subnets(namespace string) SubnetInterface {
	return newSubnets(c, namespace)
}

func (c *NsxV1alpha1Client) SubnetPorts(namespace string) SubnetPortInterface {
	return newSubnetPorts(c, namespace)
}

func (c *NsxV1alpha1Client) SubnetSets(namespace string) SubnetSetInterface {
	return newSubnetSets(c, namespace)
}

func (c *NsxV1alpha1Client) VPCs(namespace string) VPCInterface {
	return newVPCs(c, namespace)
}

func (c *NsxV1alpha1Client) VPCNetworkConfigurations() VPCNetworkConfigurationInterface {
	return newVPCNetworkConfigurations(c)
}

// NewForConfig creates a new NsxV1alpha1Client for the given config.
// NewForConfig is equivalent to NewForConfigAndClient(c, httpClient),
// where httpClient was generated with rest.HTTPClientFor(c).
func NewForConfig(c *rest.Config) (*NsxV1alpha1Client, error) {
	config := *c
	if err := setConfigDefaults(&config); err != nil {
		return nil, err
	}
	httpClient, err := rest.HTTPClientFor(&config)
	if err != nil {
		return nil, err
	}
	return NewForConfigAndClient(&config, httpClient)
}

// NewForConfigAndClient creates a new NsxV1alpha1Client for the given config and http client.
// Note the http client provided takes precedence over the configured transport values.
func NewForConfigAndClient(c *rest.Config, h *http.Client) (*NsxV1alpha1Client, error) {
	config := *c
	if err := setConfigDefaults(&config); err != nil {
		return nil, err
	}
	client, err := rest.RESTClientForConfigAndClient(&config, h)
	if err != nil {
		return nil, err
	}
	return &NsxV1alpha1Client{client}, nil
}

// NewForConfigOrDie creates a new NsxV1alpha1Client for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *NsxV1alpha1Client {
	client, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return client
}

// New creates a new NsxV1alpha1Client for the given RESTClient.
func New(c rest.Interface) *NsxV1alpha1Client {
	return &NsxV1alpha1Client{c}
}

func setConfigDefaults(config *rest.Config) error {
	gv := v1alpha1.SchemeGroupVersion
	config.GroupVersion = &gv
	config.APIPath = "/apis"
	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	return nil
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *NsxV1alpha1Client) RESTClient() rest.Interface {
	if c == nil {
		return nil
	}
	return c.restClient
}
