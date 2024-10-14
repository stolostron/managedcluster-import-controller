# Import a OCM ROSA cluster from OCM

To import a ROSA/ROSA-HCP cluster from OCM platform into ACM Hub, there are 2 ways:
1. [Import it from the console](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_management_for_kubernetes/2.11/html-single/clusters/index#import-discovered), the Console is responsible for creating the auto-import-secret.
2. Discovery creates auto-import-secret when the flag of auto import is set to `true`.

## Related information

- Related Repos:
  - [Discovery](https://github.com/stolostron/discovery)
    - Key Code: [Create AutoImportSecret in Discovery](https://github.com/stolostron/discovery/blob/67b10a3f98a648a91c4638c42e9529e521b8fbea/controllers/discoveredcluster_controller.go#L148-L168)
    - Key Person: @dislbenn
  - [Console](https://github.com/stolostron/console)
    - Key Person: @KevinFCormier
- Slack Chat:
  - [Discussion of OCM Token Migration](https://redhat-internal.slack.com/archives/C06TNDPUZKM/p1727126722361369): A great example of how 3 components work together to make this feature possible.
- Docs:
  - [OCM Token Migration Guide](https://docs.google.com/document/d/1tf6BRSJXxxnrbOJ81O9Xajgc3yVDRKdwen1W88ks84Y/edit#heading=h.jnmdcvmm87m6): After this migration, ACM provide 2 ways to import ROSA/ROSA-HCP cluster from OCM platform -- token and service account.

## Preparation

1. Go to https://console.redhat.com/openshift/token/rosa to get your OpenShift Cluster Manager API Token
2. Use `rosa describe cluster -c <your-rosa-cluster-name> -ojosn | jq -r '.id'` to get your rosa cluster ID

## Import a ROSA/ROSA-HCP cluster

1. Create a ManagedCluster on the hub cluster

```sh
oc apply -f - <<EOF
apiVersion: cluster.open-cluster-management.io/v1
kind: ManagedCluster
metadata:
  name: <your_cluster_name>
spec:
  hubAcceptsClient: true
EOF
```

2. Create a `auto-import-secret` secret with type `auto-import/rosa` in your managed cluster namespace

```sh
oc apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: auto-import-secret
  namespace: <your_cluster_name>
stringData:
  auth_method: "offline-token"
  api_token: <your_openshift_cluster_manager_api_token>
  cluster_id: <your_rosa_cluster_id>
type: auto-import/rosa
EOF
```

If you are using service account, the auto-import-secret should look like this:

```sh
apiVersion: v1
kind: Secret
metadata:
  name: auto-import-secret
  namespace: <your_cluster_name>
stringData:
  auth_method: "service-account"
  client_id: <your_service_account_client_id>
  client_secret: <your_service_account_client_secret>
  cluster_id: <your_rosa_cluster_id>
```

There are three optional options that can be added to secret for development purposes

- `api_url`, The OpenShift API URL, the default value is https://api.openshift.com, it can be set to https://api.integration.openshift.com or https://api.staging.openshift.com
- `token_url`, OpenID token URL. the default value is https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/token
- `retry_times`, The number of retries to obtain the ROSA cluster kube token, the default value is 20. The interval between each retry is 30 seconds.

**Note**: The import controller will create a temporary cluster admin user `acm-import` with a temporary htPasswdIDProvider `acm-import` for your cluster (the name `acm-import` is hard coded), the import controller will use this user to fetch your cluster kube token and use this token to deploy the Klusterlet in your cluster. After your cluster is imported, the import controller will delete the temporary user and htPasswdIDProvider.
