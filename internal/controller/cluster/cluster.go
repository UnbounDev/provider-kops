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

	"github.com/crossplane/provider-kops/apis/v1alpha1"
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
	name := managed.ControllerName(v1alpha1.ClusterGroupKind)
	log = o.Logger

	cps := []managed.ConnectionPublisher{managed.NewAPISecretPublisher(mgr.GetClient(), mgr.GetScheme())}
	if o.Features.Enabled(features.EnableAlphaExternalSecretStores) {
		cps = append(cps, connection.NewDetailsManager(mgr.GetClient(), apisv1alpha1.StoreConfigGroupVersionKind))
	}

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.ClusterGroupVersionKind),
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
		For(&v1alpha1.Cluster{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube         client.Client
	usage        resource.Tracker
	newServiceFn func(creds []byte, pubSshKey []byte) (*kopsClient, error)
}

// Connect typically produces an ExternalClient by:
// 1. Tracking that the managed resource is using a ProviderConfig.
// 2. Getting the managed resource's ProviderConfig.
// 3. Getting the credentials specified by the ProviderConfig.
// 4. Using the credentials to form a client.
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.Cluster)
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
	cr, ok := mg.(*v1alpha1.Cluster)
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

	// These fmt statements should be removed in the real implementation.
	// fmt.Printf("Observing: %+v", cr)

	cluster, err := c.service.observeCluster(ctx, cr)
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

		// initialize status to a "ready" state and alter this based on annotations & diff
		cr.Status.Status = apisv1alpha1.Ready
		annotations := cr.GetAnnotations()

		// check lock to prevent concurrent updates to state etc..
		//	_, resourceReconciling := annotations[providerKopsReconcilePending]
		//	if resourceReconciling {
		//		// TODO: print log indicating that the resource may not be updated, instruct
		//		// removal of the lock annotation if needed
		//		return mo, nil
		//	} else {
		//		if err := c.annotateCluster(ctx, cr, map[string]string{providerKopsReconcilePending: ""}); err != nil {
		//			return mo, err
		//		}
		//	}

		// check lock to prevent concurrent updates to state etc..
		_, resourceLocked := annotations[providerKopsUpdateLocked]
		if resourceLocked {
			// TODO: print log indicating that the resource may not be updated, instruct
			// removal of the lock annotation if needed
			cr.SetConditions(xpv1.Unavailable())
			return mo, nil
		}

		// check for initial creation annotations
		_, resourceCreating := annotations[providerKopsCreatePending]
		_, resourceCreated := annotations[providerKopsCreateComplete]
		if resourceCreating && !resourceCreated {
			cr.Status.Status = apisv1alpha1.Updating
		}

		// now run validation w/ fallback for authentication to the cluster
		output, err := c.service.validateCluster(ctx, cr, []string{})
		if err != nil && strings.Contains(err.Error(), errNoAuth) {
			if err := c.service.authenticateToCluster(ctx, cr, []string{}); err != nil {
				return managed.ExternalObservation{}, err
			}
		}

		if len(output.Failures) == 0 && err == nil {
			cr.Status.Status = apisv1alpha1.Ready

			changelog, err := c.service.diffCluster(ctx, cr)
			if err != nil {
				return managed.ExternalObservation{}, err
			}
			if len(changelog) > 0 {
				// set status to prompt update
				cr.Status.Status = apisv1alpha1.Updating
				if changelogjson, err := json.Marshal(changelog); err == nil {
					log.Debug(fmt.Sprintf("Change detected: %s", string(changelogjson)))
				}
			}
			// TODO(ab): check annotations for trigger to perform update
			// TODO(ab): check annotations for trigger to perform rolling-update
		}
		if err != nil {
			log.Info(fmt.Sprintf("WARNING OBSERVER ERROR: %s; %s", err.Error(), output))
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
		mo.ResourceExists = true
		mo.ResourceUpToDate = false
		// NB! We set the status to `Unavailable` bc the cluster is not
		// "available" for use by the controller, it may be the case that
		// a user needs to manually intervene and fix the cluster state
		// before the controller can resume management
		cr.SetConditions(xpv1.Unavailable())
	}

	return mo, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Cluster)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotCluster)
	}

	if _, ok := cr.Annotations[crossplaneCreateSucceeded]; ok {
		log.Debug(fmt.Sprintf("Already creating: %s", cr.Name))
		return managed.ExternalCreation{}, nil
	}
	if err := c.lockCluster(ctx, cr, []string{providerKopsCreatePending, providerKopsUpdateLocked}); err != nil {
		return managed.ExternalCreation{}, err
	}

	log.Info(fmt.Sprintf("Begin creating: %s", cr.Name))
	log.Debug(fmt.Sprintf("%+v", cr))

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

		// TODO(ab): we need to provide lifecycle management for keypairs, rather
		// than only providing bootstrap creation
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
		if len(cr.Spec.ForProvider.Keypairs) > 0 {
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

	connDetails := managed.ConnectionDetails{
		"kubeconfig": []byte{},
	}

	return managed.ExternalCreation{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: connDetails,
	}, nil
}

func (c *external) getCluster(ctx context.Context, cr *v1alpha1.Cluster) (*v1alpha1.Cluster, error) {
	fcrn := types.NamespacedName{
		Name:      cr.Name,
		Namespace: cr.Namespace,
	}
	fcr := &v1alpha1.Cluster{}
	if err := c.kube.Get(ctx, fcrn, fcr); err != nil {
		return fcr, err
	}
	return fcr, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.Cluster)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotCluster)
	}

	_, resourceLocked := cr.Annotations[providerKopsUpdateLocked]
	if resourceLocked {
		log.Debug(fmt.Sprintf("Already updating %s", cr.Name))
		return managed.ExternalUpdate{}, nil
	}

	if err := c.lockCluster(ctx, cr, []string{providerKopsUpdateLocked}); err != nil {
		return managed.ExternalUpdate{}, err
	}

	log.Info(fmt.Sprintf("Begin updating: %+v", cr.Name))

	// don't block when updating the cluster, this takes a while..
	go func() {
		bgCtx := context.Background()
		if err := c.service.updateCluster(bgCtx, cr); err != nil {
			log.Info(fmt.Sprintf("UPDATE ERROR: %s; %+v", err.Error(), err))
		}

		if err := c.service.rollingUpdateCluster(ctx, cr); err != nil {
			log.Info(fmt.Sprintf("ROLLING UPDATE ERROR: %s; %+v", err.Error(), err))
		}

		if err := c.unlockCluster(bgCtx, cr, []string{providerKopsUpdateLocked}); err != nil {
			log.Info(fmt.Sprintf("WARNING: %s; %+v", err.Error(), err))
		}

		log.Info(fmt.Sprintf("Update complete for %s", cr.Name))
	}()

	b, err := os.ReadFile(getKubeConfigFilePath(cr))
	if err != nil {
		log.Info(fmt.Sprintf("WARNING: kubeconfig file not found at '%s'", getKubeConfigFilePath(cr)))
	}
	connDetails := managed.ConnectionDetails{
		"kubeconfig": b,
	}

	return managed.ExternalUpdate{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: connDetails,
	}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*v1alpha1.Cluster)
	if !ok {
		return errors.New(errNotCluster)
	}

	log.Info(fmt.Sprintf("Deleting: %s", cr.Name))

	meta.RemoveFinalizer(cr, finalizer)
	if err := c.kube.Update(ctx, cr); err != nil {
		return err
	}

	return nil
}

func (c *external) annotateCluster(ctx context.Context, cr *v1alpha1.Cluster, annotations map[string]string) error {
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

func (c *external) lockCluster(ctx context.Context, cr *v1alpha1.Cluster, lockKeys []string) error {
	annotations := map[string]string{}
	for _, k := range lockKeys {
		annotations[k] = ""
	}
	return c.annotateCluster(ctx, cr, annotations)
}

func (c *external) unlockCluster(ctx context.Context, cr *v1alpha1.Cluster, lockKeys []string) error {
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
