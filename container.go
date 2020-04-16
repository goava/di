package di

import (
	"fmt"
	"reflect"
)

// New constructs container with provided options. Example usage (simplified):
//
// Define constructors and invocations:
//
// 	func NewHTTPServer(mux *http.ServeMux) *http.Server {
// 		return &http.Server{
// 			Handler: mux,
// 		}
// 	}
//
// 	func NewHTTPServeMux() *http.ServeMux {
// 		return http.ServeMux{}
// 	}
//
// 	func StartServer(server *http.Server) error {
//		return server.ListenAndServe()
//	}
//
// Use it with container:
//
// 	container, err := di.New(
// 		di.Provide(NewHTTPServer),
// 		di.Provide(NewHTTPServeMux),
//		di.Invoke(StartServer),
// 	)
// 	if err != nil {
//		// handle error
//	}
func New(options ...Option) (_ *Container, err error) {
	c := &Container{
		logger:     nopLogger{},
		providers:  map[id]provider{},
		values:     map[id]reflect.Value{},
		prototypes: map[id]bool{},
		cleanups:   []func(){},
	}
	// apply container options
	for _, opt := range options {
		opt.apply(c)
	}
	// initial providing
	provideErr := errProvideFailed{}
	for _, provide := range c.initial.provides {
		if err := c.Provide(provide.constructor, provide.options...); err != nil {
			provideErr = provideErr.Append(provide.frame, err)
		}
	}
	if len(provideErr) > 0 {
		return nil, provideErr
	}
	// provide container to advanced usage e.g. conditional providing
	_ = c.Provide(func() *Container { return c })
	// error omitted because if logger could not be resolved it will be default
	_ = c.Resolve(&c.logger)
	// initial invokes
	for _, invoke := range c.initial.invokes {
		if err := c.Invoke(invoke.fn, invoke.options...); err != nil {
			return nil, errInvokeFailed{invoke.frame, err}
		}
	}
	// initial resolves
	for _, resolve := range c.initial.resolves {
		if err := c.Resolve(resolve.target, resolve.options...); err != nil {
			return nil, errResolveFailed{resolve.frame, err}
		}
	}
	return c, nil
}

// Container is a dependency injection container.
type Container struct {
	// Logger that logs internal actions.
	logger Logger
	// Initial options will be processed on di.New().
	initial struct {
		// Array of di.Provide() options.
		provides []provideOptions
		// Array of di.Invoke() options.
		invokes []invokeOptions
		// Array of di.Resolve() options.
		resolves []resolveOptions
	}
	// Mapping from id to provider that can provide value for that id.
	providers map[id]provider
	// Mapping from id to already instantiated value for that id.
	values map[id]reflect.Value
	// Prototype mapping.
	prototypes map[id]bool
	// Array of provider cleanups.
	cleanups []func()
}

// Provide provides to container reliable way to build type. The constructor will be invoked lazily on-demand.
// For more information about constructors see Constructor interface. ProvideOption can add additional behavior to
// the process of type resolving.
func (c *Container) Provide(constructor Constructor, options ...ProvideOption) error {
	params := ProvideParams{}
	// apply provide options
	for _, opt := range options {
		opt.apply(&params)
	}
	// create constructor provider
	prov, err := newProviderConstructor(params.Name, constructor)
	if err != nil {
		return err
	}
	cleanup := prov.ctorType == ctorCleanup || prov.ctorType == ctorCleanupError
	if cleanup && params.IsPrototype {
		return fmt.Errorf("%s: cleanup not supported with prototype providers", prov.ID())
	}
	if _, ok := c.providers[prov.ID()]; ok {
		// duplicate types not allowed
		return fmt.Errorf("%s already exists in dependency graph", prov.ID())
	}
	c.providers[prov.ID()] = prov
	// save prototype flag
	c.prototypes[prov.ID()] = params.IsPrototype
	// process di.As() options and create group of interfaces
	if err := c.processInterfaces(prov, params.Interfaces); err != nil {
		return err
	}
	return nil
}

func (c *Container) processInterfaces(prov provider, interfaces []Interface) error {
	iprovs := make([]*providerInterface, 0, len(interfaces))
	// process interfaces
	for _, i := range interfaces {
		// create interface provider
		iprov, err := newProviderInterface(prov, i)
		if err != nil {
			return err
		}
		existing, ok := c.providers[iprov.ID()]
		if !ok {
			c.providers[iprov.ID()] = iprov
		}
		// if provider already exists resolve it as interface restricted, but it can exists in group
		_, alreadyStub := existing.(*providerStub)
		if ok && !alreadyStub {
			stub := newProviderStub(iprov.ID(), "have several implementations")
			c.providers[iprov.ID()] = stub
		}
		iprovs = append(iprovs, iprov)
	}
	// process group for interfaces
	for _, iprov := range iprovs {
		groupID := id{
			Type: reflect.SliceOf(iprov.ID().Type),
		}
		existing, ok := c.providers[groupID]
		if ok {
			// if group node already exists use it
			existing.(*providerGroup).Add(prov.ID())
		}
		if !ok {
			group := newProviderGroup(iprov.ID())
			group.Add(prov.ID())
			c.providers[groupID] = group
		}
	}
	return nil
}

// Resolve resolves type and fills target pointer.
//
//	var server *http.Server
//	if err := container.Resolve(&server); err != nil {
//		// handle error
//	}
func (c *Container) Resolve(into interface{}, options ...ResolveOption) error {
	if into == nil {
		return fmt.Errorf("resolve target must be a pointer, got nil")
	}
	if reflect.ValueOf(into).Kind() != reflect.Ptr {
		return fmt.Errorf("resolve target must be a pointer, got %s", reflect.TypeOf(into))
	}
	params := ResolveParams{}
	// apply extract options
	for _, opt := range options {
		opt.apply(&params)
	}
	typ := reflect.TypeOf(into).Elem()
	param := parameter{
		name: params.Name,
		typ:  typ,
	}
	value, err := param.ResolveValue(c)
	if err != nil {
		return err
	}
	targetValue := reflect.ValueOf(into).Elem()
	targetValue.Set(value)
	return nil
}

// Invoke calls the function fn. It parses function parameters. Looks for it in a container.
// And invokes function with them. See Invocation for details.
func (c *Container) Invoke(fn Invocation, options ...InvokeOption) error {
	// params := InvokeParams{}
	// for _, opt := range options {
	// 	opt.apply(&params)
	// }
	invoker, err := newInvoker(fn)
	if err != nil {
		return err
	}
	return invoker.Invoke(c)
}

// Has checks that type exists in container, if not it return false.
//
// 	var server *http.Server
//	if container.Has(&server) {
//		// handle server existence
//	}
//
// It like Resolve() but doesn't instantiate a type.
func (c *Container) Has(into interface{}, options ...ResolveOption) bool {
	if into == nil {
		return false
	}
	params := ResolveParams{}
	// apply options
	for _, opt := range options {
		opt.apply(&params)
	}
	typ := reflect.TypeOf(into)
	param := parameter{
		name: params.Name,
		typ:  typ.Elem(),
	}
	_, exists := param.ResolveProvider(c)
	return exists
}

// Cleanup runs destructors in reverse order that was been created.
func (c *Container) Cleanup() {
	for i := len(c.cleanups) - 1; i >= 0; i-- {
		c.cleanups[i]()
	}
}
