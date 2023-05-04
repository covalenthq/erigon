package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/compute/metadata"
	credentials "cloud.google.com/go/iam/credentials/apiv1"
	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
	credentialspb "google.golang.org/genproto/googleapis/iam/credentials/v1"
)

func signObject(obj *storage.ObjectHandle) (signedObjURLs []string, err error) {
	var signedObjURL string
	signedObjURL, err = publisherSignURL(obj.BucketName(), obj.ObjectName())
	if err != nil {
		return
	}

	return []string{signedObjURL}, err
}

func publishFileToObject(ctx context.Context, traceLogFile *os.File, obj *storage.ObjectHandle) (signedObjURLs []string, err error) {
	_, err = traceLogFile.Seek(0, 0)
	if err != nil {
		return
	}

	writer := obj.NewWriter(ctx)

	_, err = io.Copy(writer, traceLogFile)
	if err != nil {
		return
	}

	err = writer.Close()
	if err != nil {
		return
	}

	var signedObjURL string
	signedObjURL, err = publisherSignURL(obj.BucketName(), obj.ObjectName())
	if err != nil {
		return
	}

	err = os.Remove(traceLogFile.Name())
	if err != nil {
		return
	}

	return []string{signedObjURL}, err
}

func publisherGetServiceAccount() (found_acct string, err error) {
	var service_account_lns string
	service_account_lns, err = metadata.Get("instance/service-accounts/")
	if err != nil {
		return
	}

	service_accounts := strings.Split(service_account_lns, "\n")

	for _, acct := range service_accounts {
		acct = strings.Trim(acct, "/")
		if len(acct) > 0 && acct != "default" {
			found_acct = acct
			return
		}
	}

	err = fmt.Errorf("no non-default service account found")
	return
}

func publisherSignBlobWithServiceAccount(svcAccount string, blob []byte) ([]byte, error) {
	auth_scopes, err := metadata.Scopes("")
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	cred, err := credentials.NewIamCredentialsClient(ctx, option.WithScopes(auth_scopes...))
	if err != nil {
		return nil, err
	}
	defer cred.Close()

	svcAcctResourcePath := fmt.Sprintf("projects/-/serviceAccounts/%s", svcAccount)

	resp, err := cred.SignBlob(ctx, &credentialspb.SignBlobRequest{
		Name:      svcAcctResourcePath,
		Delegates: []string{svcAcctResourcePath},
		Payload:   blob,
	})
	if err != nil {
		return nil, err
	}

	return resp.SignedBlob, nil
}

func publisherSignURL(bucketName, objectKeyPath string) (string, error) {
	acct, err := publisherGetServiceAccount()
	if err != nil {
		return acct, err
	}

	return storage.SignedURL(bucketName, objectKeyPath, &storage.SignedURLOptions{
		GoogleAccessID: acct,
		SignBytes: func(input []byte) ([]byte, error) {
			return publisherSignBlobWithServiceAccount(acct, input)
		},
		Method:  "GET",
		Expires: time.Now().Add(48 * time.Hour),
	})
}
