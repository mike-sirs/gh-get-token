# github token getter

A small application written in Golang built to ease the process of GitHub token retrieval

Prerequisite:
- Create GitHub application, download private PEM key
- Install GitHub application to your organization
- Pass the following data to the getter
- application id, installation id, location of the private key, name of k8s secret where it will save and update the access token

E.G: 
```bash
gha_get_token -a 12345 -i 123123123 -k /tmp/gha/private-key.pem -n default -s gha-test-token
```

Flags:
- "-a", default: ""  - "Github app ID."
- "-i", default: 0 -  "Github app installation ID."
- "-k", default: ""  - "Path to github app private key file."
- "-n", default: "default" - "K8S secret namespace."
- "-s", default: "" - "K8S secret name."
- "-t", default: 600 -  "Key expiration time in seconds."
- "-u", default: "token", "K8S token user name.")
