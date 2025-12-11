// Copyright 2025 BWI GmbH and Artifact Conduit contributors
// SPDX-License-Identifier: Apache-2.0

package rest

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.opendefense.cloud/kit/apiserver/resource"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/names"
)

// errNotAcceptable indicates the resource doesn't support Table conversion.
type errNotAcceptable struct {
	resource schema.GroupResource
}

func (e errNotAcceptable) Error() string {
	return fmt.Sprintf("the resource %s does not support being converted to a Table", e.resource)
}

func (e errNotAcceptable) Status() metav1.Status {
	return metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    http.StatusNotAcceptable,
		Reason:  metav1.StatusReason("NotAcceptable"),
		Message: e.Error(),
	}
}

var swaggerMetadataDescriptions = metav1.ObjectMeta{}.SwaggerDoc()

// Strategy defines the set of hooks and behaviors used by the API server for resource storage operations.
// It combines create, update, delete, and table conversion strategies, plus a predicate matcher for filtering.
type Strategy interface {
	// Match returns a predicate for filtering resources by label and field selectors.
	Match(label labels.Selector, field fields.Selector) storage.SelectionPredicate
	rest.RESTUpdateStrategy
	rest.RESTCreateStrategy
	rest.RESTDeleteStrategy
	rest.TableConvertor
}

var _ Strategy = DefaultStrategy{}

// DefaultStrategy is a generic implementation of Strategy.
// It delegates most behaviors to interfaces implemented by the underlying Object, if present.
// If the Object does not implement an override interface, DefaultStrategy provides a fallback.
type DefaultStrategy struct {
	// Object is the resource instance whose interfaces may override default behaviors.
	Object runtime.Object
	// ObjectTyper provides type information for the resource.
	runtime.ObjectTyper
	// TableConvertor is used for table output if the object does not implement TableConverter.
	TableConvertor rest.TableConvertor
}

// NewDefaultStrategy constructs a DefaultStrategy for a given resource type.
// obj: a sample instance of the resource
// objTyper: type information provider
// gr: group/resource descriptor for table conversion
func NewDefaultStrategy(obj runtime.Object, objTyper runtime.ObjectTyper, gr schema.GroupResource) *DefaultStrategy {
	return &DefaultStrategy{
		Object:         obj,
		ObjectTyper:    objTyper,
		TableConvertor: rest.NewDefaultTableConvertor(gr),
	}
}

// GenerateName returns a generated name for a resource, using the object's NameGenerator if present.
func (d DefaultStrategy) GenerateName(base string) string {
	if d.Object == nil {
		return names.SimpleNameGenerator.GenerateName(base)
	}
	if n, ok := d.Object.(NameGenerator); ok {
		return n.GenerateName(base)
	}
	return names.SimpleNameGenerator.GenerateName(base)
}

// NamespaceScoped returns true if the resource is namespaced, using the object's Scoper if present.
func (d DefaultStrategy) NamespaceScoped() bool {
	if d.Object == nil {
		return true
	}
	if n, ok := d.Object.(Scoper); ok {
		return n.NamespaceScoped()
	}
	return true
}

// PrepareForCreate normalizes the object before creation, delegating to PrepareForCreater if implemented.
func (DefaultStrategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
	if v, ok := obj.(PrepareForCreater); ok {
		v.PrepareForCreate(ctx)
	}
}

// PrepareForUpdate normalizes the object before update.
// If the object has a status subresource, status is copied from old to new.
// If PrepareForUpdater is implemented, it is called to further normalize.
func (DefaultStrategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
	if v, ok := obj.(resource.ObjectWithStatusSubResource); ok {
		// Copy status from old to new to avoid spec-only updates modifying status.
		old.(resource.ObjectWithStatusSubResource).CopyStatusTo(v)
	}
	if v, ok := obj.(PrepareForUpdater); ok {
		v.PrepareForUpdate(ctx, old)
	}
}

// Validate delegates to the object's Validater interface if present, otherwise returns no errors.
func (DefaultStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	if v, ok := obj.(Validater); ok {
		return v.Validate(ctx)
	}
	return field.ErrorList{}
}

// AllowCreateOnUpdate returns true if the object allows creation via update (PUT), using AllowCreateOnUpdater if present.
func (d DefaultStrategy) AllowCreateOnUpdate() bool {
	if d.Object == nil {
		return false
	}
	if n, ok := d.Object.(AllowCreateOnUpdater); ok {
		return n.AllowCreateOnUpdate()
	}
	return false
}

// AllowUnconditionalUpdate returns true if the object allows unconditional updates, using AllowUnconditionalUpdater if present.
func (d DefaultStrategy) AllowUnconditionalUpdate() bool {
	if d.Object == nil {
		return false
	}
	if n, ok := d.Object.(AllowUnconditionalUpdater); ok {
		return n.AllowUnconditionalUpdate()
	}
	return false
}

// Canonicalize mutates the object into a canonical form if Canonicalizer is implemented.
func (DefaultStrategy) Canonicalize(obj runtime.Object) {
	if c, ok := obj.(Canonicalizer); ok {
		c.Canonicalize()
	}
}

// ValidateUpdate delegates to the object's ValidateUpdater interface if present, otherwise returns no errors.
func (DefaultStrategy) ValidateUpdate(ctx context.Context, obj, old runtime.Object) field.ErrorList {
	if v, ok := obj.(ValidateUpdater); ok {
		return v.ValidateUpdate(ctx, old)
	}
	return field.ErrorList{}
}

// Match returns a SelectionPredicate for filtering resources by label and field selectors.
func (DefaultStrategy) Match(label labels.Selector, field fields.Selector) storage.SelectionPredicate {
	return storage.SelectionPredicate{
		Label:    label,
		Field:    field,
		GetAttrs: GetAttrs,
	}
}

// ConvertToTable returns a Table representation of the object, using TableConverter if implemented.
func (d DefaultStrategy) ConvertToTable(
	ctx context.Context, obj runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	if c, ok := obj.(TableConverter); ok {
		// Object implements our TableConverter, so let it do the work on it's own.
		return c.ConvertToTable(ctx, tableOptions)
	}
	// We will do it DefaultStrategy here.
	var table metav1.Table
	fn := func(obj runtime.Object) error {
		m, err := meta.Accessor(obj)
		if err != nil {
			gr := schema.GroupResource{}
			if info, ok := genericapirequest.RequestInfoFrom(ctx); ok {
				gr = schema.GroupResource{Group: info.APIGroup, Resource: info.Resource}
			}
			return errNotAcceptable{resource: gr}
		}
		table.Rows = append(table.Rows, metav1.TableRow{
			Cells:  []interface{}{m.GetName(), m.GetCreationTimestamp().Time.UTC().Format(time.RFC3339)},
			Object: runtime.RawExtension{Object: obj},
		})
		return nil
	}
	switch {
	case meta.IsListType(obj):
		if err := meta.EachListItem(obj, fn); err != nil {
			return nil, err
		}
	default:
		if err := fn(obj); err != nil {
			return nil, err
		}
	}
	if m, err := meta.ListAccessor(obj); err == nil {
		table.ResourceVersion = m.GetResourceVersion()
		table.Continue = m.GetContinue()
		table.RemainingItemCount = m.GetRemainingItemCount()
	} else {
		if m, err := meta.CommonAccessor(obj); err == nil {
			table.ResourceVersion = m.GetResourceVersion()
		}
	}
	if opt, ok := tableOptions.(*metav1.TableOptions); !ok || !opt.NoHeaders {
		table.ColumnDefinitions = []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name", Description: swaggerMetadataDescriptions["name"]},
			{Name: "Created At", Type: "date", Description: swaggerMetadataDescriptions["creationTimestamp"]},
		}
	}
	return &table, nil
}

// WarningsOnCreate returns any warnings for create operations (default: none).
func (d DefaultStrategy) WarningsOnCreate(ctx context.Context, obj runtime.Object) []string {
	return nil
}

// WarningsOnUpdate returns any warnings for update operations (default: none).
func (d DefaultStrategy) WarningsOnUpdate(ctx context.Context, obj, old runtime.Object) []string {
	return nil
}

// PrepareForUpdaterStrategy is a wrapper for RESTUpdateStrategy that allows custom update normalization via OverrideFn.
type PrepareForUpdaterStrategy struct {
	rest.RESTUpdateStrategy
	// OverrideFn is called to perform custom normalization during update.
	OverrideFn func(ctx context.Context, obj, old runtime.Object)
}

// PrepareForUpdate calls the custom OverrideFn if set, otherwise does nothing.
func (s *PrepareForUpdaterStrategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
	if s.OverrideFn != nil {
		s.OverrideFn(ctx, obj, old)
	}
}
