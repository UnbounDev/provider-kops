// heavily influenced by the examples in https://github.com/kubernetes/kops/blob/v1.29.0/examples/kops-api-example/up.go

package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/crossplane/provider-kops/apis/v1alpha1"
	apisv1alpha1 "github.com/crossplane/provider-kops/apis/v1alpha1"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

func (k *kopsClient) observeCluster(ctx context.Context, cr *apisv1alpha1.Cluster) (*apisv1alpha1.KopsClusterSpec, map[string]*apisv1alpha1.KopsInstanceGroupSpec, error) {
	clusterYaml := &clusterYaml{}
	instanceGroupSpecMap := map[string]*apisv1alpha1.KopsInstanceGroupSpec{}

	// pull the cluster definition
	{
		var stdOut, stdErr bytes.Buffer

		//nolint:gosec
		cmd := exec.CommandContext(
			ctx,
			"kops",
			"get",
			"cluster",
			"--output=yaml",
			fmt.Sprintf("--name=%s", getClusterExternalName(cr)),
			fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
		)
		cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)
		cmd.Stdout = &stdOut
		cmd.Stderr = &stdErr
		if err := cmd.Run(); err != nil {
			return &apisv1alpha1.KopsClusterSpec{}, instanceGroupSpecMap, errors.Wrapf(err, "cmd: %s; stderr: %s; stdout: %s", cmd.String(), stdErr.String(), stdOut.String())
		}

		output := stdOut.Bytes()
		// log.Debug(string(output))

		if err := yaml.Unmarshal(output, clusterYaml); err != nil {
			return &apisv1alpha1.KopsClusterSpec{}, instanceGroupSpecMap, errors.Wrapf(err, "fucked that")
		}
	}

	// pull the instance group definitions
	{
		var stdOut, stdErr bytes.Buffer

		//nolint:gosec
		cmd := exec.CommandContext(
			ctx,
			"kops",
			"get",
			"instancegroups",
			"--output=yaml",
			fmt.Sprintf("--name=%s", getClusterExternalName(cr)),
			fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
		)
		cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)
		cmd.Stdout = &stdOut
		cmd.Stderr = &stdErr
		if err := cmd.Run(); err != nil {
			return &apisv1alpha1.KopsClusterSpec{}, instanceGroupSpecMap, errors.Wrapf(err, "cmd: %s; stderr: %s; stdout: %s", cmd.String(), stdErr.String(), stdOut.String())
		}
		output := stdOut.Bytes()
		// log.Debug(string(output))

		instanceGroupYamls := []*instanceGroupYaml{}
		dec := yaml.NewDecoder(bytes.NewReader(output))
		for {
			var ig instanceGroupYaml
			if dec.Decode(&ig) != nil {
				break
			}
			instanceGroupYamls = append(instanceGroupYamls, &ig)
		}
		for _, ig := range instanceGroupYamls {
			instanceGroupSpecMap[ig.Metadata.Name] = &ig.Spec
		}

	}

	return &clusterYaml.Spec, instanceGroupSpecMap, nil
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
		"-v5",
		fmt.Sprintf("-f%s", getKopsClusterFilename(cr, fileSuffix)),
		fmt.Sprintf("--name=%s", getClusterExternalName(cr)),
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
		fmt.Sprintf("--name=%s", getClusterExternalName(cr)),
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
			fmt.Sprintf("--name=%s", getClusterExternalName(cr)),
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

func (k *kopsClient) createKeypair(_ context.Context, cr *v1alpha1.Cluster, kp v1alpha1.KeypairSpec, privateKeyData []byte) error {
	args := []string{}
	if kp.Primary {
		args = append(args, "--primary=true")
	}

	kpFile, err := os.CreateTemp(tmpDir, "*.crt")
	if err != nil {
		return err
	}
	pkFile, err := os.CreateTemp(tmpDir, "*.pem")
	if err != nil {
		return err
	}

	if kp.Cert != nil {
		if _, err := kpFile.WriteString(*kp.Cert); err != nil {
			return err
		}
		if err := kpFile.Close(); err != nil {
			return err
		}
		args = append(args, fmt.Sprintf("--cert=%s", kpFile.Name()))
	}
	if len(privateKeyData) > 0 {
		if _, err = pkFile.Write(privateKeyData); err != nil {
			return err
		}
		if err := pkFile.Close(); err != nil {
			return err
		}
		args = append(args, fmt.Sprintf("--key=%s", pkFile.Name()))
	}
	//nolint:gosec
	cmd := exec.Command(
		"kops",
		"create",
		"keypair",
		"-v5",
		kp.Keypair,
		fmt.Sprintf("--name=%s", getClusterExternalName(cr)),
		fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
	)
	cmd.Args = append(cmd.Args, args...)
	cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)

	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrap(err, string(output))
	} else {
		log.Debug(string(output))
	}

	deferRemove(kpFile)
	deferRemove(pkFile)
	return nil
}

func (k *kopsClient) createSecret(_ context.Context, cr *v1alpha1.Cluster, secret v1alpha1.SecretSpec, secretData []byte) error {
	args := []string{}
	f, err := os.CreateTemp(tmpDir, fmt.Sprintf("%s_*.secret", secret.Kind))
	if len(secretData) > 0 {
		if err != nil {
			return err
		}
		if _, err = f.Write(secretData); err != nil {
			return err
		}
		if err = f.Close(); err != nil {
			return err
		}
		args = append(args, fmt.Sprintf("--filename=%s", f.Name()))
	}
	//nolint:gosec
	cmd := exec.Command(
		"kops",
		"create",
		"secret",
		"-v5",
		secret.Kind,
		fmt.Sprintf("--name=%s", getClusterExternalName(cr)),
		fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
	)
	cmd.Args = append(cmd.Args, args...)
	cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)

	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrap(err, string(output))
	} else {
		log.Debug(string(output))
	}

	deferRemove(f)
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
		"-v5",
		fmt.Sprintf("--name=%s", getClusterExternalName(cr)),
		fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
	)
	cmd.Args = append(cmd.Args, extraArgs...)
	cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)
	log.Info(fmt.Sprintf("run: %s", cmd.String()))

	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrap(err, string(output))
	} else {
		log.Debug(string(output))
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
		fmt.Sprintf("--name=%s", getClusterExternalName(cr)),
		fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
	)
	cmd.Args = append(cmd.Args, extraArgs...)
	cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)
	log.Info(fmt.Sprintf("run: %s", cmd.String()))

	cmd.Stdout = &stdOut
	cmd.Stderr = &stdErr
	err := cmd.Run()
	output := stdOut.Bytes()
	errString := stdErr.String()

	// log.Debug(string(output))

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

func (k *kopsClient) diffClusterV2(_ context.Context, cr *v1alpha1.Cluster) []observedDelta {
	clusterYaml, igYamls := buildKopsYamlStructs(cr)
	changes := []observedDelta{}

	if !cmp.Equal(cr.Status.AtProvider.ClusterSpec, &clusterYaml.Spec) {
		diff := cmp.Diff(cr.Status.AtProvider.ClusterSpec, &clusterYaml.Spec)
		log.Info(diff)
		changes = append(changes, observedDelta{
			Operation: updateDelta,
			Resource:  cluster,
			Diff:      diff,
		})
	}

	for _, igDesired := range igYamls {
		igSource := cr.Status.AtProvider.InstanceGroupSpecs[igDesired.Metadata.Name]
		if igSource == nil {
			changes = append(changes, observedDelta{
				Operation: createDelta,
				Resource:  instanceGroup,
			})
		} else if !cmp.Equal(igSource, &igDesired.Spec) {
			diff := cmp.Diff(igSource, &igDesired.Spec)
			changes = append(changes, observedDelta{
				Operation: updateDelta,
				Resource:  instanceGroup,
				Diff:      diff,
			})
		}
	}

	return changes
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
			fmt.Sprintf("--name=%s", getClusterExternalName(cr)),
			fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
		)
		cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)

		if output, err := cmd.CombinedOutput(); err != nil {
			return errors.Wrap(err, string(output))
		} else {
			log.Debug(string(output))
		}

		for i := range cr.Spec.ForProvider.InstanceGroups {
			ig := cr.Spec.ForProvider.InstanceGroups[i]

			//nolint:gosec
			cmd := exec.Command(
				"kops",
				"replace",
				fmt.Sprintf("-f%s", getKopsInstanceGroupFilename(cr, &ig, fileSuffix)),
				fmt.Sprintf("--name=%s", getClusterExternalName(cr)),
				fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
			)
			cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)

			if output, err := cmd.CombinedOutput(); err != nil {
				return errors.Wrap(err, string(output))
			} else {
				log.Debug(string(output))
			}
		}

	}

	//nolint:gosec
	cmd := exec.CommandContext(
		ctx,
		"kops",
		"update",
		"cluster",
		fmt.Sprintf("--name=%s", getClusterExternalName(cr)),
		fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
		"--yes",
	)
	cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)
	log.Info(fmt.Sprintf("run: %s", cmd.String()))

	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrap(err, string(output))
	} else {
		log.Debug(fmt.Sprintf("Applied Update:%s", string(output)))
	}

	return nil
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
		fmt.Sprintf("--name=%s", getClusterExternalName(cr)),
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
	log.Info(fmt.Sprintf("run: %s", cmd.String()))

	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrap(err, string(output))
	} else {
		log.Debug(fmt.Sprintf("Applied Update:%s", string(output)))
	}
	return nil
}
