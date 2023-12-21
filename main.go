package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/pelletier/go-toml/v2"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	coreV1Types "k8s.io/client-go/kubernetes/typed/core/v1"
)

var (
	configPath    = flag.String("c", "/etc/gh_get_token.conf", "Token getter config path, default is '/etc/gh_token_getter.conf'.")
	secretsClient coreV1Types.SecretInterface
)

type githubApp struct {
	AppID      string `toml:"app_id"`
	InstallID  string `toml:"install_id"`
	AppPemPath string `toml:"app_pem_path"`
}

type k8sSecret struct {
	Name           string            `toml:"name"`
	Namespace      string            `toml:"namespace"`
	DockerConfAnno map[string]string `toml:"dockerconf_anno,omitempty"`
	BasicaAuthAnno map[string]string `toml:"basicauth_anno,omitempty"`
}

type config struct {
	GithubApp githubApp `toml:"github_app"`
	K8sSecret k8sSecret `toml:"k8s_secret"`
}

func loadConfig(path string) (config, error) {
	var cfg config

	content, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("error reading config file: %w", err)
	}

	if err := toml.Unmarshal(content, &cfg); err != nil {
		return cfg, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return cfg, nil
}

// Get github installation token
func getInstallationToken(pemPath, appID string) (string, error) {
	pem, err := os.ReadFile(pemPath)
	if err != nil {
		return "", fmt.Errorf("error reading pem file: %w", err)
	}

	pk, err := jwt.ParseRSAPrivateKeyFromPEM(pem)
	if err != nil {
		return "", fmt.Errorf("error parsing RSA private key: %w", err)
	}

	claims := jwt.RegisteredClaims{
		Issuer:    appID,
		IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Second * 10)),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Second * 300)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signedToken, err := token.SignedString(pk)
	if err != nil {
		return "", fmt.Errorf("error signing token: %w", err)
	}

	return signedToken, nil
}

// id is application installation id, t is a token
func getAccessToken(installID, token string) (string, error) {
	ghAPI := fmt.Sprintf("https://api.github.com/app/installations/%s/access_tokens", installID)
	req, err := http.NewRequest("POST", ghAPI, nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Add("Accept", "application/vnd.github.v3+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error performing request: %w", err)
	}
	defer resp.Body.Close()

	var gat map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&gat); err != nil {
		return "", fmt.Errorf("error decoding response: %w", err)
	}

	accessToken, ok := gat["token"].(string)
	if !ok {
		return "", fmt.Errorf("token is not a string")
	}

	return accessToken, nil
}

func initK8SClient(ns string) {
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Error getting in-cluster config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating kubernetes client: %v", err)
	}

	secretsClient = clientset.CoreV1().Secrets(ns)
}

func handleSecrets(ctx context.Context, token string, ks k8sSecret) {
	if _, err := manageSecret(ctx, ks.Name+"-opaque", ks.Namespace, "Opaque", map[string]string{"token": token}, ks.BasicaAuthAnno); err != nil {
		log.Printf("Error managing opaque secret: %v", err)
	}

	if _, err := manageSecret(ctx, ks.Name, ks.Namespace, "kubernetes.io/basic-auth", map[string]string{"username": "token", "password": token}, ks.BasicaAuthAnno); err != nil {
		log.Printf("Error managing basic auth secret: %v", err)
	}

	dockerConfigJSON := fmt.Sprintf(`{"auths":{"ghcr.io":{"auth": "token:%s"}}}`, token)
	if _, err := manageSecret(ctx, ks.Name+"-ghcr", ks.Namespace, "kubernetes.io/dockerconfigjson", map[string]string{".dockerconfigjson": dockerConfigJSON}, ks.DockerConfAnno); err != nil {
		log.Printf("Error managing GHCR secret: %v", err)
	}
}

func manageSecret(ctx context.Context, name, namespace, secretType string, stringData, annotations map[string]string) (*coreV1.Secret, error) {
	_, err := secretsClient.Get(ctx, name, metaV1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("error getting secret: %w", err)
	}

	newSecret := &coreV1.Secret{
		ObjectMeta: metaV1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
		StringData: stringData,
		Type:       coreV1.SecretType(secretType),
	}

	if errors.IsNotFound(err) {
		return secretsClient.Create(ctx, newSecret, metaV1.CreateOptions{})
	}

	return secretsClient.Update(ctx, newSecret, metaV1.UpdateOptions{})
}

func main() {
	flag.Parse()
	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	initK8SClient(cfg.K8sSecret.Namespace)
	ctx, cancelFn := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancelFn()

	iToken, err := getInstallationToken(cfg.GithubApp.AppPemPath, cfg.GithubApp.AppID)
	if err != nil {
		log.Fatalf("Failed to get installation token: %v", err)
	}

	aToken, err := getAccessToken(cfg.GithubApp.InstallID, iToken)
	if err != nil {
		log.Fatalf("Failed to get access token: %v", err)
	}

	handleSecrets(ctx, aToken, cfg.K8sSecret)
}
