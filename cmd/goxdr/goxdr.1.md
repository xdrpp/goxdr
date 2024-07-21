% goxdr(1)
% David Mazi&egrave;res
%

# NAME

goxdr - Go XDR compiler

# SYNOPSIS

goxdr [-b|-B] [-fmt] [-i _import_] [-p _package_] \
\ \ \ \ \ \ [-o _output.go_] [_file.x_ ...]

# DESCRIPTION

goxdr compiles an RFC4506 XDR interface file to a set of go data
structures that can be either marshaled to or unmarshaled from
standard XDR binary format or traversed for other purposes such as
pretty-printing.  It does not rely on go's reflection facilities, and
so can be used to special-case handling of different XDR typedefs that
represent identical go types.

Because go's type system is not very extensible, goxdr uses a _dual
type system_.  Every type `T` declared in an XDR file compiles to two
corresponding go types, a _native type_ `T` convenient for
programming, and an _XdrType_, `XdrType_T`, with methods that
facilitate generic traversal and marshaling.  `XdrType_T` is
equivalent to `*T` when it is possible to implement appropriate
methods on `T`, but this is not always the case.  However, for all XDR
types `T`, there is a method `XDR_T(*T)XdrType_T` to cast a value to
its XdrType.

goxdr's native types map to the most intuitive go equivalent:  strings
map to strings, pointers map to pointers, fixed-size arrays map to
arrays, and variable-length arrays map to slices, without new type
declarations that might complicate assignment.  E.g., the XDR `typedef
string mystring<32>` is just a string, and so can be assigned from a
string.  This does mean you can assign a string longer than 32 bytes,
but length limits are enforced during both marshaling and
unmarshaling.

## Type representations

To be consistent with go's symbol export policy, all types, enum
constants, and struct/union fields defined in an XDR file are
capitalized in the corresponding go representation.  Source XDR types
are mapped to their equivalent go types as follows (note that `T`
cannot be `opaque` or `string` in this table):

    Source type     Native type    notes
    --------------  ------------   ------------------------------
    bool            bool           use capital TRUE and FALSE
    int             int32
    unsigned int    uint32
    hyper           int64
    unsigned hyper  uint64
    float           float32
    double          float64
    quadruple       float128       but float128 is not defined
    string<n>       string
    opaque<n>       []byte
    opaque[n]       [n]byte
    enum T          type T int32
    T*              *T             for any XDR type T
    T<n>            []T            for any XDR type T
    T[n]            [n]T           for any XDR type T

Each XDR `typedef` is compiled to a go type alias (`type Alias =
Original`).  However, each has its own XdrType, allowing generic
traversal code to special-case the use of type aliases.  Among other
places, this is useful for pretty-printing code that displays the same
underlying type in different ways.  For example, if your XDR source
specifies:

~~~~{.c}
typedef int meters;
typedef int seconds;
~~~~

it will compile to native types:

~~~~{.go}
type Meters = int32
type Seconds = int32
~~~~

But you can treat instances of each specially in your marshaling
function by testing for type `XdrType_Meters` or `XdrType_Seconds`.
Alternatively, you can also use the `XdrTypeName()string` method to
obtain the string "Meters" or "Seconds."

Each XDR `enum` declaration compiles to a defined type whose
representation is an `int32`.  The constants of the enum are defined
as go constants of the new defined type.

RFC4506 defines `bool` in XDR source as equivalent to an `enum` with
name identifiers `TRUE` and `FALSE`.  goxdr translates XDR's `bool` to
go's native `bool` type.  However, following the RFC, goxdr still
requires you to use capital `TRUE` and `FALSE`.  (The identifiers
`true` and `false` will get translated to exported go identifiers
`True` and `False`, which is probably not what you want.)

An XDR `struct` is compiled to a defined type represented as a go
struct containing each field of the XDR struct.

An XDR `union` is compiled to a data structure with one public field
for the discriminant and one method for each non-void "arm
declaration" (i.e., declaration in a case statement) that returns a
pointer to a value of the appropriate type.  There is no need to
initialize the union when setting the discriminant; changing its value
just causes the appropriate method to return a non-nil pointer.
Invoking the wrong method for the current discriminant value calls
panic.

As an example, the following XDR source:

~~~~{.c}
enum myenum {
    tag1 = 1,
    tag2 = 2,
    tag3 = 3
};

union myunion switch (myenum discriminant) {
    case tag1:
        int one;
    case tag2:
        string two<>;
    default:
        void;
};
~~~~

compiles to this go code:

~~~~{.go}
type Myenum int32
const (
    Tag1 Myenum = 1
    Tag2 Myenum = 2
    Tag3 Myenum = 3
)
func XDR_Myenum(v *Myenum) *Myenum { return v }

type Myunion struct {
    Discriminant Myenum
    ...
}
func (u *Myunion) One() *int32 {...}
func (u *Myunion) Two() *string {...}
func XDR_Myunion(x XDR) *Myunion { return v }
~~~~

## Convenience methods

XDR `union` types have an `XdrValid() bool` method that returns
whether the discriminant is in a valid state.  If the `union` type
does not have a default case, another method `XdrValidTags()
map[uint32]bool` specifies the set of valid discriminant values.  For
`union` types that are not valid by default, because go's default 0
value does not correspond to a valid discriminant, a method
`XdrInitialize()` places them in a valid state.  Similarly, `enum`
types that can't be statically verified to be valid with value 0
(either because no name is assigned to constant 0, or because names
are assigned to constants in a different file), have an
`XdrInitialize()` method that sets them to the first name in the
`enum` definition.

## The XDR interface

As previously mentioned, for every native type `T` generated by goxdr
(where `T` is the capitalized go type), including typedefs, there is a
corresponding XdrType into which one can cast the native type by means
of a generated function:

~~~~{.go}
func XDR_T(v *T) XdrType_T {...}
~~~~

All XdrTypes support the `XdrType` interface:

~~~~{.go}
type XdrType interface {
    XdrTypeName() string
    XdrValue() interface{}
    XdrPointer() interface{}
    XdrMarshal(XDR, string)
}
~~~~

The XdrType can be marshaled, unmarshaled, or otherwise traversed by
means of the `XdrMarshal(x XDR, name string)` method.  Note the `name`
argument has no effect for RFC4506-compliant binary marshaling, and
can safely be supplied as the empty string `""`.  However, when
traversing an XDR type for other purposes such as pretty-printing,
`name` will be set to the nested name of the field (with components
separated by period).

The argument `x` implements the XDR interface and determines what
XDR_T actually does (i.e., marshal or unmarshal).  It has the
following interface:

~~~~{.go}
type XDR interface {
    Marshal(name string, val XdrType)
    Sprintf(string, ...interface{}) string
}
~~~~

`Sprintf` is expected to be a copy of `fmt.Sprintf`.  However, XDR
back-ends that do not make use of the `name` argument (notably
marshaling to RFC4506 binary format) can save some overhead by
returning an empty string.  Hence, the two sensible implementations of
`Sprintf` are:

~~~~{.go}
func (xp *MyXDR1) Sprintf(f string, args ...interface{}) string {
    return fmt.Sprintf(f, args...)
}

func (xp *MyXDR2) Sprintf(f string, args ...interface{}) string {
    return ""
}
~~~~

`Marshal` is the method that actually does whatever work will be
applied to the data structure.  The second argument, `val`, will be
the go value that must be marshaled/unmarshaled.  To simplify data
structure traversal, XdrTypes implement various more specific
interfaces that extend `XdrType` allowing many different types to be
handled identically.  Specifically:

* For bool and all 32-bit numeric types (including the size of
variable-length arrays), The XdrType implements `XdrNum32`, which
allows the value to be extracted and set as a `uint32`.

* For all 64-bit numeric types, the XdrType imlements `XdrNum64`,
which allows the value to be extracted and set as a `uint64`.

* For `struct` and `union` types, the XdrType is just a pointer to the
type being marshaled.  However, these types implement the
`XdrAggregate` interface, which extends `XdrType` with the method
`XdrRecurse(x XDR, name string)` that recursively marshals every field
of the type.  `union` types also implement the `XdrUnion` interface.

* An `enum` type `T` also just uses `*T` as its XdrType, but `enum`
types implement `XdrNum32` instead of `XdrAggregate`.  They also
implement the `XdrEnum` interface, which provides access to symbolic
names via the `XdrEnumNames() map[int32]string` method.

* Fixed-length arrays (other than `opaque[]`) have a generated XdrType
that implements the `XdrArray` interface, which extends
`XdrAggregate`.  Calling `XdrRecurse` on an array iterates over the
array to marshal each element individually.

* Variable-length arrays (other than `opaque<>`) also use a generated
XdrType implementing the `XdrVec` interface, which also extends
`XdrAggregate`.  The `XdrRecurse` method first calls `Marshal` on a
value of `XdrSize` to get or set the size of the array, then calls
`Marshal` on each element of the array as with fixed-length arrays.

* Similar to variable-length arrays, pointers use a generated XdrType
that implements the `XdrPtr` interface, which extends `XdrAggregate`.
The `XdrRecurse` method first calls `Marshal` on another generated
type that implements the `XdrNum32` interface (capable of containing
the value 0 or 1 to indicate nil or value-present), then, if the
pointer is non-nil, it calls `Marshal` on the underlying value.

* `string` has `XdrType` of `XdrString`, which also encodes the size
bound of the string and implements the `XdrVarBytes` and `XdrBytes`
interfaces that extend `XdrType`.

* `opaque<>` has an `XdrType` of `XdrVecOpaque`, which also implements
the `XdrVarBytes` and `XdrBytes` interfaces that extend `XdrType`.

* `opaque[]` is passed as a generated type implementing
`XdrArrayOpaque`, which extends `XdrBytes` but not `XdrVarBytes`.

For most types, you can recover the native type from the XdrType via
the the `XdrPointer()` and `XdrValue()` methods, which return an
`interface{}`.  One exception is arrays (including `opaque[]`), for
which `XdrValue()` returns a slice to avoid copying the entire array.
Also, the fake `bool` on which `Marshal` is called for a pointer type
supports `XdrValue()` but returns `nil` from `XdrPointer()` since
there is no actual `bool` to point to.

The `XdrTypeName()` method returns a string describing the underlying
type as declared in the XDR file, including any `typedef` aliases
used.  The string returned may have a suffix of "*", "?", "<>", or
"[]" to indicate pointers, the boolean associated with a pointer, a
variable-length array, and a fixed-length array, respectively.  If you
want the actual size or bound that would go inside the "[]" or "<>",
you will need to obtain these from the `XdrArraySize()uint32` or
`XdrBound()uint32` method respectively.

The table below summarizes the (overlapping) interfaces implemented by
the the different XdrTypes generated.  In the table, `T` stands for a
complete standalone XDR type (so not `string` or `opaque`).  Basic
marshaling can be performed in a type switch statement handling
interfaces that cover all types, for instance `XdrNum32`, `XdrNum64`,
`XdrBytes`, and `XdrAggregate`.

    Interface       Implemented for XDR source types
    -----------     --------------------------------------------
    XdrNum32        bool, [unsigned] int, enums, float,
                    size, pointer present flag
    XdrNum64        [unsigned] hyper, double
    XdrArray        T[n]
    XdrVec          T<n>
    XdrPtr          T*
    XdrEnum         enum T
    XdrUnion        union T
    XdrBytes        string<n>, opaque[n], opaque<n>
    XdrVarBytes     string<n>, opaque<n>
    XdrArrayOpaque  opaque[n]
    XdrAggregate    struct T, union T, T*, T<n>, T[n]
    XdrTypedef      typedef BaseT T
    fmt.Stringer    all types in XdrNum{32,64} and XdrBytes
    fmt.Scanner     all types in XdrNum{32,64} and XdrBytes
    XdrType         all XDR types

## XDR functions

As previously mentioned, each (capitalized) type `T` output by goxdr
also has function `XDR_T` that returns an instance of `XdrType`.  For
`struct`, `union`, and `enum` types, this function is the identity
function:

~~~~{.go}
func XDR_T(v *T) XdrType_T { return v }
~~~~

For other types, however, this returns a defined type implementing the
interfaces described in the previous subsection.  As an example, the
following function in the pre-defined boilerplate casts an ordinary
`*int32` into the defined type `*XdrInt32`, which implements the
`XdrNum32` interface:

~~~~{.go}
type XdrInt32 int32
type XdrType_int32 = *XdrInt32
// ... implementation of XdrNum32 methods ...
func (XdrInt32) XdrTypeName() string { return "int32" }
func (v *XdrInt32) XdrPointer() interface{} { return (*int32)(v) }
func (v XdrInt32) XdrValue() interface{} { return int32(v) }
func (v *XdrInt32) XdrMarshal(x XDR, name string) { x.Marshal(name, v) }
func XDR_int32(v *int32) *XdrInt32 { return (*XdrInt32)(v) }
~~~~

The following table lists the concrete types passed to the `Marshal`
method.  Note that types listed as `generated` get passed as a
different defined type for each underlying type `T`.  The defined type
makes the size bound availble via an `XdrBound()` method, since that
information cannot conveniently be encoded as part of the go type.

    XDR type        Marshaled as    notes
    --------------  --------------  -------------------------------
    bool            *XdrBool
    int             *XdrInt32
    unsigned int    *XdrUint32
    float           *XdrFloat32
    hyper           *XdrInt64
    unsigned hyper  *XdrUint64
    double          *XdrFloat64
    string<n>       XdrString
    opaque<n>       XdrVecOpaque
    opaque[n]       generated
    T               *T              for struct, enum, union
    T[n]            generated
    T*              generated
    T<n>            generated
    size            *XdrSize        when recursing in T<n>
    typedef         generated       XdrBaseType(v) gives inner type

Note that an XDR `Marshal(name string, v XdrType)` method can use a
type switch to special-case certain interfaces and types.  If you test
`v` against interfaces (e.g., `XdrNum32`), it will work regardless of
typedefs.  If you test `v` for specific types, such as `XdrString` or
`XdrType_int32`, it will not work for typedefs.  You can, however,
switch on `XdrBaseType(v)` to check the underlying base type.  You can
also, of course, switch on the native type returned by v.XdrPointer(),
which is not affected by typedefs.  `XdrTypeName()` is also useful to
check for typedefs, but to do so should be called on `v` rather than
`XdrBaseType(v)`.

`XdrMarshal` methods panic with type `XdrError` (a user-defined
string) if the input is invalid or a value is out of range.

## Pre-defined XDR types

The types `XdrOut`, `XdrIn`, and `XdrPrint` in the boilerplate code
(by default package `"github.com/xdrpp/goxdr/xdr"`) implement the
`XDR` interface and perform RFC4506 binary marshaling, RFC4506 binary
unmarshaling, and pretty-printing, respectively.

~~~~{.go}
type XdrOut struct {
    Out io.Writer
}
type XdrIn struct {
    In io.Reader
}
type XdrPrint struct {
    Out io.Writer
}
~~~~

## Program and version declarations

Each version declaration inside a program declaration gets compiled
down to an interface with the same name as the version.  For example
this declaration

~~~~{.c}
program my_prog {
  version my_vers {
    void null(void) = 1;
    int Increment(int) = 2;
    void MultiArg(int, int) = 3;
  } = 1;
} = 0x20000000;
~~~~

yields the following interface:

~~~~{.go}
type My_vers interface {
    Null()
    Increment(*int32) *int32
    MultiArg(*int32, *int32)
}
~~~~

In addition, goxdr creates a type that implements the `My_vers`
interface (for use in clients):

~~~~{.go}
type My_vers_Client struct {
    XdrSend func(XdrProc) error
}
func (c My_vers_Client) Null() {...}
func (c My_vers_Client) Increment(a1 *int32) *int32 {...}
func (c My_vers_Client) MultiArg(a1 *int32, a2 *int32) {...}
~~~~

The methods all bundle their argument and result types into a type
implementing `XdrProc`, and pass it to a function `XdrSend`.  An
`XdrProc` instance contains all the information necessary to marshal a
remote procedure call and its result, namely the program, version, and
procedure numbers as well as both the arguments and results ready to
be marshaled in `XdrType` format.  `GetArg()` returns the arguments
supplied by the user, while `GetRes()` returns a result type expected
to be overwritten by the result of the RPC.

~~~~{.go}
type XdrProc interface {
    Prog() uint32
    Vers() uint32
    Proc() uint32
    ProgName() string
    VersName() string
    ProcName() string
    GetArg() XdrType
    GetRes() XdrType
}
~~~~

For the server side, goxdr generates a type `My_vers_Server` that
takes an instance of `My_vers` and allows lookup of argument and
result types by procedure number.  Specifically, `My_vers_Server` just
requires an instance of `My_vers`, and then generically exposes it
through the `XdrSrv` interface.

~~~~{.go}
type My_vers_Server struct {
    Srv My_vers
}
func (s My_vers_Server) GetProc(p uint32) XdrSrvProc {...}
var _ XdrSrv = My_vers_Server{}    // implements XdrSrv interface
~~~~

`XdrSrv` provides everything an RFC5531 RPC library needs to marshal
and unmarshal arguments.  The `Do()` method of an `XdrSrvProc` calls
the underlying method on `My_vers_Server`.  Hence, program-independent
RPC code can call `proc := GetProc()` to get the `XdrSrvProc`, then
unmarshal `proc.GetArg()`, then call `proc.Do()` to handle the call,
and finally marshal the result from `proc.GetRes()`.

~~~~{.go}
type XdrSrvProc interface {
    XdrProc
    Do()
}

type XdrSrv interface {
    Prog() uint32
    Vers() uint32
    ProgName() string
    VersName() string
    GetProc(uint32) XdrSrvProc
}
~~~~

# OPTIONS

goxdr supports the following options:

`-help`
:   Print a brief usage message.

`-b`
:   goxdr by default imports `"github.com/xdrpp/goxdr/xdr"`, a module
with boilerplate code to assist in marshaling and unmarshaling values,
including code for interfaces such as `XDR` and `XdrNum32` as well as
helper types implementing these interfaces (`XdrInt32`, `XdrUint32`,
etc.).  This option suppresses that default import.  This can be
useful if you are importing another package that includes the
boilerplate (see `-B`).

`-B`
:   Causes goxdr to emit the boilerplate into its output instead of
importing it.  Implies `-b`.  Note only one copy of the boilerplate
should be included in a package.  If you use goxdr to compile all XDR
input files to a single go file (the recommended usage), then you will
get only one copy of the boilerplate with `-B`.  However, if you
compile different XDR files into different go files, you will need to
specify `-b` with each XDR input file to avoid including the
boilerplate, then run goxdr with no input files (`goxdr -B -o
goxdr_boilerplate.go`) to get one copy of the boilerplate.  You should
also use `-b` if you are importing another package that already
includes the boilerplate using the `-i` option below.

`-enum-comments`
:   When an enum has one or more constants annotated with a comment,
this options causes goxdr to emit a method `XdrEnumComments()
map[int32]string` that contains the comment turned into a string.  The
option is useful if, for instance, you have an enum encoding various
error conditions.  In that case you can put a human-readable
description of the error condition as a comment in the XDR source
file, and access the text of that comment from your program.

`-fmt`
:	Pass the generated code through `gofmt -s` to simplify and format
it.

`-i` _import_path_
:   Add the directive <tt>import . "_import_path_"</tt> at the top of
the output file.  This is needed when XDR files in the current package
require XDR structures defined in a different package, since XDR
itself provides no way to specify package scoping.

`-lax-discriminants`
:   Cast all discriminants and cases (except bool) to int32, so that
you can use discriminants and cases that are different enum types.

`-o` _output.go_
:   Write the output to file _output.go_ instead of standard output.

`-p` _package_
:   Specify the package name to use for the generated code.  The
default is for the generated code to declare `package main`.

# EXAMPLES

To serialize a data structure of type `MyType`:

~~~~{.go}
func serialize_Mytype(val *MyType) []byte {
    buf := &bytes.Buffer{}
    XDR_MyType(val).XdrMarshal(&XdrOut{ buf }, "")
    return buf.Bytes()
}
~~~~

To serialize/unserialize an arbitrary instance of `XdrType`:

~~~~{.go}
func serialize(val XdrType) []byte {
    buf := &bytes.Buffer{}
    val.XdrMarshal(&XdrOut{ buf }, "")
    return buf.Bytes()
}

func deserialize(val XdrType, in []byte) (e error) {
    defer func() {
        switch i := recover().(type) {
        case nil:
        case XdrError:
            e = i
        default:
            panic(i)
        }
    }()
    val.XdrMarshal(&XdrIn{ bytes.NewBuffer(in) }, "")
    return nil
}
~~~~

To pretty-print an arbitrary XDR-defined data structure, but
special-case any fields of type `MySpecialStruct` by formatting them
with a function called `MySpecialString(*MySpecialStruct)`, you can do
the following:

~~~~{.go}
type XdrMyPrint struct {
    Out io.Writer
}

func (xp *XdrMyPrint) Sprintf(f string, args ...interface{}) string {
    return fmt.Sprintf(f, args...)
}

func (xp *XdrMyPrint) Marshal(name string, i XdrType) {
    switch v := i.(type) {
    case *MySpecialStruct:
        fmt.Fprintf(xp.Out, "%s: %s\n", name, MySpecialString(v))
    case fmt.Stringer:
        fmt.Fprintf(xp.Out, "%s: %s\n", name, v.String())
    case XdrPtr:
        fmt.Fprintf(xp.Out, "%s._present: %v\n", name, v.GetPresent())
        v.XdrMarshalValue(xp, name)
    case XdrVec:
        fmt.Fprintf(xp.Out, "%s.len: %d\n", name, v.GetVecLen())
        v.XdrMarshalN(xp, name, v.GetVecLen())
    case XdrAggregate:
        v.XdrRecurse(xp, name)
    default:
        fmt.Fprintf(xp.Out, "%s: %v\n", name, i)
    }
}

func MyXdrToString(t XdrType) string {
    out := &strings.Builder{}
    t.XdrMarshal(&XdrMyPrint{out}, "")
    return out.String()
}
~~~~

# SEE ALSO

rpcgen(1), xdrc(1)

<https://tools.ietf.org/html/rfc4506>,
<https://tools.ietf.org/html/rfc5531>

# BUGS

goxdr is not hygienic.  Because it capitalizes symbols, it could
produce a name clash if two symbols differ only in the capitalization
of the first letter.  Moreover, it introduces various helper types and
functions that begin `XDR_` or `Xdr`, so could produce incorrect code
if users employ such identifiers in XDR files.  Though RFC4506
disallows identifiers that start with underscore, goxdr accepts them
and produces code with inconsistent export semantics (since underscore
cannot be capitalized).

With `-lax-discriminants`, when unions use type bool as a
discriminant, goxdr generates incorrect code unless it knows that the
discriminant is of type bool.  (This is because go provides no uniform
syntax for converting both enums and bools to int32.)  goxdr tries to
figure out when the union discriminant is of type bool by following
typedefs in the file, but this doesn't work work if you use type
aliases defined in a different file.

IEEE 754 floating point allows for many different NaN (not a number)
values.  The marshaling code simply takes whatever binary value go has
sitting in memory, byteswapping on little-endian machines.  Other
languages and XDR implemenations may produce different NaN values from
the same code.  Hence, in the presence of floating point, the
marshaled output of seemingly deterministic code may vary across
implementations.
