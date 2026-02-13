package k8s

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"jabberwocky238/console/dblayer"
	"log"
	"net"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"slices"
)

type DomainStatus string

const (
	DomainStatusPending DomainStatus = "pending"
	DomainStatusSuccess DomainStatus = "success"
	DomainStatusError   DomainStatus = "error"
)

type CustomDomain struct {
	ID        int          `json:"id"`
	CDID      string       `json:"cdid"`
	Domain    string       `json:"domain"`
	Target    string       `json:"target"`
	TXTName   string       `json:"txt_name"`
	TXTValue  string       `json:"txt_value"`
	Status    DomainStatus `json:"status"`
	UserUID   string       `json:"user_uid"`
	CreatedAt time.Time    `json:"created_at"`
}

// generateVerifyToken generates a random verification token
func generateVerifyToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// NewCustomDomain creates a new custom domain verification request
func NewCustomDomain(userUID, domain, target string) (*CustomDomain, error) {
	cdid := generateVerifyToken()[:8]
	token := generateVerifyToken()
	txtName := fmt.Sprintf("_combinator-verify.%s", domain)
	txtValue := fmt.Sprintf("combinator-verify=%s", token)

	err := dblayer.CreateCustomDomain(cdid, userUID, domain, target, txtName, txtValue, string(DomainStatusPending))
	if err != nil {
		return nil, err
	}

	cd := &CustomDomain{
		CDID:      cdid,
		Domain:    domain,
		Target:    target,
		TXTName:   txtName,
		TXTValue:  txtValue,
		Status:    DomainStatusPending,
		UserUID:   userUID,
		CreatedAt: time.Now(),
	}

	log.Printf("[customdomain] Created custom domain request: %s -> %s (TXT: %s = %s)", domain, target, txtName, txtValue)
	return cd, nil
}

// VerifyTXT checks if the TXT record is correctly set via DNS lookup
func (cd *CustomDomain) VerifyTXT() bool {
	records, err := net.LookupTXT(cd.TXTName)
	if err != nil {
		log.Printf("[customdomain] TXT lookup failed for %s: %v", cd.TXTName, err)
		return false
	}

	if slices.Contains(records, cd.TXTValue) {
		log.Printf("[customdomain] TXT record verified: %s = %s", cd.TXTName, cd.TXTValue)
		return true
	}

	log.Printf("[customdomain] TXT record not found or mismatch for %s (expected: %s, found: %v)", cd.TXTName, cd.TXTValue, records)
	return false
}

// VerifyCNAME checks if the CNAME record points to the correct target
func (cd *CustomDomain) VerifyCNAME() bool {
	cname, err := net.LookupCNAME(cd.Domain)
	if err != nil {
		log.Printf("[customdomain] CNAME lookup failed for %s: %v", cd.Domain, err)
		return false
	}

	// Remove trailing dot from CNAME result
	if len(cname) > 0 && cname[len(cname)-1] == '.' {
		cname = cname[:len(cname)-1]
	}

	// Check if CNAME matches target (with or without trailing dot)
	targetWithoutDot := cd.Target
	if len(targetWithoutDot) > 0 && targetWithoutDot[len(targetWithoutDot)-1] == '.' {
		targetWithoutDot = targetWithoutDot[:len(targetWithoutDot)-1]
	}

	if cname == targetWithoutDot || cname == cd.Target {
		log.Printf("[customdomain] CNAME record verified: %s -> %s", cd.Domain, cname)
		return true
	}

	log.Printf("[customdomain] CNAME record mismatch for %s (expected: %s, found: %s)", cd.Domain, cd.Target, cname)
	return false
}

// StartVerification starts the verification loop (5s interval, 12 times max = 60s total)
func (cd *CustomDomain) StartVerification() {
	go func() {
		for i := range 12 {
			time.Sleep(5 * time.Second)

			// Check both TXT and CNAME records
			txtVerified := cd.VerifyTXT()
			cnameVerified := cd.VerifyCNAME()

			if txtVerified && cnameVerified {
				log.Printf("[customdomain] Verification successful for %s (attempt %d/12)", cd.Domain, i+1)
				cd.Status = DomainStatusSuccess
				dblayer.UpdateCustomDomainStatus(cd.CDID, string(DomainStatusSuccess))

				// Create IngressRoute and request certificate
				if err := cd.CreateIngressRoute(); err != nil {
					log.Printf("[customdomain] Failed to create IngressRoute for %s: %v", cd.Domain, err)
					cd.Status = DomainStatusError
					dblayer.UpdateCustomDomainStatus(cd.CDID, string(DomainStatusError))
				}
				return
			}

			log.Printf("[customdomain] Verification attempt %d/12 for %s (TXT: %v, CNAME: %v)", i+1, cd.Domain, txtVerified, cnameVerified)
		}

		// Failed after 12 attempts
		cd.Status = DomainStatusError
		dblayer.UpdateCustomDomainStatus(cd.CDID, string(DomainStatusError))
		log.Printf("[customdomain] Verification failed for %s after 12 attempts (60s)", cd.Domain)
	}()
}

// CreateIngressRoute creates an ExternalName Service and IngressRoute for the custom domain
// Uses HTTP-01 challenge for ZeroSSL certificate
func (cd *CustomDomain) CreateIngressRoute() error {
	if DynamicClient == nil || K8sClient == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	ctx := context.Background()
	name := fmt.Sprintf("custom-domain-%s", cd.CDID)
	tlsSecretName := fmt.Sprintf("custom-domain-tls-%s", cd.CDID)

	// Create ExternalName Service pointing to target domain
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: IngressNamespace,
			Labels: map[string]string{
				"app":      "custom-domain",
				"user-uid": cd.UserUID,
			},
		},
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: cd.Target,
		},
	}
	if _, err := K8sClient.CoreV1().Services(IngressNamespace).Create(ctx, svc, metav1.CreateOptions{}); err != nil {
		log.Printf("[customdomain] Failed to create service for %s: %v", cd.Domain, err)
		return fmt.Errorf("create service failed: %w", err)
	}
	log.Printf("[customdomain] Created ExternalName service: %s -> %s", name, cd.Target)

	// Create cert-manager Certificate for the custom domain (HTTP-01 challenge)
	cert := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Certificate",
			"metadata": map[string]any{
				"name":      name,
				"namespace": IngressNamespace,
				"labels": map[string]any{
					"app":      "custom-domain",
					"user-uid": cd.UserUID,
				},
			},
			"spec": map[string]any{
				"secretName": tlsSecretName,
				"dnsNames":   []any{cd.Domain},
				"issuerRef": map[string]any{
					"name": "zerossl-issuer",
					"kind": "ClusterIssuer",
				},
			},
		},
	}
	if _, err := DynamicClient.Resource(certificateGVR).Namespace(IngressNamespace).Create(ctx, cert, metav1.CreateOptions{}); err != nil {
		log.Printf("[customdomain] Failed to create certificate for %s: %v", cd.Domain, err)
		return fmt.Errorf("create certificate failed: %w", err)
	}
	log.Printf("[customdomain] Created Certificate with HTTP-01 challenge: %s", cd.Domain)

	// Create IngressRoute
	ingressRoute := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "traefik.io/v1alpha1",
			"kind":       "IngressRoute",
			"metadata": map[string]any{
				"name":      name,
				"namespace": IngressNamespace,
				"labels": map[string]any{
					"app":      "custom-domain",
					"user-uid": cd.UserUID,
				},
			},
			"spec": map[string]any{
				"entryPoints": []any{"websecure"},
				"routes": []any{
					map[string]any{
						"match": fmt.Sprintf("Host(`%s`)", cd.Domain),
						"kind":  "Rule",
						"services": []any{
							map[string]any{
								"name": name,
								"port": 443,
							},
						},
					},
				},
				"tls": map[string]any{
					"secretName": tlsSecretName,
				},
			},
		},
	}

	if _, err := DynamicClient.Resource(IngressRouteGVR).Namespace(IngressNamespace).Create(ctx, ingressRoute, metav1.CreateOptions{}); err != nil {
		log.Printf("[customdomain] Failed to create IngressRoute for %s: %v", cd.Domain, err)
		return fmt.Errorf("create ingressroute failed: %w", err)
	}

	log.Printf("[customdomain] Created IngressRoute for %s with TLS secret %s", cd.Domain, tlsSecretName)
	return nil
}

// GetCustomDomain returns a custom domain by CDID
func GetCustomDomain(cdid string) (*CustomDomain, error) {
	cd, err := dblayer.GetCustomDomain(cdid)
	if err != nil {
		return nil, err
	}
	return &CustomDomain{
		ID:        cd.ID,
		CDID:      cd.CDID,
		Domain:    cd.Domain,
		Target:    cd.Target,
		TXTName:   cd.TXTName,
		TXTValue:  cd.TXTValue,
		Status:    DomainStatus(cd.Status),
		UserUID:   cd.UserUID,
		CreatedAt: cd.CreatedAt,
	}, nil
}

// ListCustomDomains returns all custom domains for a user
func ListCustomDomains(userUID string) []*CustomDomain {
	dbDomains, err := dblayer.ListCustomDomains(userUID)
	if err != nil {
		return nil
	}

	var result []*CustomDomain
	for _, cd := range dbDomains {
		result = append(result, &CustomDomain{
			ID:        cd.ID,
			CDID:      cd.CDID,
			Domain:    cd.Domain,
			Target:    cd.Target,
			TXTName:   cd.TXTName,
			TXTValue:  cd.TXTValue,
			Status:    DomainStatus(cd.Status),
			UserUID:   cd.UserUID,
			CreatedAt: cd.CreatedAt,
		})
	}
	return result
}

// DeleteCustomDomain deletes a custom domain, Service and IngressRoute
func DeleteCustomDomain(cdid string) error {
	// Get domain info before deletion for TXT cleanup
	_, err := GetCustomDomain(cdid)

	// Delete from database
	if err = dblayer.DeleteCustomDomain(cdid); err != nil {
		return err
	}

	ctx := context.Background()
	name := fmt.Sprintf("custom-domain-%s", cdid)

	// Delete Service
	if K8sClient != nil {
		K8sClient.CoreV1().Services(IngressNamespace).Delete(ctx, name, metav1.DeleteOptions{})
	}

	// Delete IngressRoute AND Certificate
	if DynamicClient != nil {
		DynamicClient.Resource(IngressRouteGVR).Namespace(IngressNamespace).Delete(ctx, name, metav1.DeleteOptions{})
		DynamicClient.Resource(certificateGVR).Namespace(IngressNamespace).Delete(ctx, name, metav1.DeleteOptions{})
	}

	log.Printf("[customdomain] Deleted custom domain resources for %s", cdid)
	return nil
}
