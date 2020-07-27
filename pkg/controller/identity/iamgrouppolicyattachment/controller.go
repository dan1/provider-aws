/*
Copyright 2019 The Crossplane Authors.

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

package iamgrouppolicyattachment

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsiam "github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/crossplane/provider-aws/apis/identity/v1alpha1"
	awsv1alpha3 "github.com/crossplane/provider-aws/apis/v1alpha3"
	awsclients "github.com/crossplane/provider-aws/pkg/clients"
	"github.com/crossplane/provider-aws/pkg/clients/iam"
)

const (
	errNotGroupInstance = "managed resource is not an Group instance custom resource"

	errCreateGroupClient = "cannot create IAM Group client"
	errGetProvider       = "cannot get provider"
	errGetProviderSecret = "cannot get provider secret"

	errUnexpectedObject = "The managed resource is not an GroupPolicyAttachment resource"
	errGet              = "failed to get GroupPolicyAttachments for group"
	errAttach           = "failed to attach the policy to group"
	errDetach           = "failed to detach the policy to group"
)

// SetupIAMGroupPolicyAttachment adds a controller that reconciles
// IAMGroupPolicyAttachments.
func SetupIAMGroupPolicyAttachment(mgr ctrl.Manager, l logging.Logger) error {
	name := managed.ControllerName(v1alpha1.IAMGroupPolicyAttachmentGroupKind)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		For(&v1alpha1.IAMGroupPolicyAttachment{}).
		Complete(managed.NewReconciler(mgr,
			resource.ManagedKind(v1alpha1.IAMGroupPolicyAttachmentGroupVersionKind),
			managed.WithExternalConnecter(&connector{kube: mgr.GetClient(), newClientFn: iam.NewGroupPolicyAttachmentClient}),
			managed.WithConnectionPublishers(),
			managed.WithReferenceResolver(managed.NewAPISimpleReferenceResolver(mgr.GetClient())),
			managed.WithInitializers(),
			managed.WithLogger(l.WithValues("controller", name)),
			managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name)))))
}

type connector struct {
	kube        client.Client
	newClientFn func(ctx context.Context, credentials []byte, region string, auth awsclients.AuthMethod) (iam.GroupPolicyAttachmentClient, error)
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.IAMGroupPolicyAttachment)
	if !ok {
		return nil, errors.New(errNotGroupInstance)
	}

	p := &awsv1alpha3.Provider{}
	if err := c.kube.Get(ctx, types.NamespacedName{Name: cr.Spec.ProviderReference.Name}, p); err != nil {
		return nil, errors.Wrap(err, errGetProvider)
	}

	if aws.BoolValue(p.Spec.UseServiceAccount) {
		groupPolicyClient, err := c.newClientFn(ctx, []byte{}, p.Spec.Region, awsclients.UsePodServiceAccount)
		return &external{client: groupPolicyClient, kube: c.kube}, errors.Wrap(err, errCreateGroupClient)
	}

	if p.GetCredentialsSecretReference() == nil {
		return nil, errors.New(errGetProviderSecret)
	}

	s := &corev1.Secret{}
	n := types.NamespacedName{Namespace: p.Spec.CredentialsSecretRef.Namespace, Name: p.Spec.CredentialsSecretRef.Name}
	if err := c.kube.Get(ctx, n, s); err != nil {
		return nil, errors.Wrap(err, errGetProviderSecret)
	}

	groupClient, err := c.newClientFn(ctx, s.Data[p.Spec.CredentialsSecretRef.Key], p.Spec.Region, awsclients.UseProviderSecret)
	return &external{client: groupClient, kube: c.kube}, errors.Wrap(err, errCreateGroupClient)
}

type external struct {
	client iam.GroupPolicyAttachmentClient
	kube   client.Client
}

func (e *external) Observe(ctx context.Context, mgd resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mgd.(*v1alpha1.IAMGroupPolicyAttachment)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errUnexpectedObject)
	}

	observed, err := e.client.ListAttachedGroupPoliciesRequest(&awsiam.ListAttachedGroupPoliciesInput{
		GroupName: &cr.Spec.ForProvider.GroupName,
	}).Send(ctx)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(resource.Ignore(iam.IsErrorNotFound, err), errGet)
	}

	var attachedPolicyObject *awsiam.AttachedPolicy
	for i, policy := range observed.AttachedPolicies {
		if cr.Spec.ForProvider.PolicyARN == aws.StringValue(policy.PolicyArn) {
			attachedPolicyObject = &observed.AttachedPolicies[i]
			break
		}
	}

	if attachedPolicyObject == nil {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	cr.SetConditions(runtimev1alpha1.Available())

	cr.Status.AtProvider = v1alpha1.IAMGroupPolicyAttachmentObservation{
		AttachedPolicyARN: aws.StringValue(attachedPolicyObject.PolicyArn),
	}

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: true,
	}, nil
}

func (e *external) Create(ctx context.Context, mgd resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mgd.(*v1alpha1.IAMGroupPolicyAttachment)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errUnexpectedObject)
	}

	cr.SetConditions(runtimev1alpha1.Creating())

	_, err := e.client.AttachGroupPolicyRequest(&awsiam.AttachGroupPolicyInput{
		PolicyArn: &cr.Spec.ForProvider.PolicyARN,
		GroupName: &cr.Spec.ForProvider.GroupName,
	}).Send(ctx)

	return managed.ExternalCreation{}, errors.Wrap(err, errAttach)
}

func (e *external) Update(_ context.Context, _ resource.Managed) (managed.ExternalUpdate, error) {
	// Updating any field will create a new Group-Policy attachment in AWS, which will be
	// irrelevant/out-of-sync to the original defined attachment.
	// It is encouraged to instead create a new IAMGroupPolicyAttachment resource.
	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, mgd resource.Managed) error {
	cr, ok := mgd.(*v1alpha1.IAMGroupPolicyAttachment)
	if !ok {
		return errors.New(errUnexpectedObject)
	}

	cr.Status.SetConditions(runtimev1alpha1.Deleting())

	_, err := e.client.DetachGroupPolicyRequest(&awsiam.DetachGroupPolicyInput{
		PolicyArn: &cr.Spec.ForProvider.PolicyARN,
		GroupName: &cr.Spec.ForProvider.GroupName,
	}).Send(ctx)

	if iam.IsErrorNotFound(err) {
		return nil
	}

	return errors.Wrap(err, errDetach)
}