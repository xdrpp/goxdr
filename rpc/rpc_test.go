
package rpc_test

// import "github.com/xdrpp/goxdr/rpc"
import "testing"

type Server struct {}
func (*Server) Test_null() {}
func (*Server) Test_inc(i int32) int32 { return i + 1 }
func (*Server) Test_add(i, j int32) int32 { return i + j }
func (*Server) Test_string(s string) string {
	return "Hello " + s
}
var _ TEST_V1 = &Server{}

func TestNull(t *testing.T) {
}
