package servingcert

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"open-cluster-management.io/sdk-go/pkg/basecontroller/factory"
	"open-cluster-management.io/sdk-go/pkg/certrotation"
)

const (
	defaultSigningCertValidity   = time.Hour * 24 * 365
	defaultTargetCertValidity    = time.Hour * 24 * 30
	defaultResyncInterval        = time.Minute * 10
	DefaultCABundleConfigmapName = "ca-bundle-configmap"
	DefaultSignerSecretName      = "signer-secret"
)

type TargetServingCertOptions struct {
	Name      string   // the target serving cert secret name
	LoadDir   string   // load the secret to the local dir if LoadDir is not empty.
	HostNames []string // the host names for the serving cert. it is usually <service name>.<namespace name>.svc .
}

type ServingCertController struct {
	namespace                          string
	targetCertValidity, resyncInterval time.Duration
	kubeClient                         kubernetes.Interface
	signingRotation                    certrotation.SigningRotation
	caBundleRotation                   certrotation.CABundleRotation
	targetRotations                    []certrotation.TargetRotation
}

func NewServingCertController(namespace string, kubeClient kubernetes.Interface) *ServingCertController {
	return &ServingCertController{
		namespace:          namespace,
		targetCertValidity: defaultTargetCertValidity,
		resyncInterval:     defaultResyncInterval,
		kubeClient:         kubeClient,

		signingRotation: certrotation.SigningRotation{
			Namespace:        namespace,
			Name:             DefaultSignerSecretName,
			SignerNamePrefix: fmt.Sprintf("%s-signer", namespace),
			Validity:         defaultSigningCertValidity,
			Client:           kubeClient.CoreV1(),
		},
		caBundleRotation: certrotation.CABundleRotation{
			Namespace: namespace,
			Name:      DefaultCABundleConfigmapName,
			Client:    kubeClient.CoreV1(),
		},
		targetRotations: []certrotation.TargetRotation{},
	}
}

// WithSignerNamePrefix is to configure the singer name prefix in the certs. The default is <namespace>-singer.
func (c *ServingCertController) WithSignerNamePrefix(signerNamePrefix string) *ServingCertController {
	c.signingRotation.SignerNamePrefix = signerNamePrefix
	return c
}

// WithSigningCertValidity is to configure the rotation validity time duration for the signing cert and key. The default is 365 days.
func (c *ServingCertController) WithSigningCertValidity(validity time.Duration) *ServingCertController {
	c.signingRotation.Validity = validity
	return c
}

// WithSignerSecretName is to configure the singer secret name for ca bundle. the default is signer-secret.
func (c *ServingCertController) WithSignerSecretName(secretName string) *ServingCertController {
	c.signingRotation.Name = secretName
	return c
}

// WithCABundleConfigMapName is to configure the ca bundle configmap name. the default is ca-bundle-configmap.
func (c *ServingCertController) WithCABundleConfigMapName(caBundleConfigMapName string) *ServingCertController {
	c.caBundleRotation.Name = caBundleConfigMapName
	return c
}

// WithTargetCertValidity is to configure the rotation validity time duration for the serving cert. The default is 30 days.
func (c *ServingCertController) WithTargetCertValidity(validity time.Duration) *ServingCertController {
	c.targetCertValidity = validity
	return c
}

// WithResyncInterval is to configure the re-sync interval for the controller. The default is 10 minutes.
func (c *ServingCertController) WithResyncInterval(validity time.Duration) *ServingCertController {
	c.resyncInterval = validity
	return c
}

// WithTargetServingCerts is to configure the target serving cert secret name, host names and load dir.
// The host name is usually <service name>.<namespace>.svc .
// Load the secret to the local dir if LoadDir is not empty.
func (c *ServingCertController) WithTargetServingCerts(targets []TargetServingCertOptions) *ServingCertController {
	for _, target := range targets {
		targetRotation := certrotation.TargetRotation{
			Namespace: c.namespace,
			Name:      target.Name,
			LoadDir:   target.LoadDir,
			Validity:  c.targetCertValidity,
			HostNames: target.HostNames,
			Client:    c.kubeClient.CoreV1(),
		}
		c.targetRotations = append(c.targetRotations, targetRotation)
	}
	return c
}

func (c *ServingCertController) Start(ctx context.Context) {
	newOnTermInformer := func(name string) informers.SharedInformerFactory {
		return informers.NewSharedInformerFactoryWithOptions(c.kubeClient, 5*time.Minute,
			informers.WithNamespace(c.namespace),
			informers.WithTweakListOptions(func(options *metav1.ListOptions) {
				options.FieldSelector = fields.OneTermEqualSelector("metadata.name", name).String()
			}))
	}
	configmapInformerFT := newOnTermInformer(c.caBundleRotation.Name)
	c.caBundleRotation.Lister = configmapInformerFT.Core().V1().ConfigMaps().Lister()
	configmapInformer := configmapInformerFT.Core().V1().ConfigMaps().Informer()

	signingSecretInformerFT := newOnTermInformer(c.signingRotation.Name)
	c.signingRotation.Lister = signingSecretInformerFT.Core().V1().Secrets().Lister()

	secretInformerFTs := []informers.SharedInformerFactory{signingSecretInformerFT}
	secretInformers := []factory.Informer{signingSecretInformerFT.Core().V1().Secrets().Informer()}

	for index := range c.targetRotations {
		targetSecretInformerFT := newOnTermInformer(c.targetRotations[index].Name)
		targetSecretInformer := targetSecretInformerFT.Core().V1().Secrets()
		c.targetRotations[index].Lister = targetSecretInformer.Lister()
		secretInformerFTs = append(secretInformerFTs, targetSecretInformerFT)
		secretInformers = append(secretInformers, targetSecretInformer.Informer())
	}

	queueKeysFunc := func(obj runtime.Object) []string {
		key, _ := cache.MetaNamespaceKeyFunc(obj)
		return []string{key}
	}

	controller := factory.New().ResyncEvery(c.resyncInterval).
		WithInformersQueueKeysFunc(queueKeysFunc, configmapInformer).
		WithInformersQueueKeysFunc(queueKeysFunc, secretInformers...).
		WithSync(c.sync).
		ToController("cert-rotation-controller")

	configmapInformerFT.Start(ctx.Done())
	for _, secretInformerFT := range secretInformerFTs {
		secretInformerFT.Start(ctx.Done())
	}

	go controller.Run(ctx, 1)
}

func (c *ServingCertController) sync(ctx context.Context, syncCtx factory.SyncContext, key string) error {
	signingCertKeyPair, err := c.signingRotation.EnsureSigningCertKeyPair()
	if err != nil {
		return fmt.Errorf("failed to ensure signing cert key pair: %v", err)
	}

	caBundleCerts, err := c.caBundleRotation.EnsureConfigMapCABundle(signingCertKeyPair)
	if err != nil {
		return fmt.Errorf("failed to ensure signing cert CA bundle configmap: %v", err)
	}

	var errs []error
	for _, targetRotation := range c.targetRotations {
		if err := targetRotation.EnsureTargetCertKeyPair(signingCertKeyPair, caBundleCerts); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.NewAggregate(errs)
}
