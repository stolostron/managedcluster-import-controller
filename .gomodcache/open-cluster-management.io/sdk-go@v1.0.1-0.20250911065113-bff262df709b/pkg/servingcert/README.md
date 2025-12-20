# This controller is used to generate a self-signed CA Bundle configMap and serving cert secrets in a specified namespace.

## Usage

1. New a controller and start.

```go
    servingcert.NewServingCertController("my-namespace", kubeClient).
    WithTargetServingCerts([]servingcert.TargetServingCertOptions{
    {
        Name:      "my-target-serving-cert",
        HostNames: []string{"my-target-serving-cert.my-namespace.svc"},
    },
    }).Start(ctx)
```

The controller will create an CA Bundle configMap named `ca-bundle-configmap` which is self-signed
with the singer secret named `signer-secret`.  And the target serving cert secret "my-target-serving-cert"
will be signed with the CA Bundle and created in the same namespace.

2. Permissions for the `kubeClient`.

The RBAC of the `kubeClient` in `NewServingCertController` must have `GET/LIST/WATCH/CREATE/UPDATE` permissions
for the configMap and Secrets in the specified namespace, including the CA Bundle configMap, signer secret, and
all target serving cert secrets.

3. Options:

    * `WithSignerNamePrefix(signerNamePrefix string)` is to configure the singer name prefix in the certs.
    The default is `<namespace>-singer`.

    * `WithSignerSecretName(secretName string)` is to configure the singer secret name for ca bundle.
    the default is `signer-secret`.

    * `WithCABundleConfigmapName(caBundleConfigmapName string)` is to configure the ca bundle configMap name.
    the default is `ca-bundle-configmap`.

    * `WithSigningCertValidity(validity time.Duration)` is to configure the rotation validity time duration
    for the signing cert and key. The default is 365 days.

    * `WithTargetCertValidity(validity time.Duration)` is to configure the rotation validity time duration for
    the target serving cert secret. The default is 30 days.

    * ` WithResyncInterval(validity time.Duration)` is to configure the re-sync interval for the controller.
    The default is 10 minutes.

    * ` WithTargetServingCerts(targets []TargetServingCertOptions)` is to configure the target serving cert secret name,
    host names and load dir. The host name is usually `<service name>.<namespace>.svc`. Load the secret to the
    local directory if LoadDir is not empty, and the `tls.crt` and `tls.key` files will be created in the dir.

4. How to get CA Bundle data?

    We can get the CA Bundle data from `ca-bundle.crt` data in the CA Bundle configMap.
    ```go
    caBundleConfigMap, err := kubeClient.CoreV1().ConfigMaps(namespace).Get(context.Background(),
    DefaultCABundleConfigmapName, metav1.GetOptions{})

    caBundle := caBundleConfigMap.Data["ca-bundle.crt"]
    ```
