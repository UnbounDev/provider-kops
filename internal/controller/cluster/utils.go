package cluster

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	apisv1alpha1 "github.com/crossplane/provider-kops/apis/v1alpha1"
	"github.com/pkg/errors"
	"gopkg.in/ini.v1"
	"gopkg.in/yaml.v3"
)

const (
	errNoAuth         = "k8s creds are invalid"
	errKopsValidation = "kops validation failed"

	fileSuffixCreate  = "create"
	fileSuffixObserve = "observe"
	fileSuffixUpdate  = "update"

	tmpDir = "/tmp"
)

type kopsClient struct {
	awsCredentials awsCredentials
	pubSshKey      string
}

type kopsValidationResponse struct {
	Failures []kopsValidationFailureSpec `json:"failures"`
	Nodes    []kopsValidationNodeSpec    `json:"nodes"`
}

type kopsValidationFailureSpec struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Message string `json:"message"`
}

type kopsValidationNodeSpec struct {
	Name     string `json:"name"`
	Zone     string `json:"zone"`
	Role     string `json:"role"`
	Hostname string `json:"hostname"`
	Status   string `json:"status"`
}

type clusterYaml struct {
	ApiVersion string                           `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                           `yaml:"kind" json:"kind"`
	Metadata   apisv1alpha1.MetadataClusterSpec `yaml:"metadata" json:"metadata"`
	Spec       apisv1alpha1.KopsClusterSpec     `yaml:"spec" json:"spec"`
}

type instanceGroupYaml struct {
	ApiVersion string                             `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                             `yaml:"kind" json:"kind"`
	Metadata   metadataInstanceGroupSpec          `yaml:"metadata" json:"metadata"`
	Spec       apisv1alpha1.KopsInstanceGroupSpec `yaml:"spec" json:"spec"`
}

type metadataInstanceGroupSpec struct {
	Name   string            `yaml:"name"`
	Labels map[string]string `yaml:"labels,omitempty"`
}

type observedDelta struct {
	Operation        observedDeltaOperation   `json:"operation"`
	Resource         observedDeltaResource    `json:"resource"`
	Diff             string                   `json:"diff"`
	ResourceFilePath string                   `json:"resourceFilePath,omitempty"`
	ResourceSecret   *apisv1alpha1.SecretSpec `json:"resourceSecret,omitempty"`
}

type observedDeltaOperation string
type observedDeltaResource string

const (
	createDelta observedDeltaOperation = "create"
	updateDelta observedDeltaOperation = "update"
	deleteDelta observedDeltaOperation = "delete"

	clusterResourceDelta       observedDeltaResource = "cluster"
	instanceGroupResourceDelta observedDeltaResource = "instanceGroup"
	secretResourceDelta        observedDeltaResource = "secret"
)

func getClusterExternalName(cr *apisv1alpha1.Cluster) string {
	if externalName, ok := cr.Annotations[crossplaneExternalName]; ok {
		return externalName
	} else {
		return cr.Name
	}
}

func modifyClusterYaml(cr *apisv1alpha1.Cluster, cy *clusterYaml) {

	cy.Spec.ConfigBase = filepath.Join(cr.Spec.ForProvider.State, getClusterExternalName(cr))
	// when the state is in s3 the filepath.Join function messes up the s3 prefix via the s3://->s3:/ modification,
	// we want to keep the convenience of using filepath.Join, so this line is just a patch for the case where
	// ConfigBase is housed in s3
	cy.Spec.ConfigBase = strings.Replace(cy.Spec.ConfigBase, "s3:/", "s3://", 1)
	// clusterYaml.Spec.MasterInternalName = fmt.Sprintf("api.internal.%s", cr.Name)
}

func modifyInstanceGroupYaml(cr *apisv1alpha1.Cluster, ig *apisv1alpha1.InstanceGroupSpec, igy *instanceGroupYaml) {

	if reflect.ValueOf(igy.Metadata.Labels).IsNil() {
		igy.Metadata.Labels = make(map[string]string)
	}
	if reflect.ValueOf(igy.Spec.NodeLabels).IsNil() {
		igy.Spec.NodeLabels = make(map[string]string)
	}
	igy.Metadata.Labels["kops.k8s.io/cluster"] = getClusterExternalName(cr)
	igy.Spec.NodeLabels["kops.k8s.io/instancegroup"] = ig.Name

	if cr.Spec.ForProvider.Cluster.ClusterAutoscaler.Enabled && ig.Spec.Role != apisv1alpha1.InstanceGroupRoleControlPlane {

		if reflect.ValueOf(igy.Spec.CloudLabels).IsNil() {
			igy.Spec.CloudLabels = make(map[string]string)
		}

		igy.Spec.CloudLabels["k8s.io/cluster-autoscaler/enabled"] = ""
		igy.Spec.CloudLabels["k8s.io/cluster-autoscaler/node-template/label"] = ""
		igy.Spec.CloudLabels[fmt.Sprintf("k8s.io/cluster-autoscaler/%s", getClusterExternalName(cr))] = ""
		igy.Spec.NodeLabels["autoscaling.k8s.io/nodegroup"] = fmt.Sprintf("%s.%s", ig.Name, getClusterExternalName(cr))
	}

	if igy.Spec.Role == apisv1alpha1.InstanceGroupRoleControlPlane {
		igy.Spec.Role = apisv1alpha1.InstanceGroupRoleMaster
	}
}

func buildKopsYamlStructs(cr *apisv1alpha1.Cluster) (clusterYaml, []instanceGroupYaml) {

	clusterYaml := clusterYaml{
		ApiVersion: "kops.k8s.io/v1alpha2",
		Kind:       "Cluster",
		Metadata: apisv1alpha1.MetadataClusterSpec{
			Name: getClusterExternalName(cr),
		},
		Spec: cr.Spec.ForProvider.Cluster,
	}

	modifyClusterYaml(cr, &clusterYaml)

	instanceGroupYamls := []instanceGroupYaml{}
	for i := range cr.Spec.ForProvider.InstanceGroups {
		ig := cr.Spec.ForProvider.InstanceGroups[i]
		instanceGroupYaml := instanceGroupYaml{
			ApiVersion: "kops.k8s.io/v1alpha2",
			Kind:       "InstanceGroup",
			Metadata: metadataInstanceGroupSpec{
				Name: ig.Name,
			},
			Spec: ig.Spec,
		}

		modifyInstanceGroupYaml(cr, &ig, &instanceGroupYaml)
		instanceGroupYamls = append(instanceGroupYamls, instanceGroupYaml)
	}

	return clusterYaml, instanceGroupYamls
}

func getKubeConfigFilePath(cr *apisv1alpha1.Cluster) string {
	return fmt.Sprintf("/tmp/%s_kubeconfig", strings.ReplaceAll(getClusterExternalName(cr), ".", "-"))
}

func getKopsCliEnv(cr *apisv1alpha1.Cluster, client *kopsClient) []string {
	env := []string{}

	kubeConfig := fmt.Sprintf("KUBECONFIG=%s", getKubeConfigFilePath(cr))
	env = append(env, kubeConfig)

	awsAccessKeyID := fmt.Sprintf("AWS_ACCESS_KEY_ID=%s", client.awsCredentials.AccessKeyID)
	if client.awsCredentials.AccessKeyID != "" {
		env = append(env, awsAccessKeyID)
	}

	awsSecretAccessKey := fmt.Sprintf("AWS_SECRET_ACCESS_KEY=%s", client.awsCredentials.SecretAccessKey)
	if client.awsCredentials.SecretAccessKey != "" {
		env = append(env, awsSecretAccessKey)
	}

	awsSessionToken := fmt.Sprintf("AWS_SESSION_TOKEN=%s", client.awsCredentials.SessionToken)
	if client.awsCredentials.SessionToken != "" {
		env = append(env, awsSessionToken)
	}

	return env
}

func getKopsClusterFilename(cr *apisv1alpha1.Cluster, fileSuffix string) string {
	return fmt.Sprintf("/tmp/%s_%s.yaml", strings.ReplaceAll(getClusterExternalName(cr), ".", "-"), fileSuffix)
}

func getKopsClusterPubSshKeyFilename(cr *apisv1alpha1.Cluster, fileSuffix string) string {
	return fmt.Sprintf("/tmp/%s_%s.pub", strings.ReplaceAll(getClusterExternalName(cr), ".", "-"), fileSuffix)
}

func getKopsInstanceGroupFilename(cr *apisv1alpha1.Cluster, ig *apisv1alpha1.InstanceGroupSpec, fileSuffix string) string {
	return fmt.Sprintf("/tmp/%s_%s_%s.yaml",
		strings.ReplaceAll(getClusterExternalName(cr), ".", "-"),
		strings.ReplaceAll(ig.Name, ".", "-"),
		fileSuffix,
	)
}

func getKopsInstanceGroupFilenameFromYaml(cr *apisv1alpha1.Cluster, ig *instanceGroupYaml, fileSuffix string) string {
	return fmt.Sprintf("/tmp/%s_%s_%s.yaml",
		strings.ReplaceAll(getClusterExternalName(cr), ".", "-"),
		strings.ReplaceAll(ig.Metadata.Name, ".", "-"),
		fileSuffix,
	)
}

func writeBlockToFile(f *os.File, b []byte) error {
	if _, err := f.WriteString("---\n"); err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		return err
	}
	return nil
}

func deferRemove(f *os.File) {
	defer func() {
		if err := f.Close(); err != nil {
			log.Debug(fmt.Sprintf("Error closing file %s; err: %+v", f.Name(), err))
		}
		if err := os.Remove(f.Name()); err != nil {
			log.Debug(fmt.Sprintf("Error removing file %s; err: %+v", f.Name(), err))
		}
	}()
}

func writeKopsYamlFile(cr *apisv1alpha1.Cluster, pubSshKey string, fileSuffix string) error {

	clusterYaml, instanceGroupYamls := buildKopsYamlStructs(cr)

	{
		fC, err := os.Create(getKopsClusterFilename(cr, fileSuffix))
		if err != nil {
			return err
		}

		b, err := yaml.Marshal(&clusterYaml)
		if err != nil {
			return err
		}
		if err := writeBlockToFile(fC, b); err != nil {
			return err
		}
		if err := fC.Sync(); err != nil {
			return err
		}

		if err := fC.Close(); err != nil {
			return err
		}
	}

	for i := range cr.Spec.ForProvider.InstanceGroups {
		ig := cr.Spec.ForProvider.InstanceGroups[i]
		igy := instanceGroupYamls[i]

		fIg, err := os.Create(getKopsInstanceGroupFilename(cr, &ig, fileSuffix))
		if err != nil {
			return err
		}

		b, err := yaml.Marshal(&igy)
		if err != nil {
			return err
		}
		if err := writeBlockToFile(fIg, b); err != nil {
			return err
		}
		if err := fIg.Sync(); err != nil {
			return err
		}

		if err := fIg.Close(); err != nil {
			return err
		}
	}

	{
		fS, err := os.Create(getKopsClusterPubSshKeyFilename(cr, fileSuffix))
		if err != nil {
			return err
		}
		if _, err := fS.WriteString(pubSshKey); err != nil {
			return err
		}
		if err := fS.Sync(); err != nil {
			return err
		}
		if err := fS.Close(); err != nil {
			return err
		}
	}

	return nil
}

// credentialsIDSecret retrieves AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY from the data which contains
// aws credentials under given profile
// Example:
// [default]
// aws_access_key_id = <YOUR_ACCESS_KEY_ID>
// aws_secret_access_key = <YOUR_SECRET_ACCESS_KEY>
//
// Attribution; almost entirely taken direct from:
// https://github.com/crossplane-contrib/provider-aws/blob/d17c9dad7b6d8af7dfcbb225cfb3ffd1f5fa7f17/pkg/utils/connect/aws/config.go#L222-L255
func credentialsIDSecret(data []byte, profile string) (awsCredentials, error) {
	config, err := ini.InsensitiveLoad(data)
	if err != nil {
		return awsCredentials{}, errors.Wrap(err, "cannot parse credentials secret")
	}

	iniProfile, err := config.GetSection(profile)
	if err != nil {
		return awsCredentials{}, errors.Wrap(err, fmt.Sprintf("cannot get %s profile in credentials secret", profile))
	}

	accessKeyID := iniProfile.Key("aws_access_key_id")
	secretAccessKey := iniProfile.Key("aws_secret_access_key")
	sessionToken := iniProfile.Key("aws_session_token")

	if accessKeyID == nil || secretAccessKey == nil || sessionToken == nil {
		return awsCredentials{}, errors.New("returned key can be empty but cannot be nil")
	}

	return awsCredentials{
		AccessKeyID:     accessKeyID.Value(),
		SecretAccessKey: secretAccessKey.Value(),
		SessionToken:    sessionToken.Value(),
	}, nil
}

type awsCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}
