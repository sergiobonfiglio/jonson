package jonson

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime/debug"
	"strings"
	"time"
)

var TypeContext = reflect.TypeOf((**Context)(nil)).Elem()

type Context struct {
	parent        context.Context
	provider      Provider
	methodHandler *MethodHandler
	values        []*valueItem
	finalized     bool
}

type Finalizeable interface {
	Finalize([]error) error
}

type valueItem struct {
	rt    reflect.Type
	val   any
	valid bool
}

func NewContext(parent context.Context, provider Provider, methodHandler *MethodHandler) *Context {
	ctx := &Context{
		parent:        parent,
		provider:      provider,
		methodHandler: methodHandler,
	}
	ctx.StoreValue(TypeContext, ctx)
	return ctx
}

func (c *Context) Fork() *Context {
	return NewContext(c, c.provider, c.methodHandler)
}

func (c *Context) StoreValue(rt reflect.Type, val any) {
	for i := range c.values {
		if c.values[i].rt == rt {
			panic(errors.New("value of type " + rt.String() + " is already stored"))
		}
	}

	c.values = append(c.values, &valueItem{
		rt:    rt,
		val:   val,
		valid: true,
	})
}

// Invalidate invalidates a value @ context.
// The value will be removed from context and needs to be
// re-required. Invalidation might e.g. happen during
// some value changes due to login or register.
// You can invalidate multiple values at once

func (c *Context) Invalidate(rt ...reflect.Type) {
	toInvalidate := map[reflect.Type]struct{}{}
	for _, v := range rt {
		toInvalidate[v] = struct{}{}
	}
	vals := []*valueItem{}
	for _, v := range c.values {
		if _, ok := toInvalidate[v.rt]; ok {
			continue
		}
		vals = append(vals, v)
	}
	c.values = vals
}

func (c *Context) debugRecursionLoop(inst reflect.Type) error {
	out := []string{}
	for _, v := range c.values {
		if !v.valid {
			break
		}
		out = append(out, fmt.Sprintf("%v", v.rt))
	}
	stackTrace := debug.Stack()

	return fmt.Errorf("recursion loop while resolving %v:\n-----------\n%s\n-------------\n%s", inst, strings.Join(out, "\n--> "), string(stackTrace))
}

// func (c *Context) Require[T any]() T {
func (c *Context) Require(inst reflect.Type) any {
	if c.finalized {
		panic(errors.New("context is already finalized"))
	}
	if (inst.Kind() != reflect.Ptr || inst.Elem().Kind() != reflect.Struct) && inst.Kind() != reflect.Interface {
		panic(errors.New("inst must either be a ptr or an interface"))
	}

	for i := range c.values {
		if c.values[i].rt == inst {
			if !c.values[i].valid {
				panic(c.debugRecursionLoop(inst))
			}
			return c.values[i].val
		}
	}

	v := &valueItem{
		rt: inst,
	}
	c.values = append(c.values, v)

	// try to instantiate
	v.val = c.provider.Provide(c, inst)
	v.valid = true
	return v.val
}

func (c *Context) Finalize(err error) error {
	if c.finalized {
		return err
	}
	c.finalized = true

	var errors []error
	if err != nil {
		errors = append(errors, err)
	}

	// finalize from end to front
	for i := len(c.values) - 1; i >= 0; i-- {
		if f, ok := c.values[i].val.(Finalizeable); ok {
			if e := f.Finalize(errors); e != nil {
				errors = append(errors, e)
			}
		}
	}
	c.values = nil

	if len(errors) == 0 {
		return nil
	}

	if len(errors) == 1 && errors[0] == err {
		return err
	}

	// remodel sub errors
	errs := make([]*Error, len(errors))
	for i := range errors {
		if e, ok := errors[i].(*Error); ok {
			errs[i] = e
		} else {
			errs[i] = ErrInternal.CloneWithData(&ErrorData{
				Debug: c.methodHandler.errorEncoder.Encode(errors[i].Error()),
			})
		}
	}

	// return error (we might change to a more specific error code here?)
	return ErrInternal.CloneWithData(&ErrorData{
		Debug:   c.methodHandler.errorEncoder.Encode("finalization failed"),
		Details: []*Error{},
	})
}

func (c *Context) CallMethod(method string, payload any, bindata []byte) (any, error) {
	v, err := c.methodHandler.CallMethod(c, method, payload, bindata)
	if err != nil {
		return nil, err
	}
	if v != nil {
		return v, nil
	}
	return nil, nil
}

// methods below are for fulfilling the go library context.Context interface

var _ (context.Context) = (*Context)(nil)

func (c *Context) Deadline() (time.Time, bool) {
	return c.parent.Deadline()
}

func (c *Context) Done() <-chan struct{} {
	return c.parent.Done()
}

func (c *Context) Err() error {
	return c.parent.Err()
}

func (c *Context) Value(key any) any {
	return c.parent.Value(key)
}
