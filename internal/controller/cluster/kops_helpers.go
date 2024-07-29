// heavily influenced by the examples in https://github.com/kubernetes/kops/blob/v1.29.0/examples/kops-api-example/up.go

package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/provider-kops/apis/v1alpha1"
	apisv1alpha1 "github.com/crossplane/provider-kops/apis/v1alpha1"
	"github.com/pkg/errors"
	diff "github.com/r3labs/diff/v3"
	"gopkg.in/yaml.v3"

	api "k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/client/simple"
	"k8s.io/kops/pkg/client/simple/vfsclientset"
	"k8s.io/kops/util/pkg/vfs"
)

func buildKopsClientset(cr *apisv1alpha1.Cluster) (simple.Clientset, error) {

	vfsContext := vfs.NewVFSContext()
	registryBase, err := vfsContext.BuildVfsPath(cr.Spec.ForProvider.State)
	if err != nil {
		return nil, err
	}

	clientset := vfsclientset.NewVFSClientset(vfsContext, registryBase)
	return clientset, nil
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

func (k *kopsClient) createCluster(_ context.Context, cr *v1alpha1.Cluster, log logging.Logger) error {

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
		"-v5",
		fmt.Sprintf("-f%s", getKopsClusterFilename(cr, fileSuffix)),
		fmt.Sprintf("--name=%s", cr.Name),
		fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
	)
	cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrap(err, string(output))
	} else {
		log.Debug(string(output))
	}

	// now create the pub ssh secret
	//nolint:gosec
	cmd = exec.Command(
		"kops",
		"create",
		"-v5",
		"sshpublickey",
		fmt.Sprintf("-i%s", getKopsClusterPubSshKeyFilename(cr, fileSuffix)),
		fmt.Sprintf("--name=%s", cr.Name),
		fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
	)
	cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrap(err, string(output))
	} else {
		log.Debug(string(output))
	}

	// now create all instance group configs
	for _, ig := range cr.Spec.ForProvider.InstanceGroups {
		//nolint:gosec
		cmd := exec.Command(
			"kops",
			"create",
			"-v5",
			fmt.Sprintf("-f%s", getKopsInstanceGroupFilename(cr, &ig, fileSuffix)),
			fmt.Sprintf("--name=%s", cr.Name),
			fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
		)
		cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)

		if output, err := cmd.CombinedOutput(); err != nil {
			return errors.Wrap(err, string(output))
		} else {
			log.Debug(string(output))
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
	cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)
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
	cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)
	cmd.Stdout = &stdOut
	cmd.Stderr = &stdErr
	err := cmd.Run()
	output := stdOut.Bytes()
	errString := stdErr.String()

	fmt.Printf("\n%s\n", output)
	if err != nil {
		if strings.Contains(errString, "error listing nodes: Unauthorized") {
			err = errors.Wrap(err, errNoAuth)
		} else if strings.Contains(errString, "cannot load kubecfg setting") {
			// for the case where we haven't initialized the cluster state locally
			err = errors.Wrap(err, errNoAuth)
		} else if jsonError := json.Unmarshal(output, res); jsonError != nil {
			return res, errors.Wrapf(jsonError, "error in json unmarshal for:%s; err string:%s", string(output), errString)
		}
		return res, err
	} else {
		if jsonError := json.Unmarshal(output, res); jsonError != nil {
			return res, errors.Wrapf(jsonError, "error in json unmarshal for:%s; err string:%s", string(output), errString)
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

	sourceInstanceGroupYamlsMap := map[string]instanceGroupYaml{}
	for _, ig := range sourceInstanceGroupYamls {
		sourceInstanceGroupYamlsMap[ig.Metadata.Name] = ig
	}

	externalClusterYaml := clusterYaml{}
	externalInstanceGroupYamlsMap := map[string]instanceGroupYaml{}

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
	cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)
	cmd.Stdout = &stdOut
	cmd.Stderr = &stdErr
	if err := cmd.Run(); err != nil {
		return diffChangelog, errors.Wrapf(err, "cmd: %s; stderr: %s; stdout: %s", cmd.String(), stdErr.String(), stdOut.String())
	}
	output := stdOut.Bytes()

	if err := yaml.Unmarshal(output, &externalClusterYaml); err != nil {
		return diffChangelog, errors.Wrapf(err, "stdout: %s", string(output))
	}

	{
		var stdOut, stdErr bytes.Buffer

		//nolint:gosec
		cmd := exec.CommandContext(
			ctx,
			"kops",
			"get",
			"instancegroups",
			"--output=yaml",
			fmt.Sprintf("--name=%s", cr.Name),
			fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
		)
		cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)
		cmd.Stdout = &stdOut
		cmd.Stderr = &stdErr
		if err := cmd.Run(); err != nil {
			return diffChangelog, errors.Wrapf(err, "cmd: %s; stderr: %s; stdout: %s", cmd.String(), stdErr.String(), stdOut.String())
		}
		output = stdOut.Bytes()

		dec := yaml.NewDecoder(bytes.NewReader(output))
		for {
			var ig instanceGroupYaml
			if dec.Decode(&ig) != nil {
				break
			}
			externalInstanceGroupYamlsMap[ig.Metadata.Name] = ig
		}
	}

	changelog1, err := diff.Diff(externalClusterYaml.Spec, sourceClusterYaml.Spec)
	if err != nil {
		return diffChangelog, err
	}

	changelog2, err := diff.Diff(externalInstanceGroupYamlsMap, sourceInstanceGroupYamlsMap)
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
		cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)

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
			cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)

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
	cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)

	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrap(err, string(output))
	} else {
		fmt.Printf("\napplied update:\n\n%s", string(output))
	}

	return k.rollingUpdateCluster(ctx, cr)
}

func (k *kopsClient) rollingUpdateCluster(ctx context.Context, cr *v1alpha1.Cluster) error {

	// assign rolling update defaults based on kops cli default values
	defaultFalse := bool(false)
	defaultTrue := bool(true)
	defaultInt32_2 := int32(2)
	defaultInterval5s := string("5s")
	defaultInterval15s := string("15s")
	defaultInterval15m0s := string("15m0s")

	rollingUpdateOpts := cr.Spec.ForProvider.RollingUpdateOpts

	if rollingUpdateOpts.BastionInterval == nil {
		rollingUpdateOpts.BastionInterval = &defaultInterval15s
	}
	if rollingUpdateOpts.CloudOnly == nil {
		rollingUpdateOpts.CloudOnly = &defaultFalse
	}
	if rollingUpdateOpts.ControlPlaneInterval == nil {
		rollingUpdateOpts.ControlPlaneInterval = &defaultInterval15s
	}
	if rollingUpdateOpts.DrainTimeout == nil {
		rollingUpdateOpts.DrainTimeout = &defaultInterval15m0s
	}
	if rollingUpdateOpts.FailOnDrainError == nil {
		rollingUpdateOpts.FailOnDrainError = &defaultTrue
	}
	if rollingUpdateOpts.FailOnValidateError == nil {
		rollingUpdateOpts.FailOnValidateError = &defaultTrue
	}
	if rollingUpdateOpts.Force == nil {
		rollingUpdateOpts.Force = &defaultFalse
	}
	if rollingUpdateOpts.NodeInterval == nil {
		rollingUpdateOpts.NodeInterval = &defaultInterval15s
	}
	if rollingUpdateOpts.PostDrainDelay == nil {
		rollingUpdateOpts.PostDrainDelay = &defaultInterval5s
	}
	if rollingUpdateOpts.ValidateCount == nil {
		rollingUpdateOpts.ValidateCount = &defaultInt32_2
	}
	if rollingUpdateOpts.ValidationTimeout == nil {
		rollingUpdateOpts.ValidationTimeout = &defaultInterval15m0s
	}

	//nolint:gosec
	cmd := exec.CommandContext(
		ctx,
		"kops",
		"rolling-update",
		"cluster",
		fmt.Sprintf("--name=%s", cr.Name),
		fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
		fmt.Sprintf("--bastion-interval=%s", *rollingUpdateOpts.BastionInterval),
		fmt.Sprintf("--cloudonly=%t", *rollingUpdateOpts.CloudOnly),
		fmt.Sprintf("--control-plane-interval=%s", *rollingUpdateOpts.ControlPlaneInterval),
		fmt.Sprintf("--drain-timeout=%s", *rollingUpdateOpts.DrainTimeout),
		fmt.Sprintf("--fail-on-drain-error=%t", *rollingUpdateOpts.FailOnDrainError),
		fmt.Sprintf("--fail-on-validate-error=%t", *rollingUpdateOpts.FailOnValidateError),
		fmt.Sprintf("--force=%t", *rollingUpdateOpts.Force),
		fmt.Sprintf("--node-interval=%s", *rollingUpdateOpts.NodeInterval),
		fmt.Sprintf("--post-drain-delay=%s", *rollingUpdateOpts.PostDrainDelay),
		fmt.Sprintf("--validate-count=%d", *rollingUpdateOpts.ValidateCount),
		fmt.Sprintf("--validation-timeout=%s", *rollingUpdateOpts.ValidationTimeout),
		"--yes",
	)
	cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)

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
