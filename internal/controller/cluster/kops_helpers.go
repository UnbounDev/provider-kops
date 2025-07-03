// heavily influenced by the examples in https://github.com/kubernetes/kops/blob/v1.29.0/examples/kops-api-example/up.go

package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/crossplane/crossplane-runtime/pkg/resource"
	apisv1alpha1 "github.com/crossplane/provider-kops/apis/v1alpha1"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (k *kopsClient) observeCluster(ctx context.Context, cr *apisv1alpha1.Cluster) (*apisv1alpha1.KopsClusterSpec, map[string]*apisv1alpha1.KopsInstanceGroupSpec, []*apisv1alpha1.SecretObservation, error) {
	clusterYaml := &clusterYaml{}
	instanceGroupSpecMap := map[string]*apisv1alpha1.KopsInstanceGroupSpec{}
	secretsObservation := []*apisv1alpha1.SecretObservation{}

	// pull the cluster definition
	{
		output, err := k.kopsGet(ctx, cr, "cluster", []string{"--output=yaml"})
		if err != nil {
			return &clusterYaml.Spec, instanceGroupSpecMap, secretsObservation, errors.Wrapf(err, "error getting cluster %s", getClusterExternalName(cr))
		}

		if err := yaml.Unmarshal(output, clusterYaml); err != nil {
			return &clusterYaml.Spec, instanceGroupSpecMap, secretsObservation, errors.Wrapf(err, "error rendering cluster %s", getClusterExternalName(cr))
		}
	}

	// pull the instance group definitions
	{
		output, err := k.kopsGet(ctx, cr, "instancegroups", []string{"--output=yaml"})
		if err != nil {
			// ignore "not found" errors since those are just edge cases as the cluster is being created
			if strings.Contains(err.Error(), "Error: no InstanceGroup objects found") {
				return &clusterYaml.Spec, instanceGroupSpecMap, secretsObservation, nil
			} else {
				return &clusterYaml.Spec, instanceGroupSpecMap, secretsObservation, errors.Wrapf(err, "error getting instancegroups")
			}
		}

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

	// pull the list of secrets (requires that the cluster exists..)
	{
		output, err := k.kopsGet(ctx, cr, "secrets", []string{})
		if err != nil {
			// ignore "not found" errors since those are just edge cases as the cluster is being created
			if strings.Contains(err.Error(), "Error: no secrets found") {
				return &clusterYaml.Spec, instanceGroupSpecMap, secretsObservation, nil
			} else {
				return &clusterYaml.Spec, instanceGroupSpecMap, secretsObservation, errors.Wrapf(err, "error getting secrets")
			}
		}

		lines := strings.Split(string(output), "\n")
		// drop the first element since it's the header..
		secrets := lines[1:]
		for _, s := range secrets {
			kind := "System"
			if slices.Contains([]string{"ciliumpassword", "dockerconfig", "encryptionconfig"}, s) {
				kind = s
			}
			if s != "" {
				secretsObservation = append(secretsObservation, &apisv1alpha1.SecretObservation{Name: s, Kind: kind})
			}
		}
	}

	return &clusterYaml.Spec, instanceGroupSpecMap, secretsObservation, nil
}

func (k *kopsClient) diffClusterV2(_ context.Context, cr *apisv1alpha1.Cluster) []observedDelta {
	fileSuffix := fileSuffixUpdate
	clusterYaml, igYamls := buildKopsYamlStructs(cr)
	changes := []observedDelta{}

	if !cmp.Equal(cr.Status.AtProvider.ClusterSpec, &clusterYaml.Spec) {
		diff := cmp.Diff(cr.Status.AtProvider.ClusterSpec, &clusterYaml.Spec)
		changes = append(changes, observedDelta{
			Operation:        updateDelta,
			Resource:         clusterResourceDelta,
			ResourceFilePath: getKopsClusterFilename(cr, fileSuffix),
			Diff:             diff,
		})
	}

	for _, igDesired := range igYamls {
		igDesired := igDesired
		igSource := cr.Status.AtProvider.InstanceGroupSpecs[igDesired.Metadata.Name]
		if igSource == nil {
			changes = append(changes, observedDelta{
				Operation:        createDelta,
				Resource:         instanceGroupResourceDelta,
				ResourceFilePath: getKopsInstanceGroupFilenameFromYaml(cr, &igDesired, fileSuffix),
			})
		} else if !cmp.Equal(igSource, &igDesired.Spec) {
			diff := cmp.Diff(igSource, &igDesired.Spec)
			changes = append(changes, observedDelta{
				Operation:        updateDelta,
				Resource:         instanceGroupResourceDelta,
				ResourceFilePath: getKopsInstanceGroupFilenameFromYaml(cr, &igDesired, fileSuffix),
				Diff:             diff,
			})
		}
	}

	sourceSecrets := []string{}
	for _, secretObserved := range cr.Status.AtProvider.Secrets {
		sourceSecrets = append(sourceSecrets, secretObserved.Name)
	}
	for _, secretDesired := range cr.Spec.ForProvider.Secrets {
		secretDesired := secretDesired
		if !slices.Contains(sourceSecrets, secretDesired.Kind) {
			changes = append(changes, observedDelta{
				Operation:      createDelta,
				Resource:       secretResourceDelta,
				Diff:           fmt.Sprintf("create %s secret from %+v", secretDesired.Name, secretDesired.Value),
				ResourceSecret: &secretDesired,
			})
		}
	}

	return changes
}

func (k *kopsClient) createCluster(_ context.Context, cr *apisv1alpha1.Cluster) error {

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
		fmt.Sprintf("--name=%s", getClusterExternalName(cr)),
		fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
	)
	cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)
	log.Info(fmt.Sprintf("run: %s", cmd.String()))

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
		"sshpublickey",
		fmt.Sprintf("-i%s", getKopsClusterPubSshKeyFilename(cr, fileSuffix)),
		fmt.Sprintf("--name=%s", getClusterExternalName(cr)),
		fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
	)
	cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)
	log.Info(fmt.Sprintf("run: %s", cmd.String()))

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
			fmt.Sprintf("-f%s", getKopsInstanceGroupFilename(cr, &ig, fileSuffix)),
			fmt.Sprintf("--name=%s", getClusterExternalName(cr)),
			fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
		)
		cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)
		log.Info(fmt.Sprintf("run: %s", cmd.String()))

		if output, err := cmd.CombinedOutput(); err != nil {
			return errors.Wrap(err, string(output))
		} else {
			log.Debug(string(output))
		}

	}

	return nil
}

func (k *kopsClient) createKeypair(ctx context.Context, kube client.Client, cr *apisv1alpha1.Cluster, kp apisv1alpha1.KeypairSpec) error {

	privateKeyData := []byte{}
	if kp.Key != nil {
		privateKeySource, err := resource.CommonCredentialExtractor(ctx, kp.Key.Value.Source, kube, kp.Key.Value.CommonCredentialSelectors)
		privateKeyData = append(privateKeyData, privateKeySource...)
		if err != nil {
			return err
		}
	}

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
		kp.Keypair,
		fmt.Sprintf("--name=%s", getClusterExternalName(cr)),
		fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
	)
	cmd.Args = append(cmd.Args, args...)
	cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)
	log.Info(fmt.Sprintf("run: %s", cmd.String()))

	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrap(err, string(output))
	} else {
		log.Debug(string(output))
	}

	deferRemove(kpFile)
	deferRemove(pkFile)
	return nil
}

func (k *kopsClient) createSecret(ctx context.Context, kube client.Client, cr *apisv1alpha1.Cluster, secret apisv1alpha1.SecretSpec) error {

	secretData, err := resource.CommonCredentialExtractor(ctx, secret.Value.Source, kube, secret.Value.CommonCredentialSelectors)
	if err != nil {
		return errors.Wrapf(err, "create secret error for %s", secret.Name)
	}

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
		secret.Kind,
		fmt.Sprintf("--name=%s", getClusterExternalName(cr)),
		fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
	)
	cmd.Args = append(cmd.Args, args...)
	cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)
	log.Info(fmt.Sprintf("run: %s", cmd.String()))

	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrap(err, string(output))
	} else {
		log.Debug(string(output))
	}

	deferRemove(f)
	return nil
}

// validateCluster runs the kops cli command to validate a kops cluster, we need to use
// exec here bc the kops golang library does not provide direct access to this method
func (k *kopsClient) validateCluster(ctx context.Context, cr *apisv1alpha1.Cluster, extraArgs []string) (*kopsValidationResponse, error) {

	var stdOut, stdErr bytes.Buffer
	res := &kopsValidationResponse{}

	if err := k.forceKubecfgSNI(ctx, cr); err != nil {
		// for the case where we haven't initialized the cluster state locally
		if strings.Contains(err.Error(), "error executing jsonpath") {
			return res, errors.Wrapf(err, errNoAuth)
		}
		return res, err
	}

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

func (k *kopsClient) updateCluster(ctx context.Context, kube client.Client, cr *apisv1alpha1.Cluster) error {

	fileSuffix := fileSuffixUpdate

	upsertClusterOrInstanceGroup := func(d observedDelta) error {

		resourceFilePath := d.ResourceFilePath
		operation := ""
		switch d.Operation {
		case createDelta:
			operation = "create"
		case updateDelta:
			operation = "replace"
		case deleteDelta:
			return fmt.Errorf("delete operation not yet supported")
		}

		//nolint:gosec
		cmd := exec.Command(
			"kops",
			operation,
			fmt.Sprintf("-f%s", resourceFilePath),
			fmt.Sprintf("--name=%s", getClusterExternalName(cr)),
			fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
		)
		cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)
		log.Info(fmt.Sprintf("run: %s", cmd.String()))

		if output, err := cmd.CombinedOutput(); err != nil {
			return errors.Wrap(err, string(output))
		} else {
			log.Debug(string(output))
		}
		return nil
	}

	updateSecret := func(d observedDelta) error {
		if d.Operation == createDelta {
			return k.createSecret(ctx, kube, cr, *d.ResourceSecret)
		}
		return nil
	}

	// first construct the yaml files
	if err := writeKopsYamlFile(cr, k.pubSshKey, fileSuffix); err != nil {
		return err
	}

	// get the delta...
	deltas := k.diffClusterV2(ctx, cr)

	// adjust the deltas...
	for _, d := range deltas {
		if d.Resource == clusterResourceDelta || d.Resource == instanceGroupResourceDelta {
			if err := upsertClusterOrInstanceGroup(d); err != nil {
				return err
			}
		}
		if d.Resource == secretResourceDelta {
			if err := updateSecret(d); err != nil {
				return err
			}
		}
	}

	if err := k.kopsUpdateCluster(ctx, cr); err == nil {
		if err := k.forceKubecfgSNI(ctx, cr); err != nil {
			return errors.Wrapf(err, "force kubecfg sni for %s", getClusterExternalName(cr))
		}
		return nil
	} else {
		return err
	}
}

func (k *kopsClient) rollingUpdateCluster(ctx context.Context, cr *apisv1alpha1.Cluster) error {

	// assign rolling update defaults based on kops cli default values
	defaultFalse := bool(false)
	defaultTrue := bool(true)
	defaultInt32_2 := int32(2)
	defaultInterval5s := string("5s")
	defaultInterval15s := string("15s")
	defaultInterval15m0s := string("15m0s")

	rollingUpdateOpts := cr.Spec.ForProvider.RollingUpdateOpts

	if rollingUpdateOpts.Enabled == &defaultFalse {
		log.Debug(fmt.Sprintf("Skipped Rolling Update for %s", getClusterExternalName(cr)))
		return nil
	}

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
		fmt.Sprintf("--control-plane-interval=%s", *rollingUpdateOpts.ControlPlaneInterval),
		fmt.Sprintf("--drain-timeout=%s", *rollingUpdateOpts.DrainTimeout),
		fmt.Sprintf("--fail-on-drain-error=%t", *rollingUpdateOpts.FailOnDrainError),
		fmt.Sprintf("--fail-on-validate-error=%t", *rollingUpdateOpts.FailOnValidateError),
		fmt.Sprintf("--node-interval=%s", *rollingUpdateOpts.NodeInterval),
		fmt.Sprintf("--post-drain-delay=%s", *rollingUpdateOpts.PostDrainDelay),
		fmt.Sprintf("--validate-count=%d", *rollingUpdateOpts.ValidateCount),
		fmt.Sprintf("--validation-timeout=%s", *rollingUpdateOpts.ValidationTimeout),
		"--yes",
	)
	if *rollingUpdateOpts.CloudOnly {
		cmd.Args = append(cmd.Args, "--cloudonly")
	}
	if *rollingUpdateOpts.Force {
		cmd.Args = append(cmd.Args, "--force")
	}
	cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)
	log.Info(fmt.Sprintf("run: %s", cmd.String()))

	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrap(err, string(output))
	} else {
		log.Debug(fmt.Sprintf("Applied Update to %s : %s", getClusterExternalName(cr), string(output)))
	}
	return nil
}

func (k *kopsClient) forceKubecfgSNI(ctx context.Context, cr *apisv1alpha1.Cluster) error {

	// overwrite the tls server name to force usage of the kops api dns
	server := ""

	//nolint:gosec
	cmd := exec.CommandContext(
		ctx,
		"kubectl",
		"config",
		"view",
		fmt.Sprintf("-ojsonpath='{.clusters[?(@.name == \"%s\")].cluster.server}'", getClusterExternalName(cr)),
	)
	cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)
	log.Info(fmt.Sprintf("run: %s", cmd.String()))

	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrapf(err, "cmd: %s; output: %s", cmd.String(), string(output))
	} else {
		server = strings.TrimSpace(string(output))
	}

	// Extract the domain from the server URL
	u, err := url.Parse(strings.Trim(server, "'"))
	if err != nil {
		return errors.Wrapf(err, "failed to parse server URL: %s", server)
	}
	serverDomain := u.Hostname()

	//nolint:gosec
	cmd = exec.CommandContext(
		ctx,
		"kubectl",
		"config",
		"set-cluster",
		getClusterExternalName(cr),
		"--tls-server-name",
		serverDomain,
	)
	cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)
	log.Info(fmt.Sprintf("run: %s", cmd.String()))

	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrapf(err, "cmd: %s; output: %s", cmd.String(), string(output))
	}

	return nil
}

func (k *kopsClient) kopsExportKubecfgAdmin(ctx context.Context, cr *apisv1alpha1.Cluster, extraArgs []string) error {

	//nolint:gosec
	cmd := exec.CommandContext(
		ctx,
		"kops",
		"export",
		"kubecfg",
		"--admin",
		fmt.Sprintf("--name=%s", getClusterExternalName(cr)),
		fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
	)
	cmd.Args = append(cmd.Args, extraArgs...)
	cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)
	log.Info(fmt.Sprintf("run: %s", cmd.String()))

	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Wrapf(err, "cmd: %s; output: %s", cmd.String(), string(output))
	}

	if err := k.forceKubecfgSNI(ctx, cr); err != nil {
		return errors.Wrapf(err, "force kubecfg sni for %s", getClusterExternalName(cr))
	}

	return nil
}

func (k *kopsClient) kopsGet(ctx context.Context, cr *apisv1alpha1.Cluster, resource string, extraArgs []string) ([]byte, error) {

	var stdOut, stdErr bytes.Buffer

	//nolint:gosec
	cmd := exec.CommandContext(
		ctx,
		"kops",
		"get",
		resource,
		fmt.Sprintf("--name=%s", getClusterExternalName(cr)),
		fmt.Sprintf("--state=%s", cr.Spec.ForProvider.State),
	)
	cmd.Args = append(cmd.Args, extraArgs...)
	cmd.Env = append(cmd.Env, getKopsCliEnv(cr, k)...)
	log.Debug(fmt.Sprintf("run: %s", cmd.String()))

	cmd.Stdout = &stdOut
	cmd.Stderr = &stdErr
	if err := cmd.Run(); err != nil {
		return []byte{}, errors.Wrapf(err, "cmd: %s; stderr: %s; stdout: %s", cmd.String(), stdErr.String(), stdOut.String())
	}

	output := stdOut.Bytes()
	// log.Debug(string(output))
	return output, nil
}

func (k *kopsClient) kopsUpdateCluster(ctx context.Context, cr *apisv1alpha1.Cluster) error {

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
