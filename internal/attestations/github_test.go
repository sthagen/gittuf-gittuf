// Copyright The gittuf Authors
// SPDX-License-Identifier: Apache-2.0

package attestations

import (
	"encoding/base64"
	"fmt"
	"path"
	"testing"

	"github.com/gittuf/gittuf/internal/gitinterface"
	"github.com/gittuf/gittuf/internal/signerverifier/dsse"
	"github.com/gittuf/gittuf/internal/signerverifier/sigstore"
	sslibdsse "github.com/gittuf/gittuf/internal/third_party/go-securesystemslib/dsse"
	"github.com/gittuf/gittuf/internal/tuf"
	tufv01 "github.com/gittuf/gittuf/internal/tuf/v01"
	"github.com/secure-systems-lab/go-securesystemslib/signerverifier"
	"github.com/stretchr/testify/assert"
)

func TestSetGitHubPullRequestApprovalAttestation(t *testing.T) {
	testRef := "refs/heads/main"
	testAnotherRef := "refs/heads/feature"
	testID := gitinterface.ZeroHash.String()
	baseURL := "https://github.com"
	baseHost := "github.com"
	appName := "github"

	approvers := []tuf.Principal{
		&tufv01.Key{
			KeyID:   "jane.doe@example.com::https://oidc.example.com",
			KeyType: sigstore.KeyType,
			Scheme:  sigstore.KeyScheme,
			KeyVal: signerverifier.KeyVal{
				Identity: "jane.doe@example.com",
				Issuer:   "https://oidc.example.com",
			},
		},
	}

	mainZeroZero := createGitHubPullRequestApprovalAttestationEnvelope(t, testRef, testID, testID, approvers)
	featureZeroZero := createGitHubPullRequestApprovalAttestationEnvelope(t, testAnotherRef, testID, testID, approvers)

	tmpDir := t.TempDir()
	repo := gitinterface.CreateTestGitRepository(t, tmpDir, false)

	attestations := &Attestations{}

	// Add auth for first branch
	err := attestations.SetGitHubPullRequestApprovalAttestation(repo, mainZeroZero, baseURL, 1, appName, testRef, testID, testID)
	assert.Nil(t, err)
	assert.Contains(t, attestations.codeReviewApprovalAttestations, path.Join(GitHubPullRequestApprovalAttestationPath(testRef, testID, testID), base64.URLEncoding.EncodeToString([]byte(appName))))
	assert.NotContains(t, attestations.codeReviewApprovalAttestations, path.Join(GitHubPullRequestApprovalAttestationPath(testAnotherRef, testID, testID), base64.URLEncoding.EncodeToString([]byte(appName))))
	assert.Equal(t, GitHubPullRequestApprovalAttestationPath(testRef, testID, testID), attestations.codeReviewApprovalIndex[fmt.Sprintf("%s::%d", baseHost, 1)])

	// Add auth for the other branch
	err = attestations.SetGitHubPullRequestApprovalAttestation(repo, featureZeroZero, baseURL, 2, appName, testAnotherRef, testID, testID)
	assert.Nil(t, err)
	assert.Contains(t, attestations.codeReviewApprovalAttestations, path.Join(GitHubPullRequestApprovalAttestationPath(testRef, testID, testID), base64.URLEncoding.EncodeToString([]byte(appName))))
	assert.Contains(t, attestations.codeReviewApprovalAttestations, path.Join(GitHubPullRequestApprovalAttestationPath(testAnotherRef, testID, testID), base64.URLEncoding.EncodeToString([]byte(appName))))
	assert.Equal(t, GitHubPullRequestApprovalAttestationPath(testRef, testID, testID), attestations.codeReviewApprovalIndex[fmt.Sprintf("%s::%d", baseHost, 1)])
	assert.Equal(t, GitHubPullRequestApprovalAttestationPath(testAnotherRef, testID, testID), attestations.codeReviewApprovalIndex[fmt.Sprintf("%s::%d", baseHost, 2)])
}

func TestGetGitHubPullRequestApprovalAttestation(t *testing.T) {
	testRef := "refs/heads/main"
	testAnotherRef := "refs/heads/feature"
	testID := gitinterface.ZeroHash.String()
	baseURL := "https://github.com"
	appName := "github"

	approvers := []tuf.Principal{
		&tufv01.Key{
			KeyID:   "jane.doe@example.com::https://oidc.example.com",
			KeyType: sigstore.KeyType,
			Scheme:  sigstore.KeyScheme,
			KeyVal: signerverifier.KeyVal{
				Identity: "jane.doe@example.com",
				Issuer:   "https://oidc.example.com",
			},
		},
	}

	mainZeroZero := createGitHubPullRequestApprovalAttestationEnvelope(t, testRef, testID, testID, approvers)
	featureZeroZero := createGitHubPullRequestApprovalAttestationEnvelope(t, testAnotherRef, testID, testID, approvers)

	tmpDir := t.TempDir()
	repo := gitinterface.CreateTestGitRepository(t, tmpDir, false)

	attestations := &Attestations{}

	err := attestations.SetGitHubPullRequestApprovalAttestation(repo, mainZeroZero, baseURL, 1, appName, testRef, testID, testID)
	if err != nil {
		t.Fatal(err)
	}
	err = attestations.SetGitHubPullRequestApprovalAttestation(repo, featureZeroZero, baseURL, 2, appName, testAnotherRef, testID, testID)
	if err != nil {
		t.Fatal(err)
	}

	mainAuth, err := attestations.GetGitHubPullRequestApprovalAttestationFor(repo, appName, testRef, testID, testID)
	assert.Nil(t, err)
	assert.Equal(t, mainZeroZero, mainAuth)

	featureAuth, err := attestations.GetGitHubPullRequestApprovalAttestationFor(repo, appName, testAnotherRef, testID, testID)
	assert.Nil(t, err)
	assert.Equal(t, featureZeroZero, featureAuth)
}

func createGitHubPullRequestApprovalAttestationEnvelope(t *testing.T, refName, fromID, toID string, approvers []tuf.Principal) *sslibdsse.Envelope {
	t.Helper()

	authorization, err := NewGitHubPullRequestApprovalAttestation(refName, fromID, toID, approvers, nil)
	if err != nil {
		t.Fatal(err)
	}
	env, err := dsse.CreateEnvelope(authorization)
	if err != nil {
		t.Fatal(err)
	}

	return env
}
