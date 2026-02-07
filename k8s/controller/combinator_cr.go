package controller

import (
	"context"
	"encoding/json"
	"fmt"

	"jabberwocky238/console/k8s"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// CreateCombinatorAppCR creates or updates a CombinatorApp CR
func CreateCombinatorAppCR(client dynamic.Interface, ownerID string, config string) error {
	name := fmt.Sprintf("combinator-%s", ownerID)

	cr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": Group + "/" + Version,
			"kind":       CombinatorKind,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": k8s.CombinatorNamespace,
			},
			"spec": map[string]interface{}{
				"ownerID": ownerID,
				"config":  config,
			},
		},
	}

	ctx := context.Background()
	res := client.Resource(CombinatorAppGVR).Namespace(k8s.CombinatorNamespace)

	existing, err := res.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		_, err = res.Create(ctx, cr, metav1.CreateOptions{})
		return err
	}

	cr.SetResourceVersion(existing.GetResourceVersion())
	_, err = res.Update(ctx, cr, metav1.UpdateOptions{})
	return err
}

// UpdateCombinatorAppConfig updates only the spec.config field of a CombinatorApp CR
func UpdateCombinatorAppConfig(client dynamic.Interface, ownerID string, config string) error {
	name := fmt.Sprintf("combinator-%s", ownerID)

	ctx := context.Background()
	res := client.Resource(CombinatorAppGVR).Namespace(k8s.CombinatorNamespace)

	existing, err := res.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get CR %s: %w", name, err)
	}

	spec, _ := existing.Object["spec"].(map[string]interface{})
	if spec == nil {
		spec = map[string]interface{}{}
	}
	spec["config"] = config
	existing.Object["spec"] = spec

	_, err = res.Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

// GetCombinatorAppConfig reads the spec.config from a CombinatorApp CR
func GetCombinatorAppConfig(client dynamic.Interface, ownerID string) (*Combinator, error) {
	name := fmt.Sprintf("combinator-%s", ownerID)

	ctx := context.Background()
	res := client.Resource(CombinatorAppGVR).Namespace(k8s.CombinatorNamespace)

	cr, err := res.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get CR %s: %w", name, err)
	}

	return combinatorFromUnstructured(cr), nil
}

// GetCombinatorConfig reads and parses the config directly into CombinatorConfig
func GetCombinatorConfig(client dynamic.Interface, ownerID string) (*CombinatorConfig, error) {
	combinator, err := GetCombinatorAppConfig(client, ownerID)
	if err != nil {
		return nil, err
	}
	return combinator.ParseConfig()
}

// UpdateCombinatorConfig marshals CombinatorConfig and writes it back to the CR
func UpdateCombinatorConfig(client dynamic.Interface, ownerID string, cfg *CombinatorConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return UpdateCombinatorAppConfig(client, ownerID, string(data))
}

// DeleteCombinatorAppCR deletes a CombinatorApp CR
func DeleteCombinatorAppCR(client dynamic.Interface, ownerID string) error {
	name := fmt.Sprintf("combinator-%s", ownerID)
	return client.Resource(CombinatorAppGVR).
		Namespace(k8s.CombinatorNamespace).
		Delete(context.Background(), name, metav1.DeleteOptions{})
}

// combinatorFromUnstructured extracts a Combinator from an unstructured CR
func combinatorFromUnstructured(u *unstructured.Unstructured) *Combinator {
	spec, _ := u.Object["spec"].(map[string]interface{})
	if spec == nil {
		return nil
	}
	ownerID := fmt.Sprintf("%v", spec["ownerID"])
	config := fmt.Sprintf("%v", spec["config"])

	return &Combinator{
		UserUID: ownerID,
		Config:  config,
	}
}

// EmptyCombinatorConfig returns an empty config JSON string
func EmptyCombinatorConfig() string {
	cfg := CombinatorConfig{
		RDBs: []RDBItem{},
		KVs:  []KVItem{},
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return string(data)
}
