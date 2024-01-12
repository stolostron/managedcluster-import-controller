// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package helpers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	sdk "github.com/openshift-online/ocm-sdk-go"
	clustersmgmtv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/library-go/pkg/oauth/tokenrequest"
	"github.com/sethvargo/go-password/password"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"
)

/* #nosec */
const (
	defaultAPIServerURL = "https://api.openshift.com"
	defaultTokenURL     = "https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/token"

	clusterAdminGroup = "cluster-admins"

	importHTPasswdIDProvider = "acm-import"
	importHTPasswdUser       = "acm-import"

	rosaImportRetryPeriod = 30 * time.Second
	defaultRetryTimes     = 20 // default timeout will be `defaultRetryTimes*rosaImportRetryPeriod (10 mins)
)

type RosaKubeConfigGetter struct {
	apiServerURL      string
	tokenURL          string
	token             string
	clusterID         string
	importUserPasswd  string
	totalRetryTimes   int
	currentRetryTimes int
}

func NewRosaKubeConfigGetter() *RosaKubeConfigGetter {
	return &RosaKubeConfigGetter{
		apiServerURL:      defaultAPIServerURL,
		tokenURL:          defaultTokenURL,
		totalRetryTimes:   defaultRetryTimes,
		currentRetryTimes: 0,
	}
}

func (g *RosaKubeConfigGetter) SetAPIServerURL(apiServer string) {
	g.apiServerURL = apiServer
}

func (g *RosaKubeConfigGetter) SetTokenURL(tokenURL string) {
	g.tokenURL = tokenURL
}

func (g *RosaKubeConfigGetter) SetToken(token string) {
	g.token = token
}

func (g *RosaKubeConfigGetter) SetClusterID(clusterID string) {
	g.clusterID = clusterID
}

func (g *RosaKubeConfigGetter) SetRetryTimes(retryTimes string) {
	retryTimesInt, err := strconv.Atoi(retryTimes)
	if err != nil {
		klog.Warningf("retry times is invalid, using default retry times (%d), %v", defaultRetryTimes, err)
		return
	}

	if retryTimesInt <= 0 {
		klog.Warningf("retry times is less than or equal to 0, using default retry times (%d), %v", defaultRetryTimes, err)
		return
	}

	g.totalRetryTimes = retryTimesInt
}

func (g *RosaKubeConfigGetter) KubeConfig() (bool, *clientcmdapi.Config, error) {
	connection, err := g.newConnection()
	if err != nil {
		return false, nil, err
	}
	defer connection.Close()

	// Get the client for the resource that manages the collection of clusters:
	clusterClient := connection.ClustersMgmt().V1().Clusters().Cluster(g.clusterID)
	resp, err := clusterClient.Get().Send()
	if err != nil {
		return false, nil, err
	}

	api, ok := resp.Body().GetAPI()
	if !ok {
		return false, nil, fmt.Errorf("rosa cluster %s api url is not found", g.clusterID)
	}

	if len(g.importUserPasswd) == 0 {
		importUserPassword, err := createImportUserWithHTPasswdIDProvider(clusterClient)
		if err != nil {
			return false, nil, err
		}

		// add the acm import user to cluster admin group
		adminsClient := clusterClient.Groups().Group(clusterAdminGroup).Users()
		if err := addImportUserToClusterAdminGroup(adminsClient); err != nil {
			return false, nil, err
		}

		g.importUserPasswd = importUserPassword
	}

	token, err := tokenrequest.RequestTokenWithChallengeHandlers(&rest.Config{
		Host:     api.URL(),
		Username: importHTPasswdUser,
		Password: g.importUserPasswd,
	})
	if err != nil {
		if g.shouldRetry(clusterClient) {
			klog.Infof("Failed to get kubeconfig for rosa cluster %s, retry after %d seconds, %v",
				g.clusterID, rosaImportRetryPeriod/time.Second, err)
			return true, nil, fmt.Errorf("kubeconfig for rosa cluster %s is not ready, retry after %d seconds",
				g.clusterID, rosaImportRetryPeriod/time.Second)
		}

		return false, nil, fmt.Errorf("failed to get kubeconfig for rosa cluster %s after %d seconds, err",
			g.clusterID, (rosaImportRetryPeriod*time.Duration(g.totalRetryTimes))/time.Second)
	}

	return false, buildKubeConfigFileWithToken(api.URL(), token), nil
}

func (g *RosaKubeConfigGetter) Cleanup() error {
	connection, err := g.newConnection()
	if err != nil {
		return err
	}
	defer connection.Close()

	errs := []error{}
	clusterClient := connection.ClustersMgmt().V1().Clusters().Cluster(g.clusterID)
	// the cluster token is requested, delete the id provider and remove the import user from cluster admin group
	if err := deleteHTPasswdIDProvider(clusterClient.IdentityProviders(), g.clusterID); err != nil {
		errs = append(errs, err)
	}

	if err := removeImportUserFromClusterAdminGroup(clusterClient.Groups().Group(clusterAdminGroup).Users()); err != nil {
		errs = append(errs, err)
	}

	return utilerrors.NewAggregate(errs)
}

func (g *RosaKubeConfigGetter) newConnection() (*sdk.Connection, error) {
	logger, err := sdk.NewGoLoggerBuilder().
		Debug(true).
		Build()
	if err != nil {
		return nil, err
	}

	// Create the connection, and remember to close it:
	return sdk.NewConnectionBuilder().
		Logger(logger).
		URL(g.apiServerURL).
		TokenURL(g.tokenURL).
		Tokens(g.token).
		Build()
}

func (g *RosaKubeConfigGetter) shouldRetry(clusterClient *clustersmgmtv1.ClusterClient) bool {
	if g.currentRetryTimes >= g.totalRetryTimes {
		// request the cluster token timeout, delete its id provider and remove the import user from cluster admin group
		klog.Warningf("stop to retry getting kube token for rosa cluster %s, reach the retry times limit (%d)",
			g.clusterID, g.totalRetryTimes)
		if err := deleteHTPasswdIDProvider(clusterClient.IdentityProviders(), g.clusterID); err != nil {
			klog.Warningf("failed to delete the htPasswd id provider %s for rosa cluster %s, %v",
				importHTPasswdIDProvider, g.clusterID, err)
		}

		if err := removeImportUserFromClusterAdminGroup(clusterClient.Groups().Group(clusterAdminGroup).Users()); err != nil {
			klog.Warningf("failed to remove the import user %s from cluster admin group for rosa cluster %s, %v",
				importHTPasswdIDProvider, g.clusterID, err)
		}

		return false
	}

	g.currentRetryTimes++
	return true
}

func createImportUserWithHTPasswdIDProvider(clusterClient *clustersmgmtv1.ClusterClient) (string, error) {
	// try to find a htPasswd provider for acm import user
	idProvidersClient := clusterClient.IdentityProviders()
	providerID, err := findHTPasswdIDProvider(idProvidersClient)
	if err != nil {
		return "", err
	}

	if len(providerID) == 0 {
		return createHTPasswdIDProviderWithUser(idProvidersClient)
	}

	// try to find the acm import user in the htPasswd provider
	userID, err := findHTPasswdUser(idProvidersClient, providerID)
	if err != nil {
		return "", err
	}

	if len(userID) == 0 {
		return addHTPasswdUser(idProvidersClient, providerID)
	}

	return updateHTPasswdUserPassword(idProvidersClient, providerID, userID)
}

func findHTPasswdIDProvider(client *clustersmgmtv1.IdentityProvidersClient) (string, error) {
	providerID := ""
	idProviders, err := client.List().Send()
	if err != nil {
		return providerID, err
	}

	providers, ok := idProviders.GetItems()
	if !ok {
		return providerID, nil
	}

	providers.Each(func(provider *clustersmgmtv1.IdentityProvider) bool {
		if provider == nil {
			return true
		}

		if provider.Type() != clustersmgmtv1.IdentityProviderTypeHtpasswd {
			return true
		}

		if provider.Name() != importHTPasswdIDProvider {
			return true
		}

		providerID = provider.ID()

		return false
	})

	return providerID, nil
}

func findHTPasswdUser(client *clustersmgmtv1.IdentityProvidersClient, providerID string) (string, error) {
	userID := ""

	users, err := client.IdentityProvider(providerID).HtpasswdUsers().List().Send()
	if err != nil {
		return "", err
	}

	users.Items().Each(func(user *clustersmgmtv1.HTPasswdUser) bool {
		if user == nil {
			return true
		}

		if user.Username() != importHTPasswdUser {
			return true
		}

		userID = user.ID()

		return false
	})

	return userID, nil
}

func findImportUserFromClusterAdminGroup(client *clustersmgmtv1.UsersClient) (bool, error) {
	userResp, err := client.User(importHTPasswdUser).Get().Send()
	if err != nil {
		if userResp != nil && userResp.Status() == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func addImportUserToClusterAdminGroup(client *clustersmgmtv1.UsersClient) error {
	found, err := findImportUserFromClusterAdminGroup(client)
	if err != nil {
		return err
	}

	if found {
		return nil
	}
	user, err := clustersmgmtv1.NewUser().ID(importHTPasswdUser).Build()
	if err != nil {
		return err
	}

	_, err = client.Add().Body(user).Send()
	return err
}

func createHTPasswdIDProviderWithUser(client *clustersmgmtv1.IdentityProvidersClient) (string, error) {
	pw, err := password.Generate(20, 10, 0, false, false)
	if err != nil {
		return "", err
	}

	htPasswdUserBuilder := clustersmgmtv1.NewHTPasswdUser().Username(importHTPasswdUser).Password(pw)
	htPasswdUserListBuilder := clustersmgmtv1.NewHTPasswdUserList().Items(htPasswdUserBuilder)
	htPasswdIDProvider, err := clustersmgmtv1.NewIdentityProvider().
		Name(importHTPasswdIDProvider).
		Type(clustersmgmtv1.IdentityProviderTypeHtpasswd).
		Htpasswd(clustersmgmtv1.NewHTPasswdIdentityProvider().Users(htPasswdUserListBuilder)).
		Build()
	if err != nil {
		return "", err
	}

	if _, err := client.Add().Body(htPasswdIDProvider).Send(); err != nil {
		return "", err
	}
	return pw, nil
}

func addHTPasswdUser(client *clustersmgmtv1.IdentityProvidersClient, providerID string) (string, error) {
	pw, err := password.Generate(20, 10, 0, false, false)
	if err != nil {
		return "", err
	}
	htPasswdUser, err := clustersmgmtv1.NewHTPasswdUser().Username(importHTPasswdUser).Password(pw).Build()
	if err != nil {
		return "", err
	}
	if _, err = client.IdentityProvider(providerID).HtpasswdUsers().Add().Body(htPasswdUser).Send(); err != nil {
		return "", err
	}

	return pw, nil
}

func updateHTPasswdUserPassword(client *clustersmgmtv1.IdentityProvidersClient, providerID, userID string) (string, error) {
	pw, err := password.Generate(20, 10, 0, false, false)
	if err != nil {
		return "", err
	}
	htPasswdUser, err := clustersmgmtv1.NewHTPasswdUser().Password(pw).Build()
	if err != nil {
		return "", err
	}
	if _, err = client.IdentityProvider(providerID).HtpasswdUsers().
		HtpasswdUser(userID).Update().Body(htPasswdUser).Send(); err != nil {
		return "", err
	}

	return pw, nil
}

func deleteHTPasswdIDProvider(client *clustersmgmtv1.IdentityProvidersClient, clusterID string) error {
	providerID, err := findHTPasswdIDProvider(client)
	if err != nil {
		return err
	}

	if len(providerID) == 0 {
		return nil
	}

	_, err = client.IdentityProvider(providerID).Delete().Send()
	return err
}

func removeImportUserFromClusterAdminGroup(client *clustersmgmtv1.UsersClient) error {
	found, err := findImportUserFromClusterAdminGroup(client)
	if err != nil {
		return err
	}

	if !found {
		return nil
	}

	_, err = client.User(importHTPasswdUser).Delete().Send()
	return err
}
