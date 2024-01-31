[comment]: # ( Copyright Contributors to the Open Cluster Management project )

# Auto import a ROSA cluster

We can import a ROSA/ROSA-HCP cluster into ACM Hub with `auto-import-secret`

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
  api_token: <your_openshift_cluster_manager_api_token>
  cluster_id: <your_rosa_cluster_id>
type: auto-import/rosa
EOF
```

There are three optional options that can be added to secret for development purposes

- `api_url`, The OpenShift API URL, the default value is https://api.openshift.com, it can be set to https://api.integration.openshift.com or https://api.staging.openshift.com
- `token_url`, OpenID token URL. the default value is https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/token
- `retry_times`, The number of retries to obtain the ROSA cluster kube token, the default value is 20. The interval between each retry is 30 seconds.

**Note**: The import controller will create a temporary cluster admin user `acm-import` with a temporary htPasswdIDProvider `acm-import` for your cluster (the name `acm-import` is hard coded), the import controller will use this user to fetch your cluster kube token and use this token to deploy the Klusterlet in your cluster. After your cluster is imported, the import controller will delete the temporary user and htPasswdIDProvider.
