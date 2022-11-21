// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/caas"
	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/secrets/provider"
)

var logger = loggo.GetLogger("juju.secrets.provider.kubernetes")

const (
	// Store is the name of the Kubernetes secrets store.
	Store = "kubernetes"
)

// NewProvider returns a Kubernetes secrets provider.
func NewProvider() provider.SecretStoreProvider {
	return k8sProvider{Store}
}

type k8sProvider struct {
	name string
}

func (p k8sProvider) Type() string {
	return p.name
}

// Initialise is not used.
func (p k8sProvider) Initialise(m provider.Model) error {
	return nil
}

// CleanupModel is not used.
func (p k8sProvider) CleanupModel(m provider.Model) error {
	return nil
}

// CleanupSecrets removes rules of the role associated with the removed secrets.
func (p k8sProvider) CleanupSecrets(m provider.Model, tag names.Tag, removed provider.SecretRevisions) error {
	if tag == nil {
		// This should never happen.
		// Because this method is used for uniter facade only.
		return errors.New("empty tag")
	}
	if len(removed) == 0 {
		return nil
	}
	cloudSpec, err := cloudSpecForModel(m)
	if err != nil {
		return errors.Trace(err)
	}
	cfg, err := m.Config()
	if err != nil {
		return errors.Trace(err)
	}

	broker, err := NewCaas(context.TODO(), environs.OpenParams{
		ControllerUUID: m.ControllerUUID(),
		Cloud:          cloudSpec,
		Config:         cfg,
	})
	if err != nil {
		return errors.Trace(err)
	}
	_, err = broker.EnsureSecretAccessToken(tag, nil, nil, removed.Names())
	return errors.Trace(err)
}

func cloudSpecToStoreConfig(controllerUUID string, cfg *config.Config, spec cloudspec.CloudSpec) (*provider.StoreConfig, error) {
	cred, err := json.Marshal(spec.Credential)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &provider.StoreConfig{
		StoreType: Store,
		Params: map[string]interface{}{
			"controller-uuid":     controllerUUID,
			"model-name":          cfg.Name(),
			"model-type":          cfg.Type(),
			"model-uuid":          cfg.UUID(),
			"endpoint":            spec.Endpoint,
			"ca-certs":            spec.CACertificates,
			"is-controller-cloud": spec.IsControllerCloud,
			"credential":          string(cred),
		},
	}, nil
}

// StoreConfig returns the config needed to create a k8s secrets store.
// TODO(wallyworld) - only allow access to the specified secrets
func (p k8sProvider) StoreConfig(m provider.Model, tag names.Tag, owned provider.SecretRevisions, read provider.SecretRevisions) (*provider.StoreConfig, error) {
	logger.Tracef("getting k8s store config for %q, owned %v, read %v", tag, owned, read)
	cloudSpec, err := cloudSpecForModel(m)
	if err != nil {
		return nil, errors.Trace(err)
	}

	cfg, err := m.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}
	controllerUUID := m.ControllerUUID()
	if tag == nil {
		return cloudSpecToStoreConfig(controllerUUID, cfg, cloudSpec)
	}

	broker, err := NewCaas(context.TODO(), environs.OpenParams{
		ControllerUUID: controllerUUID,
		Cloud:          cloudSpec,
		Config:         cfg,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	token, err := broker.EnsureSecretAccessToken(tag, owned.Names(), read.Names(), nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cred, err := k8scloud.UpdateCredentialWithToken(*cloudSpec.Credential, token)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudSpec.Credential = &cred

	if cloudSpec.IsControllerCloud {
		// The cloudspec used for controller has a fake endpoint (address and port)
		// because we ignore the endpoint and load the in-cluster credential instead.
		// So we have to clean up the endpoint here for uniter to use.

		host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
		if len(host) != 0 && len(port) != 0 {
			cloudSpec.Endpoint = "https://" + net.JoinHostPort(host, port)
			logger.Tracef("patching endpoint to %q", cloudSpec.Endpoint)
			cloudSpec.IsControllerCloud = false
		}
	}
	return cloudSpecToStoreConfig(controllerUUID, cfg, cloudSpec)
}

type Broker interface {
	caas.SecretsStore
	caas.SecretsProvider
}

// NewCaas is patched for testing.
var NewCaas = newCaas

func newCaas(ctx context.Context, args environs.OpenParams) (Broker, error) {
	return caas.New(ctx, args)
}

// NewStore returns a k8s backed secrets store.
func (p k8sProvider) NewStore(cfg *provider.StoreConfig) (provider.SecretsStore, error) {
	modelName := cfg.Params["model-name"].(string)
	modelType := cfg.Params["model-type"].(string)
	modelUUID := cfg.Params["model-uuid"].(string)
	modelCfg, err := config.New(config.UseDefaults, map[string]interface{}{
		config.NameKey: modelName,
		config.TypeKey: modelType,
		config.UUIDKey: modelUUID,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudSpec := cloudspec.CloudSpec{
		Type:              "kubernetes",
		Name:              "secret-access",
		Endpoint:          cfg.Params["endpoint"].(string),
		IsControllerCloud: cfg.Params["is-controller-cloud"].(bool),
	}
	var ok bool
	cloudSpec.CACertificates, ok = cfg.Params["ca-certs"].([]string)
	if !ok {
		certs := cfg.Params["ca-certs"].([]interface{})
		cloudSpec.CACertificates = make([]string, len(certs))
		for i, cert := range certs {
			cloudSpec.CACertificates[i] = fmt.Sprintf("%s", cert)
		}
	}
	var cred cloud.Credential
	err = json.Unmarshal([]byte(cfg.Params["credential"].(string)), &cred)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudSpec.Credential = &cred

	broker, err := NewCaas(context.TODO(), environs.OpenParams{
		ControllerUUID: cfg.Params["controller-uuid"].(string),
		Cloud:          cloudSpec,
		Config:         modelCfg,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &k8sStore{broker: broker}, nil
}

func cloudSpecForModel(m provider.Model) (cloudspec.CloudSpec, error) {
	c, err := m.Cloud()
	if err != nil {
		return cloudspec.CloudSpec{}, errors.Trace(err)
	}
	cloudCredential, err := m.CloudCredential()
	if err != nil {
		return cloudspec.CloudSpec{}, errors.Trace(err)
	}
	if cloudCredential == nil {
		return cloudspec.CloudSpec{}, errors.NotValidf("cloud credential for %s is empty", m.UUID())
	}
	return cloudspec.MakeCloudSpec(c, "", cloudCredential)
}

type k8sStore struct {
	broker caas.SecretsStore
}

// GetContent implements SecretsStore.
func (k k8sStore) GetContent(ctx context.Context, providerId string) (secrets.SecretValue, error) {
	return k.broker.GetJujuSecret(ctx, providerId)
}

// DeleteContent implements SecretsStore.
func (k k8sStore) DeleteContent(ctx context.Context, providerId string) error {
	return k.broker.DeleteJujuSecret(ctx, providerId)
}

// SaveContent implements SecretsStore.
func (k k8sStore) SaveContent(ctx context.Context, uri *secrets.URI, revision int, value secrets.SecretValue) (string, error) {
	return k.broker.SaveJujuSecret(ctx, uri.Name(revision), value)
}
