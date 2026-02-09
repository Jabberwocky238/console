package controller

import (
	"context"
	"fmt"
	"log"
	"time"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func EnsureCRD(config *rest.Config) error {
	client, err := apiextclient.NewForConfig(config)
	if err != nil {
		return err
	}

	crd := &apiextv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: WorkerResource + "." + Group,
		},
		Spec: apiextv1.CustomResourceDefinitionSpec{
			Group: Group,
			Names: apiextv1.CustomResourceDefinitionNames{
				Plural:     WorkerResource,
				Singular:   "workerapp",
				Kind:       WorkerKind,
				ShortNames: []string{"wa"},
			},
			Scope: apiextv1.NamespaceScoped,
			Versions: []apiextv1.CustomResourceDefinitionVersion{
				{
					Name:    Version,
					Served:  true,
					Storage: true,
					Schema: &apiextv1.CustomResourceValidation{
						OpenAPIV3Schema: workerAppSchema(),
					},
					Subresources: &apiextv1.CustomResourceSubresources{
						Status: &apiextv1.CustomResourceSubresourceStatus{},
					},
				},
			},
		},
	}

	ctx := context.Background()
	crdClient := client.ApiextensionsV1().CustomResourceDefinitions()

	_, err = crdClient.Get(ctx, crd.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = crdClient.Create(ctx, crd, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		log.Printf("[controller] CRD %s created, waiting for it to be established", crd.Name)
	} else if err != nil {
		return fmt.Errorf("get CRD %s: %w", crd.Name, err)
	} else {
		log.Printf("[controller] CRD %s already exists", crd.Name)
		return nil
	}

	// Wait for CRD to become Established
	for i := 0; i < 30; i++ {
		time.Sleep(time.Second)
		got, err := crdClient.Get(ctx, crd.Name, metav1.GetOptions{})
		if err != nil {
			continue
		}
		for _, c := range got.Status.Conditions {
			if c.Type == apiextv1.Established && c.Status == apiextv1.ConditionTrue {
				log.Printf("[controller] CRD %s established", crd.Name)
				return nil
			}
		}
	}
	return fmt.Errorf("CRD %s not established after 30s", crd.Name)
}

func workerAppSchema() *apiextv1.JSONSchemaProps {
	return &apiextv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextv1.JSONSchemaProps{
			"spec": {
				Type: "object",
				Required: []string{
					"workerID", "ownerID",
					"image", "port",
				},
				Properties: map[string]apiextv1.JSONSchemaProps{
					"workerID": {Type: "string"},
					"ownerID":  {Type: "string"},
					"ownerSK":  {Type: "string"},
					"image":    {Type: "string"},
					"port":     {Type: "integer"},
				},
			},
			"status": {
				Type: "object",
				Properties: map[string]apiextv1.JSONSchemaProps{
					"phase":   {Type: "string"},
					"message": {Type: "string"},
				},
			},
		},
	}
}
