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
	State string `json:"state" yaml:"state"`

	Cluster KopsClusterSpec `json:"cluster" yaml:"cluster"`

	InstanceGroups []InstanceGroupSpec `json:"instanceGroups" yaml:"instanceGroups"`

	RollingUpdateOpts RollingUpdateOptsSpec `json:"rollingUpdateOpts,omitempty" yaml:"rollingUpdateOpts,omitempty"`

	Keypairs []KeypairSpec `json:"keypairs,omitempty" yaml:"keypairs,omitempty"`

	Secrets []SecretSpec `json:"secrets,omitempty" yaml:"secrets,omitempty"`

	// Cluster is the spec provided for the kops api ClusterSpec; ref:
	// https://pkg.go.dev/k8s.io/kops@v1.29.0/pkg/apis/kops#ClusterSpec
	// Cluster api.ClusterSpec `json:"cluster"`
}

type RollingUpdateOptsSpec struct {
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	// +kubebuilder:default="15s"
	BastionInterval *string `json:"bastionInterval,omitempty" yaml:"bastionInterval,omitempty"`
	// +kubebuilder:default=false
	CloudOnly *bool `json:"cloudOnly,omitempty" yaml:"cloudOnly,omitempty"`
	// +kubebuilder:default="15s"
	ControlPlaneInterval *string `json:"controlPlaneInterval,omitempty" yaml:"controlPlaneInterval,omitempty"`
	// +kubebuilder:default="15m0s"
	DrainTimeout *string `json:"drainTimeout,omitempty" yaml:"drainTimeout,omitempty"`
	// +kubebuilder:default=true
	FailOnDrainError *bool `json:"failOnDrainError,omitempty" yaml:"failOnDrainError,omitempty"`
	// +kubebuilder:default=true
	FailOnValidateError *bool `json:"failOnValidateError,omitempty" yaml:"failOnValidateError,omitempty"`
	// +kubebuilder:default=false
	Force *bool `json:"force,omitempty" yaml:"force,omitempty"`
	// +kubebuilder:default="15s"
	NodeInterval *string `json:"nodeInterval,omitempty" yaml:"nodeInterval,omitempty"`
	// +kubebuilder:default="15s"
	PostDrainDelay *string `json:"postDrainDelay,omitempty" yaml:"postDrainDelay,omitempty"`
	// +kubebuilder:default=2
	ValidateCount *int32 `json:"validateCount,omitempty" yaml:"validateCount,omitempty"`
	// +kubebuilder:default="15m0s"
	ValidationTimeout *string `json:"validationTimeout,omitempty" yaml:"validationTimeout,omitempty"`
}

// *****
// ***** BEGIN KopsClusterSpec and related *****
// *****

// KopsClusterSpec
// for options description; ref: https://github.com/kubernetes/kops/blob/master/docs/cluster_spec.md
// for code gen; ref: https://book.kubebuilder.io/reference/markers
type KopsClusterSpec struct {
	Assets                 AssetsSpec                 `yaml:"assets,omitempty" json:"assets,omitempty"`
	Containerd             ContainerdConfigSpec       `yaml:"containerd,omitempty" json:"containerd,omitempty"`
	AdditionalPolicies     AdditionalPoliciesSpec     `yaml:"additionalPolicies,omitempty" json:"additionalPolicies,omitempty"`
	API                    APISpec                    `yaml:"api,omitempty" json:"api,omitempty"`
	Authorization          *AuthorizationSpec         `yaml:"authorization,omitempty" json:"authorization,omitempty"`
	Channel                string                     `yaml:"channel,omitempty" json:"channel,omitempty"`
	CloudLabels            map[string]string          `yaml:"cloudLabels,omitempty" json:"cloudLabels,omitempty"`
	CloudProvider          string                     `yaml:"cloudProvider,omitempty" json:"cloudProvider,omitempty"`
	ClusterAutoscaler      ClusterAutoscalerSpec      `yaml:"clusterAutoscaler,omitempty" json:"clusterAutoscaler,omitempty"`
	NodeProblemDetector    NodeProblemDetectorSpec    `yaml:"nodeProblemDetector,omitempty" json:"nodeProblemDetector,omitempty"`
	NodeTerminationHandler NodeTerminationHandlerSpec `yaml:"nodeTerminationHandler,omitempty" json:"nodeTerminationHandler,omitempty"`
	ConfigBase             string                     `yaml:"configBase,omitempty" json:"configBase,omitempty"`
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

// NodeProblemDetectorSpec
// for options description; ref: https://github.com/kubernetes/kops/blob/69baa44474e7dc7b8f238adeacc349c22070fca9/pkg/apis/kops/componentconfig.go#L1068
type NodeProblemDetectorSpec struct {
	// Enabled enables the NodeProblemDetector.
	// +kubebuilder:default=false
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`

	// Image is the NodeProblemDetector container image used.
	Image *string `yaml:"image,omitempty" json:"image,omitempty"`

	// MemoryRequest of NodeProblemDetector container.
	// +kubebuilder:default="80Mi"
	MemoryRequest *string `yaml:"memoryRequest,omitempty" json:"memoryRequest,omitempty"`

	// CPURequest of NodeProblemDetector container.
	// +kubebuilder:default="10m"
	CPURequest *string `yaml:"cpuRequest,omitempty" json:"cpuRequest,omitempty"`

	// MemoryLimit of NodeProblemDetector container.
	// +kubebuilder:default="80Mi"
	MemoryLimit *string `yaml:"memoryLimit,omitempty" json:"memoryLimit,omitempty"`

	// CPULimit of NodeProblemDetector container.
	CPULimit *string `yaml:"cpuLimit,omitempty" json:"cpuLimit,omitempty"`
}

// NodeTerminationHandlerSpec
// for options description; ref: https://github.com/kubernetes/kops/blob/69baa44474e7dc7b8f238adeacc349c22070fca9/pkg/apis/kops/componentconfig.go#L995
type NodeTerminationHandlerSpec struct {
	// DeleteSQSMsgIfNodeNotFound makes node termination handler delete the SQS Message from the SQS Queue if the targeted node is not found.
	// Only used in Queue Processor mode.
	// +kubebuilder:default=false
	DeleteSQSMsgIfNodeNotFound *bool `yaml:"deleteSQSMsgIfNodeNotFound,omitempty" json:"deleteSQSMsgIfNodeNotFound,omitempty"`

	// Enabled enables the node termination handler.
	// +kubebuilder:default=false
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`

	// EnableSpotInterruptionDraining makes node termination handler drain nodes when spot interruption termination notice is received.
	// Cannot be disabled in queue-processor mode.
	// +kubebuilder:default=true
	EnableSpotInterruptionDraining *bool `yaml:"enableSpotInterruptionDraining,omitempty" json:"enableSpotInterruptionDraining,omitempty"`

	// EnableScheduledEventDraining makes node termination handler drain nodes before the maintenance window starts for an EC2 instance scheduled event.
	// Cannot be disabled in queue-processor mode.
	// +kubebuilder:default=true
	EnableScheduledEventDraining *bool `yaml:"enableScheduledEventDraining,omitempty" json:"enableScheduledEventDraining,omitempty"`

	// EnableRebalanceMonitoring makes node termination handler cordon nodes when the rebalance recommendation notice is received.
	// In queue-processor mode, cannot be enabled without rebalance draining.
	// +kubebuilder:default=false
	EnableRebalanceMonitoring *bool `yaml:"enableRebalanceMonitoring,omitempty" json:"enableRebalanceMonitoring,omitempty"`

	// EnableRebalanceDraining makes node termination handler drain nodes when the rebalance recommendation notice is received.
	// +kubebuilder:default=false
	EnableRebalanceDraining *bool `yaml:"enableRebalanceDraining,omitempty" json:"enableRebalanceDraining,omitempty"`

	// EnablePrometheusMetrics enables the "/metrics" endpoint.
	// +kubebuilder:default=false
	EnablePrometheusMetrics *bool `yaml:"prometheusEnable,omitempty" json:"prometheusEnable,omitempty"`

	// EnableSQSTerminationDraining enables queue-processor mode which drains nodes when an SQS termination event is received.
	// +kubebuilder:default=true
	EnableSQSTerminationDraining *bool `yaml:"enableSQSTerminationDraining,omitempty" json:"enableSQSTerminationDraining,omitempty"`

	// ExcludeFromLoadBalancers makes node termination handler will mark for exclusion from load balancers before node are cordoned.
	// +kubebuilder:default=true
	ExcludeFromLoadBalancers *bool `yaml:"excludeFromLoadBalancers,omitempty" json:"excludeFromLoadBalancers,omitempty"`

	// ManagedASGTag is the tag used to determine which nodes NTH can take action on
	// This field has kept its name even though it now maps to the --managed-tag flag due to keeping the API stable.
	// Node termination handler does no longer check the ASG for this tag, but the actual EC2 instances.
	// +kubebuilder:default="aws-node-termination-handler/managed"
	ManagedASGTag *string `yaml:"managedASGTag,omitempty" json:"managedASGTag,omitempty"`

	// PodTerminationGracePeriod is the time in seconds given to each pod to terminate gracefully.
	// If negative, the default value specified in the pod will be used, which defaults to 30 seconds if not specified for the pod.
	// +kubebuilder:default=-1
	PodTerminationGracePeriod *int32 `yaml:"podTerminationGracePeriod,omitempty" json:"podTerminationGracePeriod,omitempty"`

	// TaintNode makes node termination handler taint nodes when an interruption event occurs.
	// +kubebuilder:default=false
	TaintNode *bool `yaml:"taintNode,omitempty" json:"taintNode,omitempty"`

	// MemoryLimit of NodeTerminationHandler container.
	MemoryLimit *string `yaml:"memoryLimit,omitempty" json:"memoryLimit,omitempty"`

	// MemoryRequest of NodeTerminationHandler container.
	// +kubebuilder:default="64Mi"
	MemoryRequest *string `yaml:"memoryRequest,omitempty" json:"memoryRequest,omitempty"`

	// CPURequest of NodeTerminationHandler container.
	// +kubebuilder:default="50m"
	CPURequest *string `yaml:"cpuRequest,omitempty" json:"cpuRequest,omitempty"`

	// Version is the container image tag used.
	Version *string `yaml:"version,omitempty" json:"version,omitempty"`

	// Replaces the default webhook message template.
	WebhookTemplate *string `yaml:"webhookTemplate,omitempty" json:"webhookTemplate,omitempty"`

	// If specified, posts event data to URL upon instance interruption action.
	WebhookURL *string `yaml:"webhookURL,omitempty" json:"webhookURL,omitempty"`
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

// KubeDNSSpec
// for options description; ref: https://github.com/kubernetes/kops/blob/69baa44474e7dc7b8f238adeacc349c22070fca9/pkg/apis/kops/cluster.go#L568
type KubeDNSSpec struct {
	Provider string `yaml:"provider" json:"provider"`

	// NodeLocalDNS specifies the configuration for the node-local-dns addon
	NodeLocalDNS *NodeLocalDNSConfig `yaml:"nodeLocalDNS,omitempty" json:"nodeLocalDNS,omitempty"`
}

// NodeLocalDNSConfig
// for options description; ref: https://github.com/kubernetes/kops/blob/69baa44474e7dc7b8f238adeacc349c22070fca9/pkg/apis/kops/cluster.go#L604
type NodeLocalDNSConfig struct {
	// Enabled activates the node-local-dns addon.
	// +kubebuilder:default=false
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	// ExternalCoreFile is used to provide a complete NodeLocalDNS CoreFile by the user - ignores other provided flags which modify the CoreFile.
	ExternalCoreFile string `yaml:"externalCoreFile,omitempty" json:"externalCoreFile,omitempty"`
	// AdditionalConfig is used to provide additional config for node local dns by the user - it will include the original CoreFile made by kOps.
	AdditionalConfig string `yaml:"additionalConfig,omitempty" json:"additionalConfig,omitempty"`
	// Image overrides the default container image used for node-local-dns addon.
	Image *string `yaml:"image,omitempty" json:"image,omitempty"`
	// Local listen IP address. It can be any IP in the 169.254.20.0/16 space or any other IP address that can be guaranteed to not collide with any existing IP.
	LocalIP string `yaml:"localIP,omitempty" json:"localIP,omitempty"`
	// If enabled, nodelocal dns will use kubedns as a default upstream.
	ForwardToKubeDNS *bool `yaml:"forwardToKubeDNS,omitempty" json:"forwardToKubeDNS,omitempty"`
	// MemoryRequest specifies the memory requests of each node-local-dns container in the daemonset. Default 5Mi.
	MemoryRequest *string `yaml:"memoryRequest,omitempty" json:"memoryRequest,omitempty"`
	// CPURequest specifies the cpu requests of each node-local-dns container in the daemonset. Default 25m.
	CPURequest *string `yaml:"cpuRequest,omitempty" json:"cpuRequest,omitempty"`
	// PodAnnotations makes possible to add additional annotations to node-local-dns.
	// Default: none
	PodAnnotations map[string]string `yaml:"podAnnotations,omitempty" json:"podAnnotations,omitempty"`
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
	metav1.TypeMeta   `yaml:",inline" json:",inline"`
	metav1.ObjectMeta `yaml:"metadata,omitempty" json:"metadata,omitempty"`

	Spec   ClusterSpec   `yaml:"spec" json:"spec"`
	Status ClusterStatus `yaml:"status,omitempty" json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterList contains a list of Cluster
type ClusterList struct {
	metav1.TypeMeta `yaml:",inline" json:",inline"`
	metav1.ListMeta `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	Items           []Cluster `yaml:"items" json:"items"`
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
