/*
Copyright 2021 The Crossplane Authors.

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

package v1alpha1

import (
	"context"
	"github.com/crossplane/crossplane-runtime/pkg/reference"
	iamv1beta1 "github.com/crossplane/provider-aws/apis/identity/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResolveReferences of DBCluster
func (mg *DBCluster)  ResolveReferences(ctx context.Context, c client.Reader) error {
	r := reference.NewAPIResolver(c, mg)

	//rsp, err := r.Resolve(ctx, reference.ResolutionRequest{
	//	CurrentValue: reference.FromPtrValue(mg.Spec.ForProvider.DBSubnetGroupName),
	//	Reference:    mg.GetProviderConfigReference(),
	//	//Selector:     mg.Spec.ForProvider.
	//	To:           reference.To{Managed: &DBC, List: &DBSubnetGroupList{}},
	//	Extract:      iamv1beta1.IAMRoleARN(),
	//})
	//
	//if err != nil {
	//	return errors.Wrap(err, "spec.forProvider.roleArn")
	//}

	return nil
}