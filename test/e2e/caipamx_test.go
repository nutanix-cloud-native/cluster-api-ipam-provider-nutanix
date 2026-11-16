// Copyright 2026 Nutanix. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/gob"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	ipamv1 "sigs.k8s.io/cluster-api/api/ipam/v1beta2"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/bootstrap"
	capiclusterctl "sigs.k8s.io/cluster-api/test/framework/clusterctl"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/nutanix-cloud-native/prism-go-client/environment/credentials"
	"github.com/nutanix-cloud-native/prism-go-client/environment/types"

	v1alpha2 "github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/api/v1alpha2"
	pcclient "github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/internal/client"
)

var (
	artifactsFolder      string
	bootstrapClusterName string
	bootstrapKindImage   string
	clusterctlConfigPath string
	configPath           string
	skipResourceCleanup  bool

	bootstrapClusterProvider bootstrap.ClusterProvider
	bootstrapClusterProxy    framework.ClusterProxy
	cfg                      testConfig
	e2eConfig                *capiclusterctl.E2EConfig
)

//nolint:gochecknoinits // Idiomatic pattern for e2e flag registration.
func init() {
	flag.StringVar(&artifactsFolder, "e2e.artifacts-folder", "", "folder for e2e test artifacts")
	flag.StringVar(
		&bootstrapClusterName,
		"e2e.bootstrap-cluster-name",
		"caipamx-e2e",
		"bootstrap kind cluster name prefix",
	)
	flag.StringVar(&bootstrapKindImage, "e2e.bootstrap-kind-image", "", "kind node image")
	flag.StringVar(&configPath, "e2e.config", "", "path to the e2e test config file")
	flag.BoolVar(
		&skipResourceCleanup,
		"e2e.skip-resource-cleanup",
		false,
		"skip e2e resource cleanup",
	)
}

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "caipamx-e2e")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	Expect(configPath).To(BeAnExistingFile(), "e2e.config must be an existing file")
	Expect(os.MkdirAll(artifactsFolder, 0o755)).To(Succeed())

	e2eConfig = loadE2EConfig(configPath)
	cfg = mustLoadConfig(e2eConfig.Variables)
	clusterctlConfigPath = createClusterctlLocalRepository(e2eConfig)
	bootstrapClusterProvider, bootstrapClusterProxy = setupBootstrapCluster(
		initScheme(),
		e2eConfig.ManagementClusterName,
	)
	setupComplete := false
	defer func() {
		if !setupComplete && !skipResourceCleanup {
			tearDown()
		}
	}()
	initBootstrapCluster()

	var configBuf bytes.Buffer
	Expect(gob.NewEncoder(&configBuf).Encode(cfg)).To(Succeed())
	setupComplete = true

	return []byte(strings.Join([]string{
		artifactsFolder,
		base64.StdEncoding.EncodeToString(configBuf.Bytes()),
		bootstrapClusterProxy.GetKubeconfigPath(),
	}, ","))
}, func(data []byte) {
	parts := strings.Split(string(data), ",")
	Expect(parts).To(HaveLen(3))

	artifactsFolder = parts[0]

	configBytes, err := base64.StdEncoding.DecodeString(parts[1])
	Expect(err).NotTo(HaveOccurred())
	Expect(gob.NewDecoder(bytes.NewBuffer(configBytes)).Decode(&cfg)).To(Succeed())

	bootstrapClusterProxy = framework.NewClusterProxy("bootstrap", parts[2], initScheme())
})

var _ = SynchronizedAfterSuite(func() {}, func() {
	if !skipResourceCleanup {
		tearDown()
	}
})

var _ = Describe("CAIPAMX", func() {
	It(
		"reserves and unreserves an IPAddressClaim through Prism Central",
		func(ctx SpecContext) {
			reserveAndUnreserveIPAddressClaim(ctx, &cfg, bootstrapClusterProxy.GetClient())
		},
		SpecTimeout(10*time.Minute),
	)

	It(
		"allocates and releases a FloatingIPClaim through Prism Central",
		func(ctx SpecContext) {
			allocateAndReleaseFloatingIPClaim(ctx, &cfg, bootstrapClusterProxy.GetClient())
		},
		SpecTimeout(10*time.Minute),
	)

	It(
		"associates, unbinds, and rebinds a FloatingIP through Prism Central",
		func(ctx SpecContext) {
			associateUnbindAndRebindFloatingIP(ctx, &cfg, bootstrapClusterProxy.GetClient())
		},
		SpecTimeout(15*time.Minute),
	)
})

func reserveAndUnreserveIPAddressClaim(
	ctx context.Context,
	cfg *testConfig,
	k8sClient ctrlclient.Client,
) {
	namespace := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "caipamx-e2e-",
		},
	}
	if err := k8sClient.Create(ctx, &namespace); err != nil {
		Fail(fmt.Sprintf("create namespace: %v", err))
	}

	secret := credentialsSecret(namespace.Name, cfg)
	pool := nutanixIPPool(namespace.Name, cfg, poolSubnetIdentifier(ctx, cfg, cfg.Subnet))
	claim := ipAddressClaim(namespace.Name, pool.Name)
	var claimUID string
	reservationReleased := false

	DeferCleanup(func(ctx context.Context) {
		cleanupCtx, cleanupCancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cleanupCancel()
		cleanupObject(cleanupCtx, k8sClient, &claim)
		cleanupObject(cleanupCtx, k8sClient, &pool)
		cleanupObject(cleanupCtx, k8sClient, &secret)
		cleanupObject(cleanupCtx, k8sClient, &namespace)
		if !reservationReleased && claimUID != "" {
			cleanupNutanixReservation(cleanupCtx, cfg, claimUID)
		}
	}, NodeTimeout(5*time.Minute))

	if err := k8sClient.Create(ctx, &secret); err != nil {
		Fail(fmt.Sprintf("create credentials secret: %v", err))
	}
	if err := k8sClient.Create(ctx, &pool); err != nil {
		Fail(fmt.Sprintf("create NutanixIPPool: %v", err))
	}
	if err := k8sClient.Create(ctx, &claim); err != nil {
		Fail(fmt.Sprintf("create IPAddressClaim: %v", err))
	}
	claimUID = string(claim.UID)

	address := waitForIPAddress(ctx, k8sClient, namespace.Name, claim.Name)
	GinkgoWriter.Printf(
		"reserved IP %s through IPAddressClaim %s/%s\n",
		address.Spec.Address,
		claim.Namespace,
		claim.Name,
	)

	if err := k8sClient.Delete(ctx, &claim); err != nil {
		Fail(fmt.Sprintf("delete IPAddressClaim: %v", err))
	}
	waitForIPAddressDeleted(ctx, k8sClient, namespace.Name, claim.Name)
	reservationReleased = true
}

type testConfig struct {
	Address           string
	Port              uint16
	Subnet            string
	Cluster           string
	Username          string
	Password          string
	Insecure          bool
	ExternalSubnet    string
	FloatingIPProject string
	PrivateSubnet     string
}

func loadE2EConfig(configPath string) *capiclusterctl.E2EConfig {
	configBytes, err := os.ReadFile(configPath)
	Expect(err).NotTo(HaveOccurred(), "failed to read e2e config from %s", configPath)

	config := &capiclusterctl.E2EConfig{}
	Expect(yaml.Unmarshal(configBytes, config)).To(
		Succeed(),
		"failed to parse e2e config from %s",
		configPath,
	)
	Expect(
		config.ResolveReleases(context.TODO()),
	).To(Succeed(), "failed to resolve e2e config releases")
	config.Defaults()
	config.AbsPaths(filepath.Dir(configPath))
	validateE2EConfig(config)

	return config
}

func validateE2EConfig(config *capiclusterctl.E2EConfig) {
	Expect(config.ManagementClusterName).ToNot(BeEmpty(), "managementClusterName is required")
	Expect(
		config.GetProviderVersions("cluster-api"),
	).ToNot(BeEmpty(), "cluster-api provider is required")
	Expect(config.GetProviderVersions("kubeadm")).ToNot(BeEmpty(), "kubeadm provider is required")
	Expect(
		config.IPAMProviders(),
	).To(ContainElement("nutanix"), "nutanix IPAM provider is required")
}

func createClusterctlLocalRepository(config *capiclusterctl.E2EConfig) string {
	ipamConfig := config.DeepCopy()
	ipamConfig.Providers = nil
	for _, provider := range config.Providers {
		if provider.Type == "IPAMProvider" {
			ipamConfig.Providers = append(ipamConfig.Providers, provider)
		}
	}
	Expect(ipamConfig.Providers).ToNot(BeEmpty(), "no IPAM providers found in e2e config")

	clusterctlConfig := capiclusterctl.CreateRepository(
		context.TODO(),
		capiclusterctl.CreateRepositoryInput{
			E2EConfig:        ipamConfig,
			RepositoryFolder: filepath.Join(artifactsFolder, "repository"),
		},
	)
	Expect(clusterctlConfig).To(BeAnExistingFile(), "failed to create clusterctl local repository")
	return clusterctlConfig
}

func mustLoadConfig(variables map[string]string) testConfig {
	cfg, missing := loadConfig(variables)
	if len(missing) == 0 {
		return cfg
	}

	message := "missing required Nutanix e2e config variables: " + strings.Join(missing, ", ")
	if env("E2E_REQUIRE_NUTANIX_ENV") == "true" {
		Fail(message)
	}
	Skip(message)
	return testConfig{}
}

func loadConfig(variables map[string]string) (cfg testConfig, missing []string) {
	missing = missingConfigVariables(variables)
	if len(missing) > 0 {
		return testConfig{}, missing
	}

	endpoint, err := parseEndpoint(variables["NUTANIX_ENDPOINT"])
	if err != nil {
		Fail(fmt.Sprintf("parse NUTANIX_ENDPOINT: %v", err))
		return testConfig{}, nil
	}

	port := endpoint.Port()
	if port == "" {
		port = variables["NUTANIX_PORT"]
	}
	if port == "" {
		port = "9440"
	}
	parsedPort, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		Fail(fmt.Sprintf("parse NUTANIX_PORT: %v", err))
		return testConfig{}, nil
	}

	privateSubnet := variables["NUTANIX_PRIVATE_SUBNET"]
	if privateSubnet == "" {
		privateSubnet = variables["NUTANIX_SUBNET"]
	}

	return testConfig{
		Address:           endpoint.Hostname(),
		Port:              uint16(parsedPort),
		Subnet:            variables["NUTANIX_SUBNET"],
		Cluster:           variables["NUTANIX_CLUSTER"],
		Username:          variables["NUTANIX_USER"],
		Password:          variables["NUTANIX_PASSWORD"],
		Insecure:          strings.EqualFold(variables["NUTANIX_INSECURE"], "true"),
		ExternalSubnet:    variables["NUTANIX_EXTERNAL_SUBNET"],
		FloatingIPProject: variables["NUTANIX_FLOATING_IP_PROJECT"],
		PrivateSubnet:     privateSubnet,
	}, nil
}

func requireFloatingIPConfig(cfg *testConfig) {
	if cfg.ExternalSubnet != "" {
		return
	}
	message := "missing required floating-IP e2e config variable: NUTANIX_EXTERNAL_SUBNET"
	if env("E2E_REQUIRE_NUTANIX_ENV") == "true" {
		Fail(message)
	}
	Skip(message)
}

func missingConfigVariables(variables map[string]string) []string {
	requiredVariables := []string{
		"NUTANIX_ENDPOINT",
		"NUTANIX_SUBNET",
		"NUTANIX_USER",
		"NUTANIX_PASSWORD",
	}

	var missing []string
	for _, key := range requiredVariables {
		if variables[key] == "" {
			missing = append(missing, key)
		}
	}

	return missing
}

func credentialsSecret(namespace string, cfg *testConfig) corev1.Secret {
	credentialsData, err := json.Marshal([]map[string]any{
		{
			"type": "basic_auth",
			"data": map[string]any{
				"prismCentral": map[string]string{
					"username": cfg.Username,
					"password": cfg.Password,
				},
			},
		},
	})
	if err != nil {
		Fail(fmt.Sprintf("marshal Nutanix credentials: %v", err))
	}

	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pc-creds-for-ipam",
			Namespace: namespace,
		},
		StringData: map[string]string{
			credentials.KeyName: string(credentialsData),
		},
	}
}

func nutanixIPPool(
	namespace string,
	cfg *testConfig,
	subnet v1alpha2.NutanixResourceIdentifier,
) v1alpha2.NutanixIPPool {
	pool := v1alpha2.NutanixIPPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e2e-pool",
			Namespace: namespace,
		},
		Spec: v1alpha2.NutanixIPPoolSpec{
			PrismCentral: v1alpha2.PrismCentral{
				Address: cfg.Address,
				Port:    cfg.Port,
				CredentialsSecretRef: v1alpha2.LocalSecretRef{
					Name: "pc-creds-for-ipam",
				},
				Insecure: cfg.Insecure,
			},
			Subnet: subnet,
		},
	}
	if cfg.Cluster != "" {
		pool.Spec.Cluster = ptr.To(nutanixResourceIdentifier(cfg.Cluster))
	}
	return pool
}

func nutanixResourceIdentifier(nameOrID string) v1alpha2.NutanixResourceIdentifier {
	if _, err := uuid.Parse(nameOrID); err == nil {
		return v1alpha2.NewNutanixResourceIdentifierUUID(nameOrID)
	}
	return v1alpha2.NewNutanixResourceIdentifierName(nameOrID)
}

// poolSubnetIdentifier returns a subnet identifier. Named subnets require a cluster
// for CRD validation; when NUTANIX_CLUSTER is empty (overlay), resolve the name to a UUID.
func poolSubnetIdentifier(
	ctx context.Context,
	cfg *testConfig,
	subnetNameOrID string,
) v1alpha2.NutanixResourceIdentifier {
	id := nutanixResourceIdentifier(subnetNameOrID)
	if id.IsUUID() || cfg.Cluster != "" {
		return id
	}
	subnet, err := newE2EPrismClient(cfg).Networking().GetSubnet(
		ctx,
		subnetNameOrID,
		pcclient.GetSubnetOpts{},
	)
	Expect(err).NotTo(HaveOccurred(), "resolve subnet %q", subnetNameOrID)
	return v1alpha2.NewNutanixResourceIdentifierUUID(subnet.ExtID().String())
}

func ipAddressClaim(namespace, poolName string) ipamv1.IPAddressClaim {
	return ipamv1.IPAddressClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e2e-claim",
			Namespace: namespace,
		},
		Spec: ipamv1.IPAddressClaimSpec{
			PoolRef: ipamv1.IPPoolReference{
				APIGroup: v1alpha2.GroupVersion.Group,
				Kind:     v1alpha2.NutanixIPPoolKind,
				Name:     poolName,
			},
		},
	}
}

func initScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(ipamv1.AddToScheme(scheme))
	utilruntime.Must(v1alpha2.AddToScheme(scheme))
	return scheme
}

func setupBootstrapCluster(
	scheme *runtime.Scheme,
	clusterNamePrefix string,
) (bootstrap.ClusterProvider, framework.ClusterProxy) {
	if clusterNamePrefix == "" {
		clusterNamePrefix = bootstrapClusterName
	}
	clusterName := uniqueBootstrapClusterName(clusterNamePrefix)
	clusterProvider := bootstrap.CreateKindBootstrapClusterAndLoadImages(
		context.TODO(),
		bootstrap.CreateKindBootstrapClusterAndLoadImagesInput{
			Name:            clusterName,
			CustomNodeImage: bootstrapKindImage,
			LogFolder:       filepath.Join(artifactsFolder, "kind"),
		},
	)
	Expect(clusterProvider).ToNot(BeNil(), "Failed to create a bootstrap cluster")

	clusterProxy := framework.NewClusterProxy(
		"bootstrap",
		clusterProvider.GetKubeconfigPath(),
		scheme,
	)
	Expect(clusterProxy).ToNot(BeNil(), "Failed to get a bootstrap cluster proxy")
	return clusterProvider, clusterProxy
}

func uniqueBootstrapClusterName(prefix string) string {
	const maxKindClusterNameLength = 45
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	if len(suffix) > 8 {
		suffix = suffix[len(suffix)-8:]
	}
	maxPrefixLength := maxKindClusterNameLength - len(suffix) - 1
	if len(prefix) > maxPrefixLength {
		prefix = strings.TrimRight(prefix[:maxPrefixLength], "-")
	}
	return fmt.Sprintf("%s-%s", prefix, suffix)
}

func initBootstrapCluster() {
	const providerContract = "v1beta2"
	capiProviders := e2eConfig.GetProviderLatestVersionsByContract(providerContract, "cluster-api")
	kubeadmProviders := e2eConfig.GetProviderLatestVersionsByContract(providerContract, "kubeadm")
	ipamProviders := e2eConfig.GetProviderLatestVersionsByContract(
		providerContract,
		e2eConfig.IPAMProviders()...)
	Expect(capiProviders).To(HaveLen(1), "expected exactly one cluster-api provider")
	Expect(kubeadmProviders).To(HaveLen(1), "expected exactly one kubeadm provider")
	Expect(
		ipamProviders,
	).To(ContainElement(HavePrefix("nutanix:")), "expected nutanix IPAM provider")

	capiclusterctl.Init(
		context.TODO(),
		capiclusterctl.InitInput{
			ClusterctlConfigPath:  clusterctlConfigPath,
			KubeconfigPath:        bootstrapClusterProxy.GetKubeconfigPath(),
			CoreProvider:          capiProviders[0],
			BootstrapProviders:    kubeadmProviders,
			ControlPlaneProviders: kubeadmProviders,
			IPAMProviders:         ipamProviders,
			LogFolder: filepath.Join(
				artifactsFolder,
				"clusters",
				bootstrapClusterProxy.GetName(),
			),
		},
	)
}

func tearDown() {
	if bootstrapClusterProxy != nil {
		bootstrapClusterProxy.Dispose(context.TODO())
	}
	if bootstrapClusterProvider != nil {
		bootstrapClusterProvider.Dispose(context.TODO())
	}
}

func cleanupObject(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	obj ctrlclient.Object,
) {
	if err := k8sClient.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
		GinkgoWriter.Printf(
			"cleanup %T %s/%s failed: %v\n",
			obj,
			obj.GetNamespace(),
			obj.GetName(),
			err,
		)
	}
}

func cleanupNutanixReservation(
	ctx context.Context,
	cfg *testConfig,
	clientContext string,
) {
	cleanupNutanixReservationForSubnet(ctx, cfg, cfg.Subnet, clientContext)
}

func cleanupNutanixReservationForSubnet(
	ctx context.Context,
	cfg *testConfig,
	subnet string,
	clientContext string,
) {
	endpoint := &url.URL{
		Scheme: "https",
		Host:   net.JoinHostPort(cfg.Address, strconv.Itoa(int(cfg.Port))),
	}
	nutanixClient, err := pcclient.GetClient(&e2eClientParams{
		endpoint: endpoint,
		username: cfg.Username,
		password: cfg.Password,
		insecure: cfg.Insecure,
	})
	if err != nil {
		GinkgoWriter.Printf(
			"cleanup reservation for client context %q failed to create client: %v\n",
			clientContext,
			err,
		)
		return
	}

	// UnreserveIPs blocks until the underlying Prism task completes and is
	// idempotent if the reservation was already released; bound the wait.
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	if _, err := nutanixClient.Networking().UnreserveIPs(
		ctx,
		pcclient.UnreserveIPClientContext(clientContext),
		subnet,
		pcclient.UnreserveIPOpts{
			Cluster: cfg.Cluster,
		},
	); err != nil {
		GinkgoWriter.Printf(
			"cleanup reservation for client context %q failed: %v\n",
			clientContext,
			err,
		)
	}
}

type e2eClientParams struct {
	endpoint *url.URL
	username string
	password string
	insecure bool
}

func (c *e2eClientParams) ManagementEndpoint() types.ManagementEndpoint {
	return types.ManagementEndpoint{
		Address: c.endpoint,
		ApiCredentials: types.ApiCredentials{
			Username: c.username,
			Password: c.password,
		},
		Insecure: c.insecure,
	}
}

func (c *e2eClientParams) Key() string {
	return c.endpoint.String()
}

func waitForIPAddress(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	namespace,
	name string,
) ipamv1.IPAddress {
	var address ipamv1.IPAddress
	if err := wait.PollUntilContextTimeout(ctx, time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		err := k8sClient.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: name}, &address)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		if address.Spec.Address == "" {
			return false, nil
		}

		return true, nil
	}); err != nil {
		Fail(fmt.Sprintf("wait for IPAddress %s/%s: %v", namespace, name, err))
	}

	return address
}

func waitForIPAddressDeleted(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	namespace,
	name string,
) {
	if err := wait.PollUntilContextTimeout(ctx, time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		var address ipamv1.IPAddress
		err := k8sClient.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: name}, &address)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}

		return false, nil
	}); err != nil {
		Fail(fmt.Sprintf("wait for IPAddress %s/%s to be deleted: %v", namespace, name, err))
	}
}

func allocateAndReleaseFloatingIPClaim(
	ctx context.Context,
	cfg *testConfig,
	k8sClient ctrlclient.Client,
) {
	requireFloatingIPConfig(cfg)

	namespace := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "caipamx-fip-e2e-",
		},
	}
	Expect(k8sClient.Create(ctx, &namespace)).To(Succeed())

	secret := credentialsSecret(namespace.Name, cfg)
	pool := nutanixFloatingIPPool(
		namespace.Name,
		cfg,
		poolSubnetIdentifier(ctx, cfg, cfg.ExternalSubnet),
	)
	claim := floatingIPClaim(namespace.Name, pool.Name)
	var allocatedExtID string
	released := false

	DeferCleanup(func(ctx context.Context) {
		cleanupCtx, cleanupCancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cleanupCancel()
		logFloatingIPResources(cleanupCtx, k8sClient, namespace.Name, "cleanup")
		cleanupObject(cleanupCtx, k8sClient, &claim)
		cleanupObject(cleanupCtx, k8sClient, &pool)
		cleanupObject(cleanupCtx, k8sClient, &secret)
		cleanupObject(cleanupCtx, k8sClient, &namespace)
		if !released && allocatedExtID != "" {
			cleanupPrismFloatingIP(cleanupCtx, cfg, allocatedExtID)
		}
	}, NodeTimeout(5*time.Minute))

	Expect(k8sClient.Create(ctx, &secret)).To(Succeed())
	Expect(k8sClient.Create(ctx, &pool)).To(Succeed())
	Expect(k8sClient.Create(ctx, &claim)).To(Succeed())
	logFloatingIPResources(ctx, k8sClient, namespace.Name, "created")

	fip := waitForFloatingIPAllocated(ctx, k8sClient, namespace.Name, claim.Name)
	allocatedExtID = fip.Spec.ExtID
	GinkgoWriter.Printf(
		"allocated floating IP %s (%s) through FloatingIPClaim %s/%s\n",
		fip.Spec.Address,
		fip.Spec.ExtID,
		claim.Namespace,
		claim.Name,
	)
	logFloatingIPResources(ctx, k8sClient, namespace.Name, "allocated")

	prismFIP := getPrismFloatingIP(ctx, cfg, fip.Spec.ExtID)
	Expect(prismFIP.Address).To(Equal(fip.Spec.Address))

	Expect(k8sClient.Delete(ctx, &claim)).To(Succeed())
	waitForFloatingIPDeleted(ctx, k8sClient, namespace.Name, claim.Name)
	Eventually(func(g Gomega) {
		_, err := newE2EPrismClient(cfg).FloatingIPs().Get(ctx, allocatedExtID)
		g.Expect(err).To(HaveOccurred())
		g.Expect(strings.Contains(strings.ToLower(err.Error()), "not found") ||
			strings.Contains(err.Error(), "ENTITY_NOT_FOUND") ||
			strings.Contains(
				err.Error(),
				"does not exist",
			)).To(BeTrue(), "unexpected get error: %v", err)
	}).WithTimeout(2 * time.Minute).WithPolling(time.Second).Should(Succeed())
	released = true
}

func associateUnbindAndRebindFloatingIP(
	ctx context.Context,
	cfg *testConfig,
	k8sClient ctrlclient.Client,
) {
	requireFloatingIPConfig(cfg)

	namespace := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "caipamx-fip-bind-e2e-",
		},
	}
	Expect(k8sClient.Create(ctx, &namespace)).To(Succeed())

	secret := credentialsSecret(namespace.Name, cfg)
	pool := nutanixFloatingIPPool(
		namespace.Name,
		cfg,
		poolSubnetIdentifier(ctx, cfg, cfg.ExternalSubnet),
	)
	claim := floatingIPClaim(namespace.Name, pool.Name)
	clientContext := fmt.Sprintf("caipamx-e2e-fip-%d", time.Now().UnixNano())
	var allocatedExtID string
	privateReserved := false
	var privateIP string
	released := false

	DeferCleanup(func(ctx context.Context) {
		cleanupCtx, cleanupCancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cleanupCancel()
		logFloatingIPResources(cleanupCtx, k8sClient, namespace.Name, "cleanup")
		cleanupObject(cleanupCtx, k8sClient, &claim)
		cleanupObject(cleanupCtx, k8sClient, &pool)
		cleanupObject(cleanupCtx, k8sClient, &secret)
		cleanupObject(cleanupCtx, k8sClient, &namespace)
		if privateReserved {
			cleanupNutanixReservationForSubnet(cleanupCtx, cfg, cfg.PrivateSubnet, clientContext)
		}
		if !released && allocatedExtID != "" {
			cleanupPrismFloatingIP(cleanupCtx, cfg, allocatedExtID)
		}
	}, NodeTimeout(5*time.Minute))

	prismClient := newE2EPrismClient(cfg)
	privateSubnet, err := prismClient.Networking().GetSubnet(
		ctx,
		cfg.PrivateSubnet,
		pcclient.GetSubnetOpts{Cluster: cfg.Cluster},
	)
	Expect(err).NotTo(HaveOccurred())
	if privateSubnet.VPCExtID() == "" {
		Skip(
			fmt.Sprintf(
				"private subnet %q has no VPC reference; skipping floating-IP association",
				cfg.PrivateSubnet,
			),
		)
	}

	reserved, err := prismClient.Networking().ReserveIPs(
		ctx,
		pcclient.ReserveIPCountFunc(1),
		privateSubnet.ExtID().String(),
		pcclient.ReserveIPOpts{
			ClientContext: clientContext,
		},
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(reserved).To(HaveLen(1))
	privateReserved = true
	privateIP = reserved[0].String()

	Expect(k8sClient.Create(ctx, &secret)).To(Succeed())
	Expect(k8sClient.Create(ctx, &pool)).To(Succeed())
	Expect(k8sClient.Create(ctx, &claim)).To(Succeed())
	logFloatingIPResources(ctx, k8sClient, namespace.Name, "created")

	fip := waitForFloatingIPAllocated(ctx, k8sClient, namespace.Name, claim.Name)
	allocatedExtID = fip.Spec.ExtID
	logFloatingIPResources(ctx, k8sClient, namespace.Name, "allocated")

	binding1 := floatingIPBinding(
		namespace.Name,
		"bind-1",
		claim.Name,
		privateIP,
		privateSubnet.VPCExtID(),
	)
	Expect(k8sClient.Create(ctx, &binding1)).To(Succeed())
	waitForFloatingIPAssociation(ctx, k8sClient, namespace.Name, claim.Name, privateIP)
	logFloatingIPResources(ctx, k8sClient, namespace.Name, "associated")
	prismFIP := getPrismFloatingIP(ctx, cfg, allocatedExtID)
	Expect(prismFIP.AssociationType).To(Equal(pcclient.FloatingIPAssociationPrivateIP))
	Expect(prismFIP.PrivateIPAddress).To(Equal(privateIP))

	Expect(k8sClient.Delete(ctx, &binding1)).To(Succeed())
	waitForFloatingIPReservationCleared(ctx, k8sClient, namespace.Name, claim.Name)
	logFloatingIPResources(ctx, k8sClient, namespace.Name, "unbound")
	Eventually(func(g Gomega) {
		current := getPrismFloatingIP(ctx, cfg, allocatedExtID)
		g.Expect(current.AssociationType).To(Equal(pcclient.FloatingIPAssociationNone))
		g.Expect(current.ExtID).To(Equal(allocatedExtID))
	}).WithTimeout(2 * time.Minute).WithPolling(time.Second).Should(Succeed())

	binding2 := floatingIPBinding(
		namespace.Name,
		"bind-2",
		claim.Name,
		privateIP,
		privateSubnet.VPCExtID(),
	)
	Expect(k8sClient.Create(ctx, &binding2)).To(Succeed())
	waitForFloatingIPAssociation(ctx, k8sClient, namespace.Name, claim.Name, privateIP)
	logFloatingIPResources(ctx, k8sClient, namespace.Name, "rebound")
	prismFIP = getPrismFloatingIP(ctx, cfg, allocatedExtID)
	Expect(prismFIP.AssociationType).To(Equal(pcclient.FloatingIPAssociationPrivateIP))
	Expect(prismFIP.PrivateIPAddress).To(Equal(privateIP))

	Expect(k8sClient.Delete(ctx, &binding2)).To(Succeed())
	waitForFloatingIPReservationCleared(ctx, k8sClient, namespace.Name, claim.Name)
	logFloatingIPResources(ctx, k8sClient, namespace.Name, "unbound-again")
	Expect(k8sClient.Delete(ctx, &claim)).To(Succeed())
	waitForFloatingIPDeleted(ctx, k8sClient, namespace.Name, claim.Name)
	Eventually(func(g Gomega) {
		_, err := newE2EPrismClient(cfg).FloatingIPs().Get(ctx, allocatedExtID)
		g.Expect(err).To(HaveOccurred())
	}).WithTimeout(2 * time.Minute).WithPolling(time.Second).Should(Succeed())
	released = true
}

func nutanixFloatingIPPool(
	namespace string,
	cfg *testConfig,
	subnet v1alpha2.NutanixResourceIdentifier,
) v1alpha2.NutanixFloatingIPPool {
	pool := v1alpha2.NutanixFloatingIPPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e2e-fip-pool",
			Namespace: namespace,
		},
		Spec: v1alpha2.NutanixFloatingIPPoolSpec{
			PrismCentral: v1alpha2.PrismCentral{
				Address: cfg.Address,
				Port:    cfg.Port,
				CredentialsSecretRef: v1alpha2.LocalSecretRef{
					Name: "pc-creds-for-ipam",
				},
				Insecure: cfg.Insecure,
			},
			Subnet: subnet,
		},
	}
	if cfg.Cluster != "" {
		pool.Spec.Cluster = ptr.To(nutanixResourceIdentifier(cfg.Cluster))
	}
	if cfg.FloatingIPProject != "" {
		pool.Spec.Project = ptr.To(nutanixResourceIdentifier(cfg.FloatingIPProject))
	}
	return pool
}

func floatingIPClaim(namespace, poolName string) v1alpha2.FloatingIPClaim {
	return v1alpha2.FloatingIPClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e2e-fip-claim",
			Namespace: namespace,
		},
		Spec: v1alpha2.FloatingIPClaimSpec{
			PoolRef: v1alpha2.FloatingIPPoolReference{
				Name: poolName,
			},
		},
	}
}

func floatingIPBinding(
	namespace, name, floatingIPName, address, vpcExtID string,
) v1alpha2.FloatingIPBinding {
	return v1alpha2.FloatingIPBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha2.FloatingIPBindingSpec{
			FloatingIPRef: v1alpha2.LocalObjectReference{Name: floatingIPName},
			PrivateIP: &v1alpha2.FloatingIPPrivateIPAssociation{
				Address: address,
				VPC:     nutanixResourceIdentifier(vpcExtID),
			},
		},
	}
}

func newE2EPrismClient(cfg *testConfig) pcclient.Client {
	endpoint := &url.URL{
		Scheme: "https",
		Host:   net.JoinHostPort(cfg.Address, strconv.Itoa(int(cfg.Port))),
	}
	nutanixClient, err := pcclient.GetClient(&e2eClientParams{
		endpoint: endpoint,
		username: cfg.Username,
		password: cfg.Password,
		insecure: cfg.Insecure,
	})
	Expect(err).NotTo(HaveOccurred())
	return nutanixClient
}

func getPrismFloatingIP(ctx context.Context, cfg *testConfig, extID string) *pcclient.FloatingIP {
	fip, err := newE2EPrismClient(cfg).FloatingIPs().Get(ctx, extID)
	Expect(err).NotTo(HaveOccurred())
	Expect(fip).NotTo(BeNil())
	return fip
}

func cleanupPrismFloatingIP(ctx context.Context, cfg *testConfig, extID string) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := newE2EPrismClient(cfg).FloatingIPs().Delete(ctx, extID); err != nil {
		GinkgoWriter.Printf("cleanup floating IP %q failed: %v\n", extID, err)
	}
}

func waitForFloatingIPAllocated(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	namespace,
	name string,
) v1alpha2.FloatingIP {
	var fip v1alpha2.FloatingIP
	if err := wait.PollUntilContextTimeout(ctx, time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		err := k8sClient.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: name}, &fip)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return fip.Spec.ExtID != "" && fip.Spec.Address != "", nil
	}); err != nil {
		logFloatingIPResources(ctx, k8sClient, namespace, "wait-allocated-failed")
		Fail(fmt.Sprintf("wait for FloatingIP %s/%s: %v", namespace, name, err))
	}
	return fip
}

func waitForFloatingIPDeleted(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	namespace,
	name string,
) {
	if err := wait.PollUntilContextTimeout(ctx, time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		var fip v1alpha2.FloatingIP
		err := k8sClient.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: name}, &fip)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	}); err != nil {
		logFloatingIPResources(ctx, k8sClient, namespace, "wait-deleted-failed")
		Fail(fmt.Sprintf("wait for FloatingIP %s/%s to be deleted: %v", namespace, name, err))
	}
}

func waitForFloatingIPAssociation(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	namespace,
	name,
	privateIP string,
) {
	if err := wait.PollUntilContextTimeout(ctx, time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		var fip v1alpha2.FloatingIP
		err := k8sClient.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: name}, &fip)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		if fip.Status.Association == nil {
			return false, nil
		}
		if fip.Status.Association.Type != v1alpha2.FloatingIPAssociationPrivateIP {
			return false, nil
		}
		if fip.Status.Association.PrivateIP == nil {
			return false, nil
		}
		return fip.Status.Association.PrivateIP.Address == privateIP, nil
	}); err != nil {
		logFloatingIPResources(ctx, k8sClient, namespace, "wait-association-failed")
		Fail(fmt.Sprintf("wait for FloatingIP %s/%s association: %v", namespace, name, err))
	}
}

func waitForFloatingIPReservationCleared(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	namespace,
	name string,
) {
	if err := wait.PollUntilContextTimeout(ctx, time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		var fip v1alpha2.FloatingIP
		err := k8sClient.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: name}, &fip)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return fip.Status.Reservation == nil, nil
	}); err != nil {
		logFloatingIPResources(ctx, k8sClient, namespace, "wait-reservation-clear-failed")
		Fail(fmt.Sprintf("wait for FloatingIP %s/%s reservation clear: %v", namespace, name, err))
	}
}

func logFloatingIPResources(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	namespace, step string,
) {
	body := renderFloatingIPResources(ctx, k8sClient, namespace)
	// GinkgoWriter is omitted from passing specs. Stdout is what the e2e log shows.
	fmt.Fprintf(os.Stdout, "floating IP resources [%s] namespace=%s\n%s", step, namespace, body)
	GinkgoWriter.Printf("floating IP resources [%s] namespace=%s\n%s", step, namespace, body)
	if artifactsFolder == "" {
		return
	}

	dir := filepath.Join(artifactsFolder, "floating-ip", namespace)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		GinkgoWriter.Printf("create floating IP resource log dir: %v\n", err)
		return
	}
	path := filepath.Join(dir, step+".yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		GinkgoWriter.Printf("write floating IP resource log %s: %v\n", path, err)
	}
}

func renderFloatingIPResources(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	namespace string,
) string {
	var buf strings.Builder
	writeFloatingIPList(&buf, "NutanixFloatingIPPool", func() ([]ctrlclient.Object, error) {
		list := &v1alpha2.NutanixFloatingIPPoolList{}
		if err := k8sClient.List(ctx, list, ctrlclient.InNamespace(namespace)); err != nil {
			return nil, err
		}
		objects := make([]ctrlclient.Object, len(list.Items))
		for i := range list.Items {
			objects[i] = &list.Items[i]
		}
		return objects, nil
	})
	writeFloatingIPList(&buf, "FloatingIPClaim", func() ([]ctrlclient.Object, error) {
		list := &v1alpha2.FloatingIPClaimList{}
		if err := k8sClient.List(ctx, list, ctrlclient.InNamespace(namespace)); err != nil {
			return nil, err
		}
		objects := make([]ctrlclient.Object, len(list.Items))
		for i := range list.Items {
			objects[i] = &list.Items[i]
		}
		return objects, nil
	})
	writeFloatingIPList(&buf, "FloatingIP", func() ([]ctrlclient.Object, error) {
		list := &v1alpha2.FloatingIPList{}
		if err := k8sClient.List(ctx, list, ctrlclient.InNamespace(namespace)); err != nil {
			return nil, err
		}
		objects := make([]ctrlclient.Object, len(list.Items))
		for i := range list.Items {
			objects[i] = &list.Items[i]
		}
		return objects, nil
	})
	writeFloatingIPList(&buf, "FloatingIPBinding", func() ([]ctrlclient.Object, error) {
		list := &v1alpha2.FloatingIPBindingList{}
		if err := k8sClient.List(ctx, list, ctrlclient.InNamespace(namespace)); err != nil {
			return nil, err
		}
		objects := make([]ctrlclient.Object, len(list.Items))
		for i := range list.Items {
			objects[i] = &list.Items[i]
		}
		return objects, nil
	})
	return buf.String()
}

func writeFloatingIPList(
	buf *strings.Builder,
	kind string,
	list func() ([]ctrlclient.Object, error),
) {
	objects, err := list()
	if err != nil {
		fmt.Fprintf(buf, "# list %s: %v\n", kind, err)
		return
	}
	if len(objects) == 0 {
		fmt.Fprintf(buf, "# no %s\n", kind)
		return
	}
	for _, obj := range objects {
		obj.SetManagedFields(nil)
		raw, err := yaml.Marshal(obj)
		if err != nil {
			fmt.Fprintf(buf, "# marshal %s %s: %v\n", kind, obj.GetName(), err)
			continue
		}
		buf.WriteString("---\n")
		buf.Write(raw)
		if len(raw) == 0 || raw[len(raw)-1] != '\n' {
			buf.WriteByte('\n')
		}
	}
}

func parseEndpoint(rawEndpoint string) (*url.URL, error) {
	endpoint := strings.TrimSpace(rawEndpoint)
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}

	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	if parsedEndpoint.Host == "" {
		return nil, fmt.Errorf("endpoint must include a host")
	}

	return parsedEndpoint, nil
}

func env(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}
