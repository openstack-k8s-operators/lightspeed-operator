/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	common_helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	apiv1beta1 "github.com/openstack-k8s-operators/lightspeed-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// validPEM is a self-signed test certificate generated once in TestMain.
var validPEM []byte

func TestMain(m *testing.M) {
	now := time.Now()
	data, err := makeSelfSignedPEM(now.Add(-1*time.Hour), now.Add(10*365*24*time.Hour))
	if err != nil {
		log.Fatalf("failed to generate test certificate: %v", err)
	}
	validPEM = data

	os.Exit(m.Run())
}

func makeSelfSignedPEM(notBefore, notAfter time.Time) ([]byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-cert"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageCertSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: derBytes,
	}), nil
}

// generateSelfSignedPEM creates a self-signed certificate PEM with the given
// validity window. Useful for producing expired or far-future test certs.
func generateSelfSignedPEM(t *testing.T, notBefore, notAfter time.Time) []byte {
	t.Helper()

	data, err := makeSelfSignedPEM(notBefore, notAfter)
	if err != nil {
		t.Fatalf("failed to generate certificate: %v", err)
	}

	return data
}

// ---------------------------------------------------------------------------
// getCertsFromPEM tests
// ---------------------------------------------------------------------------

func TestGetCertsFromPEM_ValidSingleCert(t *testing.T) {
	cab := &caBundle{}
	err := cab.getCertsFromPEM(validPEM)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cab.certs) != 1 {
		t.Fatalf("expected 1 cert, got %d", len(cab.certs))
	}
	if cab.certs[0].cert.Subject.CommonName != "test-cert" {
		t.Errorf("unexpected CN: %s", cab.certs[0].cert.Subject.CommonName)
	}
}

func TestGetCertsFromPEM_ValidMultipleCerts(t *testing.T) {
	now := time.Now()
	cert1 := generateSelfSignedPEM(t, now.Add(-1*time.Hour), now.Add(10*365*24*time.Hour))
	cert2 := generateSelfSignedPEM(t, now.Add(-2*time.Hour), now.Add(10*365*24*time.Hour))

	combined := append(cert1, cert2...)

	cab := &caBundle{}
	err := cab.getCertsFromPEM(combined)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cab.certs) != 2 {
		t.Fatalf("expected 2 certs, got %d", len(cab.certs))
	}
}

func TestGetCertsFromPEM_InvalidBlockType(t *testing.T) {
	privKeyPEM := []byte(`-----BEGIN PRIVATE KEY-----
MC4CAQAwBQYDK2VwBCIEIOlahWCjbH4UqSYP3tqOmP6MuEnMxOPHsYulrfWZ/3aV
-----END PRIVATE KEY-----
`)
	cab := &caBundle{}
	err := cab.getCertsFromPEM(privKeyPEM)
	if err == nil {
		t.Fatal("expected error for non-CERTIFICATE block, got nil")
	}
	if !strings.Contains(err.Error(), "PRIVATE KEY") {
		t.Errorf("error should mention the invalid block type, got: %v", err)
	}
}

func TestGetCertsFromPEM_InvalidX509Data(t *testing.T) {
	// A CERTIFICATE block with garbage bytes inside.
	garbage := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: []byte("this is not valid DER data"),
	})

	cab := &caBundle{}
	err := cab.getCertsFromPEM(garbage)
	if err == nil {
		t.Fatal("expected error for invalid X.509 data, got nil")
	}
	if !strings.Contains(err.Error(), "invalid certificate") {
		t.Errorf("error should mention invalid certificate, got: %v", err)
	}
}

func TestGetCertsFromPEM_NilInput(t *testing.T) {
	cab := &caBundle{}
	err := cab.getCertsFromPEM(nil)
	if err == nil {
		t.Fatal("expected error for nil input, got nil")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("error should mention nil, got: %v", err)
	}
}

func TestGetCertsFromPEM_EmptyInput(t *testing.T) {
	cab := &caBundle{}
	err := cab.getCertsFromPEM([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error for empty input: %v", err)
	}
	if len(cab.certs) != 0 {
		t.Errorf("expected 0 certs for empty input, got %d", len(cab.certs))
	}
}

func TestGetCertsFromPEM_NoPEMBlocks(t *testing.T) {
	// Non-empty but contains no PEM blocks (just plain text).
	cab := &caBundle{}
	err := cab.getCertsFromPEM([]byte("hello world, no PEM here"))
	if err == nil {
		t.Fatal("expected error for input with no PEM blocks, got nil")
	}
	if !strings.Contains(err.Error(), "trailing non-PEM data") {
		t.Errorf("expected trailing non-PEM data error, got: %v", err)
	}
}

func TestGetCertsFromPEM_Deduplication(t *testing.T) {
	// The same valid PEM certificate concatenated twice.
	doubled := append(validPEM, validPEM...)

	cab := &caBundle{}
	err := cab.getCertsFromPEM(doubled)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cab.certs) != 1 {
		t.Fatalf("expected 1 cert after dedup, got %d", len(cab.certs))
	}
}

func TestGetCertsFromPEM_DeduplicationAcrossCalls(t *testing.T) {
	// Calling getCertsFromPEM twice with the same data should still deduplicate.
	cab := &caBundle{}
	if err := cab.getCertsFromPEM(validPEM); err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}
	if err := cab.getCertsFromPEM(validPEM); err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}
	if len(cab.certs) != 1 {
		t.Fatalf("expected 1 cert after dedup across calls, got %d", len(cab.certs))
	}
}

func TestGetCertsFromPEM_ExpiredCertSkipped(t *testing.T) {
	// Generate a certificate that expired yesterday.
	now := time.Now()
	expiredPEM := generateSelfSignedPEM(t, now.Add(-48*time.Hour), now.Add(-24*time.Hour))

	cab := &caBundle{}
	err := cab.getCertsFromPEM(expiredPEM)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cab.certs) != 0 {
		t.Fatalf("expected 0 certs (expired should be skipped), got %d", len(cab.certs))
	}
}

func TestGetCertsFromPEM_MixedExpiredAndValid(t *testing.T) {
	now := time.Now()
	expiredPEM := generateSelfSignedPEM(t, now.Add(-48*time.Hour), now.Add(-24*time.Hour))
	combined := append(expiredPEM, validPEM...)

	cab := &caBundle{}
	err := cab.getCertsFromPEM(combined)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cab.certs) != 1 {
		t.Fatalf("expected 1 cert (only the valid one), got %d", len(cab.certs))
	}
	if cab.certs[0].cert.Subject.CommonName != "test-cert" {
		t.Errorf("expected the valid cert to remain, got CN=%s", cab.certs[0].cert.Subject.CommonName)
	}
}

// ---------------------------------------------------------------------------
// encodePEM tests
// ---------------------------------------------------------------------------

func TestEncodePEM_RoundTrip(t *testing.T) {
	// Parse the valid cert, encode it, parse again, compare.
	cab := &caBundle{}
	if err := cab.getCertsFromPEM(validPEM); err != nil {
		t.Fatalf("initial parse failed: %v", err)
	}

	encoded := cab.encodePEM()
	if len(encoded) == 0 {
		t.Fatal("encodePEM returned empty output")
	}

	cab2 := &caBundle{}
	if err := cab2.getCertsFromPEM(encoded); err != nil {
		t.Fatalf("round-trip parse failed: %v", err)
	}

	if len(cab2.certs) != len(cab.certs) {
		t.Fatalf("cert count mismatch after round-trip: original=%d, decoded=%d",
			len(cab.certs), len(cab2.certs))
	}

	if cab.certs[0].hash != cab2.certs[0].hash {
		t.Error("certificate hash mismatch after round-trip")
	}
}

func TestEncodePEM_MultipleCerts(t *testing.T) {
	now := time.Now()
	cert1 := generateSelfSignedPEM(t, now.Add(-1*time.Hour), now.Add(10*365*24*time.Hour))
	cert2 := generateSelfSignedPEM(t, now.Add(-2*time.Hour), now.Add(10*365*24*time.Hour))
	combined := append(cert1, cert2...)

	cab := &caBundle{}
	if err := cab.getCertsFromPEM(combined); err != nil {
		t.Fatalf("initial parse failed: %v", err)
	}
	if len(cab.certs) != 2 {
		t.Fatalf("expected 2 certs, got %d", len(cab.certs))
	}

	encoded := cab.encodePEM()

	cab2 := &caBundle{}
	if err := cab2.getCertsFromPEM(encoded); err != nil {
		t.Fatalf("round-trip parse failed: %v", err)
	}
	if len(cab2.certs) != 2 {
		t.Fatalf("expected 2 certs after round-trip, got %d", len(cab2.certs))
	}
}

func TestEncodePEM_EmptyBundle(t *testing.T) {
	cab := &caBundle{}
	encoded := cab.encodePEM()
	if encoded != nil {
		t.Errorf("expected nil for empty bundle, got %d bytes", len(encoded))
	}
}

func TestEncodePEM_ProducesValidPEMBlocks(t *testing.T) {
	cab := &caBundle{}
	if err := cab.getCertsFromPEM(validPEM); err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	encoded := cab.encodePEM()

	// Verify each PEM block can be decoded and is of type CERTIFICATE.
	rest := encoded
	blockCount := 0
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		blockCount++
		if block.Type != "CERTIFICATE" {
			t.Errorf("expected CERTIFICATE block type, got %q", block.Type)
		}
	}
	if blockCount != 1 {
		t.Errorf("expected 1 PEM block, got %d", blockCount)
	}
}

// ---------------------------------------------------------------------------
// encodePEM / getCertsFromPEM: raw bytes preservation
// ---------------------------------------------------------------------------

func TestEncodePEM_PreservesRawCertBytes(t *testing.T) {
	cab := &caBundle{}
	if err := cab.getCertsFromPEM(validPEM); err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	// Grab the raw DER bytes from the parsed cert.
	originalRaw := cab.certs[0].cert.Raw

	encoded := cab.encodePEM()
	block, _ := pem.Decode(encoded)
	if block == nil {
		t.Fatal("failed to decode PEM from encodePEM output")
	}

	if !bytes.Equal(block.Bytes, originalRaw) {
		t.Error("encoded DER bytes do not match original raw bytes")
	}
}

// ---------------------------------------------------------------------------
// caBundle hash field
// ---------------------------------------------------------------------------

func TestCaBundleCert_HashMatchesDER(t *testing.T) {
	cab := &caBundle{}
	if err := cab.getCertsFromPEM(validPEM); err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	block, _ := pem.Decode(validPEM)
	if block == nil {
		t.Fatal("failed to decode validPEM")
	}
	expectedHash := sha256.Sum256(block.Bytes)

	if cab.certs[0].hash != expectedHash {
		t.Error("stored hash does not match SHA256 of DER bytes")
	}
}

// ---------------------------------------------------------------------------
// reconcileCABundleConfigMap tests
// ---------------------------------------------------------------------------

func newTestHelper(t *testing.T, objs ...client.Object) *common_helper.Helper {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	if err := apiv1beta1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add apiv1beta1 to scheme: %v", err)
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		Build()

	instance := &apiv1beta1.OpenStackLightspeed{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-instance",
			Namespace: "test-ns",
		},
	}

	logger := zap.New(zap.WriteTo(bytes.NewBuffer(nil)))
	h, err := common_helper.NewHelper(instance, fakeClient, nil, scheme, logger)
	if err != nil {
		t.Fatalf("failed to create helper: %v", err)
	}
	return h
}

func TestReconcileCABundle_MergesServiceCA(t *testing.T) {
	original := getOperatorCABundle
	t.Cleanup(func() { getOperatorCABundle = original })

	now := time.Now()
	systemCert := generateSelfSignedPEM(t, now.Add(-1*time.Hour), now.Add(10*365*24*time.Hour))
	serviceCACert := generateSelfSignedPEM(t, now.Add(-2*time.Hour), now.Add(10*365*24*time.Hour))
	kubeRootCACert := generateSelfSignedPEM(t, now.Add(-3*time.Hour), now.Add(10*365*24*time.Hour))

	getOperatorCABundle = func() ([]byte, error) {
		return systemCert, nil
	}

	serviceCAConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OpenShiftServiceCAConfigMap,
			Namespace: "test-ns",
		},
		Data: map[string]string{
			"service-ca.crt": string(serviceCACert),
		},
	}
	kubeRootCAConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      KubeRootCAConfigMap,
			Namespace: "test-ns",
		},
		Data: map[string]string{
			"ca.crt": string(kubeRootCACert),
		},
	}

	h := newTestHelper(t, serviceCAConfigMap, kubeRootCAConfigMap)

	instance := &apiv1beta1.OpenStackLightspeed{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-instance",
			Namespace: "test-ns",
		},
	}

	ctx := context.Background()
	err := reconcileCABundleConfigMap(h, ctx, instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultCM := &corev1.ConfigMap{}
	err = h.GetClient().Get(ctx, types.NamespacedName{
		Name:      CABundleConfigMapName,
		Namespace: "test-ns",
	}, resultCM)
	if err != nil {
		t.Fatalf("failed to get CA bundle ConfigMap: %v", err)
	}

	bundleData := resultCM.Data[CABundleKey]
	cab := &caBundle{}
	if err := cab.getCertsFromPEM([]byte(bundleData)); err != nil {
		t.Fatalf("failed to parse CA bundle: %v", err)
	}
	if len(cab.certs) != 3 {
		t.Fatalf("expected 3 certs (system + service CA + kube root CA), got %d", len(cab.certs))
	}
}

func TestReconcileCABundle_FailsWithoutServiceCA(t *testing.T) {
	original := getOperatorCABundle
	t.Cleanup(func() { getOperatorCABundle = original })

	now := time.Now()
	systemCert := generateSelfSignedPEM(t, now.Add(-1*time.Hour), now.Add(10*365*24*time.Hour))
	getOperatorCABundle = func() ([]byte, error) {
		return systemCert, nil
	}

	kubeRootCAConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      KubeRootCAConfigMap,
			Namespace: "test-ns",
		},
		Data: map[string]string{
			"ca.crt": string(systemCert),
		},
	}

	h := newTestHelper(t, kubeRootCAConfigMap)

	instance := &apiv1beta1.OpenStackLightspeed{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-instance",
			Namespace: "test-ns",
		},
	}

	err := reconcileCABundleConfigMap(h, context.Background(), instance)
	if err == nil {
		t.Fatal("expected error when service CA ConfigMap is missing, got nil")
	}
	if !strings.Contains(err.Error(), OpenShiftServiceCAConfigMap) {
		t.Errorf("expected error to mention %s, got: %v", OpenShiftServiceCAConfigMap, err)
	}
}

func TestReconcileCABundle_FailsWithoutKubeRootCA(t *testing.T) {
	original := getOperatorCABundle
	t.Cleanup(func() { getOperatorCABundle = original })

	now := time.Now()
	systemCert := generateSelfSignedPEM(t, now.Add(-1*time.Hour), now.Add(10*365*24*time.Hour))
	getOperatorCABundle = func() ([]byte, error) {
		return systemCert, nil
	}

	serviceCAConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OpenShiftServiceCAConfigMap,
			Namespace: "test-ns",
		},
		Data: map[string]string{
			"service-ca.crt": string(systemCert),
		},
	}

	h := newTestHelper(t, serviceCAConfigMap)

	instance := &apiv1beta1.OpenStackLightspeed{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-instance",
			Namespace: "test-ns",
		},
	}

	err := reconcileCABundleConfigMap(h, context.Background(), instance)
	if err == nil {
		t.Fatal("expected error when kube root CA ConfigMap is missing, got nil")
	}
	if !strings.Contains(err.Error(), KubeRootCAConfigMap) {
		t.Errorf("expected error to mention %s, got: %v", KubeRootCAConfigMap, err)
	}
}

func TestReconcileCABundle_DeduplicatesAcrossSources(t *testing.T) {
	original := getOperatorCABundle
	t.Cleanup(func() { getOperatorCABundle = original })

	now := time.Now()
	sharedCert := generateSelfSignedPEM(t, now.Add(-1*time.Hour), now.Add(10*365*24*time.Hour))

	getOperatorCABundle = func() ([]byte, error) {
		return sharedCert, nil
	}

	serviceCAConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OpenShiftServiceCAConfigMap,
			Namespace: "test-ns",
		},
		Data: map[string]string{
			"service-ca.crt": string(sharedCert),
		},
	}
	kubeRootCAConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      KubeRootCAConfigMap,
			Namespace: "test-ns",
		},
		Data: map[string]string{
			"ca.crt": string(sharedCert),
		},
	}

	h := newTestHelper(t, serviceCAConfigMap, kubeRootCAConfigMap)

	instance := &apiv1beta1.OpenStackLightspeed{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-instance",
			Namespace: "test-ns",
		},
	}

	ctx := context.Background()
	err := reconcileCABundleConfigMap(h, ctx, instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultCM := &corev1.ConfigMap{}
	err = h.GetClient().Get(ctx, types.NamespacedName{
		Name:      CABundleConfigMapName,
		Namespace: "test-ns",
	}, resultCM)
	if err != nil {
		t.Fatalf("failed to get CA bundle ConfigMap: %v", err)
	}

	bundleData := resultCM.Data[CABundleKey]
	cab := &caBundle{}
	if err := cab.getCertsFromPEM([]byte(bundleData)); err != nil {
		t.Fatalf("failed to parse CA bundle: %v", err)
	}
	if len(cab.certs) != 1 {
		t.Fatalf("expected 1 cert (same cert deduplicated across all sources), got %d", len(cab.certs))
	}
}
