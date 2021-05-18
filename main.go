package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	coreV1Types "k8s.io/client-go/kubernetes/typed/core/v1"
)

var (
	appID      = flag.String("a", "", "Github app ID.")
	installID  = flag.Uint("i", 0, "Github app installation ID.")
	pem        = flag.String("k", "", "Path to github app private key file.")
	namespace  = flag.String("n", "default", "K8S secret namespace.")
	secretname = flag.String("s", "", "K8S secret name.")
	ttl        = flag.Int64("t", 600, "Key expiration time in seconds.")

	secretsClient coreV1Types.SecretInterface
)

func errChk(e error) {
	if e != nil {
		panic(e.Error())
	}
}

func getInstToken(f string, iss string, exp int64) (signedToken string, err error) {
	pem, err := os.ReadFile(f)
	errChk(err)

	pk, err := jwt.ParseRSAPrivateKeyFromPEM(pem)
	errChk(err)

	claims := jwt.StandardClaims{
		//iss: GitHub App's identifier
		Issuer:    iss,
		IssuedAt:  time.Now().Unix() - 60,
		ExpiresAt: time.Now().Unix() + exp,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signedToken, err = token.SignedString(pk)
	errChk(err)

	return
}

// id is application installation id, t is a token
func getAccToken(id uint, t string) map[string]interface{} {
	var gat map[string]interface{}

	ghApi := fmt.Sprintf("https://api.github.com/app/installations/%d/access_tokens", id)
	req, err := http.NewRequest("POST", ghApi, nil)
	errChk(err)
	req.Header.Add("Authorization", "Bearer "+t)
	req.Header.Add("Accept", "application/vnd.github.v3+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	errChk(err)
	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(&gat)
	errChk(err)
	return gat
}

func initK8SClient(n string) {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	errChk(err)
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	errChk(err)

	secretsClient = clientset.CoreV1().Secrets(n)
}

func readSecret(sc coreV1Types.SecretInterface, n string) *coreV1.Secret {
	secret, err := sc.Get(context.TODO(), n, metaV1.GetOptions{})
	if err != nil {
		if err.Error() == fmt.Sprintf("secrets \"%s\" not found", n) {
			fmt.Printf("Secret %s not found, creating a new one.", n)
		} else {
			panic(err.Error())
		}
	}

	return secret
}

func updateSecret(sc coreV1Types.SecretInterface, t, n, s string) {
	secret := coreV1.Secret{
		TypeMeta: metaV1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metaV1.ObjectMeta{
			Name:      s,
			Namespace: n,
		},
		StringData: map[string]string{
			"token": t,
		},
		Type: "Opaque",
	}

	_, err := sc.Update(context.TODO(), &secret, metaV1.UpdateOptions{FieldManager: "tokenGetter"})
	errChk(err)
}

func createSecret(sc coreV1Types.SecretInterface, t, n, s string) {
	secret := coreV1.Secret{
		TypeMeta: metaV1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metaV1.ObjectMeta{
			Name:      s,
			Namespace: n,
		},
		StringData: map[string]string{
			"token": t,
		},
		Type: "Opaque",
	}

	_, err := sc.Create(context.TODO(), &secret, metaV1.CreateOptions{FieldManager: "tokenGetter"})
	errChk(err)
}

func main() {
	flag.Parse()
	// Init k8s in-cluster client
	initK8SClient(*namespace)

	//Get github install token
	iTkn, err := getInstToken(*pem, *appID, *ttl)
	errChk(err)
	//Get github access token
	switch aTkn := getAccToken(*installID, iTkn)["token"].(type) {
	case string:
		if readSecret(secretsClient, *secretname).Data != nil {
			updateSecret(secretsClient, aTkn, *namespace, *secretname)
			fmt.Printf("Tokent was updated at %v", time.Now())
			return
		}
		createSecret(secretsClient, aTkn, *namespace, *secretname)
	default:
		fmt.Printf("Token expects to be a string type but received %T!\n", aTkn)
	}
}
