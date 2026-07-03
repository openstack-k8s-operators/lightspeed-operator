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
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	common_helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	apiv1beta1 "github.com/openstack-k8s-operators/lightspeed-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type caCert struct {
	hash   [sha256.Size]byte
	cert   *x509.Certificate
	expire time.Time
}

type caBundle struct {
	certs []caCert
}

// getOperatorCABundle reads the system CA bundle from the operator pod's filesystem.
var getOperatorCABundle = func() ([]byte, error) {
	contents, err := os.ReadFile(SystemTLSCABundlePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read system CA bundle: %w", err)
	}
	return contents, nil
}

// getCertsFromPEM parses PEM data and adds valid certificates to the bundle.
// Rejects non-CERTIFICATE blocks and invalid X.509 data. Skips expired certs.
// Deduplicates by SHA256 hash of the raw DER bytes.
func (cab *caBundle) getCertsFromPEM(pemData []byte) error {
	if pemData == nil {
		return fmt.Errorf("certificate data is nil")
	}

	rest := pemData
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}

		if block.Type != "CERTIFICATE" {
			return fmt.Errorf("invalid PEM block type %q: only CERTIFICATE blocks are permitted", block.Type)
		}

		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return fmt.Errorf("invalid certificate: %w", err)
		}

		if time.Now().After(certificate.NotAfter) {
			continue
		}

		blockHash := sha256.Sum256(block.Bytes)
		isDuplicate := false
		for _, existing := range cab.certs {
			if existing.hash == blockHash {
				isDuplicate = true
				break
			}
		}
		if !isDuplicate {
			cab.certs = append(cab.certs, caCert{
				hash:   blockHash,
				cert:   certificate,
				expire: certificate.NotAfter,
			})
		}
	}

	if len(bytes.TrimSpace(rest)) > 0 {
		return fmt.Errorf("trailing non-PEM data (%d bytes)", len(bytes.TrimSpace(rest)))
	}

	return nil
}

// encodePEM encodes all certificates in the bundle back to PEM format.
func (cab *caBundle) encodePEM() []byte {
	var result []byte
	for _, c := range cab.certs {
		block := &pem.Block{
			Type:  "CERTIFICATE",
			Bytes: c.cert.Raw,
		}
		result = append(result, pem.EncodeToMemory(block)...)
	}
	return result
}

// mergeCertsFromConfigMap reads the Data section of the given ConfigMap
// and adds any valid certificate entries to the bundle.
func (cab *caBundle) mergeCertsFromConfigMap(h *common_helper.Helper, ctx context.Context, cmName string) error {
	cm := &corev1.ConfigMap{}
	if err := h.GetClient().Get(ctx, client.ObjectKey{
		Name:      cmName,
		Namespace: h.GetBeforeObject().GetNamespace(),
	}, cm); err != nil {
		return err
	}
	for key, certData := range cm.Data {
		if err := cab.getCertsFromPEM([]byte(certData)); err != nil {
			return fmt.Errorf("%w: key %q in ConfigMap %q: %v", ErrParseUserCA, key, cmName, err)
		}
	}
	return nil
}

// reconcileCABundleConfigMap builds a CA bundle containing the operator's
// system CA certificates, a user-provided CA ConfigMap (if specified), as well as
// the "kube-root-ca.crt" and "openshift-service-ca.crt" ConfigMaps. It then creates
// or updates the managed ConfigMap, which is mounted into application pods.
func reconcileCABundleConfigMap(h *common_helper.Helper, ctx context.Context, instance *apiv1beta1.OpenStackLightspeed) error {
	logger := h.GetLogger()
	bundle := &caBundle{}

	systemCAs, err := getOperatorCABundle()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrReadSystemCABundle, err)
	}

	if err := bundle.getCertsFromPEM(systemCAs); err != nil {
		return fmt.Errorf("%w: %v", ErrParseSystemCABundle, err)
	}

	certsCMs := []string{OpenShiftServiceCAConfigMap, KubeRootCAConfigMap}
	if instance.Spec.TLSCACertBundle != "" {
		certsCMs = append(certsCMs, instance.Spec.TLSCACertBundle)
	}

	for _, certCM := range certsCMs {
		if err := bundle.mergeCertsFromConfigMap(h, ctx, certCM); err != nil {
			return fmt.Errorf("%w %q: %v", ErrGetCAConfigMap, certCM, err)
		}
		logger.Info("CA certificates merged", "configmap", certCM)
	}

	bundlePEM := bundle.encodePEM()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CABundleConfigMapName,
			Namespace: h.GetBeforeObject().GetNamespace(),
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, h.GetClient(), cm, func() error {
		cm.Data = map[string]string{
			CABundleKey: string(bundlePEM),
		}
		return controllerutil.SetControllerReference(h.GetBeforeObject(), cm, h.GetScheme())
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCreateCABundle, err)
	}

	logger.Info("CA bundle ConfigMap reconciled", "name", cm.Name, "result", result, "certCount", len(bundle.certs))
	return nil
}
