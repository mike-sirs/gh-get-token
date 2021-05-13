package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
)

var (
	pem       = flag.String("k", "", "Path to github app private key file.")
	installID = flag.Uint("i", 0, "Github app installation ID.")
	appID     = flag.String("a", "", "Github app ID.")
	ttl       = flag.Int64("t", 600, "Key expiration time in seconds.")
)

func errChk(e error) {
	if e != nil {
		log.Fatal(e)
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

func main() {
	flag.Parse()

	tkn, err := getInstToken(*pem, *appID, *ttl)
	errChk(err)
	fmt.Println(getAccToken(*installID, tkn)["token"])
}
