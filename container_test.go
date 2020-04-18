package di_test

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/goava/di"
)

func TestContainer_Resolve(t *testing.T) {
	t.Run("resolve into nil cause error", func(t *testing.T) {
		c := NewTestContainer(t)
		err := c.Resolve(nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": resolve target must be a pointer, got nil")
	})

	t.Run("resolve into struct cause error", func(t *testing.T) {
		c := NewTestContainer(t)
		err := c.Resolve(struct{}{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": resolve target must be a pointer, got struct {}")
	})

	t.Run("resolve into string cause error", func(t *testing.T) {
		c := NewTestContainer(t)
		err := c.Resolve("string")
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": resolve target must be a pointer, got string")
	})

	t.Run("dependency not exists", func(t *testing.T) {
		c := NewTestContainer(t)
		c.MustProvide(func(int) int32 { return 0 })
		var i int32
		err := c.Resolve(&i)
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": int32: dependency int not exists in container")
	})

	t.Run("dependency constructing failed", func(t *testing.T) {
		c := NewTestContainer(t)
		c.MustProvide(func() (*http.Server, error) {
			return &http.Server{}, fmt.Errorf("server build failed")
		})
		var server *http.Server
		err := c.Resolve(&server)
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": *http.Server: server build failed")
	})

	t.Run("container resolve correct pointer", func(t *testing.T) {
		c := NewTestContainer(t)
		server := &http.Server{}
		c.MustProvide(func() *http.Server { return server })
		var extracted *http.Server
		c.MustResolvePtr(server, &extracted)
	})

	t.Run("container resolve same pointer on each resolve", func(t *testing.T) {
		c := NewTestContainer(t)
		c.MustProvide(func() *http.Server {
			return &http.Server{}
		})
		var extracted1 *http.Server
		c.MustResolve(&extracted1)
		var extracted2 *http.Server
		c.MustResolve(&extracted2)
		c.MustEqualPointer(extracted1, extracted2)
	})

	t.Run("container resolve same pointer for type and interface", func(t *testing.T) {
		c := NewTestContainer(t)
		mux := &http.ServeMux{}
		c.MustProvide(func() *http.ServeMux { return mux })
	})

	t.Run("resolve not existing type cause error", func(t *testing.T) {
		c, err := di.New(
			di.Resolve(&http.Server{}),
		)
		require.Nil(t, c)
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": type http.Server not exists in container")
	})

	t.Run("container resolve", func(t *testing.T) {
		var resolved *di.Container
		c, err := di.New(
			di.Resolve(&resolved),
		)
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("%p", c), fmt.Sprintf("%p", resolved))
	})

	t.Run("cycle cause error", func(t *testing.T) {
		c := NewTestContainer(t)
		// bool -> int32 -> int64 -> bool
		err := c.Provide(func(int32) bool { return true })
		require.NoError(t, err)
		err = c.Provide(func(int64) int32 { return 0 })
		require.NoError(t, err)
		err = c.Provide(func(bool) int64 { return 0 })
		require.NoError(t, err)
		var b bool
		err = c.Resolve(&b)
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": cycle detected") // todo: improve message
	})
}

func TestContainer_Resolve_Name(t *testing.T) {
	t.Run("resolve named definition", func(t *testing.T) {
		c := NewTestContainer(t)
		foo := &http.Server{}
		err := c.Provide(func() *http.Server { return foo }, di.WithName("server"))
		require.NoError(t, err)
		var extracted *http.Server
		err = c.Resolve(&extracted)
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": type *http.Server not exists in container")
		err = c.Resolve(&extracted, di.Name("server"))
		require.NoError(t, err)
		c.MustEqualPointer(foo, extracted)
	})
}

func TestContainer_Resolve_Interface(t *testing.T) {
	t.Run("resolve interface with several implementations cause error", func(t *testing.T) {
		c := NewTestContainer(t)
		c.MustProvide(func() *http.Server { return &http.Server{} }, new(io.Closer))
		c.MustProvide(func() *os.File { return &os.File{} }, new(io.Closer))
		var closer io.Closer
		err := c.Resolve(&closer)
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": io.Closer: have several implementations")
	})

	t.Run("resolve constructor argument", func(t *testing.T) {
		c := NewTestContainer(t)
		mux := &http.ServeMux{}
		c.MustProvide(func() *http.ServeMux { return mux }, new(http.Handler))
		c.MustProvide(func(handler http.Handler) *http.Server {
			return &http.Server{Handler: handler}
		})
		var server *http.Server
		c.MustResolve(&server)
		c.MustEqualPointer(mux, server.Handler)
	})
}

func TestContainer_Prototype(t *testing.T) {
	t.Run("container resolve new instance of prototype by each resolve", func(t *testing.T) {
		c := NewTestContainer(t)
		err := c.Provide(func() *http.Server { return &http.Server{} }, di.Prototype())
		require.NoError(t, err)
		var extracted1 *http.Server
		c.MustResolve(&extracted1)
		var extracted2 *http.Server
		c.MustResolve(&extracted2)
		c.MustNotEqualPointer(extracted1, extracted2)
	})
}

func TestContainer_Group(t *testing.T) {
	t.Run("create group and resolve it", func(t *testing.T) {
		c := NewTestContainer(t)
		server := &http.Server{}
		file := &os.File{}
		c.MustProvide(func() *http.Server { return server }, new(io.Closer))
		c.MustProvide(func() *os.File { return file }, new(io.Closer))
		var group []io.Closer
		c.MustResolve(&group)
		require.Len(t, group, 2)
		c.MustEqualPointer(server, group[0])
		c.MustEqualPointer(file, group[1])
	})

	t.Run("resolve group argument", func(t *testing.T) {
		c := NewTestContainer(t)
		server := &http.Server{}
		file := &os.File{}
		c.MustProvide(func() *http.Server { return server }, new(io.Closer))
		c.MustProvide(func() *os.File { return file }, new(io.Closer))
		type Closers []io.Closer
		c.MustProvide(func(closers []io.Closer) Closers { return closers })
		var closers Closers
		c.MustResolve(&closers)
		c.MustEqualPointer(server, closers[0])
		c.MustEqualPointer(file, closers[1])
	})

	t.Run("incorrect signature", func(t *testing.T) {
		c := NewTestContainer(t)
		err := c.Invoke(func() *http.Server { return &http.Server{} })
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": invalid invocation signature, got func() *http.Server")
	})
}

func TestContainer_Invoke(t *testing.T) {
	t.Run("invocation function with not provided dependency cause error", func(t *testing.T) {
		c := NewTestContainer(t)
		err := c.Invoke(func(server *http.Server) {})
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": type *http.Server not exists in container")
	})

	t.Run("invoke with nil error must be called", func(t *testing.T) {
		c := NewTestContainer(t)
		var invokeCalled bool
		c.MustInvoke(func() error {
			invokeCalled = true
			return nil
		})
		require.True(t, invokeCalled)
	})

	t.Run("resolve dependencies in invoke", func(t *testing.T) {
		c := NewTestContainer(t)
		server := &http.Server{}
		called := false
		c.MustProvide(func() *http.Server { return server })
		c.MustInvoke(func(in *http.Server) {
			called = true
			c.MustEqualPointer(server, in)
		})
		require.True(t, called)
	})

	t.Run("invoke return error as is", func(t *testing.T) {
		c := NewTestContainer(t)
		err := c.Invoke(func() error { return fmt.Errorf("invoke error") })
		require.EqualError(t, err, "invoke error")
	})

	t.Run("cycle cause error", func(t *testing.T) {
		c := NewTestContainer(t)
		// bool -> int32 -> int64 -> bool
		err := c.Provide(func(int32) bool { return true })
		require.NoError(t, err)
		err = c.Provide(func(int64) int32 { return 0 })
		require.NoError(t, err)
		err = c.Provide(func(bool) int64 { return 0 })
		require.NoError(t, err)
		err = c.Invoke(func(bool) {})
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": cycle detected") // todo: improve message
	})
}

func TestContainer_Provide(t *testing.T) {
	t.Run("simple constructor", func(t *testing.T) {
		c := NewTestContainer(t)
		c.MustProvide(func() *http.Server { return &http.Server{} })
	})

	t.Run("constructor with error", func(t *testing.T) {
		c := NewTestContainer(t)
		c.MustProvide(func() (*http.Server, error) { return &http.Server{}, nil })
	})

	t.Run("constructor with cleanup function", func(t *testing.T) {
		c := NewTestContainer(t)
		c.MustProvide(func() (*http.Server, func()) {
			return &http.Server{}, func() {}
		})
	})

	t.Run("constructor with cleanup and error", func(t *testing.T) {
		c := NewTestContainer(t)
		c.MustProvide(func() (*http.Server, func(), error) {
			return &http.Server{}, func() {}, nil
		})
	})

	t.Run("provide string cause error", func(t *testing.T) {
		c := NewTestContainer(t)
		err := c.Provide("string")
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": invalid constructor signature, got string")
	})

	t.Run("provide nil cause error", func(t *testing.T) {
		c := NewTestContainer(t)
		err := c.Provide(nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": invalid constructor signature, got nil")
	})

	t.Run("provide struct pointer cause error", func(t *testing.T) {
		c := NewTestContainer(t)
		err := c.Provide(&http.Server{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": invalid constructor signature, got *http.Server")
	})

	t.Run("provide constructor without result cause error", func(t *testing.T) {
		c := NewTestContainer(t)
		err := c.Provide(func() {})
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": invalid constructor signature, got func()")
	})

	t.Run("provide constructor with many resultant types cause error", func(t *testing.T) {
		c := NewTestContainer(t)
		ctor := func() (*http.Server, *http.ServeMux, error) {
			return nil, nil, nil
		}
		err := c.Provide(ctor)
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": invalid constructor signature, got func() (*http.Server, *http.ServeMux, error)")
	})

	t.Run("provide constructor with incorrect result error", func(t *testing.T) {
		c := NewTestContainer(t)
		ctor := func() (*http.Server, *http.ServeMux) {
			return nil, nil
		}
		err := c.Provide(ctor)
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), "invalid constructor signature, got func() (*http.Server, *http.ServeMux)")
	})

	t.Run("provide duplicate", func(t *testing.T) {
		c := NewTestContainer(t)
		ctor := func() *http.Server { return nil }
		c.MustProvide(ctor)
		err := c.Provide(ctor)
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": *http.Server already exists in dependency graph")
	})

	t.Run("provide as not implemented interface cause error", func(t *testing.T) {
		c := NewTestContainer(t)
		// http server not implement io.Reader interface
		err := c.Provide(func() *http.Server { return nil }, di.As(new(io.Reader)))
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": *http.Server not implement io.Reader")
	})

	t.Run("using not interface type in di.As() cause error", func(t *testing.T) {
		c := NewTestContainer(t)
		err := c.Provide(func() *http.Server { return nil }, di.As(&http.Server{}))
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": *http.Server: not a pointer to interface")
	})

	t.Run("using nil type in di.As() cause error", func(t *testing.T) {
		c := NewTestContainer(t)
		err := c.Provide(func() *http.Server { return &http.Server{} }, di.As(nil))
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": nil: not a pointer to interface")
	})
}

func TestContainer_Has(t *testing.T) {
	t.Run("exists on not compiled container return false", func(t *testing.T) {
		c := NewTestContainer(t)
		require.False(t, c.Has(nil))
	})
	t.Run("exists nil returns false", func(t *testing.T) {
		c := NewTestContainer(t)
		require.False(t, c.Has(nil))
	})
	t.Run("exists return true if type exists", func(t *testing.T) {
		c := NewTestContainer(t)
		c.MustProvide(func() *http.Server { return &http.Server{} })
		var server *http.Server
		require.True(t, c.Has(&server))
	})

	t.Run("exists return false if type not exists", func(t *testing.T) {
		c := NewTestContainer(t)
		var server *http.Server
		require.False(t, c.Has(&server))
	})

	t.Run("exists interface", func(t *testing.T) {
		c := NewTestContainer(t)
		c.MustProvide(func() *http.Server { return &http.Server{} }, new(io.Closer))
		var server io.Closer
		require.True(t, c.Has(&server))
	})

	t.Run("exists named provider", func(t *testing.T) {
		c := NewTestContainer(t)
		err := c.Provide(func() *http.Server { return &http.Server{} }, di.WithName("server"))
		require.NoError(t, err)
		var server *http.Server
		require.True(t, c.Has(&server, di.Name("server")))
	})
}

func TestContainer_ConstructorResolve(t *testing.T) {
	t.Run("resolve correct argument", func(t *testing.T) {
		c := NewTestContainer(t)
		mux := &http.ServeMux{}
		c.MustProvide(func() *http.ServeMux { return mux })
		c.MustProvide(func(mux *http.ServeMux) *http.Server {
			return &http.Server{Handler: mux}
		})
		var server *http.Server
		c.MustResolve(&server)
		c.MustEqualPointer(mux, server.Handler)
	})
}

func TestContainer_Injectable(t *testing.T) {
	t.Run("constructor with injectable pointer", func(t *testing.T) {
		c := NewTestContainer(t)
		type InjectableType struct {
			di.Inject
			Mux *http.ServeMux `di:""`
		}
		mux := &http.ServeMux{}
		c.MustProvide(func() *http.ServeMux { return mux })
		c.MustProvide(func() *InjectableType { return &InjectableType{} })
		var result *InjectableType
		c.MustResolve(&result)
		require.NotNil(t, result.Mux)
		c.MustEqualPointer(mux, result.Mux)
	})

	t.Run("provide injectable struct cause error", func(t *testing.T) {
		c := NewTestContainer(t)
		type InjectableType struct {
			di.Inject
			Mux *http.ServeMux `di:""`
		}
		mux := &http.ServeMux{}
		c.MustProvide(func() *http.ServeMux { return mux })
		err := c.Provide(func() InjectableType { return InjectableType{} })
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": di.Inject not supported for unaddressable result of constructor, use *di_test.InjectableType instead")
	})

	t.Run("container resolve injectable parameter", func(t *testing.T) {
		c := NewTestContainer(t)
		type Parameters struct {
			di.Inject
			Server *http.Server `di:""`
			File   *os.File     `di:""`
		}
		server := &http.Server{}
		file := &os.File{}
		c.MustProvide(func() *http.Server { return server })
		c.MustProvide(func() *os.File { return file })
		type Result struct {
			server *http.Server
			file   *os.File
		}
		c.MustProvide(func(params Parameters) *Result { return &Result{params.Server, params.File} })
		var extracted *Result
		c.MustResolve(&extracted)
		c.MustEqualPointer(server, extracted.server)
		c.MustEqualPointer(file, extracted.file)
	})

	t.Run("not existing injectable field cause error", func(t *testing.T) {
		c := NewTestContainer(t)
		type InjectableType struct {
			di.Inject
			Mux *http.ServeMux `di:""`
		}
		c.MustProvide(func() *InjectableType { return &InjectableType{} })
		var result *InjectableType
		err := c.Resolve(&result)
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": *di_test.InjectableType: dependency *http.ServeMux not exists in container")
	})

	t.Run("not existing and optional field set to nil", func(t *testing.T) {
		c := NewTestContainer(t)
		type InjectableType struct {
			di.Inject
			Mux *http.ServeMux `di:"" optional:"true"`
		}
		c.MustProvide(func() *InjectableType { return &InjectableType{} })
		var result *InjectableType
		fmt.Println(c.Resolve(&result))
		c.MustResolve(&result)
		require.Nil(t, result.Mux)
	})

	t.Run("nested injectable field resolved correctly", func(t *testing.T) {
		c := NewTestContainer(t)
		type NestedInjectableType struct {
			di.Inject
			Mux *http.ServeMux `di:""`
		}
		type InjectableType struct {
			di.Inject
			Nested NestedInjectableType `di:""`
		}
		mux := &http.ServeMux{}
		c.MustProvide(func() *InjectableType { return &InjectableType{} })
		c.MustProvide(func() *http.ServeMux { return mux })
		var result *InjectableType
		c.MustResolve(&result)
		require.NotNil(t, result.Nested.Mux)
		c.MustEqualPointer(mux, result.Nested.Mux)
		var nit NestedInjectableType
		c.MustResolve(&nit)
		require.NotNil(t, nit)
		c.MustEqualPointer(mux, nit.Mux)
	})

	t.Run("optional parameter may be nil", func(t *testing.T) {
		c := NewTestContainer(t)
		type Parameter struct {
			di.Inject
			Server *http.Server `di:"" optional:"true"`
		}
		type Result struct {
			server *http.Server
		}
		c.MustProvide(func(params Parameter) *Result { return &Result{server: params.Server} })
		var extracted *Result
		c.MustResolve(&extracted)
		require.Nil(t, extracted.server)
	})

	t.Run("optional group may be nil", func(t *testing.T) {
		c := NewTestContainer(t)
		type Params struct {
			di.Inject
			Handlers []http.Handler `di:"optional" optional:"true"`
		}
		c.MustProvide(func(params Params) bool {
			return params.Handlers == nil
		})
		var extracted bool
		c.MustResolve(&extracted)
		require.True(t, extracted)
	})

	t.Run("skip private fields", func(t *testing.T) {
		c := NewTestContainer(t)
		type InjectableParameter struct {
			di.Inject
			private    []http.Handler `di:""`
			Addrs      []net.Addr     `di:"" optional:"true"`
			HaveNotTag string
		}
		type InjectableType struct {
			di.Inject
			private    []http.Handler `di:""`
			Addrs      []net.Addr     `di:"" optional:"true"`
			HaveNotTag string
		}
		c.MustProvide(func(param InjectableParameter) bool {
			return param.Addrs == nil
		})
		c.MustProvide(func() *InjectableType { return &InjectableType{} })
		var extracted bool
		c.MustResolve(&extracted)
		require.True(t, extracted)
		var result *InjectableType
		c.MustResolve(&result)
	})

	t.Run("resolving not provided injectable cause error", func(t *testing.T) {
		c := NewTestContainer(t)
		type Parameter struct {
			di.Inject
			Server *http.Server `di:""`
		}
		var p Parameter
		err := c.Resolve(&p)
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": di_test.Parameter: dependency *http.Server not exists in container")
	})

	t.Run("invoke with inject dependency struct", func(t *testing.T) {
		type InjectableParam struct {
			di.Inject
			Mux *http.ServeMux `di:""`
		}
		c := NewTestContainer(t)
		mux := http.NewServeMux()
		require.NoError(t, c.Provide(func() *http.ServeMux { return mux }))
		err := c.Invoke(func(params InjectableParam) {
			c.MustEqualPointer(mux, params.Mux)
		})
		require.NoError(t, err)
	})

	t.Run("invoke with inject dependency pointer", func(t *testing.T) {
		type InjectableParam struct {
			di.Inject
			Mux *http.ServeMux `di:""`
		}
		c := NewTestContainer(t)
		mux := http.NewServeMux()
		require.NoError(t, c.Provide(func() *http.ServeMux { return mux }))
		err := c.Invoke(func(params *InjectableParam) {})
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": type *di_test.InjectableParam not exists in container")
	})
}

func TestContainer_Cleanup(t *testing.T) {
	t.Run("called", func(t *testing.T) {
		c := NewTestContainer(t)
		var cleanupCalled bool
		c.MustProvide(func() (*http.Server, func()) {
			return &http.Server{}, func() { cleanupCalled = true }
		})
		var extracted *http.Server
		c.MustResolve(&extracted)
		c.Cleanup()
		require.True(t, cleanupCalled)
	})

	t.Run("correct order", func(t *testing.T) {
		c := NewTestContainer(t)
		var cleanupCalls []string
		c.MustProvide(func(handler http.Handler) (*http.Server, func()) {
			return &http.Server{Handler: handler}, func() { cleanupCalls = append(cleanupCalls, "server") }
		})
		c.MustProvide(func() (*http.ServeMux, func()) {
			return &http.ServeMux{}, func() { cleanupCalls = append(cleanupCalls, "mux") }
		}, new(http.Handler))
		var server *http.Server
		c.MustResolve(&server)
		c.Cleanup()
		require.Equal(t, []string{"server", "mux"}, cleanupCalls)
	})

	t.Run("cleanup with prototype cause error", func(t *testing.T) {
		c := NewTestContainer(t)
		err := c.Provide(func() (*http.Server, func()) {
			return &http.Server{}, func() {}
		}, di.ProvideParams{
			IsPrototype: true,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "container_test.go:")
		require.Contains(t, err.Error(), ": cleanup not supported with prototype providers")
	})
}

// NewTestContainer
func NewTestContainer(t *testing.T) *TestContainer {
	c, err := di.New()
	require.NoError(t, err)
	return &TestContainer{t, c}
}

// TestContainer
type TestContainer struct {
	t *testing.T
	*di.Container
}

func (c *TestContainer) MustProvide(provider interface{}, as ...di.Interface) {
	err := c.Provide(provider, di.ProvideParams{Interfaces: as})
	require.NoError(c.t, err)
}

func (c *TestContainer) MustResolve(target interface{}) {
	require.NoError(c.t, c.Resolve(target))
}

// MustResolvePtr extract value from container into target and check that target and expected pointers are equal.
func (c *TestContainer) MustResolvePtr(expected, target interface{}) {
	c.MustResolve(target)

	// indirect
	actual := reflect.ValueOf(target).Elem().Interface()
	c.MustEqualPointer(expected, actual)
}

func (c *TestContainer) MustInvoke(fn interface{}) {
	require.NoError(c.t, c.Invoke(fn))
}

func (c *TestContainer) MustEqualPointer(expected interface{}, actual interface{}) {
	require.Equal(c.t,
		fmt.Sprintf("%p", actual),
		fmt.Sprintf("%p", expected),
		"actual and expected pointers should be equal",
	)
}

func (c *TestContainer) MustNotEqualPointer(expected interface{}, actual interface{}) {
	require.NotEqual(c.t,
		fmt.Sprintf("%p", actual),
		fmt.Sprintf("%p", expected),
		"actual and expected pointers should not be equal",
	)
}
