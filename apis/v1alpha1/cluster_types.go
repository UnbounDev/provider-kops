/*
Copyright 2022 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

// ClusterParameters are the configurable fields of a Cluster.
type ClusterParameters struct {

	// State is the location of the kops state file (usually an s3 bucket)
	State string `json:"state"`

	Cluster KopsClusterSpec `json:"cluster"`

	InstanceGroups []InstanceGroupSpec `json:"instanceGroups"`

	RollingUpdateOpts RollingUpdateOptsSpec `json:"rollingUpdateOpts,omitempty"`

	Keypairs []KeypairSpec `json:"keypairs,omitempty"`

	Secrets []SecretSpec `json:"secrets,omitempty"`

	// Cluster is the spec provided for the kops api ClusterSpec; ref:
	// https://pkg.go.dev/k8s.io/kops@v1.29.0/pkg/apis/kops#ClusterSpec
	// Cluster api.ClusterSpec `json:"cluster"`
}

type RollingUpdateOptsSpec struct {
	// +kubebuilder:default="15s"
	BastionInterval *string `json:"bastionInterval,omitempty"`
	// +kubebuilder:default=false
	CloudOnly *bool `json:"cloudOnly,omitempty"`
	// +kubebuilder:default="15s"
	ControlPlaneInterval *string `json:"controlPlaneInterval,omitempty"`
	// +kubebuilder:default="15m0s"
	DrainTimeout *string `json:"drainTimeout,omitempty"`
	// +kubebuilder:default=true
	FailOnDrainError *bool `json:"failOnDrainError,omitempty"`
	// +kubebuilder:default=true
	FailOnValidateError *bool `json:"failOnValidateError,omitempty"`
	// +kubebuilder:default=false
	Force *bool `json:"force,omitempty"`
	// +kubebuilder:default="15s"
	NodeInterval *string `json:"nodeInterval,omitempty"`
	// +kubebuilder:default="15s"
	PostDrainDelay *string `json:"postDrainDelay,omitempty"`
	// +kubebuilder:default=2
	ValidateCount *int32 `json:"validateCount,omitempty"`
	// +kubebuilder:default="15m0s"
	ValidationTimeout *string `json:"validationTimeout,omitempty"`
}

// *****
// ***** BEGIN KopsClusterSpec and related *****
// *****

// KopsClusterSpec
// for options description; ref: https://github.com/kubernetes/kops/blob/master/docs/cluster_spec.md
// for code gen; ref: https://book.kubebuilder.io/reference/markers
type KopsClusterSpec struct {
	Assets             AssetsSpec             `yaml:"assets,omitempty" json:"assets,omitempty"`
	Containerd         ContainerdConfigSpec   `yaml:"containerd,omitempty" json:"containerd,omitempty"`
	AdditionalPolicies AdditionalPoliciesSpec `yaml:"additionalPolicies,omitempty" json:"additionalPolicies,omitempty"`
	API                APISpec                `yaml:"api,omitempty" json:"api,omitempty"`
	Authorization      *AuthorizationSpec     `yaml:"authorization,omitempty" json:"authorization,omitempty"`
	Channel            string                 `yaml:"channel,omitempty" json:"channel,omitempty"`
	CloudLabels        map[string]string      `yaml:"cloudLabels,omitempty" json:"cloudLabels,omitempty"`
	CloudProvider      string                 `yaml:"cloudProvider,omitempty" json:"cloudProvider,omitempty"`
	ClusterAutoscaler  ClusterAutoscalerSpec  `yaml:"clusterAutoscaler,omitempty" json:"clusterAutoscaler,omitempty"`
	ConfigBase         string                 `yaml:"configBase,omitempty" json:"configBase,omitempty"`
	// +kubebuilder:validation:Enum=containerd
	ContainerRuntime string             `yaml:"containerRuntime,omitempty" json:"containerRuntime,omitempty"`
	EncryptionConfig bool               `yaml:"encryptionConfig,omitempty" json:"encryptionConfig,omitempty"`
	FileAssets       []FileAssetSpec    `yaml:"fileAssets,omitempty" json:"fileAssets,omitempty"`
	Hooks            []HookSpec         `yaml:"hooks,omitempty" json:"hooks,omitempty"`
	EtcdClusters     []EtcdClusterSpec  `yaml:"etcdClusters" json:"etcdClusters"`
	IAM              IAMSpec            `yaml:"iam,omitempty" json:"iam,omitempty"`
	KubeAPIServer    KubeAPIServerSpec  `yaml:"kubeAPIServer,omitempty" json:"kubeAPIServer,omitempty"`
	Kubelet          *KubeletConfigSpec `yaml:"kubelet,omitempty" json:"kubelet,omitempty"`
	KubeProxy        KubeProxySpec      `yaml:"kubeProxy,omitempty" json:"kubeProxy,omitempty"`
	KubeDNS          KubeDNSSpec        `yaml:"kubeDNS,omitempty" json:"kubeDNS,omitempty"`
	// +kubebuilder:default={"0.0.0.0/0"}
	KubernetesApiAccess []string `yaml:"kubernetesApiAccess,omitempty" json:"kubernetesApiAccess,omitempty"`
	// +kubebuilder:default="v1.29.6"
	KubernetesVersion string `yaml:"kubernetesVersion" json:"kubernetesVersion"`
	// MasterInternalName  string            `yaml:"masterInternalName,omitempty" json:"masterInternalName,omitempty"`
	MetricsServer     MetricsServerSpec `yaml:"metricsServer,omitempty" json:"metricsServer,omitempty"`
	CertManager       CertManagerSpec   `yaml:"certManager,omitempty" json:"certManager,omitempty"`
	NetworkCIDR       string            `yaml:"networkCIDR" json:"networkCIDR"`
	NetworkID         string            `yaml:"networkID,omitempty" json:"networkID,omitempty"`
	TagSubnets        bool              `yaml:"tagSubnets,omitempty" json:"tagSubnets,omitempty"`
	Networking        NetworkingSpec    `yaml:"networking,omitempty" json:"networking,omitempty"`
	NonMasqueradeCIDR string            `yaml:"nonMasqueradeCIDR" json:"nonMasqueradeCIDR"`
	// +kubebuilder:default={"0.0.0.0/0"}
	SSHAccess []string     `yaml:"sshAccess,omitempty" json:"sshAccess,omitempty"`
	Topology  TopologySpec `yaml:"topology,omitempty" json:"topology,omitempty"`
	// +kubebuilder:validation:Enum=automatic;external
	UpdatePolicy string       `yaml:"updatePolicy,omitempty" json:"updatePolicy,omitempty"`
	Subnets      []SubnetSpec `yaml:"subnets" json:"subnets"`
}

type MetadataClusterSpec struct {
	Name string `yaml:"name" json:"name"`
}

type AssetsSpec struct {
	ContainerRegistry string `yaml:"containerRegistry,omitempty" json:"containerRegistry,omitempty"`
}

type ContainerdConfigSpec struct {
	ConfigAdditions map[string]*string `yaml:"configAdditions,omitempty" json:"configAdditions,omitempty"`
	ConfigOverride  *string            `yaml:"configOverride,omitempty" json:"configOverride,omitempty"`
	LogLevel        *string            `yaml:"logLevel,omitempty" json:"logLevel,omitempty"`
}

type AdditionalPoliciesSpec struct {
	Node   string `yaml:"node,omitempty"   json:"node,omitempty"`
	Master string `yaml:"master,omitempty" json:"master,omitempty"`
}

type APISpec struct {
	LoadBalancer LoadBalancerSpec `yaml:"loadBalancer,omitempty" json:"loadBalancer,omitempty"`
}

type LoadBalancerSpec struct {
	Class                  string                   `yaml:"class" json:"class"`
	CrossZoneLoadBalancing bool                     `yaml:"crossZoneLoadBalancing,omitempty" json:"crossZoneLoadBalancing,omitempty"`
	IdleTimeoutSeconds     int                      `yaml:"idleTimeoutSeconds,omitempty" json:"idleTimeoutSeconds,omitempty"`
	SslCertificate         string                   `yaml:"sslCertificate,omitempty" json:"sslCertificate,omitempty"`
	SslPolicy              string                   `yaml:"sslPolicy,omitempty" json:"sslPolicy,omitempty"`
	Subnets                []LoadBalancerSubnetSpec `yaml:"subnets,omitempty" json:"subnets,omitempty"`
	Type                   string                   `yaml:"type"  json:"type"`
}

type LoadBalancerSubnetSpec struct {
	Name               string `yaml:"name" json:"name"`
	AllocationId       string `yaml:"allocationId,omitempty" json:"allocationId,omitempty"`
	PrivateIPv4Address string `yaml:"privateIPv4Address,omitempty" json:"privateIPv4Address,omitempty"`
}

type AuthorizationSpec struct {
	AlwaysAllow *AlwaysAllowAuthorizationSpec `json:"alwaysAllow,omitempty"  yaml:"alwaysAllow,omitempty"`
	RBAC        *RBACAuthorizationSpec        `json:"rbac,omitempty"         yaml:"rbac,omitempty"`
}

func (s *AuthorizationSpec) IsEmpty() bool {
	return s.RBAC.IsEmpty() && s.AlwaysAllow.IsEmpty()
}

type RBACAuthorizationSpec struct{}

func (s *RBACAuthorizationSpec) IsEmpty() bool {
	return s == nil
}

type AlwaysAllowAuthorizationSpec struct{}

func (s *AlwaysAllowAuthorizationSpec) IsEmpty() bool {
	return s == nil
}

type AuthorizationStructSpec map[string]string

type ClusterAutoscalerSpec struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
}

type DockerSpec struct {
	SkipInstall bool `yaml:"skipInstall" json:"skipInstall"`
}

// +kubebuilder:validation:Enum=ControlPlane;Node
type ClusterFileAssetRole string

const (
	ClusterFileAssetControlPlane ClusterFileAssetRole = "ControlPlane"
	ClusterFileAssetNode         ClusterFileAssetRole = "Node"
)

type FileAssetSpec struct {
	Content string `yaml:"content" json:"content"`
	// +kubebuilder:default="0440"
	Mode  string                 `yaml:"mode" json:"mode"`
	Name  string                 `yaml:"name"    json:"name"`
	Path  string                 `yaml:"path"    json:"path"`
	Roles []ClusterFileAssetRole `yaml:"roles"   json:"roles"`
}

// +kubebuilder:validation:Enum=Master;Node
type ClusterHookRole string

const (
	ClusterHookMaster ClusterHookRole = "Master"
	ClusterHookNode   ClusterHookRole = "Node"
)

type HookSpec struct {
	Manifest       string            `yaml:"manifest"       json:"manifest"`
	Name           string            `yaml:"name,omitempty" json:"name,omitempty"`
	Roles          []ClusterHookRole `yaml:"roles"          json:"roles"`
	UseRawManifest bool              `yaml:"useRawManifest" json:"useRawManifest"`
}

type EtcdClusterSpec struct {
	EtcdMembers []EtcdClusterMemberSpec `yaml:"etcdMembers" json:"etcdMembers"`
	Name        string                  `yaml:"name"    json:"name"`
	Manager     EtcdClusterManagerSpec  `yaml:"manager" json:"manager"`
}

type EtcdClusterMemberSpec struct {
	InstanceGroup   string `yaml:"instanceGroup" json:"instanceGroup"`
	EncryptedVolume bool   `yaml:"encryptedVolume" json:"encryptedVolume"`
	Name            string `yaml:"name" json:"name"`
	VolumeType      string `yaml:"volumeType" json:"volumeType"`
	VolumeSize      int    `yaml:"volumeSize" json:"volumeSize"`
}

type EtcdClusterManagerSpec struct {
	Env []EnvVarSpec `yaml:"env" json:"env"`
}

type EnvVarSpec struct {
	Name  string `yaml:"name"  json:"name"`
	Value string `yaml:"value" json:"value"`
}

type IAMSpec struct {
	AllowContainerRegistry bool `yaml:"allowContainerRegistry" json:"allowContainerRegistry"`
	Legacy                 bool `yaml:"legacy" json:"legacy"`
}

type KubeAPIServerSpec struct {
	APIAudiences               []string `yaml:"apiAudiences,omitempty"   json:"apiAudiences,omitempty"`
	AuthorizationMode          *string  `yaml:"authorizationMode,omitempty" json:"authorizationMode,omitempty"`
	AuthorizationRBACSuperUser *string  `yaml:"authorizationRBACSuperUser,omitempty" json:"authorizationRBACSuperUser,omitempty"`
	// +kubebuilder:default=/srv/kubernetes/ca.crt
	ClientCAFile string `yaml:"clientCAFile,omitempty"             json:"clientCAFile,omitempty"`
	// +kubebuilder:default=true
	DisableBasicAuth         bool   `yaml:"disableBasicAuth,omitempty"         json:"disableBasicAuth,omitempty"`
	OidcClientID             string `yaml:"oidcClientID,omitempty"             json:"oidcClientID,omitempty"`
	OidcGroupsClaim          string `yaml:"oidcGroupsClaim,omitempty"          json:"oidcGroupsClaim,omitempty"`
	OidcIssuerURL            string `yaml:"oidcIssuerURL,omitempty"            json:"oidcIssuerURL,omitempty"`
	OidcUsernameClaim        string `yaml:"oidcUsernameClaim,omitempty"        json:"oidcUsernameClaim,omitempty"`
	AuditLogMaxAge           int    `yaml:"auditLogMaxAge,omitempty"           json:"auditLogMaxAge,omitempty"`
	AuditLogMaxBackups       int    `yaml:"auditLogMaxBackups,omitempty"       json:"auditLogMaxBackups,omitempty"`
	AuditLogMaxSize          int    `yaml:"auditLogMaxSize,omitempty"          json:"auditLogMaxSize,omitempty"`
	AuditLogPath             string `yaml:"auditLogPath,omitempty"             json:"auditLogPath,omitempty"`
	AuditPolicyFile          string `yaml:"auditPolicyFile,omitempty"          json:"auditPolicyFile,omitempty"`
	AuditWebhookBatchMaxWait string `yaml:"auditWebhookBatchMaxWait,omitempty" json:"auditWebhookBatchMaxWait,omitempty"`
	AuditWebhookConfigFile   string `yaml:"auditWebhookConfigFile,omitempty"   json:"auditWebhookConfigFile,omitempty"`
}

type KubeletConfigSpec struct {
	// +kubebuilder:default=false
	AnonymousAuth              *bool  `yaml:"anonymousAuth,omitempty"              json:"anonymousAuth,omitempty"`
	AuthenticationTokenWebhook *bool  `yaml:"authenticationTokenWebhook,omitempty" json:"authenticationTokenWebhook,omitempty"`
	AuthorizationMode          string `yaml:"authorizationMode,omitempty"          json:"authorizationMode,omitempty"`
	ReadOnlyPort               int    `yaml:"readOnlyPort,omitempty"               json:"readOnlyPort,omitempty"`
	PodInfraContainerImage     string `yaml:"podInfraContainerImage,omitempty"     json:"podInfraContainerImage,omitempty"`
}

type KubeProxySpec struct {
	ProxyMode string `yaml:"proxyMode" json:"proxyMode"`
}

type KubeDNSSpec struct {
	Provider string `yaml:"provider" json:"provider"`
}

type MetricsServerSpec struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
}

type CertManagerSpec struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
}

type NetworkingSpec struct {
	Calico CalicoNetworkingSpec `yaml:"calico,omitempty" json:"calico,omitempty"`
}

type CalicoNetworkingSpec struct {
	Version string `yaml:"version" json:"version"`
}

type TopologySpec struct {
	DNS DNSTopologySpec `yaml:"dns"     json:"dns"`
	// Masters string          `yaml:"masters" json:"masters"`
	// Nodes   string          `yaml:"nodes"   json:"nodes"`
}

type DNSTopologySpec struct {
	// +kubebuilder:validation:Enum=Private;Public
	Type string `yaml:"type" json:"type"`
}

type SubnetSpec struct {
	CIDR string `yaml:"cidr" json:"cidr"`
	ID   string `yaml:"id"   json:"id"`
	Name string `yaml:"name" json:"name"`
	Type string `yaml:"type" json:"type"`
	Zone string `yaml:"zone" json:"zone"`
}

// *****
// ***** END KopsClusterSpec and related *****
// *****

// *****
// ***** BEGIN InstanceGroupSpec and related *****
// *****

type InstanceGroupSpec struct {
	Name string                `yaml:"name" json:"name"`
	Spec KopsInstanceGroupSpec `yaml:"spec" json:"spec"`
}

type KopsInstanceGroupSpec struct {
	AdditionalSecurityGroups []string          `yaml:"additionalSecurityGroups,omitempty" json:"additionalSecurityGroups,omitempty"`
	CloudLabels              map[string]string `yaml:"cloudLabels,omitempty" json:"cloudLabels,omitempty"`
	Image                    string            `yaml:"image" json:"image"`
	MachineType              string            `yaml:"machineType" json:"machineType"`
	MaxSize                  int               `yaml:"maxSize" json:"maxSize"`
	MinSize                  int               `yaml:"minSize" json:"minSize"`
	NodeLabels               map[string]string `yaml:"nodeLabels,omitempty" json:"nodeLabels,omitempty"`
	// +kubebuilder:validation:Enum=ControlPlane;Node;Master
	Role                 InstanceGroupRole                  `yaml:"role" json:"role"`
	RollingUpdate        RollingUpdateSpecInstanceGroupSpec `yaml:"rollingUpdate,omitempty" json:"rollingUpdate,omitempty"`
	RootVolumeEncryption bool                               `yaml:"rootVolumeEncryption" json:"rootVolumeEncryption"`
	RootVolumeSize       int                                `yaml:"rootVolumeSize" json:"rootVolumeSize"`
	Subnets              []string                           `yaml:"subnets" json:"subnets"`
}

type RollingUpdateSpecInstanceGroupSpec struct {
	MaxSurge string `yaml:"maxSurge" json:"maxSurge"`
}

type InstanceGroupRole string

const (
	InstanceGroupRoleControlPlane InstanceGroupRole = "ControlPlane"
	Node                          InstanceGroupRole = "Node"
	InstanceGroupRoleMaster       InstanceGroupRole = "Master"
)

// *****
// ***** END KopsClusterSpec and related *****
// *****

// *****
// ***** BEGIN KeypairSpec and related *****
// *****

type KeypairSpec struct {
	Keypair string      `yaml:"keypair" json:"keypair"`
	Cert    *string     `yaml:"cert,omitempty" json:"cert,omitempty"`
	Key     *SecretSpec `yaml:"key,omitempty" json:"key,omitempty"`
	Primary bool        `yaml:"primary" json:"primary"`
}

// *****
// ***** END KeypairSpec and related *****
// *****

// *****
// ***** BEGIN SecretSpec and related *****
// *****

type SecretSpec struct {
	Name string `yaml:"name" json:"name"`
	// +kubebuilder:validation:Enum=ciliumpassword;dockerconfig;encryptionconfig;keypair
	Kind  string      `yaml:"kind" json:"kind"`
	Value SecretValue `yaml:"value" json:"value"`
}

// ProviderCredentials required to authenticate.
type SecretValue struct {
	// Source of the secret credentials.
	// +kubebuilder:validation:Enum=None;Secret;InjectedIdentity;Environment;Filesystem
	Source xpv1.CredentialsSource `json:"source"`

	xpv1.CommonCredentialSelectors `json:",inline"`
}

// *****
// ***** END SecretsSpec and related *****
// *****

type SecretObservation struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// ClusterObservation are the observable fields of a Cluster.
type ClusterObservation struct {
	ClusterSpec        *KopsClusterSpec                  `json:"clusterSpec,omitempty"        yaml:"clusterSpec,omitempty"`
	InstanceGroupSpecs map[string]*KopsInstanceGroupSpec `json:"instanceGroupSpecs,omitempty" yaml:"instanceGroupSpecs,omitempty"`
	Secrets            []*SecretObservation              `json:"secrets,omitempty"            yaml:"secrets,omitempty"`
}

// A ClusterSpec defines the desired state of a Cluster.
type ClusterSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       ClusterParameters `json:"forProvider"`
}

type StatusSpec string

const (
	Ready       StatusSpec = "Ready"
	Creating    StatusSpec = "Creating"
	Progressing StatusSpec = "Progressing"
	Deleting    StatusSpec = "Deleting"
	Updating    StatusSpec = "Updating"
	Unknown     StatusSpec = "Unknown"
)

// A ClusterStatus represents the observed state of a Cluster.
type ClusterStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          ClusterObservation `json:"atProvider,omitempty"`
	// Status of this instance.
	Status StatusSpec `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// A Cluster is an example API type.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,kops}
type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterSpec   `json:"spec"`
	Status ClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterList contains a list of Cluster
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Cluster `json:"items"`
}

// Cluster type metadata.
var (
	ClusterKind             = reflect.TypeOf(Cluster{}).Name()
	ClusterGroupKind        = schema.GroupKind{Group: Group, Kind: ClusterKind}.String()
	ClusterKindAPIVersion   = ClusterKind + "." + SchemeGroupVersion.String()
	ClusterGroupVersionKind = SchemeGroupVersion.WithKind(ClusterKind)
)

func init() {
	SchemeBuilder.Register(&Cluster{}, &ClusterList{})
}
