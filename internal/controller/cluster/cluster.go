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

package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/connection"
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	apisv1alpha1 "github.com/crossplane/provider-kops/apis/v1alpha1"
	"github.com/crossplane/provider-kops/internal/features"
)

const (
	errNotCluster   = "managed resource is not a Cluster custom resource"
	errTrackPCUsage = "cannot track ProviderConfig usage"
	errGetPC        = "cannot get ProviderConfig"
	errGetCreds     = "cannot get credentials"

	errNewClient = "cannot create new Service"

	finalizer = "finalizer.managedresource.crossplane.io"

	crossplaneCreateSucceeded = "crossplane.io/external-create-succeeded"
	crossplaneExternalName    = "crossplane.io/external-name"

	providerKopsCreatePending        = "provider-kops.io/external-create-pending"
	providerKopsCreateComplete       = "provider-kops.io/external-create-complete"
	providerKopsReconcilePending     = "provider-kops.io/external-reconcile-pending"
	providerKopsRollingUpdatePending = "provider-kops.io/external-rolling-update-pending"
	providerKopsTriggerUpdate        = "provider-kops.io/external-update-trigger"
	providerKopsTriggerRollingUpdate = "provider-kops.io/external-rolling-update-trigger"
	providerKopsUpdateLocked         = "provider-kops.io/external-update-locked"
)

var (
	log logging.Logger

	newKopsClientService = func(creds []byte, pubSshKey []byte) (*kopsClient, error) {
		awsCredentials, err := credentialsIDSecret(creds, "default")
		if err != nil {
			return &kopsClient{}, err
		}

		if awsCredentials.AccessKeyID != "" {
			if err := os.Setenv("AWS_ACCESS_KEY_ID", awsCredentials.AccessKeyID); err != nil {
				return &kopsClient{}, err
			}
		}

		if awsCredentials.SecretAccessKey != "" {
			if err := os.Setenv("AWS_SECRET_ACCESS_KEY", awsCredentials.SecretAccessKey); err != nil {
				return &kopsClient{}, err
			}
		}

		if awsCredentials.SessionToken != "" {
			if err := os.Setenv("AWS_SESSION_TOKEN", awsCredentials.SessionToken); err != nil {
				return &kopsClient{}, err
			}
		}

		return &kopsClient{
			awsCredentials: awsCredentials,
			pubSshKey:      string(pubSshKey),
		}, nil
	}
)

// Setup adds a controller that reconciles Cluster managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(apisv1alpha1.ClusterGroupKind)
	log = o.Logger

	cps := []managed.ConnectionPublisher{managed.NewAPISecretPublisher(mgr.GetClient(), mgr.GetScheme())}
	if o.Features.Enabled(features.EnableAlphaExternalSecretStores) {
		cps = append(cps, connection.NewDetailsManager(mgr.GetClient(), apisv1alpha1.StoreConfigGroupVersionKind))
	}

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(apisv1alpha1.ClusterGroupVersionKind),
		managed.WithExternalConnecter(&connector{
			kube:         mgr.GetClient(),
			usage:        resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1alpha1.ProviderConfigUsage{}),
			newServiceFn: newKopsClientService}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithPollInterval(o.PollInterval),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithConnectionPublishers(cps...))

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&apisv1alpha1.Cluster{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube         client.Client
	usage        resource.Tracker
	newServiceFn func(creds []byte, pubSshKey []byte) (*kopsClient, error)
}

func getClusterConnectionDetails(cr *apisv1alpha1.Cluster) (managed.ConnectionDetails, error) {
	b, err := os.ReadFile(getKubeConfigFilePath(cr))
	if err != nil {
		log.Info(fmt.Sprintf("WARNING: kubeconfig file not found at '%s'", getKubeConfigFilePath(cr)))
	}
	connDetails := managed.ConnectionDetails{
		"kubeconfig": b,
	}

	return connDetails, err
}

// Connect typically produces an ExternalClient by:
// 1. Tracking that the managed resource is using a ProviderConfig.
// 2. Getting the managed resource's ProviderConfig.
// 3. Getting the credentials specified by the ProviderConfig.
// 4. Using the credentials to form a client.
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*apisv1alpha1.Cluster)
	if !ok {
		return nil, errors.New(errNotCluster)
	}

	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	pc := &apisv1alpha1.ProviderConfig{}
	if err := c.kube.Get(ctx, types.NamespacedName{Name: cr.GetProviderConfigReference().Name}, pc); err != nil {
		return nil, errors.Wrap(err, errGetPC)
	}

	creds := pc.Spec.Credentials
	credData, err := resource.CommonCredentialExtractor(ctx, creds.Source, c.kube, creds.CommonCredentialSelectors)
	if err != nil {
		return nil, errors.Wrap(err, errGetCreds)
	}

	pubSshKey := pc.Spec.PubSshKey
	pubSshKeyData, err := resource.CommonCredentialExtractor(ctx, pubSshKey.Source, c.kube, pubSshKey.CommonCredentialSelectors)
	if err != nil {
		return nil, errors.Wrap(err, errGetCreds)
	}

	svc, err := c.newServiceFn(credData, pubSshKeyData)
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{service: svc, kube: c.kube}, nil
}

// An ExternalClient observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type external struct {
	// A 'client' used to connect to the external resource API. In practice this
	// would be something like an AWS SDK client.
	service *kopsClient

	kube client.Client
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*apisv1alpha1.Cluster)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotCluster)
	}

	mo := managed.ExternalObservation{
		// Return false when the external resource does not exist. This lets
		// the managed resource reconciler know that it needs to call Create to
		// (re)create the resource, or that it has successfully been deleted.
		ResourceExists: true,

		// Return false when the external resource exists, but it not up to date
		// with the desired managed resource state. This lets the managed
		// resource reconciler know that it needs to call Update.
		ResourceUpToDate: true,

		// Return any details that may be required to connect to the external
		// resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}

	// log.Debug(fmt.Sprintf("Observing: %+v", cr))

	cluster, igs, err := c.service.observeCluster(ctx, cr)
	if err != nil {
		log.Debug(fmt.Sprintf("cluster not found: %s", err.Error()))
		if strings.Contains(err.Error(), "not found") {
			// ignore "not found" errors to allow creation of the cluster
			// TODO: change to identify deletion case
			cr.Status.Status = apisv1alpha1.Creating
		} else {
			return managed.ExternalObservation{}, err
		}
	} else if cluster != nil {

		cr.Status.AtProvider.ClusterSpec = cluster
		cr.Status.AtProvider.InstanceGroupSpecs = igs
		connDetails, _ := getClusterConnectionDetails(cr)
		mo.ConnectionDetails = connDetails

		// initialize status to a "unknown" state and alter this based on annotations & diff
		cr.Status.Status = apisv1alpha1.Unknown
		annotations := cr.GetAnnotations()

		// check for initial creation annotations
		_, resourceCreating := annotations[providerKopsCreatePending]
		_, resourceCreated := annotations[providerKopsCreateComplete]
		if resourceCreating && !resourceCreated {
			cr.Status.Status = apisv1alpha1.Progressing
		}

		// now run validation w/ fallback for authentication to the cluster
		output, err := c.service.validateCluster(ctx, cr, []string{})
		if err != nil && strings.Contains(err.Error(), errNoAuth) {
			if err := c.service.authenticateToCluster(ctx, cr, []string{}); err != nil {

				mo.ResourceUpToDate = false
				return mo, err
			}
		}

		if len(output.Failures) == 0 && err == nil {
			cr.Status.Status = apisv1alpha1.Ready

			// catch the case where the resource pre-exists and was simply discovered
			if !resourceCreated {
				cr.Annotations[providerKopsCreateComplete] = ""
			}

			changelog, err := c.service.diffClusterV2(ctx, cr)
			if err != nil {
				log.Debug(fmt.Sprintf("Diff error detected: %s", err.Error()))
				mo.ResourceUpToDate = false
				return mo, err
			}
			if len(changelog) > 0 {
				// set status to prompt update
				cr.Status.Status = apisv1alpha1.Updating
				mo.ResourceUpToDate = false
				for _, change := range changelog {
					if changeJson, err := json.Marshal(change); err != nil {
						log.Debug(fmt.Sprintf("Change detected: %s", changeJson))
					} else {
						log.Debug(fmt.Sprintf("Change detected: %+v", change))
					}
				}
			}
			// TODO(ab): check annotations for trigger to perform update
			// TODO(ab): check annotations for trigger to perform rolling-update
		}
		if err != nil && !strings.Contains(err.Error(), errNoAuth) {
			if resourceCreating {
				log.Debug(fmt.Sprintf("Validation errors on cluster still creating: %s", cr.Name))
				// TODO: it'd be really nice to include the validation errors in the xplane condition reason..
			} else {
				// TODO: in this case we need to determine if there are any _ongoing_ operations for this cluster
				log.Info(fmt.Sprintf("Cluster validation errors for %s: %s; %s", cr.Name, err.Error(), output))
			}
		}
	}

	switch cr.Status.Status {
	case apisv1alpha1.Creating:
		mo.ResourceExists = false
		cr.SetConditions(xpv1.Creating())
	case apisv1alpha1.Deleting:
		mo.ResourceExists = false
		cr.SetConditions(xpv1.Deleting())
	case apisv1alpha1.Ready:
		mo.ResourceExists = true
		cr.SetConditions(xpv1.Available())
	case apisv1alpha1.Updating:
	case apisv1alpha1.Progressing:
		mo.ResourceExists = true
		mo.ResourceUpToDate = false
		// NB! We set the status to `Unavailable` bc the cluster is not
		// "available" for use by the controller, it may be the case that
		// a user needs to manually intervene and fix the cluster state
		// before the controller can resume management
		cr.SetConditions(xpv1.Unavailable())
	case apisv1alpha1.Unknown:
	default:
		mo.ResourceUpToDate = false
		cr.Status.Status = apisv1alpha1.Unknown
		cr.SetConditions(xpv1.Unavailable())
	}

	return mo, nil
}

// Create "creates" the k8s cluster and instance groups and initial secrets based on cr parameters,
// we use k8s annotations to manage the resource lifecycle and prevent concurrent executions of the
// `kops` cli; these are:
//
// 1. On initial call to `Create`:
//   - set annotations `{providerKopsCreatePending, providerKopsUpdateLocked}`
//   - note that crossplane will _also_ set annotation `{crossplaneCreateSucceeded}` on the initial call
//
// 2. On completion of initial `Create` go func:
//   - set annotation `{providerKopsCreateComplete}`
//   - remove annotations `{providerKopsCreatePending, providerKopsUpdateLocked}`
//
// If the process crashes or otherwise requires manual intervention to correct the state of the
// external cluster, then it may be necessary to manually remove `{providerKopsCreatePending, providerKopsUpdateLocked}`
// and to manually set `{providerKopsCreateComplete}` on the clusters.kops.crossplane.io resource
func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*apisv1alpha1.Cluster)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotCluster)
	}

	connDetails := managed.ConnectionDetails{
		"kubeconfig": []byte{},
	}
	mo := managed.ExternalCreation{ConnectionDetails: connDetails}

	if _, ok := cr.Annotations[crossplaneCreateSucceeded]; ok {
		log.Debug(fmt.Sprintf("Already created: %s", cr.Name))
		return mo, nil
	}
	if _, ok := cr.Annotations[providerKopsCreatePending]; ok {
		log.Debug(fmt.Sprintf("Already creating: %s", cr.Name))
		return mo, nil
	}
	if err := c.lockCluster(ctx, cr, []string{providerKopsCreatePending, providerKopsUpdateLocked}); err != nil {
		return mo, err
	}

	log.Info(fmt.Sprintf("Begin creating: %s", cr.Name))
	// uncomment for super verbose debugging... log.Debug(fmt.Sprintf("%+v", cr))

	// don't block when updating the cluster, this takes a while..
	go func() {
		bgCtx := context.Background()
		if err := c.service.createCluster(bgCtx, cr); err != nil {
			log.Info(fmt.Sprintf("Cluster creation failed for: %s; %s; %+v", cr.Name, err.Error(), err))
			log.Info(fmt.Sprintf("Remove the %s and %s annotations if you would like the controller to retry creation", providerKopsCreatePending, providerKopsUpdateLocked))
		}

		if err := c.service.updateCluster(bgCtx, cr); err != nil {
			log.Info(fmt.Sprintf("Post create update error: %s; %+v", err.Error(), err))
		}

		// TODO(ab): we need to provide lifecycle management for keypairs and secrets,
		// rather than only providing bootstrap creation
		for _, kp := range cr.Spec.ForProvider.Keypairs {
			privateKeyData := []byte{}
			if kp.Key != nil {
				privateKeySource, err := resource.CommonCredentialExtractor(bgCtx, kp.Key.Value.Source, c.kube, kp.Key.Value.CommonCredentialSelectors)
				privateKeyData = append(privateKeyData, privateKeySource...)
				if err != nil {
					log.Info(fmt.Sprintf("Post create keypair error: %s; %+v", err.Error(), err))
				}
			}
			if err := c.service.createKeypair(bgCtx, cr, &kp, privateKeyData); err != nil {
				log.Info(fmt.Sprintf("Post create keypair error: %s; %+v", err.Error(), err))
			}
		}
		for _, secret := range cr.Spec.ForProvider.Secrets {
			secretData, err := resource.CommonCredentialExtractor(bgCtx, secret.Value.Source, c.kube, secret.Value.CommonCredentialSelectors)
			if err != nil {
				log.Info(fmt.Sprintf("Post create secret error: %s; %+v", err.Error(), err))
			} else {
				if err := c.service.createSecret(bgCtx, cr, &secret, secretData); err != nil {
					log.Info(fmt.Sprintf("Post create secret error: %s; %+v", err.Error(), err))
				}
			}
		}

		if len(cr.Spec.ForProvider.Keypairs) > 0 || len(cr.Spec.ForProvider.Secrets) > 0 {
			if err := c.service.updateCluster(bgCtx, cr); err != nil {
				log.Info(fmt.Sprintf("POST CREATE UPDATE ERROR: %s; %+v", err.Error(), err))
			}
			// force cloudonly roll for initial cluster creation when keypairs are introduced
			truePtr := bool(true)
			cr.Spec.ForProvider.RollingUpdateOpts.CloudOnly = &truePtr
			if err := c.service.rollingUpdateCluster(bgCtx, cr); err != nil {
				log.Info(fmt.Sprintf("POST CREATE ROLLING UPDATE ERROR: %s; %+v", err.Error(), err))
			}
		}

		if err := c.annotateCluster(bgCtx, cr, map[string]string{providerKopsCreateComplete: ""}); err != nil {
			log.Info(fmt.Sprintf("WARNING: %s", err.Error()))
		}
		// naive attempt to unlock cluster
		if err := c.unlockCluster(bgCtx, cr, []string{providerKopsCreatePending, providerKopsUpdateLocked}); err != nil {
			log.Info(fmt.Sprintf("WARNING: %s", err.Error()))
		}
	}()

	return mo, nil
}

// Update "updates" the k8s cluster and instance groups and initial secrets based on cr parameters,
// we use k8s annotations to manage the resource lifecycle and prevent concurrent executions of the
// `kops` cli; these are:
//
// 1. On initial call to `Update`:
//   - set annotation `{providerKopsUpdateLocked}`
//
// 2. On completion of initial `Update` go func:
//   - remove annotations `{providerKopsUpdateLocked}`
//
// Note that this means cluster and instance changes are applied _sequentially_, it's totally normal
// for a delta to exist between the desired and observed state of the resource while a previous
// update operation is ongoing.
//
// NB! Just as w/ the `Create` operation, if the process crashes or otherwise requires manual
// intervention to correct the state of the external cluster, then it may be necessary to manually
// remove `{providerKopsUpdateLocked}` from the resource
func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*apisv1alpha1.Cluster)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotCluster)
	}

	connDetails, _ := getClusterConnectionDetails(cr)
	mo := managed.ExternalUpdate{ConnectionDetails: connDetails}

	if cr.Status.Status == apisv1alpha1.Unknown {
		log.Debug(fmt.Sprintf("status of cluster %s is unknown, this could be due to validation errors, skipping updates until external state is stable", cr.Name))
		return mo, nil
	}
	if _, resourceCreated := cr.Annotations[providerKopsCreateComplete]; !resourceCreated {
		log.Debug(fmt.Sprintf("Still creating cluster %s", cr.Name))
		return mo, nil
	}
	if _, resourceLocked := cr.Annotations[providerKopsUpdateLocked]; resourceLocked {
		log.Debug(fmt.Sprintf("Already updating %s", cr.Name))
		return mo, nil
	}

	if err := c.lockCluster(ctx, cr, []string{providerKopsUpdateLocked}); err != nil {
		return mo, err
	}

	log.Info(fmt.Sprintf("Begin updating: %+v; status: %s", cr.Name, cr.Status.Status))

	// don't block when updating the cluster, this takes a while..
	go func() {
		bgCtx := context.Background()

		if err := c.service.updateCluster(bgCtx, cr); err != nil {
			log.Info(fmt.Sprintf("UPDATE ERROR: %s; %+v", err.Error(), err))
		} else if err := c.service.rollingUpdateCluster(bgCtx, cr); err != nil {
			log.Info(fmt.Sprintf("ROLLING UPDATE ERROR: %s; %+v", err.Error(), err))
		}

		if err := c.unlockCluster(bgCtx, cr, []string{providerKopsUpdateLocked}); err != nil {
			log.Info(fmt.Sprintf("WARNING: %s; %+v", err.Error(), err))
		}

		log.Info(fmt.Sprintf("Update complete for %s", cr.Name))
	}()

	return mo, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*apisv1alpha1.Cluster)
	if !ok {
		return errors.New(errNotCluster)
	}

	log.Info(fmt.Sprintf("Deleting: %s", cr.Name))
	if err := c.deleteConnectionSecret(ctx, cr); err != nil {
		log.Info(fmt.Sprintf("Unable to delete connection secret for resource %s, you may need to manually delete the secret; err: %+v", cr.Name, err))
		return err
	}

	meta.RemoveFinalizer(cr, finalizer)
	if err := c.kube.Update(ctx, cr); err != nil {
		log.Info(fmt.Sprintf("Unable to remove finalizer from resource %s, you may need to manually remove the finalizer; err: %+v", cr.Name, err))
		return err
	}

	return nil
}

func (c *external) deleteConnectionSecret(ctx context.Context, cr *apisv1alpha1.Cluster) error {
	if cr.Spec.WriteConnectionSecretToReference != nil {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cr.Spec.WriteConnectionSecretToReference.Name,
				Namespace: cr.Spec.WriteConnectionSecretToReference.Namespace,
			},
		}
		return c.kube.Delete(ctx, secret)
	}
	return nil
}

func (c *external) getCluster(ctx context.Context, cr *apisv1alpha1.Cluster) (*apisv1alpha1.Cluster, error) {
	fcrn := types.NamespacedName{
		Name:      cr.Name,
		Namespace: cr.Namespace,
	}
	fcr := &apisv1alpha1.Cluster{}
	if err := c.kube.Get(ctx, fcrn, fcr); err != nil {
		return fcr, err
	}
	return fcr, nil
}

func (c *external) annotateCluster(ctx context.Context, cr *apisv1alpha1.Cluster, annotations map[string]string) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		fcr, err := c.getCluster(ctx, cr)
		if err != nil {
			return err
		}

		for k, v := range annotations {
			fcr.Annotations[k] = v
		}
		err = c.kube.Update(ctx, fcr, []client.UpdateOption{}...)
		return err
	})
	return err
}

func (c *external) lockCluster(ctx context.Context, cr *apisv1alpha1.Cluster, lockKeys []string) error {
	annotations := map[string]string{}
	for _, k := range lockKeys {
		annotations[k] = ""
	}
	return c.annotateCluster(ctx, cr, annotations)
}

func (c *external) unlockCluster(ctx context.Context, cr *apisv1alpha1.Cluster, lockKeys []string) error {
	// clear the lock
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		fcr, err := c.getCluster(ctx, cr)
		if err != nil {
			return err
		}

		for _, k := range lockKeys {
			delete(fcr.Annotations, k)
		}
		err = c.kube.Update(ctx, fcr, []client.UpdateOption{}...)
		return err
	})
	return err
}
