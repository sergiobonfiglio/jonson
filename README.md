# Jonson

A library allowing you to expose API endpoints using [JSON-RPC 2.0](https://www.jsonrpc.org/specification).

You will be able to expose functions either using:

- a http endpoint per rpc
- a single http endpoint serving all calls
- websocket

In order to do so, jonson exists of:

- a server which exposes either the http endpoint(s) and/or a websocket connection
- a factory which allows you to _provide_ functionality to your API endpoints
- parameter validation (coming soon)
- error message encryption/decryption to hide sensitive information from the client

## Project structure

Jonson thinks in _systems_.
A system is a set of things which as a whole form emergency.
Systems also tend to interact with other systems. As a result, we would be talking
of a _system of systems_.

Let's assume an auth service (a system by itself).
An auth system consists of authorization and authentication (system of systems.).
The ideal folder structure for a jonson project following the systemic approach would
look something like this:

```txt
/<project-name>
  /cmd
    /server
      main.go
  /internal
    /systems
      /authorization
        authorization.go
      /authentication
        authentication.go
  
go.mod
```

## Remote procedure calls

When following the systemic approach, we can now start implementing our remote procedure calls.
Let's follow the example of an auth service.
The authentication endpoint might need functions like register, login and logout.
Within the autentication/authentication.go folder, we can now set up our remote procedure calls.

The remote procedure calls will be generated by the server (explained later).
In order to expose the endpoints properly, we need to follow a naming scheme:
`<MethodName>V<version>`.

A remote procedure call accepts parameters (optional) and returns a result (optional) _or_ an error.

To detect parameters which need to be marshaled/unmarshaled during the request,
add a `jonson.Params` interface within your parameters which you will be sending.

```go

// Authentication is our authentication system
type Authentication struct {
}

func NewAuthentication() *Authentication{
  return &Authentication{}
}

type RegisterV1Params {
  jonson.Params
  Username string `json:"username"`
  Password string `json:"password"`
}

type RegisterV1Result struct {
  Uuid string `json:"uuid"`
}

// RegisterV1 allows us to register a new account
func (a *Authentication) RegisterV1(ctx *jonson.Context, params *RegisterV1Params) (*RegisterV1Result, error){
  if (len(params.Username) <= 5){
    return nil, jonson.ErrInvalidParams
  }
  // put your register logic here
  return &RegisterV1Result {
    Uuid: "27fd79d0-e776-41c4-809a-3d1865b4f729",
  }, nil
}

type LoginV1Params struct {
  Username string `json:"username"`
  Password string `json:"password"`
}

// LoginV1 allows an account to log in
func (a *Authentication) LoginV1(ctx *jonson.Context, params *LoginV1Params) error{
  // put your login logic here
  return nil
}

// LoginV1 allows an account to log in
func (a *Authentication) LogoutV1(ctx *jonson.Context) error{
  // put your logout logic here
  return nil
}

```

## Factory

Let's assume, the account wants to have access to a database or the current time.
We could provide the database to the Authentication system itself (by passing a parameter to the constructor
and keeping a reference within the Authentication struct) _or_ we start diving into the
possibility to use a factory.
The factory allows us to define certain infrastructure or functional components during startup
and _provide_ those functional components during runtime.

Considering the "auth service" example, we would create a new folder `internal/infra` which we would
use to put our infrastructure setup.
In order to provide the functionality of database access and access to the current time,
we can now create a new InfrastructureProvider:

```go
type InfrastructureProvider struct {
  db *sql.Db
  newTime func() time.Time
}

func NewInfrastructureProvider(db *sql.Db, newTime func() time.Time){
  return &InfrastructureProvider{
    db: db,
    newTime: newTime,
  }
}

// @generate
type DB struct {
  *sql.DB
}

func (i *InfrastructureProvider) NewDB(ctx *jonson.Context) *DB{
  return &DB{
    DB: i.db,
  }
}

// @generate
type Time struct {
  time.Time
}

func (i *InfrastructureProvider) NewTime(ctx *jonson.Context) Time{
  return Time{
    Time: i.newTime()
  }
}
```

In order for the providers to work, we need to follow a specific naming scheme:
the functions providing a type need to start with the keyword "New" followed by the type
the provider instantiates, such as: `NewTime` returning Time.

You might have noticed the `// @generate` tag. When using jonson-generate, functions allowing you
to require the provided types will be generated.
Since we tagged Time and DB in the example above, jonson-generate will create two functions for us:

```go
func RequireTime(ctx *jonson.Context) Time{
  //
}

func RequireDB(ctx *jonson.Context) *DB{
  //
}
```

To register the providers in the factory, use `factory.RegisterProvider` passing the pointer to the InfrastructureProvider.
For details, check out the section "Putting it all together";

In case our provider is really simple, we can also use a single function:

```go
// @generate
type ServiceName string

func ProvideServiceName(ctx *jonson.Context)ServiceName{
  return "auth"
}
```

To register a simple provider function, use `factory.RegisterProviderFunc` passing the pointer to the InfrastructureProvider.
For details, check out the section "Putting it all together";

Once the types are provided by the factory, you can access them in your remote procedure calls:

```go
// LoginV1 allows an account to log in
func (a *Authentication) LoginV1(ctx *jonson.Context, params *LoginV1Params) error{
  // the factory provides the database and we can now access it here in the code
  db := infra.RequireDB(ctx)
  // put your logic here
  return nil
}
```

The generated types will be instantiated _once_ per API call and then stored within the context.
In case a provider became invalid (e.g. we were storing a session provider and the account logged out),
we can call the `context.Invalidate()` method passing the type which we need to invalidate.
The context allows us to also store new values on the fly (e.g. the user logged in and we want to provide a session)
by calling `context.StoreValue`.
NOTE: as a security feature, context.StoreValue will panic in case a provided value already exists;

## Method handler

The method handler parses all system remote procedure calls and exposes methods to call those remote procedure calls.
with `methodHandler.RegisterSystem()`, we can register a system.

For each call, the method handler will also make sure that the factory's providers will be provided to the
called functions.

Besides those functions provided by the factory, the method handler will provide a few infrastructure related
providers, such as:

```go

func RequireHttpRequest(ctx *jonson.Context)*http.Request{}
func RequireHttpResponseWriter(ctx *jonson.Context)http.ResponseWriter{}
func RequireWSClient(ctx *jonson.Context)*jonson.WSClient{}
func RequireRPCMeta(ctx *jonson.Context)*jonson.RPCMeta{}
func RequireSecret(ctx *jonson.Context)jonson.Secret{}

```

During your development, you will not need the method handler in most cases.
The method handler will be passed to the exposing technology during startup, such as:

- websocket
- http
- a combination of the above

During your development, you will not need the method handler in most cases.
Check out "Putting it all together" to see the method handler in action.

## Server

The server implements the standard http.Handler interface.
You can either use the `server.ListenAndServe()` method directly _or_ alternatively
write your own server which can use the http.Handler interface provided by
the server.

`NewServer()` accepts multiple `Handler`s which can be one of:

- rpc over http (using a single endpoint): `jonson.HttpRpcHandler`
- rpc over websocket: `jonson.WebsocketHandler`
- a single http endpoint per rpc: `jonson.HttpMethodHandler`
- default http handlers which use the http.Request and http.ResponseWriter functionality: `jonson.HttpRegexpHandler`

During startup, you can decide which endpoints you want to provide.

## Secret

In order to encrypt/decrypt server errors which shoult not be exposed to the client,
jonson.Secret allows you to implement either your own encryption/decryption or use
the one coming with jonson using `jonson.NewAESSecret()`.
For the AES secret, consider a key with 16, 24 or 32 bytes in length.
In case the key does not have any of the above mentioned lengths, your program will panic.

For debugging purposes, you might want to use the `jonson.NewDebugSecret()` which will
not encrypt/decrypt but simply pass the error to the rpc response.

## Putting it all together

In our main, we can now spin up our remote procedure calls:

```go
func main(){
  // in order to encrypt/decrypt our messages, we need a secret.
  secret := jonson.NewDebugSecret()

  // connect to mysql
  db := sql.MustConnect("")

  // let's initialize our providers first
  factory := jonson.NewFactory()

  // register a provider defining multiple provider instantiation methods
  factory.RegisterProvider(infrastructure.NewInfrastructureProvider(db, func(){
    return time.Now()
  }))

  // register a simple provider function
  factory.RegisterProviderFunc(infrastructure.ProvideServiceName)

  // let's instantiate our systems
  authentication := authentication.NewAuthentication()
  authorization := authentication.NewAuthorization()

  // let's expose the system's remote procedure calls to the method handler
  methodHandler := jonson.NewMethodHandler(factory, secret)

  // right now, our systems are parsed by the method handler but not yet exposed.

  // the rpc handler will serve all remote procedure calls from the method handler
  // once calling the /rpc http endpoint
  rpcHandler := jonson.NewHttpRpcHandler(methodHandler, "/rpc")

  // the http method handler will expose all remote procedure calls
  // as their own endpoint, such as:
  // /authentication/login.v1
  // /authentication/logout.v1
  // ...
  methodHandler := jonson.NewHttpMethodHandler(methodHandler)

  // the ws handler will handle all incoming requests using websocket on the
  // http endpoint /ws
  wsHandler := jonson.NewWebsocketHandler(methodHandler, "/ws", jonson.NewWebsocketOptions())

  // the regexp handler allows us to define
  // regular expressions which will be handled
  // using the default http.Request and http.ResponseWriter.
  regexHandler := jonson.NewHttpRegexpHandler(methodHandler)
  regexpHandler.RegisterRegexp("/health", func(ctx *jonson.Context, w http.ResponseWriter, r *http.Request, parts []string){
    w.Write("UP")
  })


  // create a new server and handle all the technologies previously defined.
  server := jonson.NewServer(
    rpcHandler,
    methodHandler,
    wsHandler,
    regexpHandler,
  );

  // last step: let's listen and serve ;-)
  server.ListenAndServe(":8080")
}
```

NOTE: the server will ask each registered handler (rpc, ws, ...) whether they will be eligible to serving
a given endpoint following the order they were passed. The first one that returns "true", wins.
In case your application is mostly used with websocket connections, it might be a good idea
to pass the wsHandler as the first argument when calling `jonson.NewServer()`.

## Exposed paths

The methods a client will try to call can be exposed with different technologies as mentioned above (websocket, http rpc or http methods).

In case you are using http methods, the paths exposed will look like this:
/<systemName>/<methodName>.v<version>.
The params you send (body) needs to match the json specification of your rpc's params.
The result will be returned within the body as json also following your rpc's return value's json specification.

For successful remote procedure calls, the http status code will be 200.
For errors during the call, the http status code will be in the 4xx and 5xx range - depending on the
error that occured. The response body will contain the json rpc error as per [specification](https://www.jsonrpc.org/specification#error_object).

In case you are using rpc over websocket or http, your methods will look the same.
However, you will have to wrap the request in the [jsonRPC request object](https://www.jsonrpc.org/specification#request_object).

```json
{
  "jsonrpc": 2.0,
  "id": 1,
  "method": "<systemName>/<methodName>.v<version>",
  "params": {},
}
```

The response will reflect the `id` sent in the request.
Each request should use its unique id per client to map the request to the response on the client's side.

```json
{
  "jsonrpc": 2.0,
  "id": 1,
  "result": {}
}
```

In case of an error response, the client will receive no result but an error in the response.

```json
{
  "jsonrpc": 2.0,
  "id": 1,
  "error": {
    "code": -32000,
    "message": "Internal server error"
  }
}
```

## Error handling

jonson predefines a few jsonRPC default errors which are defined in the spec.
You can either clone those and add your own data by calling e.g. `jonson.ErrInvalidParams.CloneWithData(yourData)`
or define your own errors by using `jonson.Error`.
A jsonRPC error consists of a message, a code and optional data.
For further details on error messages, have a look at: [jsonRPC error object](https://www.jsonrpc.org/specification#error_object)

## Advanced factory features

In most cases, you will use the provided providers using their generated `RequireXXX` functions,
such as:

```go
// LoginV1 allows an account to log in
func (a *Authentication) LoginV1(ctx *jonson.Context, params *LoginV1Params) error{
  // the factory provides the database and we can now access it here in the code
  db := infra.RequireDB(ctx)
  // put your logic here
  return nil
}
```

Furthermore, jonson allows you to use any provided type in your remote procedure call's parameters.
In case the parameter is not providable and not of type jonson.Context or a remote procedure call jonson.Params,
the function will not be called.

You can for example directly define the db as a parameter in your function and access it within your logic.

```go
// LoginV1 allows an account to log in
func (a *Authentication) LoginV1(ctx *jonson.Context, db *infra.DB, params *LoginV1Params) error{
  // put your login logic here
  return nil
}
```

This feature comes in very handy in case you do want to check whether an account is authenticated or not.

```go

type AuthenticationProvider struct {

}

// @generate
type Private struct {}

func (a *AuthenticationProvider) NewPrivate(ctx *jonson.Context)*Private{
  req := jonson.RequireHttpRequest(ctx)
  sessionId := req.Cookie("sessionId")
  if (sessionId == ""){
    panic(jonson.ErrUnauthenticated)
  }
  // more logic here

  return &Private{}
}
```

Within your endpoint, you can now use `Private` as a safeguard.
In case the calling user does not possess a valid session, the provider will
panic and the function will never be callable.

```go

type MeV1Result struct {
  Name string
}

// MeV1 returns my profile
func (a *Authentication) MeV1(ctx *jonson.Context, private *Private) (*MeV1Result, error){
  // By now, we know that the user does possess a valid session.
  // We cano now safely proceed with the function's flow
  return &MeV1Result {
    Name: "Silvio"
  }, nil
}
```

## Code generation

To create types for internal remote procedure calls (in between systems) as well as to
generate the RequireProvider() functions, use the jonson generator.

To generate types in a system (or provider), add the following line to one of your system's files.

```go
package example

//go:generate go run github.com/doejon/jonson/cmd/generate

```

Using `go generate ./...`, you should now see two new files being created within your system
containing provider and remote procedure calls:
`jonson.procedure-calls.gen.go` and `jonson.providers.gen.go`.

The procedure calls file contains all remote procedure calls specified within
the current system. Those helper methods allow us to call another system's procedure without
doing an http round trip.

In order to trigger code generation, tag your types which should be requirable with `// @generate`.

Current generation limitations:
The generator currently only works with the default method name used within jonson.

## Dedication

The whole idea for this library was born after a long iteration period with dear friends.
It is heavily influenced by one of the best mentors of my (professional) life.
