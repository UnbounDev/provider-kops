// heavily influenced by the examples in https://github.com/kubernetes/kops/blob/v1.29.0/examples/kops-api-example/up.go

package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/crossplane/provider-kops/apis/v1alpha1"
	apisv1alpha1 "github.com/crossplane/provider-kops/apis/v1alpha1"
	"github.com/pkg/errors"
	diff "github.com/r3labs/diff/v3"
	"gopkg.in/ini.v1"
	"gopkg.in/yaml.v3"

	api "k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/client/simple"
	"k8s.io/kops/pkg/client/simple/vfsclientset"
	"k8s.io/kops/util/pkg/vfs"
)

const (
	errNoAuth = "k8s creds are invalid"

	fileSuffixCreate  = "create"
	fileSuffixObserve = "observe"
	fileSuffixUpdate  = "update"
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

type diffChangelogSpec struct {
	Type string      `json:"type"`
	Path []string    `json:"path"`
	From interface{} `json:"from"`
	To   interface{} `json:"to"`
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

func buildKopsClientset(cr *apisv1alpha1.Cluster) (simple.Clientset, error) {

	vfsContext := vfs.NewVFSContext()
	registryBase, err := vfsContext.BuildVfsPath(cr.Spec.ForProvider.State)
	if err != nil {
		return nil, err
	}

	clientset := vfsclientset.NewVFSClientset(vfsContext, registryBase)
	return clientset, nil
}

func modifyClusterYaml(cr *apisv1alpha1.Cluster, cy *clusterYaml) {

	cy.Spec.ConfigBase = filepath.Join(cr.Spec.ForProvider.State, cr.Name)
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
	if reflect.ValueOf(igy.Spec.CloudLabels).IsNil() {
		igy.Spec.CloudLabels = make(map[string]string)
	}
	if reflect.ValueOf(igy.Spec.NodeLabels).IsNil() {
		igy.Spec.NodeLabels = make(map[string]string)
	}
	igy.Metadata.Labels["kops.k8s.io/cluster"] = cr.Name
	igy.Spec.NodeLabels["kops.k8s.io/instancegroup"] = ig.Name

	if cr.Spec.ForProvider.Cluster.ClusterAutoscaler.Enabled && ig.Spec.Role != apisv1alpha1.InstanceGroupRoleControlPlane {
		igy.Spec.CloudLabels["k8s.io/cluster-autoscaler/enabled"] = ""
		igy.Spec.CloudLabels["k8s.io/cluster-autoscaler/node-template/label"] = ""
		igy.Spec.CloudLabels[fmt.Sprintf("k8s.io/cluster-autoscaler/%s", cr.Name)] = ""
		igy.Spec.NodeLabels["autoscaling.k8s.io/nodegroup"] = fmt.Sprintf("%s.%s", ig.Name, cr.Name)
	}
}

func buildKopsYamlStructs(cr *apisv1alpha1.Cluster) (clusterYaml, []instanceGroupYaml, error) {

	clusterYaml := clusterYaml{
		ApiVersion: "kops.k8s.io/v1alpha2",
		Kind:       "Cluster",
		Metadata: apisv1alpha1.MetadataClusterSpec{
			Name: cr.Name,
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

	return clusterYaml, instanceGroupYamls, nil
}

func getKopsClusterFilename(cr *apisv1alpha1.Cluster, fileSuffix string) string {
	return fmt.Sprintf("/tmp/%s_%s.yaml", strings.ReplaceAll(cr.Name, ".", "-"), fileSuffix)
}

func getKopsClusterPubSshKeyFilename(cr *apisv1alpha1.Cluster, fileSuffix string) string {
	return fmt.Sprintf("/tmp/%s_%s.pub", strings.ReplaceAll(cr.Name, ".", "-"), fileSuffix)
}

func getKopsInstanceGroupFilename(cr *apisv1alpha1.Cluster, ig *apisv1alpha1.InstanceGroupSpec, fileSuffix string) string {
	return fmt.Sprintf("/tmp/%s_%s_%s.yaml",
		strings.ReplaceAll(cr.Name, ".", "-"),
		strings.ReplaceAll(ig.Name, ".", "-"),
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

func deferClose(f *os.File) {
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Printf("\n%+v\n", err)
		}
	}()
}

func writeKopsYamlFile(cr *apisv1alpha1.Cluster, pubSshKey string, fileSuffix string) error {

	clusterYaml, instanceGroupYamls, err := buildKopsYamlStructs(cr)
	if err != nil {
		return err
	}

	fC, err := os.Create(getKopsClusterFilename(cr, fileSuffix))
	if err != nil {
		return err
	}
	defer fC.Close()
	// deferClose(fC)

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

	for i := range cr.Spec.ForProvider.InstanceGroups {
		ig := cr.Spec.ForProvider.InstanceGroups[i]
		igy := instanceGroupYamls[i]

		fIg, err := os.Create(getKopsInstanceGroupFilename(cr, &ig, fileSuffix))
		if err != nil {
			return err
		}
		defer fIg.Close()
		// deferClose(fIg)

		b, err = yaml.Marshal(&igy)
		if err != nil {
			return err
		}
		if err := writeBlockToFile(fIg, b); err != nil {
			return err
		}
		if err := fIg.Sync(); err != nil {
			return err
		}
	}

	fS, err := os.Create(getKopsClusterPubSshKeyFilename(cr, fileSuffix))
	if err != nil {
		return err
	}
	defer fS.Close()
	// deferClose(fS)
	if _, err := fS.WriteString(pubSshKey); err != nil {
		return err
	}
	if err := fS.Sync(); err != nil {
		return err
	}

	return nil
}

func (k *kopsClient) observeCluster(ctx context.Context, cr *apisv1alpha1.Cluster) (*api.Cluster, error) {

	clientset, err := buildKopsClientset(cr)
	if err != nil {
		return nil, err
	}

	cluster, err := clientset.GetCluster(ctx, cr.Name)
	if err != nil {
		return nil, err
	}

	return cluster, nil
}

func (k *kopsClient) createCluster(_ context.Context, cr *v1alpha1.Cluster) error {

	fileSuffix := fileSuffixCreate

	// first construct the yaml files
	if err := writeKopsYamlFile(cr, k.pubSshKey, fileSuffix); err != nil {
		return err
	}

	// now create the cluster config
	//nolint:gosec
	cmd := exec.Command(
		"kops",
		"create",
		fmt.Sprintf("-f%s", getKopsClusterFilename(cr, fileSuffix)),
		fmt.Sprintf("--name=%s", cr.Name),
		fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrap(err, string(output))
	}

	// now create the pub ssh secret
	//nolint:gosec
	cmd = exec.Command(
		"kops",
		"create",
		"sshpublickey",
		fmt.Sprintf("-i%s", getKopsClusterPubSshKeyFilename(cr, fileSuffix)),
		fmt.Sprintf("--name=%s", cr.Name),
		fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrap(err, string(output))
	}

	// now create all instance group configs
	for _, ig := range cr.Spec.ForProvider.InstanceGroups {
		//nolint:gosec
		cmd := exec.Command(
			"kops",
			"create",
			fmt.Sprintf("-f%s", getKopsInstanceGroupFilename(cr, &ig, fileSuffix)),
			fmt.Sprintf("--name=%s", cr.Name),
			fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
		)

		if output, err := cmd.CombinedOutput(); err != nil {
			return errors.Wrap(err, string(output))
		}

	}

	return nil
}

func (k *kopsClient) authenticateToCluster(ctx context.Context, cr *v1alpha1.Cluster, extraArgs []string) error {

	//nolint:gosec
	cmd := exec.CommandContext(
		ctx,
		"kops",
		"export",
		"kubecfg",
		"--admin",
		fmt.Sprintf("--name=%s", cr.Name),
		fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
	)
	cmd.Args = append(cmd.Args, extraArgs...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrap(err, string(output))
	}

	return nil
}

// validateCluster runs the kops cli command to validate a kops cluster, we need to use
// exec here bc the kops golang library does not provide direct access to this method
func (k *kopsClient) validateCluster(ctx context.Context, cr *v1alpha1.Cluster, extraArgs []string) (*kopsValidationResponse, error) {

	var stdOut, stdErr bytes.Buffer
	res := &kopsValidationResponse{}

	//nolint:gosec
	cmd := exec.CommandContext(
		ctx,
		"kops",
		"validate",
		"cluster",
		"--output=json",
		fmt.Sprintf("--name=%s", cr.Name),
		fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
	)
	cmd.Args = append(cmd.Args, extraArgs...)
	cmd.Stdout = &stdOut
	cmd.Stderr = &stdErr
	err := cmd.Run()
	output := stdOut.Bytes()
	errString := stdErr.String()

	fmt.Printf("\n%s\n", output)
	if err != nil {
		if strings.Contains(errString, "error listing nodes: Unauthorized") {
			err = errors.Wrap(err, errNoAuth)
		} else if jsonError := json.Unmarshal(output, res); jsonError != nil {
			return res, errors.Wrapf(jsonError, "error in json unmarshal for:%s", string(output))
		}
		return res, err
	} else {
		if jsonError := json.Unmarshal(output, res); jsonError != nil {
			return res, errors.Wrapf(jsonError, "error in json unmarshal for:%s", string(output))
		}
		return res, nil
	}

}

func (k *kopsClient) diffCluster(ctx context.Context, cr *v1alpha1.Cluster) ([]diffChangelogSpec, error) {

	diffChangelog := []diffChangelogSpec{}

	sourceClusterYaml, sourceInstanceGroupYamls, err := buildKopsYamlStructs(cr)
	if err != nil {
		return diffChangelog, err
	}

	externalClusterYaml := clusterYaml{}
	externalInstanceGroupYamls := []instanceGroupYaml{}

	externalCR := &v1alpha1.Cluster{
		TypeMeta:   cr.DeepCopy().TypeMeta,
		ObjectMeta: cr.DeepCopy().ObjectMeta,
		Spec:       v1alpha1.ClusterSpec{},
	}

	var stdOut, stdErr bytes.Buffer

	//nolint:gosec
	cmd := exec.CommandContext(
		ctx,
		"kops",
		"get",
		"cluster",
		"--output=yaml",
		fmt.Sprintf("--name=%s", cr.Name),
		fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
	)
	cmd.Stdout = &stdOut
	cmd.Stderr = &stdErr
	if err := cmd.Run(); err != nil {
		return diffChangelog, errors.Wrapf(err, "cmd: %s; stderr: %s; stdout: %s", cmd.String(), stdErr.String(), stdOut.String())
	}
	output := stdOut.Bytes()

	if err := yaml.Unmarshal(output, &externalClusterYaml); err != nil {
		return diffChangelog, errors.Wrapf(err, "stdout: %s", string(output))
	}

	for _, ig := range cr.Spec.ForProvider.InstanceGroups {

		var stdOut, stdErr bytes.Buffer

		//nolint:gosec
		cmd := exec.CommandContext(
			ctx,
			"kops",
			"get",
			"instancegroup",
			ig.Name,
			"--output=yaml",
			fmt.Sprintf("--name=%s", cr.Name),
			fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
		)
		cmd.Stdout = &stdOut
		cmd.Stderr = &stdErr
		if err := cmd.Run(); err != nil {
			return diffChangelog, errors.Wrapf(err, "cmd: %s; stderr: %s; stdout: %s", cmd.String(), stdErr.String(), stdOut.String())
		}
		output := stdOut.Bytes()

		instanceGroup := v1alpha1.InstanceGroupSpec{}
		instanceGroupYaml := instanceGroupYaml{}

		if err := yaml.Unmarshal(output, &instanceGroupYaml); err != nil {
			return diffChangelog, errors.Wrapf(err, "stdout: %s", string(output))
		}

		externalCR.Spec.ForProvider.InstanceGroups = append(externalCR.Spec.ForProvider.InstanceGroups, instanceGroup)
		externalInstanceGroupYamls = append(externalInstanceGroupYamls, instanceGroupYaml)
	}

	changelog1, err := diff.Diff(externalClusterYaml.Spec, sourceClusterYaml.Spec)
	if err != nil {
		return diffChangelog, err
	}

	changelog2, err := diff.Diff(externalInstanceGroupYamls, sourceInstanceGroupYamls)
	if err != nil {
		return diffChangelog, err
	}

	for _, c := range append(changelog1, changelog2...) {

		includeD := true
		d := diffChangelogSpec{
			Type: c.Type,
			Path: c.Path,
			From: c.From,
			To:   c.To,
		}

		// ignore the kops state diff that attempts to change the `Role` from `Master` to `ControlPlane`
		// bc this state _never_ updates in the final config in the kops state store, so this would just
		// update infinitely
		f, okF := d.From.(v1alpha1.InstanceGroupRole)
		t, okT := d.To.(v1alpha1.InstanceGroupRole)
		if okF && okT && string(d.Path[len(d.Path)-1]) == "Role" && f == v1alpha1.InstanceGroupRoleMaster && t == v1alpha1.InstanceGroupRoleControlPlane {
			includeD = false
		}

		if includeD {
			diffChangelog = append(diffChangelog, d)
		}
	}

	return diffChangelog, nil
}

func (k *kopsClient) updateCluster(ctx context.Context, cr *v1alpha1.Cluster) error {

	// if we've already created everything then the naive update is to replace literally
	// _everything_ based on the desired state
	if _, ok := cr.Annotations[providerKopsCreateComplete]; ok {

		fileSuffix := fileSuffixUpdate

		// first construct the yaml files
		if err := writeKopsYamlFile(cr, k.pubSshKey, fileSuffix); err != nil {
			return err
		}

		// now naively replace _everything_

		//nolint:gosec
		cmd := exec.Command(
			"kops",
			"replace",
			fmt.Sprintf("-f%s", getKopsClusterFilename(cr, fileSuffix)),
			fmt.Sprintf("--name=%s", cr.Name),
			fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
		)

		if output, err := cmd.CombinedOutput(); err != nil {
			return errors.Wrap(err, string(output))
		}

		for i := range cr.Spec.ForProvider.InstanceGroups {
			ig := cr.Spec.ForProvider.InstanceGroups[i]

			//nolint:gosec
			cmd := exec.Command(
				"kops",
				"replace",
				fmt.Sprintf("-f%s", getKopsInstanceGroupFilename(cr, &ig, fileSuffix)),
				fmt.Sprintf("--name=%s", cr.Name),
				fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
			)

			if output, err := cmd.CombinedOutput(); err != nil {
				return errors.Wrap(err, string(output))
			}
		}

	}

	//nolint:gosec
	cmd := exec.CommandContext(
		ctx,
		"kops",
		"update",
		"cluster",
		fmt.Sprintf("--name=%s", cr.Name),
		fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
		"--yes",
	)

	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrap(err, string(output))
	} else {
		fmt.Printf("\napplied update:\n\n%s", string(output))
	}

	return nil
}

type awsCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
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
